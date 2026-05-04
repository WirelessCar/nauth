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

	"github.com/WirelessCar/nauth/api/v1alpha1"
	"github.com/WirelessCar/nauth/internal/domain/nauth"
	"github.com/WirelessCar/nauth/internal/ports/inbound"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/events"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type ClusterResolver interface {
	ResolveClusterTarget(ctx context.Context, cluster *v1alpha1.NatsCluster) (*nauth.ClusterTarget, error)
}

type NatsClusterReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	manager  inbound.ClusterManager
	resolver ClusterResolver
	reporter *statusReporter
}

func NewNatsClusterReconciler(
	k8sClient client.Client,
	scheme *runtime.Scheme,
	manager inbound.ClusterManager,
	resolver ClusterResolver,
	recorder events.EventRecorder,
) *NatsClusterReconciler {
	return &NatsClusterReconciler{
		Client:   k8sClient,
		Scheme:   scheme,
		manager:  manager,
		resolver: resolver,
		reporter: newStatusReporter(k8sClient, recorder),
	}
}

// +kubebuilder:rbac:groups=nauth.io,resources=natsclusters,verbs=get;list;watch
// +kubebuilder:rbac:groups=nauth.io,resources=natsclusters/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=core,resources=events,verbs=create;patch

func (r *NatsClusterReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	natsCluster := &v1alpha1.NatsCluster{}
	if err := r.Get(ctx, req.NamespacedName, natsCluster); err != nil {
		if apierrors.IsNotFound(err) {
			log.Info("resource not found. Ignoring since object must be deleted")
			return ctrl.Result{}, nil
		}

		log.Error(err, "Failed to get resource")
		return ctrl.Result{}, err
	}

	operatorVersion := os.Getenv(envOperatorVersion)
	if natsCluster.Status.ObservedGeneration == natsCluster.Generation && natsCluster.Status.OperatorVersion == operatorVersion {
		return ctrl.Result{}, nil
	}

	meta.SetStatusCondition(&natsCluster.Status.Conditions, metav1.Condition{
		Type:    conditionTypeReady,
		Status:  metav1.ConditionFalse,
		Reason:  conditionReasonReconciling,
		Message: "Reconciling NatsCluster",
	})
	if err := r.Status().Update(ctx, natsCluster); err != nil {
		log.Info("Failed to update the NatsCluster status", "name", natsCluster.Name, "error", err)
		return ctrl.Result{}, err
	}

	clusterTarget, err := r.resolver.ResolveClusterTarget(ctx, natsCluster)
	if err != nil {
		return r.reporter.error(ctx, natsCluster, fmt.Errorf("failed to resolve NatsCluster target: %w", err))
	}

	if err = r.manager.Validate(ctx, *clusterTarget); err != nil {
		return r.reporter.error(ctx, natsCluster, fmt.Errorf("failed to validate NatsCluster: %w", err))
	}

	natsCluster.Status.ObservedGeneration = natsCluster.Generation
	natsCluster.Status.ReconcileTimestamp = metav1.Now()
	natsCluster.Status.OperatorVersion = operatorVersion

	return r.reporter.status(ctx, natsCluster)
}

func (r *NatsClusterReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.NatsCluster{}).
		Named("natscluster").
		WithEventFilter(predicate.GenerationChangedPredicate{}).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: 1,
		}).
		Complete(r)
}
