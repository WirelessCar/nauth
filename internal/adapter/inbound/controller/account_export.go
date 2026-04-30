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
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/WirelessCar/nauth/api/v1alpha1"
	"github.com/WirelessCar/nauth/internal/domain/nauth"
	"github.com/WirelessCar/nauth/internal/ports/inbound"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
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

const exportAccountNameIndexKey string = "export.spec.accountName"

// AccountExportReconciler reconciles an AccountExport object.
type AccountExportReconciler struct {
	client.Client
	Scheme  *runtime.Scheme
	manager inbound.AccountExportManager
}

func NewAccountExportReconciler(k8sClient client.Client, scheme *runtime.Scheme, manager inbound.AccountExportManager) *AccountExportReconciler {
	return &AccountExportReconciler{
		Client:  k8sClient,
		Scheme:  scheme,
		manager: manager,
	}
}

// +kubebuilder:rbac:groups=nauth.io,resources=accountexports,verbs=get;list;watch;patch
// +kubebuilder:rbac:groups=nauth.io,resources=accountexports/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=core,resources=events,verbs=create;patch

func (r *AccountExportReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	// TODO: #11 Consider adding events

	state := &v1alpha1.AccountExport{}
	if err := r.Get(ctx, req.NamespacedName, state); err != nil {
		if apierrors.IsNotFound(err) {
			log.Info("resource not found. Ignoring since object must be deleted")
			return ctrl.Result{}, nil
		}

		log.Error(err, "Failed to get resource")
		return ctrl.Result{}, err
	}

	updateConditionFalse := func(condType string, reason string, msg string) {
		meta.SetStatusCondition(state.GetConditions(), newCondition(condType, metav1.ConditionFalse, reason, msg))
	}
	updateConditionTrue := func(condType string) {
		meta.SetStatusCondition(state.GetConditions(), newCondition(condType, metav1.ConditionTrue, conditionReasonOK, ""))
	}

	var exports nauth.Exports
	for _, rule := range state.Spec.Rules {
		export := mapToExport(rule)
		exports = append(exports, &export)
	}

	err := r.manager.ValidateExports(exports)
	if err != nil {
		updateConditionFalse(conditionTypeValidRules, conditionReasonInvalid, err.Error())
	} else {
		updateConditionTrue(conditionTypeValidRules)
		state.Status.DesiredClaim = &v1alpha1.AccountExportClaim{
			Rules:              state.Spec.Rules,
			ObservedGeneration: state.Generation,
		}
	}

	patchLabels := false

	account := &v1alpha1.Account{}
	accountErr := r.Get(ctx, types.NamespacedName{Namespace: state.Namespace, Name: state.Spec.AccountName}, account)
	if accountErr != nil {
		if !apierrors.IsNotFound(err) {
			log.Error(err, "Failed to read account", "accountName", state.Spec.AccountName)
			msg := fmt.Sprintf("failed to read account: %v", err)
			updateConditionFalse(conditionTypeBoundToAccount, conditionReasonFailed, msg)
		} else {
			// account not found
			msg := "Account not found"
			updateConditionFalse(conditionTypeBoundToAccount, conditionReasonNotFound, msg)
		}
	} else {
		// account binding
		accountID := account.GetLabel(v1alpha1.AccountLabelAccountID)
		state.Status.AccountID = accountID
		existingLabel := state.GetLabel(v1alpha1.AccountExportLabelAccountID)

		if existingLabel == "" && accountID != "" {
			// bind
			state.SetLabel(v1alpha1.AccountExportLabelAccountID, accountID)
			patchLabels = true
		} else if existingLabel != "" && existingLabel != accountID {
			// conflict
			msg := fmt.Sprintf("account export is already bound to account: %s", existingLabel)
			updateConditionFalse(conditionTypeBoundToAccount, conditionReasonConflict, msg)
		} else {
			// bound
			updateConditionTrue(conditionTypeBoundToAccount)
		}

		adoption := findAdoptionByUID(account, state.UID)
		if adoption == nil {
			// adoption not found
			updateConditionFalse(conditionTypeAdoptedByAccount, conditionReasonAdopting, "waiting for account to adopt export")
		} else {
			adoptionGen := adoption.Status.DesiredClaimObservedGeneration
			sameGeneration := adoptionGen != nil && *adoptionGen == state.Generation
			if adoption.Status.Status == metav1.ConditionTrue && sameGeneration {
				updateConditionTrue(conditionTypeAdoptedByAccount)
			} else if !sameGeneration {
				msg := fmt.Sprintf("waiting for account to adopt generation %d", state.Generation)
				updateConditionFalse(conditionTypeAdoptedByAccount, conditionReasonAdopting, msg)
			} else {
				msg := fmt.Sprintf("%s: %s", adoption.Status.Reason, adoption.Status.Message)
				updateConditionFalse(conditionTypeAdoptedByAccount, conditionReasonFailed, msg)
			}

		}
	}

	// ready condition
	if isAccountExportReady(state) {
		updateConditionTrue(conditionTypeReady)
	} else {
		updateConditionFalse(conditionTypeReady, conditionReasonReconciling, "All conditions not met")
	}

	if patchLabels {
		err = r.patchLabels(ctx, state)
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to patch labels: %w", err)
		}
		return ctrl.Result{RequeueAfter: requeueImmediately}, nil
	}

	if updateErr := r.Status().Update(ctx, state); updateErr != nil {
		log.Error(updateErr, "Failed to update status: %w", updateErr)
		return ctrl.Result{}, updateErr
	}

	return ctrl.Result{}, nil
}

func mapToExport(rule v1alpha1.AccountExportRule) nauth.Export {
	result := nauth.Export{
		Name:         rule.Name,
		Subject:      nauth.Subject(rule.Subject),
		Type:         mapExportType(rule.Type),
		ResponseType: nauth.ResponseType(rule.ResponseType),
	}
	if rule.ResponseThreshold != nil {
		result.ResponseThreshold = *rule.ResponseThreshold
	}

	if rule.Latency != nil {
		result.Latency = &nauth.ServiceLatency{
			Sampling: nauth.SamplingRate(rule.Latency.Sampling),
			Results:  nauth.Subject(rule.Latency.Results),
		}
	}
	if rule.AccountTokenPosition != nil {
		result.AccountTokenPosition = *rule.AccountTokenPosition
	}
	if rule.Advertise != nil {
		result.Advertise = *rule.Advertise
	}
	if rule.AllowTrace != nil {
		result.AllowTrace = *rule.AllowTrace
	}

	return result
}

func mapExportType(t v1alpha1.ExportType) nauth.ExportType {
	switch t {
	case v1alpha1.Service:
		return nauth.ExportTypeService
	case v1alpha1.Stream:
		return nauth.ExportTypeStream
	}

	return nauth.ExportTypeUnknown
}

func (r *AccountExportReconciler) SetupWithManager(mgr ctrl.Manager) error {
	if err := mgr.GetFieldIndexer().IndexField(
		context.Background(),
		&v1alpha1.AccountExport{},
		exportAccountNameIndexKey,
		exportBySpecAccountNameIndexFunc,
	); err != nil {
		return fmt.Errorf("failed to index AccountExport by account name: %w", err)
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.AccountExport{}, builder.WithPredicates(predicate.GenerationChangedPredicate{})).
		Named("accountexport").
		WithOptions(controller.Options{
			MaxConcurrentReconciles: 1,
		}).
		Watches(
			&v1alpha1.Account{},
			handler.EnqueueRequestsFromMapFunc(r.mapAccountToAccountExports),
			builder.WithPredicates(accountExportAccountWatchPredicate()),
		).
		Complete(r)
}

func exportBySpecAccountNameIndexFunc(rawObj client.Object) []string {
	export := rawObj.(*v1alpha1.AccountExport)
	if export.Spec.AccountName == "" {
		return nil
	}
	return []string{export.Spec.AccountName}
}

func (r *AccountExportReconciler) mapAccountToAccountExports(ctx context.Context, obj client.Object) []reconcile.Request {
	account, ok := obj.(*v1alpha1.Account)
	if !ok {
		return nil
	}

	exports := &v1alpha1.AccountExportList{}
	if err := r.List(ctx, exports,
		client.InNamespace(account.Namespace),
		client.MatchingFields{exportAccountNameIndexKey: account.Name},
	); err != nil {
		logf.FromContext(ctx).Error(err, "Failed to list AccountExports for Account watch", "account", account.Name, "namespace", account.Namespace)
		return nil
	}

	requests := make([]reconcile.Request, 0, len(exports.Items))
	for _, export := range exports.Items {
		requests = append(requests, reconcile.Request{
			NamespacedName: types.NamespacedName{
				Namespace: export.Namespace,
				Name:      export.Name,
			},
		})
	}

	return requests
}

func accountExportAccountWatchPredicate() predicate.Funcs {
	return predicate.Funcs{
		CreateFunc: func(event.CreateEvent) bool { return true },
		DeleteFunc: func(event.DeleteEvent) bool { return true },
		UpdateFunc: func(e event.UpdateEvent) bool {
			oldAccount, oldOK := e.ObjectOld.(*v1alpha1.Account)
			newAccount, newOK := e.ObjectNew.(*v1alpha1.Account)
			return oldOK && newOK && accountUpdateAffectsAccountExports(oldAccount, newAccount)
		},
		GenericFunc: func(event.GenericEvent) bool { return false },
	}
}

func accountUpdateAffectsAccountExports(oldAccount *v1alpha1.Account, newAccount *v1alpha1.Account) bool {
	if oldAccount == nil || newAccount == nil {
		return false
	}

	oldAccountID := oldAccount.GetLabel(v1alpha1.AccountLabelAccountID)
	newAccountID := newAccount.GetLabel(v1alpha1.AccountLabelAccountID)
	if oldAccountID != newAccountID {
		return true
	}

	return !reflect.DeepEqual(accountExportAdoptionSnapshot(oldAccount), accountExportAdoptionSnapshot(newAccount))
}

// Builds a comparable snapshot of export adoption state so Account changes relevant to AccountExports can be detected efficiently.
func accountExportAdoptionSnapshot(account *v1alpha1.Account) map[string]accountExportAdoptionState {
	if account == nil || account.Status.Adoptions == nil {
		return nil
	}

	adoptions := make(map[string]accountExportAdoptionState, len(account.Status.Adoptions.Exports))
	for _, adoption := range account.Status.Adoptions.Exports {
		adoptions[string(adoption.UID)] = accountExportAdoptionState{
			ObservedGeneration:             adoption.ObservedGeneration,
			Status:                         adoption.Status.Status,
			Reason:                         adoption.Status.Reason,
			Message:                        adoption.Status.Message,
			DesiredClaimObservedGeneration: adoption.Status.DesiredClaimObservedGeneration,
		}
	}
	return adoptions
}

type accountExportAdoptionState struct {
	ObservedGeneration             int64
	Status                         metav1.ConditionStatus
	Reason                         string
	Message                        string
	DesiredClaimObservedGeneration *int64
}

func (r *AccountExportReconciler) patchLabels(ctx context.Context, resource *v1alpha1.AccountExport) error {
	patchData, err := json.Marshal(map[string]map[string]map[string]string{
		"metadata": {
			"labels": resource.GetLabels(),
		},
	})
	if err != nil {
		return fmt.Errorf("failed to generate patch for label: %w", err)
	}
	if err = r.Patch(ctx, resource, client.RawPatch(types.MergePatchType, patchData)); err != nil {
		return fmt.Errorf("failed to patch label: %w", err)
	}
	return nil
}

func isAccountExportReady(state *v1alpha1.AccountExport) bool {
	allConditionsReady := conditionsReady(state.Status.Conditions, []string{
		conditionTypeBoundToAccount,
		conditionTypeValidRules,
		conditionTypeAdoptedByAccount,
	})
	claimsReady := state.Status.DesiredClaim != nil && state.Status.DesiredClaim.ObservedGeneration == state.Generation

	return allConditionsReady && claimsReady
}
