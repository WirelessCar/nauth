package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
)

type synchronizedWriter struct {
	mu     sync.Mutex
	output io.Writer
	buffer bytes.Buffer
}

func (w *synchronizedWriter) Write(data []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if _, err := w.output.Write(data); err != nil {
		return 0, err
	}
	return w.buffer.Write(data)
}

func main() {
	exitCode, err := runKuttl(os.Args[1:])
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "e2e-runner ERROR: %v\n", err)
	}
	if exitCode != 0 {
		os.Exit(exitCode)
	}
}

func runKuttl(args []string) (int, error) {
	commandArgs := append([]string{"kuttl", "test"}, args...)
	command := exec.Command("kubectl", commandArgs...)

	nauthLogs, err := startNauthLogCollector(nauthLogPath)
	if err != nil {
		return 1, fmt.Errorf("start NAuth log collector: %w", err)
	}
	defer nauthLogs.Stop()

	absoluteNauthLogPath, err := filepath.Abs(nauthLogPath)
	if err != nil {
		absoluteNauthLogPath = nauthLogPath
	}
	_, _ = fmt.Fprintf(os.Stdout, "NAuth controller logs: %s\n", absoluteNauthLogPath)

	console := &kuttlConsoleWriter{output: os.Stdout}
	stream := &synchronizedWriter{output: console}
	command.Stdout = stream
	command.Stderr = stream

	runErr := command.Run()
	if err := console.Flush(); err != nil {
		return 1, fmt.Errorf("flush KUTTL console output: %w", err)
	}

	if err := writeRawKuttlLog(stream.buffer.Bytes(), "test/logs/kuttl-run.log"); err != nil {
		return 1, fmt.Errorf("write raw KUTTL log: %w", err)
	}

	failedPaths, err := writeKuttlLogs(stream.buffer.Bytes(), kuttlLogDir)
	if err != nil {
		return 1, fmt.Errorf("write per-test KUTTL logs: %w", err)
	}

	if len(failedPaths) > 0 {
		_, _ = fmt.Fprintln(os.Stderr, logPathMessage(failedPaths))
	}

	if runErr == nil {
		return 0, nil
	}
	if exitError, ok := runErr.(*exec.ExitError); ok {
		return exitError.ExitCode(), nil
	}
	return 1, runErr
}
