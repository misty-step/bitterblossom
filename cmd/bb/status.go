package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/misty-step/bitterblossom/internal/contracts"
	"github.com/misty-step/bitterblossom/internal/lifecycle"
	"github.com/misty-step/bitterblossom/internal/sprite"
	"github.com/spf13/cobra"
)

type statusOptions struct {
	Composition string
	Org         string
	SpriteCLI   string
	Format      string
	Checkpoints bool
	Timeout     time.Duration
}

type statusDeps struct {
	getwd         func() (string, error)
	newCLI        func(binary, org string) sprite.SpriteCLI
	fleetOverview func(ctx context.Context, cli sprite.SpriteCLI, cfg lifecycle.Config, compositionPath string, opts lifecycle.FleetOverviewOpts) (lifecycle.FleetStatus, error)
	spriteDetail  func(ctx context.Context, cli sprite.SpriteCLI, cfg lifecycle.Config, name string) (lifecycle.SpriteDetailResult, error)
}

func defaultStatusDeps() statusDeps {
	return statusDeps{
		getwd: os.Getwd,
		newCLI: func(binary, org string) sprite.SpriteCLI {
			cli := sprite.NewCLIWithOrg(binary, org)
			return cli
		},
		fleetOverview: lifecycle.FleetOverview,
		spriteDetail:  lifecycle.SpriteDetail,
	}
}

func newStatusCmd() *cobra.Command {
	return newStatusCmdWithDeps(defaultStatusDeps())
}

func newStatusCmdWithDeps(deps statusDeps) *cobra.Command {
	opts := statusOptions{
		Composition: defaultLifecycleComposition,
		Org:         defaultOrg(),
		SpriteCLI:   defaultSpriteCLIPath(),
		Format:      "json",
		Checkpoints: false,
		Timeout:     2 * time.Minute,
	}

	command := &cobra.Command{
		Use:   "status [sprite-name]",
		Short: "Show fleet status or detailed status for one sprite",
		RunE: func(cmd *cobra.Command, args []string) error {
			format := strings.ToLower(strings.TrimSpace(opts.Format))
			if format != "json" && format != "text" {
				return errors.New("--format must be json or text")
			}
			if len(args) > 1 {
				return errors.New("only one sprite name can be provided")
			}

			rootDir, err := deps.getwd()
			if err != nil {
				return err
			}
			cfg := defaultLifecycleConfig(rootDir, opts.Org)
			cli := deps.newCLI(opts.SpriteCLI, opts.Org)
			runCtx, cancel := context.WithTimeout(cmd.Context(), opts.Timeout)
			defer cancel()

			if len(args) == 0 {
				_, _ = fmt.Fprintln(cmd.ErrOrStderr(), "status: fetching fleet overview")
				if opts.Checkpoints {
					_, _ = fmt.Fprintln(cmd.ErrOrStderr(), "status: fetching checkpoints (slower)")
				}

				status, err := deps.fleetOverview(runCtx, cli, cfg, opts.Composition, lifecycle.FleetOverviewOpts{
					IncludeCheckpoints: opts.Checkpoints,
				})
				if err != nil {
					return err
				}
				_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "status: loaded %d sprites\n", len(status.Sprites))
				if format == "json" {
					return contracts.WriteJSON(cmd.OutOrStdout(), "status.fleet", status)
				}
				return writeFleetStatusText(cmd.OutOrStdout(), status, opts.Composition)
			}

			_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "status: fetching detail for %s\n", args[0])
			detail, err := deps.spriteDetail(runCtx, cli, cfg, args[0])
			if err != nil {
				return err
			}
			_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "status: detail loaded for %s\n", args[0])
			if format == "json" {
				return contracts.WriteJSON(cmd.OutOrStdout(), "status.sprite", detail)
			}
			return writeSpriteDetailText(cmd.OutOrStdout(), detail)
		},
	}

	command.Flags().StringVar(&opts.Composition, "composition", opts.Composition, "Path to composition YAML")
	command.Flags().StringVar(&opts.Org, "org", opts.Org, "Sprites organization")
	command.Flags().StringVar(&opts.SpriteCLI, "sprite-cli", opts.SpriteCLI, "Path to sprite CLI")
	command.Flags().StringVar(&opts.Format, "format", opts.Format, "Output format: json|text")
	command.Flags().BoolVar(&opts.Checkpoints, "checkpoints", opts.Checkpoints, "Fetch checkpoint listings (slower for large fleets)")
	command.Flags().DurationVar(&opts.Timeout, "timeout", opts.Timeout, "Command timeout")

	return command
}

func writeFleetStatusText(out io.Writer, status lifecycle.FleetStatus, compositionPath string) error {
	if _, err := fmt.Fprintln(out, "=== Bitterblossom Fleet Status ==="); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(out); err != nil {
		return err
	}

	tw := tabwriter.NewWriter(out, 2, 2, 2, ' ', 0)
	if _, err := fmt.Fprintln(tw, "SPRITE\tSTATUS\tURL"); err != nil {
		return err
	}
	for _, item := range status.Sprites {
		spriteName := item.Name
		if item.CurrentTaskDesc != "" {
			taskLabel := item.CurrentTaskDesc
			if item.CurrentTaskID != "" {
				taskLabel = fmt.Sprintf("%s: %s", item.CurrentTaskID, item.CurrentTaskDesc)
			}
			spriteName = fmt.Sprintf("%s (%s)", item.Name, taskLabel)
		}
		if _, err := fmt.Fprintf(tw, "%s\t%s\t%s\n", spriteName, item.Status, item.URL); err != nil {
			return err
		}
	}
	if err := tw.Flush(); err != nil {
		return err
	}

	if _, err := fmt.Fprintln(out); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(out, "Composition sprites (%s):\n", compositionPath); err != nil {
		return err
	}
	for _, item := range status.Composition {
		marker := "○"
		label := "not provisioned"
		if item.Provisioned {
			marker = "✓"
			label = "provisioned"
		}
		if _, err := fmt.Fprintf(out, "  %s %s (%s)\n", marker, item.Name, label); err != nil {
			return err
		}
	}

	if len(status.Orphans) > 0 {
		if _, err := fmt.Fprintln(out); err != nil {
			return err
		}
		if _, err := fmt.Fprintln(out, "Orphan sprites (live but not in composition):"); err != nil {
			return err
		}
		for _, item := range status.Orphans {
			display := item.Name
			if item.CurrentTaskDesc != "" {
				taskLabel := item.CurrentTaskDesc
				if item.CurrentTaskID != "" {
					taskLabel = fmt.Sprintf("%s: %s", item.CurrentTaskID, item.CurrentTaskDesc)
				}
				display = fmt.Sprintf("%s (%s)", item.Name, taskLabel)
			}
			statusLabel := item.Status
			if _, err := fmt.Fprintf(out, "  ? %s (%s)\n", display, statusLabel); err != nil {
				return err
			}
		}
	}

	if _, err := fmt.Fprintln(out); err != nil {
		return err
	}
	if !status.CheckpointsIncluded {
		_, err := fmt.Fprintln(out, "Checkpoints: skipped (use --checkpoints)")
		return err
	}
	if _, err := fmt.Fprintln(out, "Checkpoints:"); err != nil {
		return err
	}
	names := make([]string, 0, len(status.Checkpoints))
	for name := range status.Checkpoints {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if _, err := fmt.Fprintf(out, "  %s: %s\n", name, status.Checkpoints[name]); err != nil {
			return err
		}
	}
	return nil
}

func writeSpriteDetailText(out io.Writer, detail lifecycle.SpriteDetailResult) error {
	if _, err := fmt.Fprintf(out, "=== Sprite: %s ===\n\n", detail.Name); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(out, "API:"); err != nil {
		return err
	}
	if len(detail.API) == 0 {
		if _, err := fmt.Fprintln(out, "(API call failed)"); err != nil {
			return err
		}
	} else {
		keys := make([]string, 0, len(detail.API))
		for key := range detail.API {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			if _, err := fmt.Fprintf(out, "%s: %v\n", key, detail.API[key]); err != nil {
				return err
			}
		}
	}

	if _, err := fmt.Fprintln(out); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(out, "Workspace:"); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(out, detail.Workspace); err != nil {
		return err
	}

	if _, err := fmt.Fprintln(out); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(out, "MEMORY.md (first 20 lines):"); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(out, detail.Memory); err != nil {
		return err
	}

	if _, err := fmt.Fprintln(out); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(out, "Checkpoints:"); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(out, detail.Checkpoints); err != nil {
		return err
	}
	return nil
}
