package main

import (
	"bytes"
	"io"
	"regexp"
)

var kuttlLoggerLine = regexp.MustCompile(`^\s*logger\.go:\d+:`)
var kuttlLifecycleLine = regexp.MustCompile(`^=== (RUN|PAUSE|CONT) `)
var kuttlTestSuiteLine = regexp.MustCompile(`^=== RUN\s+kuttl/harness$`)
var kuttlRootNameLine = regexp.MustCompile(`^=== NAME\s+kuttl$`)
var kuttlResultLine = regexp.MustCompile(`^--- (PASS|FAIL|SKIP): `)

type kuttlConsoleWriter struct {
	output    io.Writer
	pending   bytes.Buffer
	inTestRun bool
}

func (w *kuttlConsoleWriter) Write(data []byte) (int, error) {
	if _, err := w.pending.Write(data); err != nil {
		return 0, err
	}

	for {
		line := w.pending.Bytes()
		lineEnd := bytes.IndexByte(line, '\n')
		if lineEnd < 0 {
			break
		}

		line = append([]byte(nil), line[:lineEnd+1]...)
		w.pending.Next(lineEnd + 1)
		if !w.showLine(line) {
			continue
		}
		if _, err := w.output.Write(line); err != nil {
			return 0, err
		}
	}

	return len(data), nil
}

func (w *kuttlConsoleWriter) Flush() error {
	line := w.pending.Bytes()
	if len(line) == 0 {
		return nil
	}
	if w.showLine(line) {
		if _, err := w.output.Write(line); err != nil {
			return err
		}
	}
	w.pending.Reset()
	return nil
}

func (w *kuttlConsoleWriter) showLine(line []byte) bool {
	trimmed := bytes.TrimSpace(bytes.TrimSuffix(line, []byte("\n")))
	if kuttlTestSuiteLine.Match(trimmed) {
		w.inTestRun = true
	}
	return showKuttlConsoleLine(line, !w.inTestRun)
}

func showKuttlConsoleLine(line []byte, showSetupLogs bool) bool {
	trimmed := bytes.TrimSpace(bytes.TrimSuffix(line, []byte("\n")))
	if len(trimmed) == 0 {
		return false
	}
	if kuttlLoggerLine.Match(line) {
		return showSetupLogs
	}
	if kuttlLifecycleLine.Match(trimmed) ||
		kuttlRootNameLine.Match(trimmed) ||
		kuttlResultLine.Match(trimmed) {
		return true
	}
	if bytes.Equal(trimmed, []byte("PASS")) ||
		bytes.Equal(trimmed, []byte("FAIL")) ||
		bytes.Equal(trimmed, []byte("SKIP")) {
		return true
	}
	if bytes.HasPrefix(trimmed, []byte("harness.go:")) {
		return true
	}

	if bytes.HasPrefix(trimmed, []byte("===")) || bytes.HasPrefix(trimmed, []byte("---")) {
		return false
	}
	if len(line) > 0 && (line[0] == ' ' || line[0] == '\t') {
		lower := bytes.ToLower(trimmed)
		return bytes.Contains(lower, []byte("error")) || bytes.Contains(lower, []byte("failed"))
	}
	return true
}
