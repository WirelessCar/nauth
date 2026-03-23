package core_test

import (
	"strings"
	"testing"

	"github.com/WirelessCar/nauth/api/v1alpha1"
	"github.com/WirelessCar/nauth/internal/core"
)

func TestNewOperatorNatsCluster(t *testing.T) {
	t.Run("should_succeed", func(t *testing.T) {
		cluster, err := core.NewOperatorNatsCluster(v1alpha1.NatsClusterRef{
			Namespace: "operator-system",
			Name:      "nats-main",
		}, true)
		if err != nil {
			t.Fatalf("expected success, got error: %v", err)
		}
		if cluster == nil {
			t.Fatalf("expected non-nil cluster")
		}
		if cluster.ClusterRef.Namespace != "operator-system" {
			t.Fatalf("expected namespace, got %q", cluster.ClusterRef.Namespace)
		}
		if cluster.ClusterRef.Name != "nats-main" {
			t.Fatalf("expected name, got %q", cluster.ClusterRef.Name)
		}
		if !cluster.Optional {
			t.Fatalf("expected optional=true")
		}
	})

	testCases := []struct {
		testName      string
		ref           v1alpha1.NatsClusterRef
		expectedError string
	}{
		{
			testName: "should_fail_when_namespace_or_name_is_missing",
			ref: v1alpha1.NatsClusterRef{
				Namespace: "",
				Name:      "nats-main",
			},
			expectedError: "both namespace and name must be provided",
		},
		{
			testName: "should_fail_when_namespace_is_invalid",
			ref: v1alpha1.NatsClusterRef{
				Namespace: "invalid_namespace",
				Name:      "nats-main",
			},
			expectedError: "invalid namespace",
		},
		{
			testName: "should_fail_when_name_is_invalid",
			ref: v1alpha1.NatsClusterRef{
				Namespace: "operator-system",
				Name:      "NATS_MAIN",
			},
			expectedError: "invalid name",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.testName, func(t *testing.T) {
			cluster, err := core.NewOperatorNatsCluster(tc.ref, false)
			if err == nil {
				t.Fatalf("expected error, got success with cluster=%+v", cluster)
			}
			if !strings.Contains(err.Error(), tc.expectedError) {
				t.Fatalf("expected error containing %q, got %q", tc.expectedError, err.Error())
			}
		})
	}
}

func TestNewConfig(t *testing.T) {
	t.Run("should_succeed_when_all_values_are_empty", func(t *testing.T) {
		config, err := core.NewConfig(nil, "")
		if err != nil {
			t.Fatalf("expected success, got error: %v", err)
		}
		if config == nil {
			t.Fatalf("expected non-nil config")
		}
		if config.OperatorNatsCluster != nil {
			t.Fatalf("expected nil operator nats cluster")
		}
		if config.OperatorNamespace != "" {
			t.Fatalf("expected empty namespace, got %q", config.OperatorNamespace)
		}
	})

	t.Run("should_fail_when_operator_cluster_is_invalid_even_if_constructed_directly", func(t *testing.T) {
		config, err := core.NewConfig(&core.OperatorNatsCluster{
			ClusterRef: v1alpha1.NatsClusterRef{
				Namespace: "invalid_namespace",
				Name:      "nats-main",
			},
		}, "")
		if err == nil {
			t.Fatalf("expected error, got success with config=%+v", config)
		}
		if !strings.Contains(err.Error(), "invalid operator NATS cluster") {
			t.Fatalf("expected wrapped operator cluster error, got %q", err.Error())
		}
	})

	t.Run("should_fail_when_operator_namespace_is_invalid", func(t *testing.T) {
		config, err := core.NewConfig(nil, " invalid_namespace ")
		if err == nil {
			t.Fatalf("expected error, got success with config=%+v", config)
		}
		if !strings.Contains(err.Error(), "invalid operator namespace") {
			t.Fatalf("expected operator namespace validation error, got %q", err.Error())
		}
	})
}
