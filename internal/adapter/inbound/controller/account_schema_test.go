package controller

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestAccountTokenPositionSchemaValidation(t *testing.T) {
	ctx := context.Background()
	namespace := "account-token-position-validation"
	require.NoError(t, ensureNamespace(ctx, namespace))
	t.Cleanup(func() {
		require.NoError(t, k8sClient.Delete(context.Background(), &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{Name: namespace},
		}))
	})

	tests := []struct {
		name     string
		kind     string
		value    interface{}
		set      bool
		accepted bool
	}{
		{name: "account_rejects_negative", kind: "Account", value: int64(-1), set: true},
		{name: "account_rejects_boolean", kind: "Account", value: true, set: true},
		{name: "account_accepts_zero", kind: "Account", value: int64(0), set: true, accepted: true},
		{name: "account_accepts_omitted", kind: "Account", accepted: true},
		{name: "account_accepts_one", kind: "Account", value: int64(1), set: true, accepted: true},
		{name: "account_accepts_positive", kind: "Account", value: int64(2), set: true, accepted: true},
		{name: "account_export_rejects_negative", kind: "AccountExport", value: int64(-1), set: true},
		{name: "account_export_rejects_string", kind: "AccountExport", value: "hello", set: true},
		{name: "account_export_accepts_zero", kind: "AccountExport", value: int64(0), set: true, accepted: true},
		{name: "account_export_accepts_omitted", kind: "AccountExport", accepted: true},
		{name: "account_export_accepts_one", kind: "AccountExport", value: int64(1), set: true, accepted: true},
		{name: "account_export_accepts_positive", kind: "AccountExport", value: int64(2), set: true, accepted: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			object := accountTokenPositionObject(tt.kind, namespace, strings.ReplaceAll(tt.name, "_", "-"), tt.value, tt.set)
			err := k8sClient.Create(ctx, object)
			if tt.accepted {
				require.NoError(t, err)
				return
			}

			require.Error(t, err)
			require.True(t, apierrors.IsInvalid(err), "expected API server validation error, got: %v", err)
			require.Contains(t, err.Error(), "accountTokenPosition")
		})
	}
}

func accountTokenPositionObject(kind, namespace, name string, value interface{}, set bool) *unstructured.Unstructured {
	rule := map[string]interface{}{
		"name":    "token-position",
		"subject": "service.*",
		"type":    "service",
	}
	if set {
		rule["accountTokenPosition"] = value
	}

	spec := map[string]interface{}{}
	if kind == "Account" {
		spec["exports"] = []interface{}{rule}
	} else {
		spec["accountName"] = "test-account"
		spec["rules"] = []interface{}{rule}
	}

	return &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "nauth.io/v1alpha1",
		"kind":       kind,
		"metadata": map[string]interface{}{
			"name":      name,
			"namespace": namespace,
		},
		"spec": spec,
	}}
}
