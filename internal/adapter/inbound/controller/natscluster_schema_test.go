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
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func TestNatsClusterTLSSchemaValidation(t *testing.T) {
	ctx := context.Background()
	namespace := "natscluster-tls-validation"
	require.NoError(t, ensureNamespace(ctx, namespace))
	t.Cleanup(func() {
		require.NoError(t, k8sClient.Delete(context.Background(), &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{Name: namespace},
		}))
	})

	tests := []struct {
		name        string
		tls         map[string]interface{}
		accepted    bool
		errContains string
		wantCAPath  []string
		wantCAKey   string
	}{
		{
			name:        "rejects_empty_ca_ref",
			tls:         map[string]interface{}{"caRef": map[string]interface{}{}},
			errContains: "kind",
		},
		{
			name:        "rejects_ca_ref_without_kind",
			tls:         map[string]interface{}{"caRef": map[string]interface{}{"name": "ca"}},
			errContains: "kind",
		},
		{
			name:        "rejects_ca_ref_unsupported_kind",
			tls:         map[string]interface{}{"caRef": map[string]interface{}{"kind": "Pod", "name": "ca"}},
			errContains: "Unsupported value",
		},
		{
			name:        "rejects_ca_ref_without_name",
			tls:         map[string]interface{}{"caRef": map[string]interface{}{"kind": "Secret", "key": "ca.crt"}},
			errContains: "name",
		},
		{
			name:        "rejects_ca_ref_with_empty_name",
			tls:         map[string]interface{}{"caRef": map[string]interface{}{"kind": "ConfigMap", "name": ""}},
			errContains: "name",
		},
		{
			name:       "defaults_ca_ref_config_map_key",
			tls:        map[string]interface{}{"caRef": map[string]interface{}{"kind": "ConfigMap", "name": "ca"}},
			accepted:   true,
			wantCAPath: []string{"key"},
			wantCAKey:  "ca.crt",
		},
		{
			name:       "defaults_ca_ref_secret_key",
			tls:        map[string]interface{}{"caRef": map[string]interface{}{"kind": "Secret", "name": "ca"}},
			accepted:   true,
			wantCAPath: []string{"key"},
			wantCAKey:  "ca.crt",
		},
		{
			name:       "keeps_explicit_ca_ref_key",
			tls:        map[string]interface{}{"caRef": map[string]interface{}{"kind": "ConfigMap", "name": "ca", "key": "bundle.pem"}},
			accepted:   true,
			wantCAPath: []string{"key"},
			wantCAKey:  "bundle.pem",
		},
		{
			name:     "accepts_handshake_first_only",
			tls:      map[string]interface{}{"handshakeFirst": true},
			accepted: true,
		},
		{
			name:     "accepts_client_cert_secret_ref",
			tls:      map[string]interface{}{"clientCertSecretRef": map[string]interface{}{"name": "client-tls"}},
			accepted: true,
		},
		{
			// An empty block requires TLS with system roots, so the API server must keep it.
			name:     "accepts_and_keeps_empty_tls",
			tls:      map[string]interface{}{},
			accepted: true,
		},
		{
			name:     "accepts_omitted_tls",
			accepted: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			name := strings.ReplaceAll(tt.name, "_", "-")
			object := natsClusterObject(namespace, name, tt.tls)
			err := k8sClient.Create(ctx, object)
			if !tt.accepted {
				require.Error(t, err)
				require.True(t, apierrors.IsInvalid(err), "expected API server validation error, got: %v", err)
				require.Contains(t, err.Error(), tt.errContains)
				return
			}
			require.NoError(t, err)

			stored := &unstructured.Unstructured{}
			stored.SetGroupVersionKind(object.GroupVersionKind())
			require.NoError(t, k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: name}, stored))
			if tt.tls != nil {
				_, found, err := unstructured.NestedMap(stored.Object, "spec", "tls")
				require.NoError(t, err)
				require.True(t, found, "spec.tls was pruned by the API server")
			}
			if tt.wantCAKey != "" {
				path := append([]string{"spec", "tls", "caRef"}, tt.wantCAPath...)
				key, _, err := unstructured.NestedString(stored.Object, path...)
				require.NoError(t, err)
				require.Equal(t, tt.wantCAKey, key)
			}
		})
	}
}

// natsClusterObject returns an unstructured NatsCluster, with spec.tls set when tls is non-nil.
func natsClusterObject(namespace, name string, tls map[string]interface{}) *unstructured.Unstructured {
	spec := map[string]interface{}{
		"url":                             "tls://nats.example:4222",
		"operatorSigningKeySecretRef":     map[string]interface{}{"name": "op-sk"},
		"systemAccountUserCredsSecretRef": map[string]interface{}{"name": "sys-creds"},
	}
	if tls != nil {
		spec["tls"] = tls
	}
	return &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "nauth.io/v1alpha1",
		"kind":       "NatsCluster",
		"metadata": map[string]interface{}{
			"name":      name,
			"namespace": namespace,
		},
		"spec": spec,
	}}
}
