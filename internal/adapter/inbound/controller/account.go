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

	"github.com/WirelessCar/nauth/internal/domain"
	"github.com/WirelessCar/nauth/internal/ports/inbound"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/events"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/WirelessCar/nauth/api/v1alpha1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// AccountReconciler reconciles an Account object
type AccountReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	manager  inbound.AccountManager
	reporter *statusReporter
}

func NewAccountReconciler(k8sClient client.Client, scheme *runtime.Scheme, manager inbound.AccountManager, recorder events.EventRecorder) *AccountReconciler {
	return &AccountReconciler{
		Client:   k8sClient,
		Scheme:   scheme,
		manager:  manager,
		reporter: newStatusReporter(k8sClient, recorder),
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

	err := r.Get(ctx, req.NamespacedName, natsAccount)
	if err != nil {
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

		if controllerutil.ContainsFinalizer(natsAccount, finalizerAccount) {
			if managementPolicy != v1alpha1.AccountManagementPolicyObserve {
				if err := r.manager.Delete(ctx, natsAccount); err != nil {
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
	var result *domain.AccountResult
	if managementPolicy == v1alpha1.AccountManagementPolicyObserve {
		result, err = r.manager.Import(ctx, natsAccount)
		if err != nil {
			return r.reporter.error(ctx, natsAccount, fmt.Errorf("failed to import the observed account: %w", err))
		}
	} else {
		resources, err := r.collectAccountResources(ctx, natsAccount, accountID)
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to collect resources: %w", err)
		}
		result, err = r.manager.CreateOrUpdate(ctx, *resources)
		if err != nil {
			return r.reporter.error(ctx, natsAccount, fmt.Errorf("failed to apply account: %w", err))
		}
	}

	// Apply result to Account resource labels and status
	natsAccount.SetLabel(v1alpha1.AccountLabelAccountID, result.AccountID)
	natsAccount.SetLabel(v1alpha1.AccountLabelSignedBy, result.AccountSignedBy)

	// UPDATE ACCOUNT STATUS
	if result.Claims != nil {
		natsAccount.Status.Claims = *result.Claims
	}
	natsAccount.Status.Adoptions = result.Adoptions
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

func (r *AccountReconciler) collectAccountResources(ctx context.Context, account *v1alpha1.Account, accountID string) (*domain.AccountResources, error) {
	result := domain.AccountResources{Account: *account}
	if accountID != "" {
		namespace := domain.Namespace(account.Namespace)
		exports, err := r.findExportsByAccountID(ctx, namespace, accountID)
		if err != nil {
			return nil, err
		}
		result.Exports = exports.Items
	}
	return &result, nil
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

// SetupWithManager sets up the controller with the Manager.
func (r *AccountReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.Account{}).
		Named("account").
		WithEventFilter(predicate.GenerationChangedPredicate{}).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: 1,
		}).
		Complete(r)
}
