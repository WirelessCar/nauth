package main

import (
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

const nauthLogPath = "test/logs/nauth.log"

type nauthLogCollector struct {
	cancel context.CancelFunc
	done   chan struct{}
}

func startNauthLogCollector(path string) (*nauthLogCollector, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}

	logFile, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithCancel(context.Background())
	collector := &nauthLogCollector{cancel: cancel, done: make(chan struct{})}
	go collector.collect(ctx, logFile)
	return collector, nil
}

func (c *nauthLogCollector) collect(ctx context.Context, logFile *os.File) {
	defer close(c.done)
	defer func() { _ = logFile.Close() }()

	for {
		command := exec.CommandContext(ctx, "kubectl", nauthLogCommandArgs()...)
		command.Stdout = logFile
		command.Stderr = io.Discard
		_ = command.Run()

		select {
		case <-ctx.Done():
			return
		case <-time.After(time.Second):
		}
	}
}

func (c *nauthLogCollector) Stop() {
	c.cancel()
	<-c.done
}

func nauthLogCommandArgs() []string {
	args := []string{
		"logs",
		"--namespace", "nats",
		"--selector", "control-plane=controller-manager",
		"--all-containers",
		"--prefix",
		"--timestamps",
		"--follow",
	}

	if kubeconfig := os.Getenv("KUBECONFIG"); kubeconfig != "" {
		return append([]string{"--kubeconfig", kubeconfig}, args...)
	}
	if _, err := os.Stat("kubeconfig"); err == nil {
		return append([]string{"--kubeconfig", "kubeconfig"}, args...)
	}
	return args
}
