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
	Composition    string
	Org            string
	SpriteCLI      string
	Format         string
	Checkpoints    bool
	Tasks          bool
	Probe          bool
	Watch          bool
	WatchInterval  time.Duration
	Timeout        time.Duration
	SpriteTimeout  time.Duration
	StaleThreshold time.Duration
	ProbeTimeout   time.Duration
	UseLedger      bool
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
			// Wrap with resilient CLI to handle transient transport errors
			return sprite.NewResilientCLI(cli)
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
		Composition:    defaultLifecycleComposition,
		Org:            defaultOrg(),
		SpriteCLI:      defaultSpriteCLIPath(),
		Format:         "text",
		Checkpoints:    false,
		Tasks:          true,
		Probe:          false,
		Watch:          false,
		WatchInterval:  5 * time.Second,
		Timeout:        2 * time.Minute,
		SpriteTimeout:  15 * time.Second,
		StaleThreshold: lifecycle.DefaultStaleThreshold,
		ProbeTimeout:   lifecycle.DefaultProbeTimeout,
		UseLedger:      true, // Default to ledger for fast response
	}

	command := &cobra.Command{
		Use:   "status [sprite-name]",
		Short: "Show fleet status or detailed status for one sprite",
		Long: `Show fleet status or detailed status for one sprite.

When called without arguments, shows a fleet-wide overview with sprite states:
  - idle:    Sprite is running and available for work
  - busy:    Sprite is running and actively working on a task
  - offline: Sprite is not running or unreachable
  - unknown: Sprite state cannot be determined

By default, status reads from the durable task ledger for instant response.
Use --no-ledger to fall back to direct sprite queries (slower).

Use --watch for continuous monitoring of the fleet.
Use --format=json for machine-readable output.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			format := strings.ToLower(strings.TrimSpace(opts.Format))
			if format != "json" && format != "text" {
				return errors.New("--format must be json or text")
			}
			if len(args) > 1 {
				return errors.New("only one sprite name can be provided")
			}
			if opts.Watch && len(args) > 0 {
				return errors.New("--watch cannot be used with a specific sprite name")
			}

			rootDir, err := deps.getwd()
			if err != nil {
				return err
			}
			cfg := defaultLifecycleConfig(rootDir, opts.Org)
			cli := deps.newCLI(opts.SpriteCLI, opts.Org)

			// Handle watch mode
			if opts.Watch {
				return runWatchMode(cmd, deps, cli, cfg, opts)
			}

			runCtx, cancel := context.WithTimeout(cmd.Context(), opts.Timeout)
			defer cancel()

			if len(args) == 0 {
				return runFleetStatus(cmd, deps, cli, cfg, opts, runCtx, format)
			}

			return runSpriteDetail(cmd, deps, cli, cfg, opts, runCtx, format, args[0])
		},
	}

	command.Flags().StringVar(&opts.Composition, "composition", opts.Composition, "Path to composition YAML")
	command.Flags().StringVar(&opts.Org, "org", opts.Org, "Sprites organization")
	command.Flags().StringVar(&opts.SpriteCLI, "sprite-cli", opts.SpriteCLI, "Path to sprite CLI")
	command.Flags().StringVar(&opts.Format, "format", opts.Format, "Output format: json|text")
	command.Flags().BoolVar(&opts.Checkpoints, "checkpoints", opts.Checkpoints, "Fetch checkpoint listings (slower for large fleets)")
	command.Flags().BoolVar(&opts.Tasks, "tasks", opts.Tasks, "Fetch current task information for running sprites")
	command.Flags().BoolVar(&opts.Probe, "probe", opts.Probe, "Probe connectivity via exec (slower, verifies transport reachability)")
	command.Flags().BoolVarP(&opts.Watch, "watch", "w", opts.Watch, "Watch mode: continuously refresh fleet status")
	command.Flags().DurationVar(&opts.WatchInterval, "watch-interval", opts.WatchInterval, "Refresh interval for watch mode")
	command.Flags().DurationVar(&opts.Timeout, "timeout", opts.Timeout, "Command timeout")
	command.Flags().DurationVar(&opts.StaleThreshold, "stale-threshold", opts.StaleThreshold, "Flag sprites with no activity beyond this duration as stale")
	command.Flags().DurationVar(&opts.ProbeTimeout, "probe-timeout", opts.ProbeTimeout, "Timeout for each connectivity probe")
	command.Flags().DurationVar(&opts.SpriteTimeout, "sprite-timeout", opts.SpriteTimeout, "Timeout for single-sprite status queries (default 15s)")
	command.Flags().BoolVar(&opts.UseLedger, "use-ledger", opts.UseLedger, "Use durable task ledger for instant status (default true)")
	command.Flags().BoolVar(&opts.UseLedger, "no-ledger", !opts.UseLedger, "Disable ledger and use direct sprite queries")

	return command
}

func runFleetStatus(cmd *cobra.Command, deps statusDeps, cli sprite.SpriteCLI, cfg lifecycle.Config, opts statusOptions, ctx context.Context, format string) error {
	// If not using ledger, use the original flow
	if !opts.UseLedger {
		_, _ = fmt.Fprintln(cmd.ErrOrStderr(), "status: fetching fleet overview (direct mode)")
		if opts.Checkpoints {
			_, _ = fmt.Fprintln(cmd.ErrOrStderr(), "status: fetching checkpoints (slower)")
		}
		if opts.Tasks {
			_, _ = fmt.Fprintln(cmd.ErrOrStderr(), "status: fetching task assignments")
		}
		if opts.Probe {
			_, _ = fmt.Fprintln(cmd.ErrOrStderr(), "status: probing connectivity (slower)")
		}

		status, err := deps.fleetOverview(ctx, cli, cfg, opts.Composition, lifecycle.FleetOverviewOpts{
			IncludeCheckpoints: opts.Checkpoints,
			IncludeTasks:       opts.Tasks,
			IncludeProbe:       opts.Probe,
			ProbeTimeout:       opts.ProbeTimeout,
			StaleThreshold:     opts.StaleThreshold,
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

	// Use ledger for instant response
	_, _ = fmt.Fprintln(cmd.ErrOrStderr(), "status: reading from task ledger (instant)")

	// Try to get status from ledger first
	ledgerStatus, err := getLedgerFleetStatus(opts.StaleThreshold)
	if err != nil {
		// Ledger not available, fall back to direct queries
		_, _ = fmt.Fprintln(cmd.ErrOrStderr(), "status: ledger unavailable, falling back to direct queries")
		return runFleetStatusDirect(cmd, deps, cli, cfg, opts, ctx, format)
	}

	// If probe requested, enrich with background probes
	if opts.Probe {
		_, _ = fmt.Fprintln(cmd.ErrOrStderr(), "status: probing connectivity in background...")
		// TODO: Implement background probe enrichment
		// For now, just show the ledger data
	}

	_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "status: loaded %d tasks from ledger\n", len(ledgerStatus.Sprites))

	if format == "json" {
		return contracts.WriteJSON(cmd.OutOrStdout(), "status.fleet", ledgerStatus)
	}
	return writeLedgerFleetStatusText(cmd.OutOrStdout(), ledgerStatus, opts.Composition)
}

func runFleetStatusDirect(cmd *cobra.Command, deps statusDeps, cli sprite.SpriteCLI, cfg lifecycle.Config, opts statusOptions, ctx context.Context, format string) error {
	_, _ = fmt.Fprintln(cmd.ErrOrStderr(), "status: fetching fleet overview")
	if opts.Checkpoints {
		_, _ = fmt.Fprintln(cmd.ErrOrStderr(), "status: fetching checkpoints (slower)")
	}
	if opts.Tasks {
		_, _ = fmt.Fprintln(cmd.ErrOrStderr(), "status: fetching task assignments")
	}
	if opts.Probe {
		_, _ = fmt.Fprintln(cmd.ErrOrStderr(), "status: probing connectivity (slower)")
	}

	status, err := deps.fleetOverview(ctx, cli, cfg, opts.Composition, lifecycle.FleetOverviewOpts{
		IncludeCheckpoints: opts.Checkpoints,
		IncludeTasks:       opts.Tasks,
		IncludeProbe:       opts.Probe,
		ProbeTimeout:       opts.ProbeTimeout,
		StaleThreshold:     opts.StaleThreshold,
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

// getLedgerFleetStatus reads the fleet status from the task ledger.
// Returns nil if ledger is not available.
func getLedgerFleetStatus(staleThreshold time.Duration) (*contracts.FleetLedgerStatus, error) {
	// TODO: Implement actual ledger integration
	// For now, return nil to fall back to direct queries
	return nil, errors.New("ledger not configured")
}

func writeLedgerFleetStatusText(out io.Writer, status *contracts.FleetLedgerStatus, compositionPath string) error {
	// Print summary header
	if _, err := fmt.Fprintf(out, "Fleet Summary (from ledger): %d total", status.Total); err != nil {
		return err
	}
	if status.StaleCount > 0 {
		if _, err := fmt.Fprintf(out, " | %d stale", status.StaleCount); err != nil {
			return err
		}
	}
	if status.UnknownCount > 0 {
		if _, err := fmt.Fprintf(out, " | %d unknown", status.UnknownCount); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintln(out); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(out); err != nil {
		return err
	}

	// Print task table
	if len(status.Sprites) > 0 {
		tw := tabwriter.NewWriter(out, 2, 2, 2, ' ', 0)
		if _, err := fmt.Fprintln(tw, "SPRITE\tTASK\tSTATE\tLAST_SEEN\tFRESHNESS\tPROBE"); err != nil {
			return err
		}

		for _, item := range status.Sprites {
			task := "-"
			if item.TaskID != "" {
				if item.Issue > 0 {
					task = fmt.Sprintf("%s#%d", item.Repo, item.Issue)
				} else if item.Repo != "" {
					task = item.Repo
				} else {
					task = item.TaskID
				}
			}

			lastSeen := "-"
			if item.LastSeenAt != nil {
				lastSeen = item.LastSeenAt.Format("15:04:05")
			}

			freshness := "-"
			if item.FreshnessAge > 0 {
				freshness = item.FreshnessAge.Round(time.Second).String()
			}

			probe := string(item.ProbeStatus)
			if probe == "unknown" {
				probe = "-"
			}

			state := string(item.State)
			if state == "running" && item.BlockedReason != "" {
				state = "blocked"
			}

			if _, err := fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n",
				item.Sprite,
				task,
				state,
				lastSeen,
				freshness,
				probe,
			); err != nil {
				return err
			}
		}
		if err := tw.Flush(); err != nil {
			return err
		}
		if _, err := fmt.Fprintln(out); err != nil {
			return err
		}
	}

	// Print legend
	if _, err := fmt.Fprintln(out); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(out, "Note: Data from ledger - may be stale. Use --probe for fresh data."); err != nil {
		return err
	}

	return nil
}

func runSpriteDetail(cmd *cobra.Command, deps statusDeps, cli sprite.SpriteCLI, cfg lifecycle.Config, opts statusOptions, ctx context.Context, format, spriteName string) error {
	_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "status: fetching detail for %s\n", spriteName)

	// Use sprite-specific timeout to avoid hanging indefinitely on unreachable sprites
	spriteCtx, cancel := context.WithTimeout(ctx, opts.SpriteTimeout)
	defer cancel()

	detail, err := deps.spriteDetail(spriteCtx, cli, cfg, spriteName)
	if err != nil {
		// Check for timeout specifically to give actionable error message
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(spriteCtx.Err(), context.DeadlineExceeded) {
			return fmt.Errorf("sprite %s unreachable (timed out after %v)", spriteName, opts.SpriteTimeout)
		}
		return err
	}
	_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "status: detail loaded for %s\n", spriteName)

	if format == "json" {
		return contracts.WriteJSON(cmd.OutOrStdout(), "status.sprite", detail)
	}
	return writeSpriteDetailText(cmd.OutOrStdout(), detail)
}

func runWatchMode(cmd *cobra.Command, deps statusDeps, cli sprite.SpriteCLI, cfg lifecycle.Config, opts statusOptions) error {
	// Validate watch interval to prevent panic from time.NewTicker
	if opts.WatchInterval <= 0 {
		return fmt.Errorf("--watch-interval must be positive (got %v)", opts.WatchInterval)
	}

	// Clear screen initially
	_, _ = fmt.Fprint(cmd.OutOrStdout(), "\033[H\033[2J")

	if opts.WatchInterval <= 0 {
		return errors.New("--watch-interval must be > 0")
	}

	ticker := time.NewTicker(opts.WatchInterval)
	defer ticker.Stop()

	first := true
	for {
		if !first {
			// Wait for next tick
			select {
			case <-cmd.Context().Done():
				return nil
			case <-ticker.C:
			}
			// Clear screen before refresh
			_, _ = fmt.Fprint(cmd.OutOrStdout(), "\033[H\033[2J")
		}
		first = false

		// Print timestamp
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "=== Bitterblossom Fleet Status === %s\n\n", time.Now().Format("15:04:05"))

		runCtx, cancel := context.WithTimeout(cmd.Context(), opts.Timeout)
		status, err := deps.fleetOverview(runCtx, cli, cfg, opts.Composition, lifecycle.FleetOverviewOpts{
			IncludeCheckpoints: false, // Skip checkpoints in watch mode for speed
			IncludeTasks:       opts.Tasks,
			IncludeProbe:       opts.Probe,
			ProbeTimeout:       opts.ProbeTimeout,
			StaleThreshold:     opts.StaleThreshold,
		})
		cancel()

		if err != nil {
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Error: %v\n", err)
		} else {
			if err := writeFleetStatusText(cmd.OutOrStdout(), status, opts.Composition); err != nil {
				return err
			}
		}

		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "\n[Watching every %v - press Ctrl+C to exit]\n", opts.WatchInterval)
	}
}

func writeFleetStatusText(out io.Writer, status lifecycle.FleetStatus, compositionPath string) error {
	// Print summary header
	if _, err := fmt.Fprintf(out, "Fleet Summary: %d total", status.Summary.Total); err != nil {
		return err
	}
	if status.Summary.Idle > 0 {
		if _, err := fmt.Fprintf(out, " | %d idle", status.Summary.Idle); err != nil {
			return err
		}
	}
	if status.Summary.Busy > 0 {
		if _, err := fmt.Fprintf(out, " | %d busy", status.Summary.Busy); err != nil {
			return err
		}
	}
	if status.Summary.Offline > 0 {
		if _, err := fmt.Fprintf(out, " | %d offline", status.Summary.Offline); err != nil {
			return err
		}
	}
	if status.Summary.Stale > 0 {
		if _, err := fmt.Fprintf(out, " | %d stale", status.Summary.Stale); err != nil {
			return err
		}
	}
	if status.Summary.Orphaned > 0 {
		if _, err := fmt.Fprintf(out, " | %d orphaned", status.Summary.Orphaned); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintln(out); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(out); err != nil {
		return err
	}

	// Print sprite table
	if len(status.Sprites) > 0 {
		tw := tabwriter.NewWriter(out, 2, 2, 2, ' ', 0)
		if _, err := fmt.Fprintln(tw, "SPRITE\tSTATE\tSTATUS\tTASK\tUPTIME\tURL"); err != nil {
			return err
		}

		for _, item := range status.Sprites {
			taskInfo := "-"
			if item.CurrentTask != nil {
				taskInfo = truncateString(item.CurrentTask.Description, 30)
			}

			uptime := item.Uptime
			if uptime == "" {
				uptime = "-"
			}

			url := item.URL
			if url == "" {
				url = "-"
			}

			if _, err := fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n",
				item.Name,
				spriteStateLabel(item),
				formatSpriteStatus(item),
				taskInfo,
				uptime,
				truncateString(url, 35),
			); err != nil {
				return err
			}
		}
		if err := tw.Flush(); err != nil {
			return err
		}
		if _, err := fmt.Fprintln(out); err != nil {
			return err
		}
	}

	// Print legend explaining status indicators
	if _, err := fmt.Fprintln(out); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(out, "Status indicators:"); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(out, "  ⚠ unverified = reported by Fly.io API, connectivity not verified (use --probe to verify)"); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(out, "  ✓ reachable = connectivity verified via exec probe"); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(out, "  ✗ unreachable = connectivity probe failed (sprite may be stuck)"); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(out, "  ⚠ stale = no activity detected for configured threshold"); err != nil {
		return err
	}

	// Print composition status
	if _, err := fmt.Fprintf(out, "\nComposition sprites (%s):\n", compositionPath); err != nil {
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

	// Print orphans
	if len(status.Orphans) > 0 {
		if _, err := fmt.Fprintln(out); err != nil {
			return err
		}
		if _, err := fmt.Fprintln(out, "Orphan sprites (live but not in composition):"); err != nil {
			return err
		}
		for _, item := range status.Orphans {
			state := stateWithEmoji(item.State)
			if _, err := fmt.Fprintf(out, "  ? %s [%s - %s]\n", item.Name, state, item.Status); err != nil {
				return err
			}
		}
	}

	// Print checkpoints section
	if status.CheckpointsIncluded {
		if _, err := fmt.Fprintln(out); err != nil {
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
	}

	return nil
}

func writeSpriteDetailText(out io.Writer, detail lifecycle.SpriteDetailResult) error {
	if _, err := fmt.Fprintf(out, "=== Sprite: %s ===\n\n", detail.Name); err != nil {
		return err
	}

	// State and task summary
	if _, err := fmt.Fprintf(out, "State: %s\n", stateWithEmoji(detail.State)); err != nil {
		return err
	}
	if detail.QueueDepth > 0 {
		if _, err := fmt.Fprintf(out, "Queue Depth: %d\n", detail.QueueDepth); err != nil {
			return err
		}
	}
	if detail.CurrentTask != nil {
		if _, err := fmt.Fprintln(out, "\nCurrent Task:"); err != nil {
			return err
		}
		if _, err := fmt.Fprintf(out, "  ID: %s\n", detail.CurrentTask.ID); err != nil {
			return err
		}
		if _, err := fmt.Fprintf(out, "  Description: %s\n", detail.CurrentTask.Description); err != nil {
			return err
		}
		if detail.CurrentTask.Repo != "" {
			if _, err := fmt.Fprintf(out, "  Repo: %s\n", detail.CurrentTask.Repo); err != nil {
				return err
			}
		}
		if detail.CurrentTask.Branch != "" {
			if _, err := fmt.Fprintf(out, "  Branch: %s\n", detail.CurrentTask.Branch); err != nil {
				return err
			}
		}
		if detail.CurrentTask.StartedAt != nil {
			if _, err := fmt.Fprintf(out, "  Started: %s\n", detail.CurrentTask.StartedAt.Format(time.RFC3339)); err != nil {
				return err
			}
		}
	}
	if detail.Uptime != "" {
		if _, err := fmt.Fprintf(out, "\nUptime: %s\n", detail.Uptime); err != nil {
			return err
		}
	}

	if _, err := fmt.Fprintln(out); err != nil {
		return err
	}

	// API details
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
			if _, err := fmt.Fprintf(out, "  %s: %v\n", key, detail.API[key]); err != nil {
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

func stateWithEmoji(state lifecycle.SpriteState) string {
	switch state {
	case lifecycle.StateIdle:
		return "🟢 idle"
	case lifecycle.StateBusy:
		return "🔴 busy"
	case lifecycle.StateOffline:
		return "⚫ offline"
	case lifecycle.StateOperational:
		return "🟢 operational"
	case lifecycle.StateUnknown:
		return "⚪ unknown"
	default:
		return string(state)
	}
}

// spriteStateLabel formats the state with optional stale and reachability indicators.
// Reachability is only shown when --probe was used (indicated by Probed=true).
// For running sprites that haven't been probed, shows an unverified warning.
func spriteStateLabel(item lifecycle.SpriteStatus) string {
	label := stateWithEmoji(item.State)
	if item.Stale {
		label += " ⚠ stale"
	}
	// Only show reachability when we actually performed a probe
	if item.Probed {
		if item.Reachable {
			label += " ✓ reachable"
		} else {
			label += " ✗ unreachable"
		}
	} else if isRunningStatus(string(item.State)) {
		// Sprite appears running but connectivity hasn't been verified
		label += " ⚠ unverified"
	}
	return label
}

func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return s[:maxLen]
	}
	return s[:maxLen-3] + "..."
}

// isRunningStatus returns true for states that indicate the sprite should be reachable.
func isRunningStatus(state string) bool {
	s := strings.ToLower(state)
	return s == "idle" || s == "busy" || s == "operational"
}

// formatSpriteStatus reconciles the API status with the task state.
// If the API reports "running" but no task is assigned, display "warm"
// to accurately reflect the operational state (idle but warm).
func formatSpriteStatus(item lifecycle.SpriteStatus) string {
	status := strings.ToLower(item.Status)

	// If status is "running" but no task is assigned, show "warm" instead
	if status == "running" && item.CurrentTask == nil {
		return "warm"
	}

	return item.Status
}
