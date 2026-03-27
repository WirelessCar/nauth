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

	"github.com/WirelessCar/nauth/internal/adapter/outbound/k8s" // TODO: [#185] Controller must not depend on other adapter code
	"github.com/WirelessCar/nauth/internal/domain"
	"github.com/WirelessCar/nauth/internal/ports/inbound"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/events"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

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
		// The account is being deleted
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
		userList := &v1alpha1.UserList{}
		err := r.List(ctx, userList, client.MatchingLabels{k8s.LabelUserAccountID: accountID}, client.InNamespace(req.Namespace))
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

		if controllerutil.ContainsFinalizer(natsAccount, controllerAccountFinalizer) {
			if managementPolicy != k8s.LabelManagementPolicyObserveValue {
				if err := r.manager.Delete(ctx, natsAccount); err != nil {
					return r.reporter.error(ctx, natsAccount, fmt.Errorf("failed to delete account: %w", err))
				}
			}

			// remove our finalizer from the list and update it.
			controllerutil.RemoveFinalizer(natsAccount, controllerAccountFinalizer)
			if err := r.Update(ctx, natsAccount); err != nil {
				log.Info("failed to remove finalizer", "name", natsAccount.Name, "error", err)
				return ctrl.Result{}, err
			}
		}
		// Stop reconciliation as the item is being deleted
		return ctrl.Result{}, nil
	}

	operatorVersion := os.Getenv(EnvOperatorVersion)

	// Nothing has changed
	if natsAccount.Status.ObservedGeneration == natsAccount.Generation && natsAccount.Status.OperatorVersion == operatorVersion {
		// FIXME: Revise statement as we are now dependant on external (changeable) resources, like NatsCluster
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

	// RECONCILE ACCOUNT
	var result *domain.AccountResult

	_, _ = r.getLabelNatsClusterRef(natsAccount) // FIXME: Supply the previous NatsClusterRef to the manager
	if managementPolicy == k8s.LabelManagementPolicyObserveValue {
		result, err = r.manager.Import(ctx, natsAccount)
		if err != nil {
			return r.reporter.error(ctx, natsAccount, fmt.Errorf("failed to import the observed account: %w", err))
		}
	} else {
		result, err = r.manager.CreateOrUpdate(ctx, natsAccount)
		if err != nil {
			return r.reporter.error(ctx, natsAccount, fmt.Errorf("failed to apply account: %w", err))
		}
	}

	// Apply result to Account resource labels and status
	if natsAccount.Labels == nil {
		natsAccount.Labels = make(map[string]string)
	}
	natsAccount.Labels[k8s.LabelAccountID] = result.AccountID
	natsAccount.Labels[k8s.LabelAccountSignedBy] = result.AccountSignedBy
	r.setLabelNatsClusterRefLabel(result, natsAccount)

	// UPDATE ACCOUNT STATUS
	natsAccount.Status.Claims = v1alpha1.AccountClaims{}
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

	return r.reporter.status(ctx, natsAccount)
}

func (r *AccountReconciler) getLabelNatsClusterRef(natsAccount *v1alpha1.Account) (*domain.NamespacedName, error) {
	return domain.ParseNamespacedName(natsAccount.Labels[LabelAccountNatsClusterRef])
}

func (r *AccountReconciler) setLabelNatsClusterRefLabel(result *domain.AccountResult, natsAccount *v1alpha1.Account) {
	var natsClusterRef string
	if result.NatsClusterRef != nil {
		natsClusterRef = result.NatsClusterRef.Name
		natsAccount.Labels[LabelAccountNatsClusterRef] = result.NatsClusterRef.String()
	}
	if natsClusterRef == "" {
		natsClusterRef = LabelValueUnknown
	}
	natsAccount.Labels[LabelAccountNatsClusterRef] = natsClusterRef
}

// SetupWithManager sets up the controller with the Manager.
func (r *AccountReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.Account{}).
		Named("account").
		Watches(
			&v1alpha1.NatsCluster{},
			handler.EnqueueRequestsFromMapFunc(r.requestsForNatsCluster),
		).
		WithEventFilter(predicate.GenerationChangedPredicate{}).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: 1,
		}).
		Complete(r)
}

func (r *AccountReconciler) requestsForNatsCluster(ctx context.Context, obj client.Object) []reconcile.Request {
	natsCluster, ok := obj.(*v1alpha1.NatsCluster)
	if !ok {
		return nil
	}
	natsClusterRef := domain.NewNamespacedName(natsCluster.Namespace, natsCluster.Name)

	if err := natsClusterRef.Validate(); err != nil {
		logf.FromContext(ctx).Error(err, "failed to parse NamespacedName from NatsCluster reference",
			"namespace", natsCluster.Namespace,
			"name", natsCluster.Name)
		return []reconcile.Request{}
	}

	var requests []reconcile.Request
	var accounts v1alpha1.AccountList
	if err := r.List(ctx, &accounts, client.MatchingLabels{LabelAccountNatsClusterRef: natsClusterRef.String()}); err != nil {
		logf.FromContext(ctx).Error(err, "Failed to list accounts labelled with NatsCluster", "natsCluster", natsClusterRef)
		return nil
	}
	for _, account := range accounts.Items {
		requests = append(requests, reconcile.Request{NamespacedName: types.NamespacedName{
			Namespace: account.Namespace,
			Name:      account.Name,
		}})
	}

	if err := r.List(ctx, &accounts, client.MatchingLabels{LabelAccountNatsClusterRef: LabelValueUnknown}); err != nil {
		logf.FromContext(ctx).Error(err, "Failed to list accounts labelled with unknown NatsCluster")
		return nil
	}
	for _, account := range accounts.Items {
		requests = append(requests, reconcile.Request{NamespacedName: types.NamespacedName{
			Namespace: account.Namespace,
			Name:      account.Name,
		}})
	}

	return requests
}
