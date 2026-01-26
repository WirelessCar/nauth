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

	"k8s.io/client-go/tools/record"

	"github.com/WirelessCar/nauth/internal/k8s"
	"github.com/WirelessCar/nauth/internal/system"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	natsv1alpha1 "github.com/WirelessCar/nauth/api/v1alpha1"
)

// UserReconciler reconciles a User object
type UserReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	resolver system.Resolver
	reporter *statusReporter
}

func NewUserReconciler(k8sClient client.Client, scheme *runtime.Scheme, resolver system.Resolver, recorder record.EventRecorder) *UserReconciler {
	return &UserReconciler{
		Client:   k8sClient,
		Scheme:   scheme,
		resolver: resolver,
		reporter: newStatusReporter(k8sClient, recorder),
	}
}

// +kubebuilder:rbac:groups=nauth.io,resources=users,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=nauth.io,resources=users/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=nauth.io,resources=users/finalizers,verbs=update
// +kubebuilder:rbac:groups=core,resources=events,verbs=create;patch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.20.4/pkg/reconcile
func (r *UserReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	user := &natsv1alpha1.User{}

	err := r.Get(ctx, req.NamespacedName, user)
	if err != nil {
		if apierrors.IsNotFound(err) {
			log.Info("resource not found. Ignoring since object must be deleted")
			return ctrl.Result{}, nil
		}
		// Error reading the object - requeue the request.
		log.Error(err, "Failed to get resource")
		return r.reporter.error(ctx, user, err)
	}

	// USER MARKED FOR DELETION
	if !user.DeletionTimestamp.IsZero() {
		// The user is being deleted
		meta.SetStatusCondition(&user.Status.Conditions, metav1.Condition{
			Type:    controllerTypeReady,
			Status:  metav1.ConditionFalse,
			Reason:  controllerReasonReconciling,
			Message: "Deleting user",
		})

		if err := r.Status().Update(ctx, user); err != nil {
			log.Info("Failed to update the user status", "name", user.Name, "error", err)
			return ctrl.Result{}, err
		}

		// The user is being deleted
		if controllerutil.ContainsFinalizer(user, controllerUserFinalizer) {
			// Get the account for this user
			account := &natsv1alpha1.Account{}
			if err := r.Get(ctx, types.NamespacedName{Name: user.Spec.AccountName, Namespace: user.Namespace}, account); err != nil {
				if !apierrors.IsNotFound(err) {
					return r.reporter.error(ctx, user, fmt.Errorf("failed to get account: %w", err))
				}
				// Account already deleted, just clean up the user secret
				log.Info("Account not found, skipping provider delete", "account", user.Spec.AccountName)
			} else {
				// Resolve provider and delete user
				provider, err := r.resolver.ResolveForAccount(ctx, account)
				if err != nil {
					return r.reporter.error(ctx, user, fmt.Errorf("failed to resolve system provider: %w", err))
				}
				if err := provider.DeleteUser(ctx, user, account); err != nil {
					return r.reporter.error(ctx, user, fmt.Errorf("failed to delete user: %w", err))
				}
			}

			// remove our finalizer from the list and update it.
			controllerutil.RemoveFinalizer(user, controllerUserFinalizer)
			if err := r.Update(ctx, user); err != nil {
				log.Info("failed to remove finalizer", "name", user.Name, "error", err)
				return ctrl.Result{}, err
			}
		}
		// Stop reconciliation as the item is being deleted
		return ctrl.Result{}, nil
	}

	operatorVersion := os.Getenv(EnvOperatorVersion)

	// Nothing has changed
	if user.Status.ObservedGeneration == user.Generation && user.Status.OperatorVersion == operatorVersion {
		return ctrl.Result{}, nil
	}

	// RECONCILE USER - Set status & base properties

	// Add finalizer if not present
	if !controllerutil.ContainsFinalizer(user, controllerUserFinalizer) {
		controllerutil.AddFinalizer(user, controllerUserFinalizer)
		if err := r.Update(ctx, user); err != nil {
			return ctrl.Result{}, err
		}
	}

	meta.SetStatusCondition(&user.Status.Conditions, metav1.Condition{
		Type:    controllerTypeReady,
		Status:  metav1.ConditionFalse,
		Reason:  controllerReasonReconciling,
		Message: "Reconciling user",
	})
	if err := r.Status().Update(ctx, user); err != nil {
		log.Info("Failed to create the user status", "name", user.Name, "error", err)
		return ctrl.Result{}, err
	}

	// Get the account for this user
	account := &natsv1alpha1.Account{}
	if err := r.Get(ctx, types.NamespacedName{Name: user.Spec.AccountName, Namespace: user.Namespace}, account); err != nil {
		return r.reporter.error(ctx, user, fmt.Errorf("failed to get account %s: %w", user.Spec.AccountName, err))
	}

	// Resolve the system provider for this account
	provider, err := r.resolver.ResolveForAccount(ctx, account)
	if err != nil {
		return r.reporter.error(ctx, user, fmt.Errorf("failed to resolve system provider: %w", err))
	}

	// Get account ID from labels
	accountID := account.GetLabels()[k8s.LabelAccountID]

	// Check if this is a new user or an update
	var result *system.UserResult
	userID := ""
	if user.Labels != nil {
		userID = user.Labels[k8s.LabelUserID]
	}

	if userID == "" {
		result, err = provider.CreateUser(ctx, user, account)
	} else {
		result, err = provider.UpdateUser(ctx, user, account)
	}
	if err != nil {
		return r.reporter.error(ctx, user, err)
	}

	// Apply result to user labels and status
	if user.Labels == nil {
		user.Labels = make(map[string]string)
	}
	user.Labels[k8s.LabelUserID] = result.UserID
	user.Labels[k8s.LabelUserAccountID] = accountID
	user.Labels[k8s.LabelUserSignedBy] = result.UserSignedBy

	if result.Claims != nil {
		user.Status.Claims = *result.Claims
	}
	user.Status.ObservedGeneration = user.Generation
	user.Status.ReconcileTimestamp = metav1.Now()

	// UPDATE USER STATUS

	// Need to copy the status - otherwise overwritten by status from kubernetes api response during spec update
	status := user.Status.DeepCopy()
	status.OperatorVersion = operatorVersion

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
		For(&natsv1alpha1.User{}).
		Named("user").
		WithEventFilter(predicate.GenerationChangedPredicate{}).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: 1,
		}).
		Complete(r)
}
