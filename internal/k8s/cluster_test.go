package k8s

import (
	"context"
	"testing"

	"github.com/WirelessCar/nauth/api/v1alpha1"
	"github.com/WirelessCar/nauth/internal/ports"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestClusterClient_GetNatsCluster(t *testing.T) {
	t.Run("should_succeed_when_cluster_exists", func(t *testing.T) {
		scheme := runtime.NewScheme()
		require.NoError(t, v1alpha1.AddToScheme(scheme))

		want := &v1alpha1.NatsCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-cluster",
				Namespace: "test-namespace",
			},
			Spec: v1alpha1.NatsClusterSpec{
				URL: "nats://nats:4222",
				OperatorSigningKeySecretRef: v1alpha1.SecretKeyReference{
					Name: "operator-signing-key",
					Key:  "seed",
				},
				SystemAccountUserCredsSecretRef: v1alpha1.SecretKeyReference{
					Name: "system-account-user-creds",
					Key:  "user.creds",
				},
			},
		}

		reader := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(want).
			Build()
		unitUnderTest := NewNatsClusterClient(reader)

		result, err := unitUnderTest.GetNatsCluster(context.Background(), ports.NamespacedName{Namespace: "test-namespace", Name: "test-cluster"})

		require.NoError(t, err)
		require.NotNil(t, result)
		require.Equal(t, want.GetName(), result.GetName())
		require.Equal(t, want.GetNamespace(), result.GetNamespace())
		require.Equal(t, want.Spec, result.Spec)
	})

	t.Run("should_fail_when_cluster_does_not_exist", func(t *testing.T) {
		scheme := runtime.NewScheme()
		require.NoError(t, v1alpha1.AddToScheme(scheme))

		reader := fake.NewClientBuilder().
			WithScheme(scheme).
			Build()
		unitUnderTest := NewNatsClusterClient(reader)

		result, err := unitUnderTest.GetNatsCluster(context.Background(), ports.NamespacedName{Namespace: "missing-namespace", Name: "missing-cluster"})

		require.Error(t, err)
		require.Nil(t, result)
		require.ErrorContains(t, err, "failed getting NatsCluster missing-namespace/missing-cluster")
		require.ErrorContains(t, err, "not found")
	})
}
