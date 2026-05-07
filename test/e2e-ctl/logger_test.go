package main

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_logger_Infof_ShouldWriteE2EPrefixedLineToLogStream_WhenMessageIsLogged(t *testing.T) {
	var log bytes.Buffer

	newLogger(&log).Infof("message %s", "one")

	require.Equal(t, "e2e-ctl INFO: message one\n", log.String())
}

func Test_logger_Errorf_ShouldWriteCommandPrefixedLineToLogStream_WhenErrorIsLogged(t *testing.T) {
	var log bytes.Buffer

	newLogger(&log).Errorf("message %s", "one")

	require.Equal(t, "e2e-ctl ERROR: message one\n", log.String())
}

func Test_newLogger_ShouldPanic_WhenLogStreamIsNil(t *testing.T) {
	require.PanicsWithValue(t, "log stream is required", func() {
		newLogger(nil)
	})
}
