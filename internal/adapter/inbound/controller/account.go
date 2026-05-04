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
	"reflect"

	"github.com/WirelessCar/nauth/internal/domain"
	"github.com/WirelessCar/nauth/internal/domain/nauth"
	"github.com/WirelessCar/nauth/internal/ports/inbound"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
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

	"github.com/WirelessCar/nauth/api/v1alpha1"
	"k8s.io/apimachinery/pkg/runtime"
)

// AccountReconciler reconciles an Account object
type AccountReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	manager  inbound.AccountManager
	reporter *statusReporter
	features *ExperimentalFeatures
}

func NewAccountReconciler(k8sClient client.Client, scheme *runtime.Scheme, manager inbound.AccountManager, recorder events.EventRecorder, features *ExperimentalFeatures) *AccountReconciler {
	return &AccountReconciler{
		Client:   k8sClient,
		Scheme:   scheme,
		manager:  manager,
		reporter: newStatusReporter(k8sClient, recorder),
		features: features,
	}
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

	natsAccount := &v1alpha1.Account{}

	if err := r.Get(ctx, req.NamespacedName, natsAccount); err != nil {
		if apierrors.IsNotFound(err) {
			log.Info("resource not found. Ignoring since object must be deleted")
			return ctrl.Result{}, nil
		}
		// Error reading the object - requeue the request.
		log.Error(err, "Failed to get resource")
		return ctrl.Result{}, err
	}

	accountID := natsAccount.GetLabel(v1alpha1.AccountLabelAccountID)
	managementPolicy := natsAccount.GetLabel(v1alpha1.AccountLabelManagementPolicy)

	// ACCOUNT MARKED FOR DELETION
	if !natsAccount.DeletionTimestamp.IsZero() {
		// The account is being deleted
		meta.SetStatusCondition(&natsAccount.Status.Conditions, metav1.Condition{
			Type:    conditionTypeReady,
			Status:  metav1.ConditionFalse,
			Reason:  conditionReasonReconciling,
			Message: "Deleting account",
		})

		if err := r.Status().Update(ctx, natsAccount); err != nil {
			log.Info("Failed to update the account status", "name", natsAccount.Name, "error", err)
			return ctrl.Result{}, err
		}

		// Check for connected users
		userList := &v1alpha1.UserList{}
		err := r.List(ctx, userList, client.MatchingLabels{string(v1alpha1.UserLabelAccountID): accountID}, client.InNamespace(req.Namespace))
		if err != nil {
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

		// TODO: [#11] This will block the deletion and requires manual intervention to continue (removing finalizer)
		// TODO: [#11] Investigate and understand if blocking preemptively with webhooks is a better option (not allowing change)?
		adoptions := natsAccount.Status.Adoptions
		if adoptions != nil && len(adoptions.Exports) > 0 {
			return r.reporter.error(
				ctx,
				natsAccount,
				fmt.Errorf("cannot delete an account with adopted exports, found %d adoptions", len(adoptions.Exports)),
			)
		}

		if controllerutil.ContainsFinalizer(natsAccount, finalizerAccount) {
			if managementPolicy != v1alpha1.AccountManagementPolicyObserve {
				if err := r.manager.Delete(ctx, toAccountReference(natsAccount)); err != nil {
					return r.reporter.error(ctx, natsAccount, fmt.Errorf("failed to delete account: %w", err))
				}
			}

			// remove our finalizer from the list and update it.
			controllerutil.RemoveFinalizer(natsAccount, finalizerAccount)
			if err := r.Update(ctx, natsAccount); err != nil {
				log.Info("failed to remove finalizer", "name", natsAccount.Name, "error", err)
				return ctrl.Result{}, err
			}
		}
		// Stop reconciliation as the item is being deleted
		return ctrl.Result{}, nil
	}

	// RECONCILE ACCOUNT - Set status & base properties

	// Add finalizer if not present
	if !controllerutil.ContainsFinalizer(natsAccount, finalizerAccount) {
		controllerutil.AddFinalizer(natsAccount, finalizerAccount)
		if err := r.Update(ctx, natsAccount); err != nil {
			log.Info("Failed to add finalizer", "name", natsAccount.Name, "error", err)
			return ctrl.Result{}, err
		}
	}

	meta.SetStatusCondition(&natsAccount.Status.Conditions, metav1.Condition{
		Type:    conditionTypeReady,
		Status:  metav1.ConditionFalse,
		Reason:  conditionReasonReconciling,
		Message: "Reconciling account",
	})
	if err := r.Status().Update(ctx, natsAccount); err != nil {
		log.Info("Failed to create the account status", "name", natsAccount.Name, "error", err)
		return ctrl.Result{}, err
	}

	// RECONCILE ACCOUNT
	var result *nauth.AccountResult
	var adoptions *v1alpha1.AccountAdoptions
	if managementPolicy == v1alpha1.AccountManagementPolicyObserve {
		var err error
		result, err = r.manager.Import(ctx, toAccountReference(natsAccount))
		if err != nil {
			return r.reporter.error(ctx, natsAccount, fmt.Errorf("failed to import the observed account: %w", err))
		}
	} else {
		resources, adoptionRefs, err := r.collectAccountResources(ctx, natsAccount, accountID)
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to collect resources: %w", err)
		}
		result, err = r.manager.CreateOrUpdate(ctx, resources)
		if err != nil {
			return r.reporter.error(ctx, natsAccount, fmt.Errorf("failed to apply account: %w", err))
		}
		adoptions = toAPIAdoptions(result.Adoptions, adoptionRefs)
	}

	// Apply result to Account resource labels and status
	natsAccount.SetLabel(v1alpha1.AccountLabelAccountID, result.AccountID)
	natsAccount.SetLabel(v1alpha1.AccountLabelSignedBy, result.AccountSignedBy)

	// UPDATE ACCOUNT STATUS
	if result.Claims != nil {
		natsAccount.Status.Claims = *toAPIAccountClaims(result.Claims)
	}
	natsAccount.Status.Adoptions = adoptions
	natsAccount.Status.ClaimsHash = result.ClaimsHash
	natsAccount.Status.ObservedGeneration = natsAccount.Generation
	natsAccount.Status.ReconcileTimestamp = metav1.Now()

	// Need to copy the status - otherwise overwritten by status from kubernetes api response during spec update
	status := natsAccount.Status.DeepCopy()
	status.OperatorVersion = os.Getenv(envOperatorVersion)

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

	return r.reporter.status(ctx, natsAccount)
}

func toAccountReference(state *v1alpha1.Account) nauth.AccountReference {
	natsClusterRef := state.Spec.NatsClusterRef
	var clusterRef *nauth.ClusterRef
	if natsClusterRef != nil {
		namespacedName := domain.NewNamespacedName(natsClusterRef.Namespace, natsClusterRef.Name)
		if namespacedName.Namespace == "" {
			namespacedName.Namespace = state.Namespace
		}
		ref := nauth.ClusterRef(namespacedName.String())
		clusterRef = &ref
	}
	return nauth.AccountReference{
		AccountRef: domain.NamespacedName{
			Name:      state.Name,
			Namespace: state.Namespace,
		},
		AccountID:      nauth.AccountID(state.GetLabel(v1alpha1.AccountLabelAccountID)),
		NatsClusterRef: clusterRef,
	}
}

func (r *AccountReconciler) collectAccountResources(ctx context.Context, account *v1alpha1.Account, accountID string) (nauth.AccountResources, accountAdoptionRefs, error) {
	resources := nauth.AccountResources{Account: *account}
	refs := accountAdoptionRefs{}

	if accountID == "" {
		return resources, refs, nil
	}

	if r.features.AccountExportEnabled {
		namespace := domain.Namespace(account.Namespace)
		exports, err := r.findExportsByAccountID(ctx, namespace, accountID)
		if err != nil {
			return resources, refs, err
		}
		for _, exp := range exports.Items {
			claim := exp.Status.DesiredClaim
			var adpRef adoptionRef
			if claim == nil {
				adpRef = newAdoptionRef(exp.ObjectMeta, nil)
			} else {
				adpRef = newAdoptionRef(exp.ObjectMeta, &claim.ObservedGeneration)
				var nauthExports nauth.Exports
				for _, rule := range claim.Rules {
					nauthExports = append(nauthExports, toNAuthExportFromRule(rule))
				}
				resources.ExportGroups = append(resources.ExportGroups, &nauth.ExportGroup{
					Ref:     adpRef.Ref,
					Name:    exp.Name,
					Exports: nauthExports,
				})
			}
			refs.exports = append(refs.exports, &adpRef)
		}
	}

	// TODO: [#11] feature flag imports?
	if true {
		namespace := domain.Namespace(account.Namespace)
		imports, err := r.findImportsByAccountID(ctx, namespace, accountID)
		if err != nil {
			return resources, refs, err
		}
		for _, imp := range imports.Items {
			claim := imp.Status.DesiredClaim
			var adpRef adoptionRef
			if claim == nil {
				adpRef = newAdoptionRef(imp.ObjectMeta, nil)
			} else {
				adpRef = newAdoptionRef(imp.ObjectMeta, &claim.ObservedGeneration)
				var nauthImports nauth.Imports
				for _, rule := range claim.Rules {
					nauthImports = append(nauthImports, toNAuthImportFromRule(rule))
				}
				resources.ImportGroups = append(resources.ImportGroups, &nauth.ImportGroup{
					Ref:     adpRef.Ref,
					Name:    imp.Name,
					Imports: nauthImports,
				})
			}
			refs.imports = append(refs.imports, &adpRef)
		}
	}

	return resources, refs, nil
}

func (r *AccountReconciler) findExportsByAccountID(ctx context.Context, namespace domain.Namespace, accountID string) (*v1alpha1.AccountExportList, error) {
	if accountID == "" {
		return nil, fmt.Errorf("account ID required")
	}
	exports := &v1alpha1.AccountExportList{}
	err := r.List(ctx, exports, client.InNamespace(namespace), client.MatchingLabels{
		string(v1alpha1.AccountExportLabelAccountID): accountID,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list account exports: %w", err)
	}
	return exports, nil
}

func (r *AccountReconciler) findImportsByAccountID(ctx context.Context, namespace domain.Namespace, accountID string) (*v1alpha1.AccountImportList, error) {
	if accountID == "" {
		return nil, fmt.Errorf("account ID required")
	}
	imports := &v1alpha1.AccountImportList{}
	err := r.List(ctx, imports, client.InNamespace(namespace), client.MatchingLabels{
		string(v1alpha1.AccountImportLabelAccountID): accountID,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list account imports: %w", err)
	}
	return imports, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *AccountReconciler) SetupWithManager(mgr ctrl.Manager) error {
	controllerBuilder := ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.Account{}, builder.WithPredicates(predicate.GenerationChangedPredicate{})).Named("account").
		WithOptions(controller.Options{
			MaxConcurrentReconciles: 1,
		})

	if r.features.AccountExportEnabled {
		controllerBuilder = controllerBuilder.Watches(
			&v1alpha1.AccountExport{},
			handler.EnqueueRequestsFromMapFunc(r.mapAccountExportToAccounts),
			builder.WithPredicates(accountExportWatchPredicateForAccounts()),
		)
	}

	// TODO: [#11] feature toggle
	if true {
		controllerBuilder = controllerBuilder.Watches(
			&v1alpha1.AccountImport{},
			handler.EnqueueRequestsFromMapFunc(r.mapAccountImportToAccounts),
			builder.WithPredicates(accountImportWatchPredicateForAccounts()),
		)
	}

	return controllerBuilder.
		Complete(r)
}

func (r *AccountReconciler) mapAccountExportToAccounts(ctx context.Context, obj client.Object) []reconcile.Request {
	export, ok := obj.(*v1alpha1.AccountExport)
	if !ok {
		return nil
	}

	accountID := export.GetLabel(v1alpha1.AccountExportLabelAccountID)
	if accountID == "" {
		return nil
	}

	accounts := &v1alpha1.AccountList{}
	if err := r.List(ctx, accounts,
		client.InNamespace(export.Namespace),
		client.MatchingLabels{string(v1alpha1.AccountLabelAccountID): accountID},
	); err != nil {
		logf.FromContext(ctx).Error(err, "Failed to list Accounts for AccountExport watch", "accountID", accountID, "namespace", export.Namespace)
		return nil
	}

	requests := make([]reconcile.Request, 0, len(accounts.Items))
	for _, account := range accounts.Items {
		requests = append(requests, reconcile.Request{
			NamespacedName: client.ObjectKeyFromObject(&account),
		})
	}

	return requests
}

func accountExportWatchPredicateForAccounts() predicate.Funcs {
	return predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			export, ok := e.Object.(*v1alpha1.AccountExport)
			return ok && export.GetLabel(v1alpha1.AccountExportLabelAccountID) != ""
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			export, ok := e.Object.(*v1alpha1.AccountExport)
			return ok && export.GetLabel(v1alpha1.AccountExportLabelAccountID) != ""
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			oldExport, oldOK := e.ObjectOld.(*v1alpha1.AccountExport)
			newExport, newOK := e.ObjectNew.(*v1alpha1.AccountExport)
			return oldOK && newOK && accountExportUpdateAffectsAccounts(oldExport, newExport)
		},
		GenericFunc: func(event.GenericEvent) bool {
			return false
		},
	}
}

func accountExportUpdateAffectsAccounts(oldExport *v1alpha1.AccountExport, newExport *v1alpha1.AccountExport) bool {
	if oldExport == nil || newExport == nil {
		return false
	}

	oldAccountID := oldExport.GetLabel(v1alpha1.AccountExportLabelAccountID)
	newAccountID := newExport.GetLabel(v1alpha1.AccountExportLabelAccountID)
	if oldAccountID != newAccountID {
		return true
	}

	return !reflect.DeepEqual(accountExportDesiredClaimSnapshot(oldExport), accountExportDesiredClaimSnapshot(newExport))
}

func accountExportDesiredClaimSnapshot(export *v1alpha1.AccountExport) *accountExportDesiredClaimState {
	if export == nil || export.Status.DesiredClaim == nil {
		return nil
	}

	claim := export.Status.DesiredClaim
	rules := make([]v1alpha1.AccountExportRule, len(claim.Rules))
	copy(rules, claim.Rules)

	return &accountExportDesiredClaimState{
		ObservedGeneration: claim.ObservedGeneration,
		Rules:              rules,
	}
}

type accountExportDesiredClaimState struct {
	ObservedGeneration int64
	Rules              []v1alpha1.AccountExportRule
}

func (r *AccountReconciler) mapAccountImportToAccounts(ctx context.Context, obj client.Object) []reconcile.Request {
	imp, ok := obj.(*v1alpha1.AccountImport)
	if !ok {
		return nil
	}

	accountID := imp.GetLabel(v1alpha1.AccountImportLabelAccountID)
	if accountID == "" {
		return nil
	}

	accounts := &v1alpha1.AccountList{}
	if err := r.List(ctx, accounts,
		client.InNamespace(imp.Namespace),
		client.MatchingLabels{string(v1alpha1.AccountLabelAccountID): accountID},
	); err != nil {
		logf.FromContext(ctx).Error(err, "Failed to list Accounts for AccountImport watch", "accountID", accountID, "namespace", imp.Namespace)
		return nil
	}

	requests := make([]reconcile.Request, 0, len(accounts.Items))
	for _, account := range accounts.Items {
		requests = append(requests, reconcile.Request{
			NamespacedName: client.ObjectKeyFromObject(&account),
		})
	}

	return requests
}

func accountImportWatchPredicateForAccounts() predicate.Funcs {
	return predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			export, ok := e.Object.(*v1alpha1.AccountImport)
			return ok && export.GetLabel(v1alpha1.AccountImportLabelAccountID) != ""
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			export, ok := e.Object.(*v1alpha1.AccountImport)
			return ok && export.GetLabel(v1alpha1.AccountImportLabelAccountID) != ""
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			oldImport, oldOK := e.ObjectOld.(*v1alpha1.AccountImport)
			newImport, newOK := e.ObjectNew.(*v1alpha1.AccountImport)
			return oldOK && newOK && accountImportUpdateAffectsAccounts(oldImport, newImport)
		},
		GenericFunc: func(event.GenericEvent) bool {
			return false
		},
	}
}

func accountImportUpdateAffectsAccounts(oldImport *v1alpha1.AccountImport, newImport *v1alpha1.AccountImport) bool {
	if oldImport == nil || newImport == nil {
		return false
	}

	oldAccountID := oldImport.GetLabel(v1alpha1.AccountImportLabelAccountID)
	newAccountID := newImport.GetLabel(v1alpha1.AccountImportLabelAccountID)
	if oldAccountID != newAccountID {
		return true
	}

	return !reflect.DeepEqual(accountImportDesiredClaimSnapshot(oldImport), accountImportDesiredClaimSnapshot(newImport))
}

func accountImportDesiredClaimSnapshot(imp *v1alpha1.AccountImport) *accountImportDesiredClaimState {
	if imp == nil || imp.Status.DesiredClaim == nil {
		return nil
	}

	claim := imp.Status.DesiredClaim
	rules := make([]v1alpha1.AccountImportRuleDerived, len(claim.Rules))
	copy(rules, claim.Rules)

	return &accountImportDesiredClaimState{
		ObservedGeneration: claim.ObservedGeneration,
		Rules:              rules,
	}
}

type accountImportDesiredClaimState struct {
	ObservedGeneration int64
	Rules              []v1alpha1.AccountImportRuleDerived
}

type accountAdoptionRefs struct {
	exports []*adoptionRef
	imports []*adoptionRef
}

type adoptionRef struct {
	Ref                            nauth.Ref
	Name                           string
	UID                            types.UID
	ObservedGeneration             int64
	ObservedGenerationDesiredClaim *int64
}

func newAdoptionRef(resource metav1.ObjectMeta, observedGenerationDesiredClaim *int64) adoptionRef {
	return adoptionRef{
		Ref:                            nauth.Ref(resource.UID),
		Name:                           resource.Name,
		UID:                            resource.UID,
		ObservedGeneration:             resource.Generation,
		ObservedGenerationDesiredClaim: observedGenerationDesiredClaim,
	}
}
