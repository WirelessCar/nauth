package main

import (
	"fmt"
	"io"
)

type logger struct {
	stream io.Writer
}

func newLogger(logStream io.Writer) logger {
	if logStream == nil {
		panic("log stream is required")
	}
	return logger{
		stream: logStream,
	}
}

// Infof writes a human-readable informational log line to the log stream.
func (l logger) Infof(format string, args ...any) {
	l.logf("e2e-ctl INFO: "+format+"\n", args...)
}

// Errorf writes a human-readable error log line to the log stream.
func (l logger) Errorf(format string, args ...any) {
	l.logf("e2e-ctl ERROR: "+format+"\n", args...)
}

func (l logger) logf(format string, args ...any) {
	_, _ = fmt.Fprintf(l.stream, format, args...)
}
