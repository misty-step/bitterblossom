package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/misty-step/bitterblossom/internal/contracts"
	watchdogsvc "github.com/misty-step/bitterblossom/internal/watchdog"
	"github.com/spf13/cobra"
)

type watchdogOptions struct {
	Sprites       []string
	Execute       bool
	DryRun        bool
	JSON          bool
	StaleAfter    time.Duration
	MaxIterations int
	Org           string
	SpriteCLI     string
}

type watchdogDeps struct {
	newRemote  func(binary, org string) *spriteCLIRemote
	newService func(cfg watchdogsvc.Config) (watchdogRunner, error)
}

type watchdogRunner interface {
	Check(ctx context.Context, req watchdogsvc.Request) (watchdogsvc.Report, error)
}

func defaultWatchdogDeps() watchdogDeps {
	return watchdogDeps{
		newRemote: newSpriteCLIRemote,
		newService: func(cfg watchdogsvc.Config) (watchdogRunner, error) {
			return watchdogsvc.NewService(cfg)
		},
	}
}

func newWatchdogCmd() *cobra.Command {
	return newWatchdogCmdWithDeps(defaultWatchdogDeps())
}

func newWatchdogCmdWithDeps(deps watchdogDeps) *cobra.Command {
	opts := watchdogOptions{
		DryRun:        true,
		StaleAfter:    watchdogsvc.DefaultStaleAfter,
		MaxIterations: watchdogsvc.DefaultMaxRalphIterations,
		Org:           strings.TrimSpace(os.Getenv("FLY_ORG")),
		SpriteCLI:     strings.TrimSpace(os.Getenv("SPRITE_CLI")),
	}

	command := &cobra.Command{
		Use:   "watchdog",
		Short: "Run fleet health checks and optionally redispatch dead sprites",
		RunE: func(cmd *cobra.Command, args []string) error {
			if opts.Execute {
				opts.DryRun = false
			}
			if !opts.Execute && cmd.Flags().Changed("dry-run") && !opts.DryRun {
				return errors.New("watchdog: --dry-run=false requires --execute")
			}

			remote := deps.newRemote(opts.SpriteCLI, opts.Org)
			service, err := deps.newService(watchdogsvc.Config{
				Remote:             remote,
				Workspace:          watchdogsvc.DefaultWorkspace,
				StaleAfter:         opts.StaleAfter,
				MaxRalphIterations: opts.MaxIterations,
			})
			if err != nil {
				return err
			}

			report, err := service.Check(contextOrBackground(cmd.Context()), watchdogsvc.Request{
				Sprites: opts.Sprites,
				Execute: opts.Execute,
			})
			if err != nil {
				return err
			}
			if err := renderWatchdogReport(cmd, report, opts.JSON); err != nil {
				return err
			}
			if report.Summary.NeedsAttention > 0 {
				return &exitError{
					Code: 1,
					Err:  fmt.Errorf("watchdog detected %d sprite(s) needing attention", report.Summary.NeedsAttention),
				}
			}
			return nil
		},
	}

	command.Flags().StringSliceVar(&opts.Sprites, "sprite", nil, "Specific sprite(s) to check (default: all)")
	command.Flags().BoolVar(&opts.Execute, "execute", false, "Execute redispatch actions")
	command.Flags().BoolVar(&opts.DryRun, "dry-run", true, "Preview watchdog actions (default)")
	command.Flags().BoolVar(&opts.JSON, "json", false, "Emit JSON output")
	command.Flags().DurationVar(&opts.StaleAfter, "stale-after", opts.StaleAfter, "Stale threshold duration")
	command.Flags().IntVar(&opts.MaxIterations, "max-iterations", opts.MaxIterations, "MAX_ITERATIONS for redispatch recovery")
	command.Flags().StringVar(&opts.Org, "org", opts.Org, "Sprite org passed to sprite CLI")
	command.Flags().StringVar(&opts.SpriteCLI, "sprite-cli", opts.SpriteCLI, "Sprite CLI binary path")

	return command
}

func renderWatchdogReport(cmd *cobra.Command, report watchdogsvc.Report, jsonMode bool) error {
	if jsonMode {
		return contracts.WriteJSON(cmd.OutOrStdout(), "watchdog", report)
	}

	if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Watchdog %s (execute=%t stale_after=%s)\n", report.GeneratedAt.Format(time.RFC3339), report.Execute, report.StaleAfter); err != nil {
		return err
	}

	tw := tabwriter.NewWriter(cmd.OutOrStdout(), 2, 2, 2, ' ', 0)
	if _, err := fmt.Fprintln(tw, "SPRITE\tSTATE\tTASK\tELAPSED_MIN\tBRANCH\tCOMMITS_2H\tACTION"); err != nil {
		return err
	}
	for _, row := range report.Sprites {
		action := ""
		if row.Action.Type != "" {
			action = string(row.Action.Type)
			if row.Action.Executed {
				action += " (executed)"
			}
			if row.Action.Message != "" {
				action += ": " + row.Action.Message
			}
		}
		if row.Error != "" {
			action = row.Error
		}
		if _, err := fmt.Fprintf(
			tw,
			"%s\t%s\t%s\t%d\t%s\t%d\t%s\n",
			row.Sprite,
			row.State,
			row.Task,
			row.ElapsedMinutes,
			row.Branch,
			row.CommitsLast2h,
			action,
		); err != nil {
			return err
		}
	}
	if err := tw.Flush(); err != nil {
		return err
	}

	summary := report.Summary
	_, err := fmt.Fprintf(
		cmd.OutOrStdout(),
		"\nSummary: total=%d active=%d idle=%d complete=%d blocked=%d dead=%d stale=%d error=%d redispatched=%d needs_attention=%d\n",
		summary.Total,
		summary.Active,
		summary.Idle,
		summary.Complete,
		summary.Blocked,
		summary.Dead,
		summary.Stale,
		summary.Error,
		summary.Redispatched,
		summary.NeedsAttention,
	)
	return err
}
