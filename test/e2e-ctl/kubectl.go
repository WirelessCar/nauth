package main

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"os/exec"
	"strings"
	"time"
)

func kubectl(ctx context.Context, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "kubectl", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("kubectl %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(output)))
	}
	return strings.TrimSpace(string(output)), nil
}

type kubectlPortForward struct {
	localPort int
	cancel    context.CancelFunc
	done      <-chan error
	output    *bytes.Buffer
}

func startKubectlPortForward(ctx context.Context, namespace, resource string, remotePort int) (*kubectlPortForward, error) {
	localPort, err := reserveLocalPort()
	if err != nil {
		return nil, err
	}

	forwardCtx, cancel := context.WithCancel(ctx)
	output := bytes.Buffer{}
	cmd := exec.CommandContext(
		forwardCtx,
		"kubectl",
		"-n", namespace,
		"port-forward",
		"--address", "127.0.0.1",
		resource,
		fmt.Sprintf("%d:%d", localPort, remotePort),
	)
	cmd.Stdout = &output
	cmd.Stderr = &output

	if err := cmd.Start(); err != nil {
		cancel()
		return nil, fmt.Errorf("start kubectl port-forward for %s/%s: %w", namespace, resource, err)
	}

	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()

	if err := waitForLocalPort(ctx, localPort, done, &output); err != nil {
		cancel()
		return nil, fmt.Errorf("wait for kubectl port-forward for %s/%s: %w", namespace, resource, err)
	}

	return &kubectlPortForward{
		localPort: localPort,
		cancel:    cancel,
		done:      done,
		output:    &output,
	}, nil
}

func reserveLocalPort() (int, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, fmt.Errorf("reserve local port: %w", err)
	}
	defer func() {
		_ = listener.Close()
	}()

	address, ok := listener.Addr().(*net.TCPAddr)
	if !ok {
		return 0, fmt.Errorf("reserved local port has unexpected address type %T", listener.Addr())
	}
	return address.Port, nil
}

func waitForLocalPort(ctx context.Context, port int, done <-chan error, output *bytes.Buffer) error {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	address := fmt.Sprintf("127.0.0.1:%d", port)
	for {
		if ctx.Err() != nil {
			return portForwardContextError(ctx, address, output)
		}

		conn, err := net.DialTimeout("tcp", address, 100*time.Millisecond)
		if err == nil {
			_ = conn.Close()
			return nil
		}

		select {
		case <-ctx.Done():
			return portForwardContextError(ctx, address, output)
		case err := <-done:
			if err == nil {
				return fmt.Errorf("kubectl port-forward exited before %s was ready: %s", address, output.String())
			}
			return fmt.Errorf("kubectl port-forward exited early: %w: %s", err, output.String())
		case <-ticker.C:
		}
	}
}

func portForwardContextError(ctx context.Context, address string, output *bytes.Buffer) error {
	return fmt.Errorf("context done before %s was ready: %w: %s", address, ctx.Err(), output.String())
}

func (p *kubectlPortForward) Close() error {
	p.cancel()

	select {
	case <-p.done:
		return nil
	case <-time.After(2 * time.Second):
		return fmt.Errorf("timed out stopping kubectl port-forward: %s", p.output.String())
	}
}
