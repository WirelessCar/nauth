package main

import (
	"bytes"
	"context"
	"net"
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_waitForLocalPort_ShouldReturnContextError_WhenContextIsDoneBeforePortCheck(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer func(listener net.Listener) {
		_ = listener.Close()
	}(listener)

	address, ok := listener.Addr().(*net.TCPAddr)
	require.True(t, ok)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err = waitForLocalPort(ctx, address.Port, make(chan error), &bytes.Buffer{})
	require.ErrorIs(t, err, context.Canceled)
}

func Test_waitForLocalPort_ShouldIncludeKubectlOutput_WhenContextIsDone(t *testing.T) {
	output := bytes.NewBufferString("kubectl output")

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := waitForLocalPort(ctx, 1, make(chan error), output)

	require.ErrorIs(t, err, context.Canceled)
	require.ErrorContains(t, err, "kubectl output")
}
