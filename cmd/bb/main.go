package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var version = "dev"

func main() {
	if err := newRootCmd().Execute(); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func newRootCmd() *cobra.Command {
	return newRootCmdWithComposeFactory(newComposeCmd)
}

func newRootCmdWithComposeFactory(composeFactory func() *cobra.Command) *cobra.Command {
	cmd := &cobra.Command{
		Use:          "bb",
		Short:        "Bitterblossom fleet control CLI",
		SilenceUsage: true,
	}
	cmd.Version = version
	cmd.SetVersionTemplate("bb version {{.Version}}\n")

	if composeFactory != nil {
		cmd.AddCommand(composeFactory())
	}
	return cmd
}
