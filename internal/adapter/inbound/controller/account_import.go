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
	"reflect"

	"github.com/WirelessCar/nauth/api/v1alpha1"
	"github.com/WirelessCar/nauth/internal/domain"
	"github.com/WirelessCar/nauth/internal/domain/nauth"
	"github.com/WirelessCar/nauth/internal/ports/inbound"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/json"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const importAccountNameIndexKey string = "import.spec.accountName"

// AccountImportReconciler reconciles an AccountImport object.
type AccountImportReconciler struct {
	client.Client
	Scheme  *runtime.Scheme
	manager inbound.AccountImportManager
}

func NewAccountImportReconciler(k8sClient client.Client, scheme *runtime.Scheme, manager inbound.AccountImportManager) *AccountImportReconciler {
	return &AccountImportReconciler{
		Client:  k8sClient,
		Scheme:  scheme,
		manager: manager,
	}
}

// +kubebuilder:rbac:groups=nauth.io,resources=accountimports,verbs=get;list;watch;patch
// +kubebuilder:rbac:groups=nauth.io,resources=accountimports/status,verbs=get;update;patch

func (r *AccountImportReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	state := &v1alpha1.AccountImport{}
	if err := r.Get(ctx, req.NamespacedName, state); err != nil {
		if apierrors.IsNotFound(err) {
			log.Info("resource not found. Ignoring since object must be deleted")
			return ctrl.Result{}, nil
		}

		log.Error(err, "Failed to get resource")
		return ctrl.Result{}, err
	}

	importAccountRef := domain.NewNamespacedName(state.Namespace, state.Spec.AccountName)
	importAccountID := state.GetLabel(v1alpha1.AccountImportLabelAccountID)
	importAccount, importAccountCondition := r.getConditionedAccount(ctx, importAccountRef, importAccountID)
	importAccountCondition.Type = conditionTypeBoundToAccount
	if importAccount != nil {
		importAccountID = importAccount.GetLabel(v1alpha1.AccountLabelAccountID)
	}
	meta.SetStatusCondition(state.GetConditions(), importAccountCondition)

	exportAccountNamespace := state.Spec.ExportAccountRef.Namespace
	if exportAccountNamespace == "" {
		exportAccountNamespace = state.Namespace
	}
	exportAccountRef := domain.NewNamespacedName(exportAccountNamespace, state.Spec.ExportAccountRef.Name)
	exportAccountID := state.GetLabel(v1alpha1.AccountImportLabelExportAccountID)
	exportAccount, exportAccountCondition := r.getConditionedAccount(ctx, exportAccountRef, exportAccountID)
	exportAccountCondition.Type = conditionTypeBoundToExportAccount
	if exportAccount != nil {
		exportAccountID = exportAccount.GetLabel(v1alpha1.AccountLabelAccountID)
	}
	meta.SetStatusCondition(state.GetConditions(), exportAccountCondition)

	if labelsUpdated, err := r.upsertLabels(ctx, state, importAccountID, exportAccountID); err != nil {
		log.Error(err, "Failed to upsert labels")
		return ctrl.Result{}, err
	} else if labelsUpdated {
		return ctrl.Result{RequeueAfter: requeueImmediately}, nil
	}

	derivedRules, validRulesErr := r.validateImports(importAccountID, exportAccountID, state.Spec.Rules)
	var validRulesCondition metav1.Condition
	if validRulesErr != nil {
		validRulesCondition = metav1.Condition{
			Type:    conditionTypeValidRules,
			Status:  metav1.ConditionFalse,
			Reason:  conditionReasonNOK,
			Message: validRulesErr.Error(),
		}
	} else {
		validRulesCondition = metav1.Condition{
			Type:    conditionTypeValidRules,
			Status:  metav1.ConditionTrue,
			Reason:  conditionReasonOK,
			Message: "Rules validation successful",
		}

		state.Status.DesiredClaim = &v1alpha1.AccountImportClaim{
			ObservedGeneration: state.Generation,
			Rules:              derivedRules,
		}
	}

	adoptedByAccountCondition := metav1.Condition{
		Type:    conditionTypeAdoptedByAccount,
		Status:  metav1.ConditionFalse,
		Reason:  conditionReasonAdopting,
		Message: "waiting for account to adopt import",
	}

	if importAccount != nil {
		var adoption *v1alpha1.AccountAdoption
		if importAccount.Status.Adoptions != nil {
			adoption = findAdoptionByUID(importAccount.Status.Adoptions.Imports, state.UID)
		}
		if adoption != nil {
			adoptionGen := adoption.Status.DesiredClaimObservedGeneration
			sameGeneration := adoptionGen != nil && *adoptionGen == state.Generation
			if adoption.Status.Status == metav1.ConditionTrue && sameGeneration {
				adoptedByAccountCondition.Status = metav1.ConditionTrue
				adoptedByAccountCondition.Reason = conditionReasonOK
				adoptedByAccountCondition.Message = ""
			} else if !sameGeneration {
				adoptedByAccountCondition.Message = fmt.Sprintf("waiting for account to adopt generation %d", state.Generation)
			} else {
				adoptedByAccountCondition.Reason = conditionReasonFailed
				adoptedByAccountCondition.Message = fmt.Sprintf("%s: %s", adoption.Status.Reason, adoption.Status.Message)
			}
		}
	}

	r.setConditions(state, importAccountCondition, exportAccountCondition, validRulesCondition, adoptedByAccountCondition)

	if err := r.Status().Update(ctx, state); err != nil {
		log.Error(err, "Failed to update status", "namespace", state.Namespace, "name", state.GetName())
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

func (r *AccountImportReconciler) validateImports(importAccountID string, exportAccountID string, rules []v1alpha1.AccountImportRule) ([]v1alpha1.AccountImportRuleDerived, error) {
	if importAccountID == "" {
		return nil, fmt.Errorf("import account ID is required")
	} else if exportAccountID == "" {
		return nil, fmt.Errorf("export account ID is required")
	} else if len(rules) == 0 {
		return nil, fmt.Errorf("at least one rule is required")
	}

	imports, err := toNAuthImportsFromRules(exportAccountID, rules)
	if err != nil {
		return nil, fmt.Errorf("failed to convert rules to domain imports: %w", err)
	}
	if err = r.manager.ValidateImports(nauth.AccountID(importAccountID), imports); err != nil {
		return nil, fmt.Errorf("rules validation failed: %w", err)
	}
	result := make([]v1alpha1.AccountImportRuleDerived, len(imports))
	for i, imp := range imports {
		derived, err := toAPIAccountImportRuleDerived(*imp)
		if err != nil {
			return nil, fmt.Errorf("failed to convert rule to derived rule for import at index %d: %w", i, err)
		}
		result[i] = *derived
	}
	return result, nil
}

func (r *AccountImportReconciler) SetupWithManager(mgr ctrl.Manager) error {
	if err := mgr.GetFieldIndexer().IndexField(
		context.Background(),
		&v1alpha1.AccountImport{},
		importAccountNameIndexKey,
		importBySpecAccountNameIndexFunc,
	); err != nil {
		return fmt.Errorf("failed to index AccountImport by account name: %w", err)
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.AccountImport{}, builder.WithPredicates(predicate.GenerationChangedPredicate{})).
		Named("accountimport").
		WithOptions(controller.Options{
			MaxConcurrentReconciles: 1,
		}).
		Watches(
			&v1alpha1.Account{},
			handler.EnqueueRequestsFromMapFunc(r.mapAccountToAccountImports),
			builder.WithPredicates(accountImportAccountWatchPredicate()),
		).
		Complete(r)
}

func importBySpecAccountNameIndexFunc(rawObj client.Object) []string {
	imp := rawObj.(*v1alpha1.AccountImport)
	if imp.Spec.AccountName == "" {
		return nil
	}
	return []string{imp.Spec.AccountName}
}

func (r *AccountImportReconciler) mapAccountToAccountImports(ctx context.Context, obj client.Object) []reconcile.Request {
	account, ok := obj.(*v1alpha1.Account)
	if !ok {
		return nil
	}

	imports := &v1alpha1.AccountImportList{}
	if err := r.List(ctx, imports,
		client.InNamespace(account.Namespace),
		client.MatchingFields{importAccountNameIndexKey: account.Name},
	); err != nil {
		logf.FromContext(ctx).Error(err, "Failed to list AccountImports for Account watch", "account", account.Name, "namespace", account.Namespace)
		return nil
	}

	requests := make([]reconcile.Request, 0, len(imports.Items))
	for _, export := range imports.Items {
		requests = append(requests, reconcile.Request{
			NamespacedName: types.NamespacedName{
				Namespace: export.Namespace,
				Name:      export.Name,
			},
		})
	}

	return requests
}

func accountImportAccountWatchPredicate() predicate.Funcs {
	return predicate.Funcs{
		CreateFunc: func(event.CreateEvent) bool { return true },
		DeleteFunc: func(event.DeleteEvent) bool { return true },
		UpdateFunc: func(e event.UpdateEvent) bool {
			oldAccount, oldOK := e.ObjectOld.(*v1alpha1.Account)
			newAccount, newOK := e.ObjectNew.(*v1alpha1.Account)
			return oldOK && newOK && accountUpdateAffectsAccountImports(oldAccount, newAccount)
		},
		GenericFunc: func(event.GenericEvent) bool { return false },
	}
}

func accountUpdateAffectsAccountImports(oldAccount *v1alpha1.Account, newAccount *v1alpha1.Account) bool {
	if oldAccount == nil || newAccount == nil {
		return false
	}

	oldAccountID := oldAccount.GetLabel(v1alpha1.AccountLabelAccountID)
	newAccountID := newAccount.GetLabel(v1alpha1.AccountLabelAccountID)
	if oldAccountID != newAccountID {
		return true
	}

	return !reflect.DeepEqual(accountImportAdoptionSnapshot(oldAccount), accountImportAdoptionSnapshot(newAccount))
}

// Builds a comparable snapshot of import adoption state so Account changes relevant to AccountImports can be detected efficiently.
func accountImportAdoptionSnapshot(account *v1alpha1.Account) map[string]adoptionState {
	if account == nil || account.Status.Adoptions == nil {
		return nil
	}

	adoptions := make(map[string]adoptionState, len(account.Status.Adoptions.Imports))
	for _, adoption := range account.Status.Adoptions.Imports {
		adoptions[string(adoption.UID)] = adoptionState{
			ObservedGeneration:             adoption.ObservedGeneration,
			Status:                         adoption.Status.Status,
			Reason:                         adoption.Status.Reason,
			Message:                        adoption.Status.Message,
			DesiredClaimObservedGeneration: adoption.Status.DesiredClaimObservedGeneration,
		}
	}
	return adoptions
}

func (r *AccountImportReconciler) getConditionedAccount(ctx context.Context, accountRef domain.NamespacedName, boundAccountID string) (*v1alpha1.Account, metav1.Condition) {
	if err := accountRef.Validate(); err != nil {
		return nil, metav1.Condition{
			Status:  metav1.ConditionFalse,
			Reason:  string(metav1.StatusReasonInvalid),
			Message: fmt.Sprintf("Invalid Account reference %s: %q", accountRef, err),
		}
	}

	result := &v1alpha1.Account{}
	if err := r.Get(ctx, client.ObjectKey{Namespace: accountRef.Namespace, Name: accountRef.Name}, result); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, metav1.Condition{
				Status:  metav1.ConditionFalse,
				Reason:  string(metav1.StatusReasonNotFound),
				Message: fmt.Sprintf("Account %s not found", accountRef),
			}
		}
		return nil, metav1.Condition{
			Status:  metav1.ConditionFalse,
			Reason:  string(metav1.StatusReasonInternalError),
			Message: fmt.Sprintf("Failed to get Account %s: %q", accountRef, err),
		}
	}

	accountID := result.GetLabel(v1alpha1.AccountLabelAccountID)
	if accountID == "" {
		return nil, metav1.Condition{
			Status:  metav1.ConditionFalse,
			Reason:  string(metav1.StatusReasonInvalid),
			Message: fmt.Sprintf("Account %s not bound to an Account ID yet", accountRef),
		}
	}

	if boundAccountID != "" && boundAccountID != accountID {
		return nil, metav1.Condition{
			Status:  metav1.ConditionFalse,
			Reason:  string(metav1.StatusReasonConflict),
			Message: fmt.Sprintf("Account ID conflict: previously bound to %s, now found %s", boundAccountID, accountID),
		}
	}

	return result, metav1.Condition{
		Status:  metav1.ConditionTrue,
		Reason:  conditionReasonOK,
		Message: fmt.Sprintf("Account ID: %s", accountID),
	}
}

func (r *AccountImportReconciler) upsertLabels(ctx context.Context, resource *v1alpha1.AccountImport, importAccountID string, exportAccountID string) (updated bool, err error) {
	patch := false
	if importAccountID != "" && importAccountID != resource.GetLabel(v1alpha1.AccountImportLabelAccountID) {
		resource.SetLabel(v1alpha1.AccountImportLabelAccountID, importAccountID)
		patch = true
	}
	if exportAccountID != "" && exportAccountID != resource.GetLabel(v1alpha1.AccountImportLabelExportAccountID) {
		resource.SetLabel(v1alpha1.AccountImportLabelExportAccountID, exportAccountID)
		patch = true
	}

	if patch {
		patchData, err := json.Marshal(map[string]map[string]map[string]string{
			"metadata": {
				"labels": resource.GetLabels(),
			},
		})
		if err != nil {
			return false, fmt.Errorf("failed to generate patch for labels: %w", err)
		}
		if err = r.Patch(ctx, resource, client.RawPatch(types.MergePatchType, patchData)); err != nil {
			return false, fmt.Errorf("failed to patch labels: %w", err)
		}
		return true, nil
	}
	return false, nil
}

func (r *AccountImportReconciler) setConditions(state *v1alpha1.AccountImport, subConditions ...metav1.Condition) {
	ready := metav1.Condition{
		Type:               conditionTypeReady,
		ObservedGeneration: state.Generation,
		Status:             metav1.ConditionTrue,
		Reason:             conditionReasonReady,
	}
	for _, subCondition := range subConditions {
		subCondition.ObservedGeneration = state.Generation
		meta.SetStatusCondition(state.GetConditions(), subCondition)
		if subCondition.Status != metav1.ConditionTrue {
			ready.Status = metav1.ConditionFalse
			ready.Reason = conditionReasonNotReady
		}
	}
	meta.SetStatusCondition(state.GetConditions(), ready)
}
