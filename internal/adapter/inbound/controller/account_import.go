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

	"github.com/WirelessCar/nauth/api/v1alpha1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
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

	// TODO: [#11] Reconcile AccountImport
	log.Info("TODO: [#11] Reconcile AccountImport", "resource", req.NamespacedName)

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
