package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/misty-step/bitterblossom/internal/contracts"
	"github.com/misty-step/bitterblossom/internal/lifecycle"
	"github.com/misty-step/bitterblossom/internal/sprite"
	"github.com/spf13/cobra"
)

type teardownOptions struct {
	ArchiveDir string
	Force      bool
	Org        string
	SpriteCLI  string
	Timeout    time.Duration
}

type teardownDeps struct {
	getwd    func() (string, error)
	newCLI   func(binary, org string) sprite.SpriteCLI
	teardown func(ctx context.Context, cli sprite.SpriteCLI, cfg lifecycle.Config, opts lifecycle.TeardownOpts) (lifecycle.TeardownResult, error)
}

func defaultTeardownDeps() teardownDeps {
	return teardownDeps{
		getwd: os.Getwd,
		newCLI: func(binary, org string) sprite.SpriteCLI {
			cli := sprite.NewCLIWithOrg(binary, org)
			return cli
		},
		teardown: lifecycle.Teardown,
	}
}

func newTeardownCmd() *cobra.Command {
	return newTeardownCmdWithDeps(defaultTeardownDeps())
}

func newTeardownCmdWithDeps(deps teardownDeps) *cobra.Command {
	opts := teardownOptions{
		ArchiveDir: "observations/archives",
		Force:      false,
		Org:        defaultOrg(),
		SpriteCLI:  defaultSpriteCLIPath(),
		Timeout:    5 * time.Minute,
	}

	command := &cobra.Command{
		Use:   "teardown <sprite-name>",
		Short: "Export sprite learnings and destroy the sprite",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 1 {
				return errors.New("exactly one sprite name is required")
			}
			name := args[0]

			rootDir, err := deps.getwd()
			if err != nil {
				return err
			}
			cfg := defaultLifecycleConfig(rootDir, opts.Org)

			archiveDir := opts.ArchiveDir
			if !filepath.IsAbs(archiveDir) {
				archiveDir = filepath.Join(rootDir, archiveDir)
			}

			if !opts.Force {
				confirmed, err := confirmTeardown(cmd, name)
				if err != nil {
					return err
				}
				if !confirmed {
					return contracts.WriteJSON(cmd.OutOrStdout(), "teardown", map[string]any{
						"name":    name,
						"aborted": true,
					})
				}
			}

			runCtx, cancel := context.WithTimeout(cmd.Context(), opts.Timeout)
			defer cancel()

			cli := deps.newCLI(opts.SpriteCLI, opts.Org)
			result, err := deps.teardown(runCtx, cli, cfg, lifecycle.TeardownOpts{
				Name:       name,
				ArchiveDir: archiveDir,
				Force:      opts.Force,
			})
			if err != nil {
				return err
			}
			return contracts.WriteJSON(cmd.OutOrStdout(), "teardown", result)
		},
	}

	command.Flags().StringVar(&opts.ArchiveDir, "archive-dir", opts.ArchiveDir, "Archive directory for exported sprite data")
	command.Flags().BoolVar(&opts.Force, "force", opts.Force, "Skip confirmation prompt")
	command.Flags().StringVar(&opts.Org, "org", opts.Org, "Fly.io organization")
	command.Flags().StringVar(&opts.SpriteCLI, "sprite-cli", opts.SpriteCLI, "Path to sprite CLI")
	command.Flags().DurationVar(&opts.Timeout, "timeout", opts.Timeout, "Command timeout")

	return command
}

func confirmTeardown(cmd *cobra.Command, spriteName string) (bool, error) {
	if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Destroy sprite %q and its disk? [y/N] ", spriteName); err != nil {
		return false, err
	}
	reader := bufio.NewReader(cmd.InOrStdin())
	response, err := reader.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return false, err
	}
	normalized := strings.TrimSpace(strings.ToLower(response))
	return normalized == "y" || normalized == "yes", nil
}
