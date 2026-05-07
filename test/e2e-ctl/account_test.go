package main

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_assertClaimsUpdateResponse_ShouldReturnNil_WhenResponseCodeIsOK(t *testing.T) {
	err := assertClaimsUpdateResponse([]byte(`{"code":200}`))

	require.NoError(t, err)
}

func Test_assertClaimsUpdateResponse_ShouldReturnNil_WhenWrappedResponseCodeIsOK(t *testing.T) {
	err := assertClaimsUpdateResponse([]byte(`{"data":{"code":200,"message":"jwt updated"}}`))

	require.NoError(t, err)
}

func Test_assertClaimsUpdateResponse_ShouldReturnError_WhenResponseHasError(t *testing.T) {
	err := assertClaimsUpdateResponse([]byte(`{"error":{"code":500,"description":"failed"}}`))

	require.EqualError(t, err, `account JWT upload returned error code=500 description="failed"`)
}

func Test_assertClaimsUpdateResponse_ShouldReturnError_WhenWrappedResponseHasError(t *testing.T) {
	err := assertClaimsUpdateResponse([]byte(`{"data":{"error":{"code":500,"description":"failed"}}}`))

	require.EqualError(t, err, `account JWT upload returned error code=500 description="failed"`)
}
