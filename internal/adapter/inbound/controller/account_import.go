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

	"github.com/WirelessCar/nauth/api/v1alpha1"
	"github.com/WirelessCar/nauth/internal/domain"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/json"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

// AccountImportReconciler reconciles an AccountImport object.
type AccountImportReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

func NewAccountImportReconciler(k8sClient client.Client, scheme *runtime.Scheme) *AccountImportReconciler {
	return &AccountImportReconciler{
		Client: k8sClient,
		Scheme: scheme,
	}
}

// +kubebuilder:rbac:groups=nauth.io,resources=accountimports,verbs=get;list;watch

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

	if err := r.upsertLabels(ctx, state, importAccountID, exportAccountID); err != nil {
		log.Error(err, "Failed to upsert labels")
		return ctrl.Result{}, err
	}

	// TODO: [#11] Validate rules
	validRulesCondition := metav1.Condition{
		Type:    conditionTypeValidRules,
		Status:  metav1.ConditionFalse,
		Reason:  "NotImplemented",
		Message: "Rules validation not implemented",
	}

	// TODO: [#11] Check account adoption
	adoptedByAccountCondition := metav1.Condition{
		Type:    conditionTypeAdoptedByAccount,
		Status:  metav1.ConditionFalse,
		Reason:  "NotImplemented",
		Message: "Rules validation not implemented",
	}

	r.setConditions(state, importAccountCondition, exportAccountCondition, validRulesCondition, adoptedByAccountCondition)

	if err := r.Status().Update(ctx, state); err != nil {
		log.Error(err, "Failed to update status", "namespace", state.Namespace, "name", state.GetName())
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

func (r *AccountImportReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.AccountImport{}).
		Named("accountimport").
		WithEventFilter(predicate.GenerationChangedPredicate{}).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: 1,
		}).
		Complete(r)
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

func (r *AccountImportReconciler) upsertLabels(ctx context.Context, resource *v1alpha1.AccountImport, importAccountID string, exportAccountID string) error {
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
			return fmt.Errorf("failed to generate patch for labels: %w", err)
		}
		if err = r.Patch(ctx, resource, client.RawPatch(types.MergePatchType, patchData)); err != nil {
			return fmt.Errorf("failed to patch labels: %w", err)
		}
	}
	return nil
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
