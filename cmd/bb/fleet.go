package main

import (
	"bufio"
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
	"github.com/misty-step/bitterblossom/internal/fleet"
	"github.com/misty-step/bitterblossom/internal/lifecycle"
	"github.com/misty-step/bitterblossom/internal/registry"
	"github.com/misty-step/bitterblossom/internal/sprite"
	"github.com/spf13/cobra"
)

type fleetOptions struct {
	RegistryPath string
	Org          string
	SpriteCLI    string
	Composition  string
	Sync         bool
	Prune        bool
	DryRun       bool
	Format       string
	Timeout      time.Duration
}

type fleetDeps struct {
	getwd             func() (string, error)
	newCLI            func(binary, org string) sprite.SpriteCLI
	loadRegistry      func(path string) (*registry.Registry, error)
	parseComposition  func(path string) (fleet.Composition, error)
	provision         func(ctx context.Context, cli sprite.SpriteCLI, cfg lifecycle.Config, opts lifecycle.ProvisionOpts) (lifecycle.ProvisionResult, error)
	teardown          func(ctx context.Context, cli sprite.SpriteCLI, cfg lifecycle.Config, opts lifecycle.TeardownOpts) (lifecycle.TeardownResult, error)
	resolveGitHubAuth func(spriteName string, getenv func(string) string) (lifecycle.GitHubAuth, error)
	renderSettings    func(settingsPath, authToken string) (string, error)
	getenv            func(string) string
}

func defaultFleetDeps() fleetDeps {
	return fleetDeps{
		getwd:             os.Getwd,
		newCLI:            newFleetCLI,
		loadRegistry:      registry.Load,
		parseComposition:  fleet.ParseComposition,
		provision:         lifecycle.Provision,
		teardown:          lifecycle.Teardown,
		resolveGitHubAuth: lifecycle.ResolveGitHubAuth,
		renderSettings:    lifecycle.RenderSettings,
		getenv:            os.Getenv,
	}
}

func newFleetCLI(binary, org string) sprite.SpriteCLI {
	return sprite.NewCLIWithOrg(binary, org)
}

func newFleetCmd() *cobra.Command {
	return newFleetCmdWithDeps(defaultFleetDeps())
}

func newFleetCmdWithDeps(deps fleetDeps) *cobra.Command {
	opts := fleetOptions{
		RegistryPath: registry.DefaultPath(),
		Org:          defaultOrg(),
		SpriteCLI:    defaultSpriteCLIPath(),
		Composition:  defaultLifecycleComposition,
		Format:       "text",
		Timeout:      10 * time.Minute,
	}

	command := &cobra.Command{
		Use:   "fleet",
		Short: "Manage and reconcile the sprite fleet",
		Long: `Show registered sprites and reconcile fleet state.

List view shows all registered sprites with their current status:
  bb fleet

Sync mode creates missing sprites from the registry:
  bb fleet --sync

With --prune, also removes sprites that exist but aren't registered:
  bb fleet --sync --prune

Use --dry-run to preview changes without applying them:
  bb fleet --sync --prune --dry-run`,
		RunE: func(cmd *cobra.Command, args []string) error {
			format := strings.ToLower(strings.TrimSpace(opts.Format))
			if format != "json" && format != "text" {
				return errors.New("--format must be json or text")
			}

			rootDir, err := deps.getwd()
			if err != nil {
				return err
			}

			// Load registry
			reg, err := deps.loadRegistry(opts.RegistryPath)
			if err != nil {
				return fmt.Errorf("loading registry: %w", err)
			}

			// Get actual sprites from Fly.io
			ctx, cancel := context.WithTimeout(cmd.Context(), opts.Timeout)
			defer cancel()

			cli := deps.newCLI(opts.SpriteCLI, opts.Org)
			actualSprites, err := cli.List(ctx)
			if err != nil {
				return fmt.Errorf("listing sprites: %w", err)
			}

			// Build fleet status
			status := buildFleetStatus(reg, actualSprites)

			// If not syncing, just display status
			if !opts.Sync {
				if format == "json" {
					return contracts.WriteJSON(cmd.OutOrStdout(), "fleet", status)
				}
				return writeFleetText(cmd.OutOrStdout(), status, opts.RegistryPath)
			}

			// Sync mode: reconcile state
			return runFleetSync(cmd, deps, cli, rootDir, opts, reg, actualSprites, status, format)
		},
	}

	command.Flags().StringVar(&opts.RegistryPath, "registry", opts.RegistryPath, "Path to registry TOML file")
	command.Flags().StringVar(&opts.Org, "org", opts.Org, "Sprites organization")
	command.Flags().StringVar(&opts.SpriteCLI, "sprite-cli", opts.SpriteCLI, "Path to sprite CLI")
	command.Flags().StringVar(&opts.Composition, "composition", opts.Composition, "Path to composition YAML")
	command.Flags().BoolVar(&opts.Sync, "sync", false, "Reconcile fleet state (create missing sprites)")
	command.Flags().BoolVar(&opts.Prune, "prune", false, "Remove sprites not in registry (requires --sync)")
	command.Flags().BoolVar(&opts.DryRun, "dry-run", false, "Preview changes without applying them")
	command.Flags().StringVar(&opts.Format, "format", opts.Format, "Output format: json|text")
	command.Flags().DurationVar(&opts.Timeout, "timeout", opts.Timeout, "Command timeout")

	return command
}

// FleetStatus represents the status of all registered sprites
type FleetStatus struct {
	Sprites      []SpriteInfo      `json:"sprites"`
	Summary      FleetStatusSummary `json:"summary"`
	RegistryPath string            `json:"registry_path"`
}

// SpriteInfo represents one sprite's status
type SpriteInfo struct {
	Name       string    `json:"name"`
	MachineID  string    `json:"machine_id"`
	Status     string    `json:"status"`
	CreatedAt  time.Time `json:"created_at"`
	LastSeen   time.Time `json:"last_seen,omitempty"`
	Issue      int       `json:"issue,omitempty"`
	IssueRepo  string    `json:"issue_repo,omitempty"`
}

// FleetStatusSummary provides aggregated statistics
type FleetStatusSummary struct {
	Total     int `json:"total"`
	Running   int `json:"running"`
	Stopped   int `json:"stopped"`
	NotFound  int `json:"not_found"`
	Orphaned  int `json:"orphaned"`
}

// SyncResult reports the outcome of a sync operation
type SyncResult struct {
	Created   []string `json:"created,omitempty"`
	Destroyed []string `json:"destroyed,omitempty"`
	Skipped   []string `json:"skipped,omitempty"`
	Errors    []string `json:"errors,omitempty"`
	DryRun    bool     `json:"dry_run"`
}

func buildFleetStatus(reg *registry.Registry, actualSprites []string) FleetStatus {
	actualSet := make(map[string]bool, len(actualSprites))
	for _, name := range actualSprites {
		actualSet[name] = true
	}

	status := FleetStatus{
		Sprites: make([]SpriteInfo, 0, reg.Count()),
		Summary: FleetStatusSummary{},
	}

	// Build status for registered sprites
	for _, name := range reg.Names() {
		entry := reg.Sprites[name]
		spriteInfo := SpriteInfo{
			Name:      name,
			MachineID: entry.MachineID,
			CreatedAt: entry.CreatedAt,
		}

		if actualSet[name] {
			spriteInfo.Status = "running"
			spriteInfo.LastSeen = time.Now()
			status.Summary.Running++
		} else {
			spriteInfo.Status = "not found"
			status.Summary.NotFound++
		}

		status.Sprites = append(status.Sprites, spriteInfo)
		status.Summary.Total++
	}

	// Count orphaned sprites (exist but not registered)
	registeredSet := make(map[string]bool)
	for _, name := range reg.Names() {
		registeredSet[name] = true
	}
	for _, name := range actualSprites {
		if !registeredSet[name] {
			status.Summary.Orphaned++
			// Add orphaned sprites to the list with special status
			status.Sprites = append(status.Sprites, SpriteInfo{
				Name:   name,
				Status: "orphaned",
			})
		}
	}

	// Sort sprites by name
	sort.Slice(status.Sprites, func(i, j int) bool {
		return status.Sprites[i].Name < status.Sprites[j].Name
	})

	return status
}

func writeFleetText(out io.Writer, status FleetStatus, registryPath string) error {
	// Print header
	if _, err := fmt.Fprintf(out, "=== Bitterblossom Fleet ===\n"); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(out, "Registry: %s\n\n", registryPath); err != nil {
		return err
	}

	// Print summary
	if _, err := fmt.Fprintf(out, "Summary: %d registered | %d running | %d not found | %d orphaned\n\n",
		status.Summary.Total, status.Summary.Running, status.Summary.NotFound, status.Summary.Orphaned); err != nil {
		return err
	}

	if len(status.Sprites) == 0 {
		if _, err := fmt.Fprintln(out, "No sprites registered."); err != nil {
			return err
		}
		return nil
	}

	// Print sprite table
	tw := tabwriter.NewWriter(out, 2, 2, 2, ' ', 0)
	if _, err := fmt.Fprintln(tw, "SPRITE\tSTATUS\tMACHINE ID\tCREATED\tISSUE"); err != nil {
		return err
	}

	for _, s := range status.Sprites {
		createdStr := "-"
		if !s.CreatedAt.IsZero() {
			createdStr = formatDuration(time.Since(s.CreatedAt)) + " ago"
		}

		machineID := s.MachineID
		if machineID == "" {
			machineID = "-"
		}

		statusEmoji := statusWithEmoji(s.Status)

		if _, err := fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%d\n",
			s.Name,
			statusEmoji,
			truncateString(machineID, 12),
			createdStr,
			s.Issue,
		); err != nil {
			return err
		}
	}

	if err := tw.Flush(); err != nil {
		return err
	}

	return nil
}

func statusWithEmoji(status string) string {
	switch status {
	case "running":
		return "🟢 running"
	case "stopped":
		return "🔴 stopped"
	case "not found":
		return "⚪ not found"
	case "orphaned":
		return "🟠 orphaned"
	default:
		return status
	}
}

func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	if d < 24*time.Hour {
		return fmt.Sprintf("%dh", int(d.Hours()))
	}
	return fmt.Sprintf("%dd", int(d.Hours()/24))
}

func runFleetSync(cmd *cobra.Command, deps fleetDeps, cli sprite.SpriteCLI, rootDir string, opts fleetOptions,
	reg *registry.Registry, actualSprites []string, status FleetStatus, format string) error {

	cfg := defaultLifecycleConfig(rootDir, opts.Org)

	// Build sets for comparison
	registeredSet := make(map[string]bool)
	for _, name := range reg.Names() {
		registeredSet[name] = true
	}

	actualSet := make(map[string]bool)
	for _, name := range actualSprites {
		actualSet[name] = true
	}

	result := SyncResult{
		Created:   []string{},
		Destroyed: []string{},
		Skipped:   []string{},
		Errors:    []string{},
		DryRun:    opts.DryRun,
	}

	// Render settings for provisioning
	var renderedSettings string
	var settingsErr error
	if !opts.DryRun {
		settingsPath := cfg.BaseDir + "/settings.json"
		authToken := resolveLifecycleAuthToken(deps.getenv)
		if authToken == "" {
			return errors.New("fleet sync: OPENROUTER_API_KEY is required")
		}
		renderedSettings, settingsErr = deps.renderSettings(settingsPath, authToken)
		if settingsErr != nil {
			return fmt.Errorf("rendering settings: %w", settingsErr)
		}
		defer func() {
			_ = os.Remove(renderedSettings)
		}()
	}

	// Find missing sprites (registered but don't exist)
	var missing []string
	for _, name := range reg.Names() {
		if !actualSet[name] {
			missing = append(missing, name)
		}
	}
	sort.Strings(missing)

	// Create missing sprites
	for _, name := range missing {
		if opts.DryRun {
			result.Created = append(result.Created, name)
			_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "[dry-run] Would create sprite: %s\n", name)
			continue
		}

		_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Creating sprite: %s\n", name)

		// Get GitHub auth for provisioning
		auth, err := deps.resolveGitHubAuth(name, deps.getenv)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("%s: auth error: %v", name, err))
			continue
		}

		// Provision the sprite
		_, err = deps.provision(cmd.Context(), cli, cfg, lifecycle.ProvisionOpts{
			Name:             name,
			CompositionLabel: "fleet-sync",
			SettingsPath:     renderedSettings,
			GitHubAuth:       auth,
		})
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("%s: %v", name, err))
			continue
		}

		result.Created = append(result.Created, name)
	}

	// Find orphaned sprites (exist but not registered)
	if opts.Prune {
		var orphaned []string
		for _, name := range actualSprites {
			if !registeredSet[name] {
				orphaned = append(orphaned, name)
			}
		}
		sort.Strings(orphaned)

		// Confirm destruction if not dry-run
		if !opts.DryRun && len(orphaned) > 0 {
			if !confirmDestruction(cmd.InOrStdin(), cmd.OutOrStdout(), orphaned) {
				_, _ = fmt.Fprintln(cmd.ErrOrStderr(), "Prune cancelled by user")
				// Continue to output results without destroying
				goto outputResults
			}
		}

		// Destroy orphaned sprites
		for _, name := range orphaned {
			if opts.DryRun {
				result.Destroyed = append(result.Destroyed, name)
				_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "[dry-run] Would destroy sprite: %s\n", name)
				continue
			}

			_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Destroying orphaned sprite: %s\n", name)

			// Create archive directory for teardown
			archiveDir := rootDir + "/observations/archives"
			_, err := deps.teardown(cmd.Context(), cli, cfg, lifecycle.TeardownOpts{
				Name:       name,
				ArchiveDir: archiveDir,
			})
			if err != nil {
				result.Errors = append(result.Errors, fmt.Sprintf("%s: %v", name, err))
				continue
			}

			result.Destroyed = append(result.Destroyed, name)
		}
	}

outputResults:
	// Output results
	if format == "json" {
		// Build final status with sync results
		output := struct {
			Before FleetStatus `json:"before"`
			Sync   SyncResult  `json:"sync"`
		}{
			Before: status,
			Sync:   result,
		}
		return contracts.WriteJSON(cmd.OutOrStdout(), "fleet.sync", output)
	}

	// Text output
	if _, err := fmt.Fprintln(cmd.OutOrStdout()); err != nil {
		return err
	}
	if opts.DryRun {
		if _, err := fmt.Fprintln(cmd.OutOrStdout(), "=== Dry Run Results ==="); err != nil {
			return err
		}
	} else {
		if _, err := fmt.Fprintln(cmd.OutOrStdout(), "=== Sync Results ==="); err != nil {
			return err
		}
	}

	if len(result.Created) > 0 {
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Created (%d): %s\n", len(result.Created), strings.Join(result.Created, ", ")); err != nil {
			return err
		}
	}
	if len(result.Destroyed) > 0 {
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Destroyed (%d): %s\n", len(result.Destroyed), strings.Join(result.Destroyed, ", ")); err != nil {
			return err
		}
	}
	if len(result.Skipped) > 0 {
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Skipped (%d): %s\n", len(result.Skipped), strings.Join(result.Skipped, ", ")); err != nil {
			return err
		}
	}
	if len(result.Errors) > 0 {
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Errors (%d):\n", len(result.Errors)); err != nil {
			return err
		}
		for _, e := range result.Errors {
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "  - %s\n", e); err != nil {
				return err
			}
		}
	}

	if len(result.Created) == 0 && len(result.Destroyed) == 0 && len(result.Errors) == 0 {
		if _, err := fmt.Fprintln(cmd.OutOrStdout(), "No changes needed. Fleet is in sync."); err != nil {
			return err
		}
	}

	return nil
}

func confirmDestruction(stdin io.Reader, stdout io.Writer, sprites []string) bool {
	_, _ = fmt.Fprintf(stdout, "\nThe following sprites will be DESTROYED:\n")
	for _, name := range sprites {
		_, _ = fmt.Fprintf(stdout, "  - %s\n", name)
	}
	_, _ = fmt.Fprintf(stdout, "\nAre you sure? Type 'yes' to confirm: ")

	scanner := bufio.NewScanner(stdin)
	if !scanner.Scan() {
		return false
	}
	return strings.TrimSpace(scanner.Text()) == "yes"
}
