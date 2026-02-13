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
	"strings"

	synadiav1alpha1 "github.com/WirelessCar/nauth/api/synadia/v1alpha1"
	"github.com/WirelessCar/nauth/internal/cluster/synadia"
	"github.com/WirelessCar/nauth/internal/k8s/configmap"
	"github.com/WirelessCar/nauth/internal/k8s/secret"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/events"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

const defaultAPITokenKey = "token"

// SystemReconciler reconciles a System (Synadia Cloud connection) by resolving
// teamId, calling the Synadia API to list systems, and writing systemId to status.
type SystemReconciler struct {
	client.Client
	Scheme          *runtime.Scheme
	secretClient    *secret.Client
	configmapClient *configmap.Client
	reporter        *statusReporter
}

// NewSystemReconciler creates a SystemReconciler.
func NewSystemReconciler(
	k8sClient client.Client,
	scheme *runtime.Scheme,
	secretClient *secret.Client,
	configmapClient *configmap.Client,
	recorder events.EventRecorder,
) *SystemReconciler {
	return &SystemReconciler{
		Client:          k8sClient,
		Scheme:          scheme,
		secretClient:    secretClient,
		configmapClient: configmapClient,
		reporter:        newStatusReporter(k8sClient, recorder),
	}
}

// +kubebuilder:rbac:groups=synadia.nauth.io,resources=systems,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=synadia.nauth.io,resources=systems/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=core,resources=secrets,verbs=get
// +kubebuilder:rbac:groups=core,resources=configmaps,verbs=get
// +kubebuilder:rbac:groups=core,resources=events,verbs=create;patch

// Reconcile resolves the System's teamId, lists systems from the Synadia API,
// selects by systemSelector.name, and writes systemId (and conditions) to status.
func (r *SystemReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	sys := &synadiav1alpha1.System{}
	if err := r.Get(ctx, req.NamespacedName, sys); err != nil {
		if apierrors.IsNotFound(err) {
			log.Info("resource not found, ignoring")
			return ctrl.Result{}, nil
		}
		log.Error(err, "failed to get System")
		return r.reporter.error(ctx, sys, err)
	}

	// Resolve teamId from spec or teamIdFrom
	teamID, err := r.resolveTeamID(ctx, sys)
	if err != nil {
		return r.reporter.error(ctx, sys, fmt.Errorf("resolve teamId: %w", err))
	}

	// Get API token from secret
	token, err := r.getAPIToken(ctx, sys)
	if err != nil {
		return r.reporter.error(ctx, sys, fmt.Errorf("api credentials: %w", err))
	}

	// Set Reconciling condition
	meta.SetStatusCondition(&sys.Status.Conditions, metav1.Condition{
		Type:    controllerTypeReady,
		Status:  metav1.ConditionFalse,
		Reason:  controllerReasonReconciling,
		Message: "Resolving system from Synadia API",
	})
	if err := r.Status().Update(ctx, sys); err != nil {
		log.Info("failed to update System status", "name", sys.Name, "error", err)
		return ctrl.Result{}, err
	}

	// Call Synadia API: list systems and select by name
	baseURL := strings.TrimSuffix(sys.Spec.APIEndpoint, "/")
	apiClient := synadia.NewClient(baseURL, func(context.Context) (string, error) { return token, nil })
	resp, err := apiClient.ListSystems(ctx, teamID)
	if err != nil {
		return r.reporter.error(ctx, sys, fmt.Errorf("list systems: %w", err))
	}

	wantName := sys.Spec.SystemSelector.Name
	var systemID string
	for _, s := range resp.Systems {
		if s.Name == wantName {
			systemID = s.ID
			break
		}
	}
	if systemID == "" {
		return r.reporter.error(ctx, sys, fmt.Errorf("system with name %q not found in team (found %d system(s))", wantName, len(resp.Systems)))
	}

	// Write systemId to status
	sys.Status.SystemID = systemID
	return r.reporter.status(ctx, sys)
}

// resolveTeamID returns teamId from spec.TeamID or from spec.TeamIDFrom (ConfigMap/Secret).
// Exactly one of TeamID or TeamIDFrom must be set.
func (r *SystemReconciler) resolveTeamID(ctx context.Context, sys *synadiav1alpha1.System) (string, error) {
	if sys.Spec.TeamID != "" && sys.Spec.TeamIDFrom != nil {
		return "", fmt.Errorf("spec.teamId and spec.teamIdFrom are mutually exclusive")
	}
	if sys.Spec.TeamID != "" {
		return sys.Spec.TeamID, nil
	}
	if sys.Spec.TeamIDFrom == nil {
		return "", fmt.Errorf("either spec.teamId or spec.teamIdFrom must be set")
	}
	ref := sys.Spec.TeamIDFrom
	ns := ref.Namespace
	if ns == "" {
		ns = sys.Namespace
	}
	switch ref.Kind {
	case "ConfigMap":
		data, err := r.configmapClient.Get(ctx, ns, ref.Name)
		if err != nil {
			return "", err
		}
		val, ok := data[ref.Key]
		if !ok {
			return "", fmt.Errorf("configmap %s/%s has no key %q", ns, ref.Name, ref.Key)
		}
		return strings.TrimSpace(val), nil
	case "Secret":
		data, err := r.secretClient.Get(ctx, ns, ref.Name)
		if err != nil {
			return "", err
		}
		val, ok := data[ref.Key]
		if !ok {
			return "", fmt.Errorf("secret %s/%s has no key %q", ns, ref.Name, ref.Key)
		}
		return strings.TrimSpace(val), nil
	default:
		return "", fmt.Errorf("spec.teamIdFrom.kind must be ConfigMap or Secret, got %q", ref.Kind)
	}
}

// getAPIToken returns the Bearer token from the secret referenced by spec.apiCredentialsSecretRef.
func (r *SystemReconciler) getAPIToken(ctx context.Context, sys *synadiav1alpha1.System) (string, error) {
	ref := sys.Spec.APICredentialsSecretRef
	ns := ref.Namespace
	if ns == "" {
		ns = sys.Namespace
	}
	key := ref.Key
	if key == "" {
		key = defaultAPITokenKey
	}
	data, err := r.secretClient.Get(ctx, ns, ref.Name)
	if err != nil {
		return "", err
	}
	val, ok := data[key]
	if !ok {
		return "", fmt.Errorf("secret %s/%s has no key %q", ns, ref.Name, key)
	}
	return strings.TrimSpace(val), nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *SystemReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&synadiav1alpha1.System{}).
		Named("system").
		WithEventFilter(predicate.GenerationChangedPredicate{}).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: 1,
		}).
		Complete(r)
}
