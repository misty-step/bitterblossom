package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"
)

var (
	version = "dev"
	commit  = "unknown"
	date    = "unknown"
)

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
	root := newRootCommand()
	if err := root.Execute(); err != nil {
		var coded *exitError
		if errors.As(err, &coded) {
			if coded.Err != nil {
				_, _ = fmt.Fprintln(os.Stderr, coded.Err)
			}
			os.Exit(coded.Code)
		}
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	cmd := newRootCmd(stdout, stderr)
	cmd.SetArgs(args)
	cmd.SetContext(ctx)
	return cmd.Execute()
}

func newRootCommand() *cobra.Command {
	return newRootCmd(os.Stdout, os.Stderr)
}

func newRootCmd(stdout, stderr io.Writer) *cobra.Command {
	return newRootCmdWithFactories(stdout, stderr, rootCommandFactories{
		composeFactory:   newComposeCmd,
		eventsFactory:    newEventsCmd,
		watchFactory:     newWatchCmd,
		logsFactory:      newLogsCmd,
		agentFactory:     newAgentCommand,
		dispatchFactory:  newDispatchCmd,
		watchdogFactory:  newWatchdogCmd,
		provisionFactory: newProvisionCmd,
		syncFactory:      newSyncCmd,
		statusFactory:    newStatusCmd,
		teardownFactory:  newTeardownCmd,
		fleetFactory:     newFleetCmd,
		addFactory:       newAddCmd,
		removeFactory:    newRemoveCmd,
	})
}

type rootCommandFactories struct {
	composeFactory   func() *cobra.Command
	eventsFactory    func() *cobra.Command
	watchFactory     func(io.Writer, io.Writer) *cobra.Command
	logsFactory      func(io.Writer, io.Writer) *cobra.Command
	agentFactory     func() *cobra.Command
	dispatchFactory  func() *cobra.Command
	watchdogFactory  func() *cobra.Command
	provisionFactory func() *cobra.Command
	syncFactory      func() *cobra.Command
	statusFactory    func() *cobra.Command
	teardownFactory  func() *cobra.Command
	fleetFactory     func() *cobra.Command
	addFactory       func() *cobra.Command
	removeFactory    func() *cobra.Command
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

	root.AddCommand(newVersionCmd())
	if factories.composeFactory != nil {
		root.AddCommand(factories.composeFactory())
	}
	if factories.eventsFactory != nil {
		root.AddCommand(factories.eventsFactory())
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
	if factories.dispatchFactory != nil {
		root.AddCommand(factories.dispatchFactory())
	}
	if factories.watchdogFactory != nil {
		root.AddCommand(factories.watchdogFactory())
	}
	if factories.provisionFactory != nil {
		root.AddCommand(factories.provisionFactory())
	}
	if factories.syncFactory != nil {
		root.AddCommand(factories.syncFactory())
	}
	if factories.statusFactory != nil {
		root.AddCommand(factories.statusFactory())
	}
	if factories.teardownFactory != nil {
		root.AddCommand(factories.teardownFactory())
	}
	if factories.fleetFactory != nil {
		root.AddCommand(factories.fleetFactory())
	}
	if factories.addFactory != nil {
		root.AddCommand(factories.addFactory())
	}
	if factories.removeFactory != nil {
		root.AddCommand(factories.removeFactory())
	}

	return root
}

func newRootCmdWithComposeFactory(composeFactory func() *cobra.Command) *cobra.Command {
	return newRootCmdWithFactories(os.Stdout, os.Stderr, rootCommandFactories{
		composeFactory: composeFactory,
	})
}

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print bb version",
		RunE: func(cmd *cobra.Command, _ []string) error {
			_, err := fmt.Fprintf(cmd.OutOrStdout(), "bb version %s (commit %s, built %s)\n", version, commit, date)
			return err
		},
	}
}
