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
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/events"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

// AccountExportReconciler reconciles an AccountExport object.
type AccountExportReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	reporter *statusReporter
}

func NewAccountExportReconciler(k8sClient client.Client, scheme *runtime.Scheme, recorder events.EventRecorder) *AccountExportReconciler {
	return &AccountExportReconciler{
		Client:   k8sClient,
		Scheme:   scheme,
		reporter: newStatusReporter(k8sClient, recorder),
	}
}

// +kubebuilder:rbac:groups=nauth.io,resources=accountexports,verbs=get;list;watch
// +kubebuilder:rbac:groups=nauth.io,resources=accountexports/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=core,resources=events,verbs=create;patch

func (r *AccountExportReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	accountExport := &v1alpha1.AccountExport{}
	if err := r.Get(ctx, req.NamespacedName, accountExport); err != nil {
		if apierrors.IsNotFound(err) {
			log.Info("resource not found. Ignoring since object must be deleted")
			return ctrl.Result{}, nil
		}

		log.Error(err, "Failed to get resource")
		return r.reporter.error(ctx, accountExport, err)
	}

	return r.reporter.error(ctx, accountExport, fmt.Errorf("AccountExport CRD reconciliation not implemented yet"))
}

func (r *AccountExportReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.AccountExport{}).
		Named("accountexport").
		WithEventFilter(predicate.GenerationChangedPredicate{}).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: 1,
		}).
		Complete(r)
}
