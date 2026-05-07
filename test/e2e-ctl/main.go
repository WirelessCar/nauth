package main

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"
)

func main() {
	if err := newRootCommand(context.Background(), os.Stderr).Execute(); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "e2e-ctl ERROR: %v\n", err)
		os.Exit(1)
	}
}

func newRootCommand(ctx context.Context, logStream io.Writer) *cobra.Command {
	log := newLogger(logStream)
	cmd := &cobra.Command{
		Use:           "e2e-ctl",
		Short:         "Helper commands for nauth KUTTL e2e tests",
		SilenceErrors: true,
		SilenceUsage:  true,
	}
	// Keep all Cobra output on stderr so command logs are visible in KUTTL and
	// stdout stays unused.
	cmd.SetOut(log.stream)
	cmd.SetErr(log.stream)
	cmd.AddCommand(
		newAccountCommand(ctx, log),
		newAssertCommand(ctx, log),
		newJetStreamCommand(ctx, log),
	)
	return cmd
}

func mustMarkFlagRequired(cmd *cobra.Command, name string) {
	if err := cmd.MarkFlagRequired(name); err != nil {
		panic(err)
	}
}
