package core_test

import (
	"strings"
	"testing"

	"github.com/WirelessCar/nauth/internal/core"
	"github.com/WirelessCar/nauth/internal/domain/nauth"
)

func TestNewOperatorNatsCluster(t *testing.T) {
	t.Run("should_succeed", func(t *testing.T) {
		cluster, err := core.NewOperatorNatsCluster("operator-system/nats-main", true)
		if err != nil {
			t.Fatalf("expected success, got error: %v", err)
		}
		if cluster == nil {
			t.Fatalf("expected non-nil cluster")
		}
		if !cluster.Optional {
			t.Fatalf("expected optional=true")
		}
	})

	testCases := []struct {
		testName      string
		ref           nauth.ClusterRef
		expectedError string
	}{
		{
			testName:      "should_fail_when_namespace_or_name_is_missing",
			ref:           "nats-main",
			expectedError: "invalid Namespaced Name format \"nats-main\": expected namespace/name",
		},
		{
			testName:      "should_fail_when_namespace_is_invalid",
			ref:           "invalid_namespace/nats-main",
			expectedError: "namespace invalid \"invalid_namespace\"",
		},
		{
			testName:      "should_fail_when_name_is_invalid",
			ref:           "operator-system/NATS_MAIN",
			expectedError: "name invalid \"NATS_MAIN\"",
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
			ClusterRef: "invalid_namespace/nats-main",
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
