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
	"time"

	"github.com/WirelessCar/nauth/api/v1alpha1"
	"github.com/WirelessCar/nauth/internal/domain"
	"github.com/WirelessCar/nauth/internal/ports/inbound"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

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

	state := &v1alpha1.AccountExport{}
	if err := r.Get(ctx, req.NamespacedName, state); err != nil {
		if apierrors.IsNotFound(err) {
			log.Info("resource not found. Ignoring since object must be deleted")
			return ctrl.Result{}, nil
		}

		log.Error(err, "Failed to get resource")
		return ctrl.Result{}, err
	}

	account := &v1alpha1.Account{}
	if err := r.Get(ctx, types.NamespacedName{Namespace: state.Namespace, Name: state.Spec.AccountName}, account); err != nil {
		log.Error(err, "Failed to get account", "accountName", state.Spec.AccountName)
	}

	resolution := r.manager.Resolve(ctx, state, account)
	patchLabels, updateStatus := mapResolutionToStatus(resolution, state)

	if patchLabels {
		err := r.patchLabels(ctx, state)
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to patch labels: %w", err)
		}
		return ctrl.Result{RequeueAfter: time.Second * 2}, nil
	}

	if updateStatus {
		if updateErr := r.Status().Update(ctx, state); updateErr != nil {
			log.Error(updateErr, "Failed to update status")
			return ctrl.Result{}, updateErr
		}
	}

	log.Info("Reconciliation complete")
	return ctrl.Result{}, nil
}

func (r *AccountExportReconciler) SetupWithManager(mgr ctrl.Manager) error {
	// Todo: #11 Add watch on Account
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.AccountExport{}).
		Named("accountexport").
		WithEventFilter(predicate.GenerationChangedPredicate{}).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: 1,
		}).
		Complete(r)
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

func mapResolutionToStatus(resolution *domain.AccountExportResolution, state *v1alpha1.AccountExport) (patchLabels bool, updateStatus bool) {
	patchLabels = false
	updateStatus = true
	status := &state.Status

	updateConditionFalse := func(condType string, reason string, msg string) {
		meta.SetStatusCondition(&status.Conditions, newCondition(condType, metav1.ConditionFalse, reason, msg))
	}
	updateConditionTrue := func(condType string) {
		meta.SetStatusCondition(&status.Conditions, newCondition(condType, metav1.ConditionTrue, conditionReasonOK, ""))
	}

	// rule validations
	if resolution.ValidationError != nil {
		updateConditionFalse(conditionTypeValidRules, conditionReasonInvalid, resolution.ValidationError.Error())
	} else {
		updateConditionTrue(conditionTypeValidRules)
		status.DesiredClaim = &v1alpha1.AccountExportClaim{
			Rules:              resolution.DesiredClaim.Rules,
			ObservedGeneration: resolution.ObservedGeneration,
		}
	}

	// account lookup
	if resolution.AccountID != "" {
		status.AccountID = resolution.AccountID
	}

	// account binding
	switch resolution.BindingState {
	case domain.AccountBindingStateMissing:
		if resolution.AccountID != "" {
			state.SetLabel(v1alpha1.AccountExportLabelAccountID, resolution.AccountID)
			patchLabels = true
			msg := fmt.Sprintf("Binding to Account ID: %s", resolution.AccountID)
			updateConditionFalse(conditionTypeBoundToAccount, conditionReasonReconciling, msg)
		} else {
			msg := "Account not found or could not be read"
			updateConditionFalse(conditionTypeBoundToAccount, conditionReasonErrored, msg)
		}

	case domain.AccountBindingStateConflict:
		msg := fmt.Sprintf("Account ID conflict: previously bound to %s, now found %s", resolution.BoundAccountID, resolution.AccountID)
		updateConditionFalse(conditionTypeBoundToAccount, conditionReasonConflict, msg)

	case domain.AccountBindingStateBound:
		updateConditionTrue(conditionTypeBoundToAccount)

	default:
		updateConditionFalse(conditionTypeBoundToAccount, conditionReasonErrored, "Unknown binding state")
	}

	// account adoption condition
	switch resolution.AdoptionState {
	case domain.AccountAdoptionStateMissing:
		if resolution.AccountID == "" {
			msg := "Account not found or could not be read"
			updateConditionFalse(conditionTypeAdoptedByAccount, conditionReasonErrored, msg)
		} else {
			updateConditionFalse(conditionTypeAdoptedByAccount, conditionReasonErrored, resolution.AdoptionError.Error())
		}

	case domain.AccountAdoptionStateNotAdopted:
		updateConditionFalse(conditionTypeAdoptedByAccount, conditionReasonReconciling, resolution.AdoptionError.Error())

	case domain.AccountAdoptionStateAdopted:
		updateConditionTrue(conditionTypeAdoptedByAccount)

	default:
		updateConditionFalse(conditionTypeBoundToAccount, conditionReasonErrored, "Unknown adopted state")
	}

	// ready condition
	if isAccountExportReady(state) {
		updateConditionTrue(conditionTypeReady)
	} else {
		updateConditionFalse(conditionTypeReady, conditionReasonReconciling, "Reconciling")
	}

	return patchLabels, updateStatus
}

func isAccountExportReady(state *v1alpha1.AccountExport) bool {
	conditionsReady := conditionsReady(state.Status.Conditions, []string{
		conditionTypeBoundToAccount,
		conditionTypeValidRules,
		conditionTypeAdoptedByAccount,
	})
	claimsReady := state.Status.DesiredClaim != nil && state.Status.DesiredClaim.ObservedGeneration == state.Generation

	return conditionsReady && claimsReady
}
