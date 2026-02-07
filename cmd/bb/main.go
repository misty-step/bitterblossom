package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"
)

var version = "dev"

type exitError struct {
	Code int
	Err  error
}

func (e *exitError) Error() string {
	if e == nil || e.Err == nil {
		return "command failed"
	}
	return e.Err.Error()
}

func (e *exitError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func main() {
	if err := run(context.Background(), os.Args[1:], os.Stdout, os.Stderr); err != nil {
		var coded *exitError
		if errors.As(err, &coded) {
			if coded.Err != nil {
				_, _ = fmt.Fprintln(os.Stderr, coded.Err)
			}
			os.Exit(coded.Code)
		}
		_, _ = fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	cmd := newRootCmd(stdout, stderr)
	cmd.SetArgs(args)
	cmd.SetContext(ctx)
	return cmd.Execute()
}

func newRootCmd(stdout, stderr io.Writer) *cobra.Command {
	factories := rootCommandFactories{
		composeFactory: newComposeCmd,
		watchFactory:   newWatchCmd,
		logsFactory:    newLogsCmd,
	}
	return newRootCmdWithFactories(stdout, stderr, factories)
}

func newRootCommand() *cobra.Command {
	return newRootCmd(os.Stdout, os.Stderr)
}

type rootCommandFactories struct {
	composeFactory func() *cobra.Command
	watchFactory   func(io.Writer, io.Writer) *cobra.Command
	logsFactory    func(io.Writer, io.Writer) *cobra.Command
	agentFactory   func() *cobra.Command
}

func newRootCmdWithFactories(stdout, stderr io.Writer, factories rootCommandFactories) *cobra.Command {
	root := &cobra.Command{
		Use:           "bb",
		Short:         "Bitterblossom fleet CLI",
		SilenceUsage:  true,
		SilenceErrors: true,
		Version:       version,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}
	root.SetOut(stdout)
	root.SetErr(stderr)

	root.AddCommand(newVersionCmd(stdout))
	if factories.composeFactory != nil {
		root.AddCommand(factories.composeFactory())
	}
	if factories.watchFactory != nil {
		root.AddCommand(factories.watchFactory(stdout, stderr))
	}
	if factories.logsFactory != nil {
		root.AddCommand(factories.logsFactory(stdout, stderr))
	}
	if factories.agentFactory != nil {
		root.AddCommand(factories.agentFactory())
	}

	return root
}

func newRootCmdWithComposeFactory(composeFactory func() *cobra.Command) *cobra.Command {
	return newRootCmdWithFactories(os.Stdout, os.Stderr, rootCommandFactories{
		composeFactory: composeFactory,
	})
}

func newVersionCmd(stdout io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print bb version",
		RunE: func(_ *cobra.Command, _ []string) error {
			_, err := fmt.Fprintf(stdout, "bb version %s\n", version)
			return err
		},
	}
}
