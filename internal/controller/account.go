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
	"fmt"
	"os"

	"github.com/WirelessCar/nauth/internal/cluster"
	"github.com/WirelessCar/nauth/internal/k8s"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/events"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	nauthv1alpha1 "github.com/WirelessCar/nauth/api/v1alpha1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// AccountResult contains the result of account operations
// Used by account.Manager to return results to the provider
type AccountResult struct {
	AccountID       string
	AccountSignedBy string
	Claims          *nauthv1alpha1.AccountClaims
}

// additionalWatch pairs an object type with a handler for the controller to watch.
type additionalWatch struct {
	object  client.Object
	handler handler.EventHandler
}

// AccountReconciler reconciles an Account object
type AccountReconciler struct {
	client.Client
	Scheme            *runtime.Scheme
	resolver          cluster.Resolver
	reporter          *statusReporter
	additionalWatches []additionalWatch
}

// AccountReconcilerOption configures an AccountReconciler.
type AccountReconcilerOption func(*AccountReconciler)

// WithAdditionalWatch registers an extra watch source on the Account controller.
// Use this to trigger Account reconciliation when related resources change
// (e.g. TieredLimit for Synadia backends).
func WithAdditionalWatch(obj client.Object, h handler.EventHandler) AccountReconcilerOption {
	return func(r *AccountReconciler) {
		r.additionalWatches = append(r.additionalWatches, additionalWatch{object: obj, handler: h})
	}
}

func NewAccountReconciler(k8sClient client.Client, scheme *runtime.Scheme, resolver cluster.Resolver, recorder events.EventRecorder, opts ...AccountReconcilerOption) *AccountReconciler {
	r := &AccountReconciler{
		Client:   k8sClient,
		Scheme:   scheme,
		resolver: resolver,
		reporter: newStatusReporter(k8sClient, recorder),
	}
	for _, o := range opts {
		o(r)
	}
	return r
}

// +kubebuilder:rbac:groups=nauth.io,resources=accounts,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=nauth.io,resources=accounts/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=nauth.io,resources=accounts/finalizers,verbs=update
// +kubebuilder:rbac:groups=nauth.io,resources=natsclusters,verbs=get;list;watch
// +kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;watch
// +kubebuilder:rbac:groups=core,resources=events,verbs=create;patch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.20.4/pkg/reconcile
func (r *AccountReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	natsAccount := &nauthv1alpha1.Account{}

	err := r.Get(ctx, req.NamespacedName, natsAccount)
	if err != nil {
		if apierrors.IsNotFound(err) {
			log.Info("resource not found. Ignoring since object must be deleted")
			return ctrl.Result{}, nil
		}
		// Error reading the object - requeue the request.
		log.Error(err, "Failed to get resource")
		return r.reporter.error(ctx, natsAccount, err)
	}

	var accountID string
	var managementPolicy string
	{
		labels := natsAccount.GetLabels()
		if labels != nil {
			accountID = labels[k8s.LabelAccountID]
			managementPolicy = labels[k8s.LabelManagementPolicy]
		}
	}

	// ACCOUNT MARKED FOR DELETION
	if !natsAccount.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, natsAccount, accountID, managementPolicy)
	}

	operatorVersion := os.Getenv(EnvOperatorVersion)

	// Nothing has changed — skip unless the backend requires periodic sync
	if !r.resolver.RequiresPeriodicSync(natsAccount) && natsAccount.Status.ObservedGeneration == natsAccount.Generation && natsAccount.Status.OperatorVersion == operatorVersion {
		return ctrl.Result{}, nil
	}

	// RECONCILE ACCOUNT - Set status & base properties

	// Add finalizer if not present
	if !controllerutil.ContainsFinalizer(natsAccount, controllerAccountFinalizer) {
		controllerutil.AddFinalizer(natsAccount, controllerAccountFinalizer)
		if err := r.Update(ctx, natsAccount); err != nil {
			log.Info("Failed to add finalizer", "name", natsAccount.Name, "error", err)
			return ctrl.Result{}, err
		}
	}

	meta.SetStatusCondition(&natsAccount.Status.Conditions, metav1.Condition{
		Type:    controllerTypeReady,
		Status:  metav1.ConditionFalse,
		Reason:  controllerReasonReconciling,
		Message: "Reconciling account",
	})
	if err := r.Status().Update(ctx, natsAccount); err != nil {
		log.Info("Failed to create the account status", "name", natsAccount.Name, "error", err)
		return ctrl.Result{}, err
	}

	// Resolve the NatsCluster provider for this account
	provider, err := r.resolver.ResolveForAccount(ctx, natsAccount)
	if err != nil {
		return r.reporter.error(ctx, natsAccount, fmt.Errorf("failed to resolve NatsCluster provider: %w", err))
	}

	// RECONCILE ACCOUNT - Import/Create/Update the NATS Account
	var result *cluster.AccountResult
	if managementPolicy == k8s.LabelManagementPolicyObserveValue {
		result, err = provider.ImportAccount(ctx, natsAccount)
		if err != nil {
			return r.reporter.error(ctx, natsAccount, fmt.Errorf("failed to import the observed account: %w", err))
		}
	} else if accountID == "" {
		result, err = provider.CreateAccount(ctx, natsAccount)
		if err != nil {
			return r.reporter.error(ctx, natsAccount, fmt.Errorf("failed to create the account: %w", err))
		}
	} else {
		result, err = provider.UpdateAccount(ctx, natsAccount)
		if err != nil {
			return r.reporter.error(ctx, natsAccount, fmt.Errorf("failed to update the account: %w", err))
		}
	}

	// Apply result to Account resource labels and status
	if natsAccount.Labels == nil {
		natsAccount.Labels = make(map[string]string)
	}
	natsAccount.Labels[k8s.LabelAccountID] = result.AccountID
	natsAccount.Labels[k8s.LabelAccountSignedBy] = result.AccountSignedBy

	// UPDATE ACCOUNT STATUS
	if result.Claims != nil {
		natsAccount.Status.Claims = *result.Claims
	}
	natsAccount.Status.ObservedGeneration = natsAccount.Generation
	natsAccount.Status.ReconcileTimestamp = metav1.Now()

	// Need to copy the status - otherwise overwritten by status from kubernetes api response during spec update
	status := natsAccount.Status.DeepCopy()
	status.OperatorVersion = operatorVersion

	if err := r.Update(ctx, natsAccount); err != nil {
		log.Info("Failed to update the account", "name", natsAccount.Name, "error", err)
		return ctrl.Result{}, err
	}

	// Get the updated status back before updating the kubernetes api
	natsAccount.Status = *status
	if err := r.Status().Update(ctx, natsAccount); err != nil {
		log.Info("Failed to update the account status", "name", natsAccount.Name, "err", err)
		return ctrl.Result{}, err
	}

	// On account nkey rotation, patch Users to trigger credential refresh
	if result.AccountNkeyRotated {
		r.refreshUsersOnNkeyRotation(ctx, result.AccountID, req.Namespace)
	}

	// Use provider-requested requeue interval (e.g. for backends that need periodic sync)
	if result.RequeueAfter != nil && *result.RequeueAfter > 0 {
		return ctrl.Result{RequeueAfter: *result.RequeueAfter}, nil
	}
	return r.reporter.status(ctx, natsAccount)
}

// reconcileDelete handles the deletion of an Account resource, including user
// checks, provider cleanup, and finalizer removal.
func (r *AccountReconciler) reconcileDelete(ctx context.Context, natsAccount *nauthv1alpha1.Account, accountID, managementPolicy string) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	meta.SetStatusCondition(&natsAccount.Status.Conditions, metav1.Condition{
		Type:    controllerTypeReady,
		Status:  metav1.ConditionFalse,
		Reason:  controllerReasonReconciling,
		Message: "Deleting account",
	})

	if err := r.Status().Update(ctx, natsAccount); err != nil {
		log.Info("Failed to update the account status", "name", natsAccount.Name, "error", err)
		return ctrl.Result{}, err
	}

	// Check for connected users
	userList := &nauthv1alpha1.UserList{}
	if err := r.List(ctx, userList, client.MatchingLabels{k8s.LabelUserAccountID: accountID}, client.InNamespace(natsAccount.Namespace)); err != nil {
		log.Info("Failed to list users", "name", natsAccount.Name, "error", err)
		return ctrl.Result{}, err
	}

	if len(userList.Items) > 0 {
		return r.reporter.error(
			ctx,
			natsAccount,
			fmt.Errorf("cannot delete an account with associated users, found %d users", len(userList.Items)),
		)
	}

	if controllerutil.ContainsFinalizer(natsAccount, controllerAccountFinalizer) {
		if managementPolicy != k8s.LabelManagementPolicyObserveValue {
			provider, err := r.resolver.ResolveForAccount(ctx, natsAccount)
			if err != nil {
				return r.reporter.error(ctx, natsAccount, fmt.Errorf("account is marked for deletion, but cannot be removed because system credentials are unavailable (e.g. referenced NatsCluster was deleted): %w. Restore the NatsCluster or its credentials to allow cleanup", err))
			}
			if err := provider.DeleteAccount(ctx, natsAccount); err != nil {
				return r.reporter.error(ctx, natsAccount, fmt.Errorf("account is marked for deletion, but Account JWT removal failed: %w. Restore the NatsCluster or its credentials to allow cleanup", err))
			}
		}

		controllerutil.RemoveFinalizer(natsAccount, controllerAccountFinalizer)
		if err := r.Update(ctx, natsAccount); err != nil {
			log.Info("failed to remove finalizer", "name", natsAccount.Name, "error", err)
			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{}, nil
}

// refreshUsersOnNkeyRotation patches Users to trigger credential refresh when
// an account nkey is rotated.
func (r *AccountReconciler) refreshUsersOnNkeyRotation(ctx context.Context, accountID, namespace string) {
	log := logf.FromContext(ctx)

	userList := &nauthv1alpha1.UserList{}
	if listErr := r.List(ctx, userList, client.MatchingLabels{k8s.LabelUserAccountID: accountID}, client.InNamespace(namespace)); listErr != nil {
		log.Info("Failed to list users for cred refresh", "error", listErr)
		return
	}
	for i := range userList.Items {
		orig := &userList.Items[i]
		patched := orig.DeepCopy()
		if patched.Annotations == nil {
			patched.Annotations = make(map[string]string)
		}
		patched.Annotations[k8s.AnnotationRefreshCredentials] = metav1.Now().Format("2006-01-02T15:04:05Z07:00")
		if patchErr := r.Patch(ctx, patched, client.MergeFrom(orig)); patchErr != nil {
			log.Info("Failed to patch user for cred refresh", "user", orig.Name, "error", patchErr)
		}
	}
}

// SetupWithManager sets up the controller with the Manager.
func (r *AccountReconciler) SetupWithManager(mgr ctrl.Manager) error {
	b := ctrl.NewControllerManagedBy(mgr).
		For(&nauthv1alpha1.Account{}).
		Named("account").
		WithEventFilter(predicate.GenerationChangedPredicate{}).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: 1,
		})

	for _, w := range r.additionalWatches {
		b = b.Watches(w.object, w.handler)
	}

	return b.Complete(r)
}
