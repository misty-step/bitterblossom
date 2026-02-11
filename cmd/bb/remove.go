package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/misty-step/bitterblossom/internal/contracts"
	"github.com/misty-step/bitterblossom/internal/lifecycle"
	"github.com/misty-step/bitterblossom/internal/registry"
	"github.com/misty-step/bitterblossom/internal/sprite"
	"github.com/spf13/cobra"
)

type removeOptions struct {
	Force     bool
	Org       string
	SpriteCLI string
	Timeout   time.Duration
}

type removeDeps struct {
	getwd        func() (string, error)
	newCLI       func(binary, org string) sprite.SpriteCLI
	teardown     func(ctx context.Context, cli sprite.SpriteCLI, cfg lifecycle.Config, opts lifecycle.TeardownOpts) (lifecycle.TeardownResult, error)
	isBusy       func(ctx context.Context, cli sprite.SpriteCLI, name string) (bool, error)
	registryPath func() string
}

func defaultRemoveDeps() removeDeps {
	return removeDeps{
		getwd: os.Getwd,
		newCLI: func(binary, org string) sprite.SpriteCLI {
			return sprite.NewCLIWithOrg(binary, org)
		},
		teardown:     lifecycle.Teardown,
		isBusy:       checkSpriteBusy,
		registryPath: registry.DefaultPath,
	}
}

func newRemoveCmd() *cobra.Command {
	return newRemoveCmdWithDeps(defaultRemoveDeps())
}

func newRemoveCmdWithDeps(deps removeDeps) *cobra.Command {
	opts := removeOptions{
		Org:       defaultOrg(),
		SpriteCLI: defaultSpriteCLIPath(),
		Timeout:   5 * time.Minute,
	}

	cmd := &cobra.Command{
		Use:   "remove <sprite-name>",
		Short: "Remove a sprite from the fleet",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runRemove(cmd, args[0], opts, deps)
		},
	}

	cmd.Flags().BoolVar(&opts.Force, "force", false, "Skip confirmation and allow removing busy sprites")
	cmd.Flags().StringVar(&opts.Org, "org", opts.Org, "Sprites organization")
	cmd.Flags().StringVar(&opts.SpriteCLI, "sprite-cli", opts.SpriteCLI, "Path to sprite CLI")
	cmd.Flags().DurationVar(&opts.Timeout, "timeout", opts.Timeout, "Command timeout")

	return cmd
}

func runRemove(cmd *cobra.Command, name string, opts removeOptions, deps removeDeps) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return errors.New("sprite name is required")
	}

	regPath := deps.registryPath()
	reg, err := registry.Load(regPath)
	if err != nil {
		return fmt.Errorf("load registry: %w", err)
	}

	machineID, exists := reg.LookupMachine(name)
	if !exists {
		return fmt.Errorf("sprite %q not found in registry", name)
	}

	rootDir, err := deps.getwd()
	if err != nil {
		return err
	}
	cfg := defaultLifecycleConfig(rootDir, opts.Org)
	cli := deps.newCLI(opts.SpriteCLI, opts.Org)

	runCtx, cancel := context.WithTimeout(cmd.Context(), opts.Timeout)
	defer cancel()

	// Check if busy (unless --force)
	if !opts.Force {
		busy, err := deps.isBusy(runCtx, cli, name)
		if err != nil {
			// Non-fatal: can't determine state, warn but proceed to confirmation
			_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "warning: cannot determine sprite state: %v\n", err)
		} else if busy {
			return fmt.Errorf("sprite %q is busy — use --force to remove anyway", name)
		}

		confirmed, err := confirmRemove(cmd, name, machineID)
		if err != nil {
			return err
		}
		if !confirmed {
			return contracts.WriteJSON(cmd.OutOrStdout(), "remove", map[string]any{
				"name":    name,
				"aborted": true,
			})
		}
	}

	stderr := cmd.ErrOrStderr()
	_, _ = fmt.Fprintf(stderr, "  Destroying %s... ", name)

	_, err = deps.teardown(runCtx, cli, cfg, lifecycle.TeardownOpts{
		Name:       name,
		ArchiveDir: "observations/archives",
		Force:      opts.Force,
	})
	if err != nil {
		writeStatus(stderr, false)
		return fmt.Errorf("teardown %q: %w", name, err)
	}
	writeStatus(stderr, true)

	// Remove from registry atomically
	if err := registry.WithLockedRegistry(regPath, func(reg *registry.Registry) error {
		reg.Unregister(name)
		return nil
	}); err != nil {
		return fmt.Errorf("update registry: %w", err)
	}

	// Reload for accurate count
	reg, _ = registry.Load(regPath)
	total := 0
	if reg != nil {
		total = reg.Count()
	}

	_, _ = fmt.Fprintf(stderr, "Registry updated (%d sprites total).\n", total)

	return contracts.WriteJSON(cmd.OutOrStdout(), "remove", map[string]any{
		"name":    name,
		"removed": true,
		"total":   total,
	})
}

func confirmRemove(cmd *cobra.Command, name, machineID string) (bool, error) {
	label := name
	if machineID != "" {
		label = fmt.Sprintf("%s (machine %s)", name, machineID)
	}
	if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Destroy sprite %q? [y/N] ", label); err != nil {
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

func checkSpriteBusy(ctx context.Context, cli sprite.SpriteCLI, name string) (bool, error) {
	output, err := cli.Exec(ctx, name, "cat /home/sprite/workspace/STATUS.json 2>/dev/null || echo '{}'", nil)
	if err != nil {
		return false, err
	}
	// Simple heuristic: if there's a STATUS.json with content beyond {}, sprite is likely busy
	trimmed := strings.TrimSpace(output)
	return trimmed != "{}" && trimmed != "", nil
}
