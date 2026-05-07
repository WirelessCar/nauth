package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/spf13/cobra"
)

const (
	defaultNATSNamespace   = "nats"
	defaultNATSService     = "nats"
	defaultNATSPort        = 4222
	natsRequestTimeout     = time.Second
	jetStreamRetryInterval = time.Second
)

type jetStreamOptions struct {
	namespace      string
	accountName    string
	natsNamespace  string
	natsService    string
	natsURL        string
	stream         string
	subjects       []string
	timeoutSeconds int
	log            logger
}

func newJetStreamCommand(ctx context.Context, log logger) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "jetstream",
		Short: "Manage JetStream resources during e2e tests",
	}
	cmd.AddCommand(
		newJetStreamCreateStreamCommand(ctx, log),
		newJetStreamDeleteStreamCommand(ctx, log),
	)
	return cmd
}

func newJetStreamCreateStreamCommand(ctx context.Context, log logger) *cobra.Command {
	opts := defaultJetStreamOptions(log)

	cmd := &cobra.Command{
		Use:   "create-stream",
		Short: "Create a JetStream stream using Account user creds",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runJetStreamCreateStream(ctx, opts)
		},
	}
	addJetStreamFlags(cmd, opts)
	cmd.Flags().StringArrayVar(&opts.subjects, "subject", nil, "stream subject; can be repeated")
	mustMarkFlagRequired(cmd, "subject")
	return cmd
}

func newJetStreamDeleteStreamCommand(ctx context.Context, log logger) *cobra.Command {
	opts := defaultJetStreamOptions(log)

	cmd := &cobra.Command{
		Use:   "delete-stream",
		Short: "Delete a JetStream stream using Account user creds",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runJetStreamDeleteStream(ctx, opts)
		},
	}
	addJetStreamFlags(cmd, opts)
	return cmd
}

func defaultJetStreamOptions(log logger) *jetStreamOptions {
	return &jetStreamOptions{
		natsNamespace: defaultNATSNamespace,
		natsService:   defaultNATSService,
		log:           log,
	}
}

func addJetStreamFlags(cmd *cobra.Command, opts *jetStreamOptions) {
	cmd.Flags().StringVar(&opts.namespace, "namespace", "", "account namespace; defaults to KUTTL NAMESPACE")
	cmd.Flags().StringVar(&opts.accountName, "account", "", "account name")
	cmd.Flags().StringVar(&opts.natsNamespace, "nats-namespace", opts.natsNamespace, "namespace containing the NATS service")
	cmd.Flags().StringVar(&opts.natsService, "nats-service", opts.natsService, "NATS service name used for port-forward")
	cmd.Flags().StringVar(&opts.natsURL, "nats-url", "", "NATS URL; skips port-forward when set")
	cmd.Flags().StringVar(&opts.stream, "stream", "", "JetStream stream name")
	cmd.Flags().IntVar(&opts.timeoutSeconds, "timeout", 0, "timeout in seconds for the JetStream operation")
	mustMarkFlagRequired(cmd, "account")
	mustMarkFlagRequired(cmd, "stream")
	mustMarkFlagRequired(cmd, "timeout")
}

func runJetStreamCreateStream(ctx context.Context, opts *jetStreamOptions) error {
	return withJetStream(ctx, opts, func(ctx context.Context, js nats.JetStreamContext) error {
		return retryJetStream(ctx, opts, fmt.Sprintf("create JetStream stream %s", opts.stream), func(ctx context.Context) error {
			_, err := js.AddStream(&nats.StreamConfig{
				Name:     opts.stream,
				Subjects: opts.subjects,
			}, nats.Context(ctx))
			return err
		})
	})
}

func runJetStreamDeleteStream(ctx context.Context, opts *jetStreamOptions) error {
	return withJetStream(ctx, opts, func(ctx context.Context, js nats.JetStreamContext) error {
		return retryJetStream(ctx, opts, fmt.Sprintf("delete JetStream stream %s", opts.stream), func(ctx context.Context) error {
			return js.DeleteStream(opts.stream, nats.Context(ctx))
		})
	})
}

func withJetStream(ctx context.Context, opts *jetStreamOptions, run func(context.Context, nats.JetStreamContext) error) error {
	if opts.timeoutSeconds < 1 {
		return fmt.Errorf("--timeout must be at least 1")
	}
	namespace, err := namespaceFromFlagOrEnv(opts.namespace)
	if err != nil {
		return err
	}

	timeout := time.Duration(opts.timeoutSeconds) * time.Second
	timeoutCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	userCreds, err := createUserCredsForAccount(timeoutCtx, opts.log, namespace, opts.accountName)
	if err != nil {
		return err
	}

	natsURL := opts.natsURL
	if natsURL == "" {
		resource := "svc/" + opts.natsService
		opts.log.Infof("open port-forward for %s/%s", opts.natsNamespace, resource)
		forward, err := startKubectlPortForward(timeoutCtx, opts.natsNamespace, resource, defaultNATSPort)
		if err != nil {
			return err
		}
		defer func() {
			if err := forward.Close(); err != nil {
				opts.log.Errorf("failed to close port-forward: %v", err)
			}
		}()
		natsURL = fmt.Sprintf("nats://127.0.0.1:%d", forward.localPort)
	}

	opts.log.Infof("connect to NATS at %s", natsURL)
	conn, err := nats.Connect(
		natsURL,
		nats.Name("e2e-ctl"),
		nats.NoReconnect(),
		nats.Timeout(natsRequestTimeout),
		nats.UserJWTAndSeed(userCreds.jwt, userCreds.seed),
	)
	if err != nil {
		return fmt.Errorf("connect to NATS at %s: %w", natsURL, err)
	}
	defer conn.Close()

	js, err := conn.JetStream(nats.MaxWait(natsRequestTimeout))
	if err != nil {
		return fmt.Errorf("create JetStream context: %w", err)
	}

	return run(timeoutCtx, js)
}

func retryJetStream(ctx context.Context, opts *jetStreamOptions, action string, run func(context.Context) error) error {
	var lastErr error
	for attempt := 1; ; attempt++ {
		opts.log.Infof("%s (attempt %d)", action, attempt)

		attemptCtx, cancel := context.WithTimeout(ctx, natsRequestTimeout)
		err := run(attemptCtx)
		cancel()
		if err == nil {
			opts.log.Infof("%s succeeded", action)
			return nil
		}

		lastErr = err
		opts.log.Errorf("%s failed: %s", action, describeNATSError(err))

		if ctx.Err() != nil {
			return jetStreamTimeoutError(ctx, action, opts, lastErr)
		}
		select {
		case <-ctx.Done():
			return jetStreamTimeoutError(ctx, action, opts, lastErr)
		case <-time.After(jetStreamRetryInterval):
		}
	}
}

func jetStreamTimeoutError(ctx context.Context, action string, opts *jetStreamOptions, lastErr error) error {
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return fmt.Errorf("%s timed out after %d second(s): %w", action, opts.timeoutSeconds, lastErr)
	}
	return ctx.Err()
}

func describeNATSError(err error) string {
	if apiErr, ok := errors.AsType[*nats.APIError](err); ok {
		return fmt.Sprintf("JetStream API error: code=%d err_code=%d description=%q", apiErr.Code, apiErr.ErrorCode, apiErr.Description)
	}

	parts := strings.Split(strings.TrimSpace(err.Error()), "\n")
	return parts[0]
}
