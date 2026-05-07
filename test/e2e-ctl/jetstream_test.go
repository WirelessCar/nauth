package main

import (
	"errors"
	"testing"

	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/require"
)

func Test_describeNATSError_ShouldIncludeNATSAPIFields_WhenErrorIsNATSAPIError(t *testing.T) {
	err := &nats.APIError{
		Code:        500,
		ErrorCode:   10039,
		Description: "stream not found",
	}

	require.Equal(t, `JetStream API error: code=500 err_code=10039 description="stream not found"`, describeNATSError(err))
}

func Test_describeNATSError_ShouldUseFirstLine_WhenErrorHasMultipleLines(t *testing.T) {
	require.Equal(t, "first line", describeNATSError(errors.New("first line\nsecond line")))
}
