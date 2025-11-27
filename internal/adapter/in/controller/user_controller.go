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

	"github.com/WirelessCar-WDP/nauth/internal/core/domain"
	"k8s.io/client-go/tools/record"

	"github.com/WirelessCar-WDP/nauth/internal/core/domain/types"

	"github.com/WirelessCar-WDP/nauth/internal/core/ports"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	natsv1alpha1 "github.com/WirelessCar-WDP/nauth/api/v1alpha1"
)

// UserReconciler reconciles a User object
type UserReconciler struct {
	ports.UserManager
	Scheme *runtime.Scheme
	*StatusReporter
}

func NewUserReconciler(k8sClient client.Client, scheme *runtime.Scheme, userManager ports.UserManager, recorder record.EventRecorder) *UserReconciler {
	return &UserReconciler{
		Scheme:      scheme,
		UserManager: userManager,
		StatusReporter: &StatusReporter{
			Client:   k8sClient,
			Recorder: recorder,
		},
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
		return r.Result(ctx, user, err)
	}

	// USER MARKED FOR DELETION
	if !user.DeletionTimestamp.IsZero() {
		// The user is being deleted
		meta.SetStatusCondition(&user.Status.Conditions, metav1.Condition{
			Type:    types.ControllerTypeReady,
			Status:  metav1.ConditionFalse,
			Reason:  types.ControllerReasonReconciling,
			Message: "Deleting user",
		})

		if err := r.Status().Update(ctx, user); err != nil {
			log.Info("Failed to update the user status", "name", user.Name, "error", err)
			return ctrl.Result{}, err
		}

		// The user is being deleted
		if controllerutil.ContainsFinalizer(user, types.ControllerUserFinalizer) {
			if err := r.DeleteUser(ctx, user); err != nil {
				return r.Result(ctx, user, fmt.Errorf("failed to delete user: %w", err))
			}

			// remove our finalizer from the list and update it.
			controllerutil.RemoveFinalizer(user, types.ControllerUserFinalizer)
			if err := r.Update(ctx, user); err != nil {
				log.Info("failed to remove finalizer", "name", user.Name, "error", err)
				return ctrl.Result{}, err
			}
		}
		// Stop reconciliation as the item is being deleted
		return ctrl.Result{}, nil
	}

	operatorVersion := os.Getenv(domain.OperatorVersion)

	// Nothing has changed
	if user.Status.ObservedGeneration == user.Generation && user.Status.OperatorVersion == operatorVersion {
		return ctrl.Result{}, nil
	}

	// RECONCILE USER - Set status & base properties

	// Add finalizer if not present
	if !controllerutil.ContainsFinalizer(user, types.ControllerUserFinalizer) {
		controllerutil.AddFinalizer(user, types.ControllerUserFinalizer)
		if err := r.Update(ctx, user); err != nil {
			return ctrl.Result{}, err
		}
	}

	meta.SetStatusCondition(&user.Status.Conditions, metav1.Condition{
		Type:    types.ControllerTypeReady,
		Status:  metav1.ConditionFalse,
		Reason:  types.ControllerReasonReconciling,
		Message: "Reconciling user",
	})
	if err := r.Status().Update(ctx, user); err != nil {
		log.Info("Failed to create the user status", "name", user.Name, "error", err)
		return ctrl.Result{}, err
	}

	if err := r.CreateOrUpdateUser(ctx, user); err != nil {
		return r.Result(ctx, user, err)
	}

	// UPDATE USER STATUS

	// Need to copy the status - otherwise overwritten by status from kubernetes api response during spec update
	status := user.Status.DeepCopy()
	status.OperatorVersion = operatorVersion

	user.Status = natsv1alpha1.UserStatus{}
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

	return r.Result(ctx, user, nil)
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
