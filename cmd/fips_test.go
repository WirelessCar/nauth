package main

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_checkFIPS_ShouldSucceed_WhenRequiredAndEnabledWithFrozenModule(t *testing.T) {
	err := checkFIPS(fipsStatus{Enabled: true, Version: "v1.0.0"}, true)

	require.NoError(t, err)
}

func Test_checkFIPS_ShouldSucceed_WhenNotRequiredAndDisabled(t *testing.T) {
	err := checkFIPS(fipsStatus{Enabled: false, Version: "latest"}, false)

	require.NoError(t, err)
}

func Test_checkFIPS_ShouldFail_WhenRequiredAndDisabled(t *testing.T) {
	err := checkFIPS(fipsStatus{Enabled: false, Version: "latest"}, true)

	require.ErrorContains(t, err, "FIPS 140-3 mode is required but not enabled")
}

func Test_checkFIPS_ShouldFail_WhenRequiredAndModuleIsNotFrozen(t *testing.T) {
	err := checkFIPS(fipsStatus{Enabled: true, Version: "latest"}, true)

	require.ErrorContains(t, err, "not a frozen, validated module")
}

func Test_currentFIPSStatus_ShouldReportVersion(t *testing.T) {
	status := currentFIPSStatus()

	require.NotEmpty(t, status.Version)
}
