package main

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_namespaceFromFlagOrEnv_ShouldReturnFlag_WhenNamespaceIsProvided(t *testing.T) {
	t.Setenv(kuttlNamespaceEnv, "from-env")

	namespace, err := namespaceFromFlagOrEnv("from-flag")

	require.NoError(t, err)
	require.Equal(t, "from-flag", namespace)
}

func Test_namespaceFromFlagOrEnv_ShouldReturnEnv_WhenNamespaceFlagIsEmpty(t *testing.T) {
	t.Setenv(kuttlNamespaceEnv, "from-env")

	namespace, err := namespaceFromFlagOrEnv("")

	require.NoError(t, err)
	require.Equal(t, "from-env", namespace)
}

func Test_namespaceFromFlagOrEnv_ShouldReturnError_WhenNamespaceAndEnvAreEmpty(t *testing.T) {
	t.Setenv(kuttlNamespaceEnv, "")

	_, err := namespaceFromFlagOrEnv("")

	require.EqualError(t, err, "--namespace must be set or NAMESPACE must be present")
}
