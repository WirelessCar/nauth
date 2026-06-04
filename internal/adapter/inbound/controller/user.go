/*
Copyright 2025.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"context"
	"errors"
	"fmt"
	"os"
	"reflect"
	"sort"

	"github.com/WirelessCar/nauth/api/v1alpha1"
	"github.com/WirelessCar/nauth/internal/domain"
	"github.com/WirelessCar/nauth/internal/domain/nauth"
	"github.com/WirelessCar/nauth/internal/ports/inbound"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/events"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"k8s.io/apimachinery/pkg/runtime"
)

var errDependencyNotReady = errors.New("dependency not ready")

// UserReconciler reconciles a User object
type UserReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	manager  inbound.UserManager
	reporter *statusReporter
}

func NewUserReconciler(k8sClient client.Client, scheme *runtime.Scheme, manager inbound.UserManager, recorder events.EventRecorder) *UserReconciler {
	return &UserReconciler{
		Client:   k8sClient,
		Scheme:   scheme,
		manager:  manager,
		reporter: newStatusReporter(k8sClient, recorder),
	}
}

// +kubebuilder:rbac:groups=nauth.io,resources=users,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=nauth.io,resources=users/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=nauth.io,resources=users/finalizers,verbs=update
// +kubebuilder:rbac:groups=core,resources=events,verbs=create;patch
// +kubebuilder:rbac:groups=nauth.io,resources=accounts,verbs=get;list;watch
// +kubebuilder:rbac:groups=nauth.io,resources=accountsigningkeys,verbs=get;list;watch
// +kubebuilder:rbac:groups=events.k8s.io,resources=events,verbs=create;patch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.20.4/pkg/reconcile
func (r *UserReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	user := &v1alpha1.User{}

	err := r.Get(ctx, req.NamespacedName, user)
	if err != nil {
		if apierrors.IsNotFound(err) {
			log.Info("resource not found. Ignoring since object must be deleted")
			return ctrl.Result{}, nil
		}
		// Error reading the object - requeue the request.
		log.Error(err, "Failed to get resource")
		return ctrl.Result{}, err
	}

	// USER MARKED FOR DELETION
	if !user.DeletionTimestamp.IsZero() {
		// TODO: r.manager.Revoke(UserRevocationRequest) to revoke the user from the Account, see
		//  - https://github.com/WirelessCar/nauth/issues/95
		//  - https://docs.nats.io/using-nats/nats-tools/nsc/revocation

		controllerutil.RemoveFinalizer(user, finalizerUser)
		if err := r.Update(ctx, user); err != nil {
			log.Info("failed to remove finalizer", "name", user.Name, "error", err)
			return ctrl.Result{}, err
		}
		// Stop reconciliation as the item is being deleted
		return ctrl.Result{}, nil
	}

	operatorVersion := os.Getenv(envOperatorVersion)

	// Missing secret means credentials are gone — force reconciling to recreate them regardless of generation.
	secretMissing := false
	if err := r.Get(ctx, client.ObjectKey{Name: user.GetUserSecretName(), Namespace: user.Namespace}, &corev1.Secret{}); err != nil {
		if !apierrors.IsNotFound(err) {
			return ctrl.Result{}, err
		}
		secretMissing = true
	}

	signingKeyStale, err := r.isSigningKeyStale(ctx, user)
	if err != nil {
		return r.reporter.error(ctx, user, err)
	}

	// Nothing has changed
	if !secretMissing &&
		!signingKeyStale &&
		user.Status.ObservedGeneration == user.Generation &&
		user.Status.OperatorVersion == operatorVersion {
		return ctrl.Result{}, nil
	}

	// RECONCILE USER - Set status & base properties

	// Add finalizer if not present
	if !controllerutil.ContainsFinalizer(user, finalizerUser) {
		controllerutil.AddFinalizer(user, finalizerUser)
		if err := r.Update(ctx, user); err != nil {
			return ctrl.Result{}, err
		}
	}

	meta.SetStatusCondition(&user.Status.Conditions, metav1.Condition{
		Type:    conditionTypeReady,
		Status:  metav1.ConditionFalse,
		Reason:  conditionReasonReconciling,
		Message: "Reconciling user",
	})
	if err := r.Status().Update(ctx, user); err != nil {
		log.Info("Failed to create the user status", "name", user.Name, "error", err)
		return ctrl.Result{}, err
	}

	signingSecretRef, signingPublicKey, err := r.resolveSigningKeyRef(ctx, user)
	if err != nil {
		if errors.Is(err, errDependencyNotReady) {
			meta.SetStatusCondition(&user.Status.Conditions, metav1.Condition{
				Type:    conditionTypeReady,
				Status:  metav1.ConditionFalse,
				Reason:  conditionReasonNotReady,
				Message: err.Error(),
			})
			if updateErr := r.Status().Update(ctx, user); updateErr != nil {
				return ctrl.Result{}, updateErr
			}
			return ctrl.Result{RequeueAfter: requeueDependencyNotReady}, nil
		}
		return r.reporter.error(ctx, user, err)
	}

	result, err := r.manager.CreateOrUpdate(ctx, toUserRequest(user, signingSecretRef, signingPublicKey))
	if err != nil {
		return r.reporter.error(ctx, user, err)
	}

	// Apply result to User resource labels
	user.SetLabel(v1alpha1.UserLabelUserID, result.UserPublicKey)
	user.SetLabel(v1alpha1.UserLabelAccountID, result.AccountID)
	user.SetLabel(v1alpha1.UserLabelSignedBy, result.SignedBy)

	// UPDATE USER STATUS
	user.Status.Claims = specToClaims(user.Spec)
	user.Status.ObservedGeneration = user.Generation
	user.Status.ReconcileTimestamp = metav1.Now()

	// Need to copy the status - otherwise overwritten by status from kubernetes api response during spec update
	status := user.Status.DeepCopy()
	status.OperatorVersion = operatorVersion

	user.Status = v1alpha1.UserStatus{}
	if err := r.Update(ctx, user); err != nil {
		log.Info("Failed to update the user", "name", user.Name, "error", err)
		return ctrl.Result{}, err
	}

	// Get the updated status back before updating the kubernetes api
	user.Status = *status
	if err := r.Status().Update(ctx, user); err != nil {
		log.Info("Failed to update the user status", "name", user.Name, "error", err)
		return ctrl.Result{}, err
	}

	return r.reporter.status(ctx, user)
}

func (r *UserReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.User{}, builder.WithPredicates(predicate.GenerationChangedPredicate{})).
		Owns(&corev1.Secret{}).
		Named("user").
		WithOptions(controller.Options{
			MaxConcurrentReconciles: 1,
		}).
		// Account watch: re-enqueue Users whose signing-key trust set may have changed so
		// the stale check can bypass the early-return even when the User spec is unchanged.
		Watches(
			&v1alpha1.Account{},
			handler.EnqueueRequestsFromMapFunc(r.mapAccountToUsers),
			builder.WithPredicates(accountWatchPredicateForUsers()),
		).
		Complete(r)
}

func toUserRequest(user *v1alpha1.User, signingSecretRef *domain.NamespacedName, signingPublicKey string) nauth.UserRequest {
	return nauth.UserRequest{
		UserRef:                    domain.NewNamespacedName(user.Namespace, user.Name),
		AccountRef:                 domain.NewNamespacedName(user.Namespace, user.Spec.AccountName),
		AccountID:                  nauth.AccountID(user.GetLabel(v1alpha1.UserLabelAccountID)),
		DisplayName:                user.Spec.DisplayName,
		Permissions:                toNAuthUserPermissions(user.Spec.Permissions),
		Limits:                     toNAuthUserLimits(user.Spec.UserLimits),
		NatsLimits:                 toNAuthNatsLimits(user.Spec.NatsLimits),
		Owner:                      user,
		SigningPrivateKeySecretRef: signingSecretRef,
		SigningPublicKey:           signingPublicKey,
	}
}

// resolveTrustedSigningKey walks the Account -> AccountSigningKey trust chain that
// User reconciliation depends on, returning the resolved AccountSigningKey when
// every link is in place.
// Returns nil when user.Spec.SigningKeyRef is empty (the User signs with the
// Account's implicit signing key). Returns errDependencyNotReady wrapped with context
// when any of the following hold: Account or AccountSigningKey not found,
// AccountSigningKey not Ready, AccountSigningKey status.publicKey empty, ref absent
// from Account.spec.signingKeyRefs, or the public key not yet published in
// Account.status.claims.signingKeys.
func (r *UserReconciler) resolveTrustedSigningKey(ctx context.Context, user *v1alpha1.User) (*v1alpha1.AccountSigningKey, error) {
	if user.Spec.SigningKeyRef == "" {
		return nil, nil
	}

	account := &v1alpha1.Account{}
	if err := r.Get(ctx, client.ObjectKey{Namespace: user.Namespace, Name: user.Spec.AccountName}, account); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, fmt.Errorf("account %q not found: %w", user.Spec.AccountName, errDependencyNotReady)
		}
		return nil, fmt.Errorf("failed to get account %q: %w", user.Spec.AccountName, err)
	}

	ask := &v1alpha1.AccountSigningKey{}
	if err := r.Get(ctx, client.ObjectKey{Namespace: user.Namespace, Name: user.Spec.SigningKeyRef}, ask); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, fmt.Errorf("AccountSigningKey %q not found: %w", user.Spec.SigningKeyRef, errDependencyNotReady)
		}
		return nil, fmt.Errorf("failed to get AccountSigningKey %q: %w", user.Spec.SigningKeyRef, err)
	}

	if !meta.IsStatusConditionTrue(ask.Status.Conditions, conditionTypeReady) {
		return nil, fmt.Errorf("AccountSigningKey %q is not ready: %w", user.Spec.SigningKeyRef, errDependencyNotReady)
	}

	refInSpec := false
	for _, ref := range account.Spec.SigningKeyRefs {
		if ref == user.Spec.SigningKeyRef {
			refInSpec = true
			break
		}
	}
	if !refInSpec {
		return nil, fmt.Errorf("signing key ref %q is not listed in Account %q spec.signingKeyRefs: %w",
			user.Spec.SigningKeyRef, user.Spec.AccountName, errDependencyNotReady)
	}

	pubKey := ask.Status.PublicKey
	if pubKey == "" {
		return nil, fmt.Errorf("AccountSigningKey %q has no public key in status: %w", user.Spec.SigningKeyRef, errDependencyNotReady)
	}
	if !signingKeyInAccountClaims(account, pubKey) {
		return nil, fmt.Errorf("AccountSigningKey %q (public key %s) not yet in Account %q signing keys: %w",
			user.Spec.SigningKeyRef, pubKey, user.Spec.AccountName, errDependencyNotReady)
	}

	return ask, nil
}

// resolveSigningKeyRef resolves the signing-key Secret reference and the trusted
// public key for user.Spec.SigningKeyRef.
// Returns nil when SigningKeyRef is empty (implicit Account signing key).
// Returns errDependencyNotReady when the AccountSigningKey or Account is not yet
// ready or the public key is not in Account claims.
func (r *UserReconciler) resolveSigningKeyRef(ctx context.Context, user *v1alpha1.User) (*domain.NamespacedName, string, error) {
	ask, err := r.resolveTrustedSigningKey(ctx, user)
	if err != nil {
		return nil, "", err
	}
	if ask == nil {
		return nil, "", nil
	}

	secretName := ask.Status.SecretName
	if secretName == "" {
		return nil, "", fmt.Errorf("AccountSigningKey %q has no secret name in status: %w", user.Spec.SigningKeyRef, errDependencyNotReady)
	}
	return new(domain.NewNamespacedName(user.Namespace, secretName)), ask.Status.PublicKey, nil
}

func signingKeyInAccountClaims(account *v1alpha1.Account, pubKey string) bool {
	if account.Status.Claims == nil {
		return false
	}
	for _, sk := range account.Status.Claims.SigningKeys {
		if sk != nil && sk.Key == pubKey {
			return true
		}
	}
	return false
}

// isSigningKeyStale returns true when user.Spec.SigningKeyRef is set and the
// current signing-key/account trust state differs from what was last reconciled.
// Transient unavailability (missing resources, not-ready) also returns true so the
// full reconcile path can surface the condition; hard API errors return (false, err).
func (r *UserReconciler) isSigningKeyStale(ctx context.Context, user *v1alpha1.User) (bool, error) {
	ask, err := r.resolveTrustedSigningKey(ctx, user)
	if err != nil {
		if errors.Is(err, errDependencyNotReady) {
			return true, nil
		}
		return false, err
	}
	if ask == nil {
		return false, nil
	}
	return ask.Status.PublicKey != user.GetLabel(v1alpha1.UserLabelSignedBy), nil
}

func (r *UserReconciler) mapAccountToUsers(ctx context.Context, obj client.Object) []reconcile.Request {
	account, ok := obj.(*v1alpha1.Account)
	if !ok {
		return nil
	}
	users := &v1alpha1.UserList{}
	if err := r.List(ctx, users, client.InNamespace(account.Namespace)); err != nil {
		logf.FromContext(ctx).Error(err, "Failed to list Users for Account watch", "account", account.Name)
		return nil
	}
	var requests []reconcile.Request
	for i := range users.Items {
		u := &users.Items[i]
		if u.Spec.AccountName == account.Name && u.Spec.SigningKeyRef != "" {
			requests = append(requests, reconcile.Request{
				NamespacedName: client.ObjectKeyFromObject(u),
			})
		}
	}
	return requests
}

func accountWatchPredicateForUsers() predicate.Funcs {
	return predicate.Funcs{
		CreateFunc: func(event.CreateEvent) bool { return false },
		DeleteFunc: func(event.DeleteEvent) bool { return true },
		UpdateFunc: func(e event.UpdateEvent) bool {
			oldAcc, oldOK := e.ObjectOld.(*v1alpha1.Account)
			newAcc, newOK := e.ObjectNew.(*v1alpha1.Account)
			if !oldOK || !newOK {
				return false
			}
			return accountUpdateAffectsUsers(oldAcc, newAcc)
		},
		GenericFunc: func(event.GenericEvent) bool { return false },
	}
}

func accountUpdateAffectsUsers(oldAcc, newAcc *v1alpha1.Account) bool {
	if !reflect.DeepEqual(signingKeyRefSet(oldAcc.Spec.SigningKeyRefs), signingKeyRefSet(newAcc.Spec.SigningKeyRefs)) {
		return true
	}
	return !reflect.DeepEqual(accountSigningKeySet(oldAcc.Status.Claims), accountSigningKeySet(newAcc.Status.Claims))
}

func signingKeyRefSet(refs []string) []string {
	if len(refs) == 0 {
		return nil
	}
	out := make([]string, len(refs))
	copy(out, refs)
	sort.Strings(out)
	return out
}

func accountSigningKeySet(claims *v1alpha1.AccountClaims) []string {
	if claims == nil {
		return nil
	}
	keys := make([]string, 0, len(claims.SigningKeys))
	for _, sk := range claims.SigningKeys {
		if sk != nil {
			keys = append(keys, sk.Key)
		}
	}
	sort.Strings(keys)
	return keys
}

func toNAuthUserPermissions(p *v1alpha1.Permissions) *nauth.UserPermissions {
	if p == nil {
		return nil
	}
	up := &nauth.UserPermissions{
		Pub: nauth.UserSubjectPermission{Allow: p.Pub.Allow, Deny: p.Pub.Deny},
		Sub: nauth.UserSubjectPermission{Allow: p.Sub.Allow, Deny: p.Sub.Deny},
	}
	if p.Resp != nil {
		up.Resp = &nauth.UserResponsePermission{MaxMsgs: p.Resp.MaxMsgs, Expires: p.Resp.Expires}
	}
	return up
}

func toNAuthUserLimits(l *v1alpha1.UserLimits) *nauth.UserLimits {
	if l == nil {
		return nil
	}
	ul := &nauth.UserLimits{
		Src:    []string(l.Src),
		Locale: l.Locale,
	}
	for _, t := range l.Times {
		ul.Times = append(ul.Times, nauth.UserTimeRange{Start: t.Start, End: t.End})
	}
	return ul
}

func specToClaims(spec v1alpha1.UserSpec) v1alpha1.UserClaims {
	return v1alpha1.UserClaims{
		AccountName: spec.AccountName,
		DisplayName: spec.DisplayName,
		Permissions: spec.Permissions,
		UserLimits:  spec.UserLimits,
		NatsLimits:  spec.NatsLimits,
	}
}
