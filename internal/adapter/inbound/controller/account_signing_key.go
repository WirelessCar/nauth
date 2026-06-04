/*
Copyright 2026.

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
	"errors"
	"fmt"
	"os"

	"github.com/WirelessCar/nauth/api/v1alpha1"
	"github.com/WirelessCar/nauth/internal/adapter/outbound/k8s"
	"github.com/WirelessCar/nauth/internal/core"
	"github.com/WirelessCar/nauth/internal/domain"
	"github.com/WirelessCar/nauth/internal/domain/nauth"
	"github.com/WirelessCar/nauth/internal/ports/inbound"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

const (
	// defaultAccountSigningKeySecretNameSuffix is appended to an AccountSigningKey
	// resource name to derive the default name of the Secret that holds its seed,
	// used only when spec.secretName is empty in managed mode.
	defaultAccountSigningKeySecretNameSuffix = "-ac-sign"
)

// errObserveSecretNameRequired is reported when an AccountSigningKey is labelled
// for observe mode but does not name the Secret to read. Observe mode never falls
// back to the managed default name — the user must point at an existing Secret.
var errObserveSecretNameRequired = errors.New("observe mode requires spec.secretName to be set")

// AccountSigningKeyReconciler reconciles AccountSigningKey resources.
type AccountSigningKeyReconciler struct {
	client.Client
	Scheme  *runtime.Scheme
	manager inbound.AccountSigningKeyManager
}

func NewAccountSigningKeyReconciler(k8sClient client.Client, scheme *runtime.Scheme, manager inbound.AccountSigningKeyManager) *AccountSigningKeyReconciler {
	return &AccountSigningKeyReconciler{
		Client:  k8sClient,
		Scheme:  scheme,
		manager: manager,
	}
}

// +kubebuilder:rbac:groups=nauth.io,resources=accountsigningkeys,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=nauth.io,resources=accountsigningkeys/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch;create;update;patch;delete

func (r *AccountSigningKeyReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	state := &v1alpha1.AccountSigningKey{}
	if err := r.Get(ctx, req.NamespacedName, state); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, fmt.Errorf("failed to get AccountSigningKey: %w", err)
	}

	if !state.DeletionTimestamp.IsZero() {
		return ctrl.Result{}, nil
	}

	var resolveResult *nauth.AccountSigningKeyResult
	var resolveErr error

	switch {
	case isObserveMode(state):
		if state.Spec.SecretName == "" {
			resolveErr = errObserveSecretNameRequired
			break
		}
		secretRef := domain.NewNamespacedName(state.Namespace, state.Spec.SecretName)
		resolveResult, resolveErr = r.manager.Import(ctx, secretRef)
	default:
		secretRef := domain.NewNamespacedName(state.Namespace, secretNameFor(state))
		resolveResult, resolveErr = r.manager.CreateOrUpdate(ctx, nauth.AccountSigningKeyRequest{
			SecretRef: secretRef,
			Owner:     state,
		})
	}

	if resolveErr != nil {
		reason := conditionReasonFailed
		switch {
		case errors.Is(resolveErr, core.ErrSigningKeyConflict):
			reason = conditionReasonConflict
		case errors.Is(resolveErr, core.ErrSecretNotFound):
			reason = conditionReasonNotFound
		case errors.Is(resolveErr, core.ErrInvalidSeed),
			errors.Is(resolveErr, errObserveSecretNameRequired):
			reason = conditionReasonInvalid
		}
		meta.SetStatusCondition(state.GetConditions(), newCondition(conditionTypeReady, metav1.ConditionFalse, reason, resolveErr.Error()))
	} else {
		state.Status.PublicKey = resolveResult.PublicKey
		state.Status.SecretName = resolveResult.SecretName
		meta.SetStatusCondition(state.GetConditions(), newCondition(conditionTypeReady, metav1.ConditionTrue, conditionReasonReconciled, ""))
	}

	state.Status.ManagementPolicy = state.GetLabels()[k8s.LabelManagementPolicy]
	state.Status.ObservedGeneration = state.Generation
	state.Status.OperatorVersion = os.Getenv(envOperatorVersion)
	state.Status.ReconcileTimestamp = metav1.Now()

	sortConditions(state.Status.Conditions)

	if updateErr := r.Status().Update(ctx, state); updateErr != nil {
		return ctrl.Result{}, fmt.Errorf("update AccountSigningKey status: %w", updateErr)
	}

	return ctrl.Result{}, nil
}

// SetupWithManager registers the controller and its predicates with the manager.
func (r *AccountSigningKeyReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.AccountSigningKey{}, builder.WithPredicates(predicate.GenerationChangedPredicate{})).
		Owns(&corev1.Secret{}).
		Named("accountsigningkey").
		WithOptions(controller.Options{
			MaxConcurrentReconciles: 1,
		}).
		Complete(r)
}

// generatedSecretNameOf returns the default Secret name for a generated account signing key.
func generatedSecretNameOf(resourceName string) string {
	return resourceName + defaultAccountSigningKeySecretNameSuffix
}

// secretNameFor returns the resolved Secret name for state.
func secretNameFor(state *v1alpha1.AccountSigningKey) string {
	if state.Spec.SecretName != "" {
		return state.Spec.SecretName
	}
	return generatedSecretNameOf(state.Name)
}

// isObserveMode reports whether state is in observe mode.
func isObserveMode(state *v1alpha1.AccountSigningKey) bool {
	return state.GetLabels()[k8s.LabelManagementPolicy] == k8s.ManagementPolicyObserve
}
