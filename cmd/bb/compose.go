package main

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/misty-step/bitterblossom/internal/contracts"
	"github.com/misty-step/bitterblossom/internal/fleet"
	"github.com/misty-step/bitterblossom/internal/sprite"
	"github.com/spf13/cobra"
)

// isNotFoundError reports whether a CLI error indicates the sprite does not exist.
func isNotFoundError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "not found") || strings.Contains(msg, "does not exist")
}

type composeOptions struct {
	CompositionPath string
	Org             string
	SpriteCLI       string
	JSON            bool
	Execute         bool
}

type composeDeps struct {
	parseComposition func(path string) (fleet.Composition, error)
	newCLI           func(binary, org string) sprite.SpriteCLI
}

func defaultComposeDeps() composeDeps {
	return composeDeps{
		parseComposition: fleet.ParseComposition,
		newCLI: func(binary, org string) sprite.SpriteCLI {
			return sprite.NewCLIWithOrg(binary, org)
		},
	}
}

func newComposeCmd() *cobra.Command {
	return newComposeCmdWithDeps(defaultComposeDeps())
}

func newComposeCmdWithDeps(deps composeDeps) *cobra.Command {
	opts := composeOptions{
		CompositionPath: defaultLifecycleComposition,
		Org:             defaultOrg(),
		SpriteCLI:       defaultSpriteCLIPath(),
	}

	cmd := &cobra.Command{
		Use:   "compose",
		Short: "Composition-driven fleet reconciliation",
	}

	cmd.PersistentFlags().StringVar(&opts.CompositionPath, "composition", opts.CompositionPath, "Path to composition YAML")
	cmd.PersistentFlags().StringVar(&opts.Org, "org", opts.Org, "Sprites organization")
	cmd.PersistentFlags().StringVar(&opts.SpriteCLI, "sprite-cli", opts.SpriteCLI, "Path to sprite CLI")
	cmd.PersistentFlags().BoolVar(&opts.JSON, "json", false, "Emit JSON output")

	diffCmd := &cobra.Command{
		Use:   "diff",
		Short: "Show reconciliation actions without executing",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runComposeDiff(cmd.Context(), cmd, opts, deps)
		},
	}

	applyCmd := &cobra.Command{
		Use:   "apply",
		Short: "Apply reconciliation actions (--dry-run by default)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runComposeApply(cmd.Context(), cmd, opts, deps)
		},
	}
	applyCmd.Flags().BoolVar(&opts.Execute, "execute", false, "Execute reconciliation actions (default is dry-run)")

	statusCmd := &cobra.Command{
		Use:   "status",
		Short: "Show current composition vs desired",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runComposeStatus(cmd.Context(), cmd, opts, deps)
		},
	}

	cmd.AddCommand(diffCmd, applyCmd, statusCmd)
	return cmd
}

func runComposeDiff(ctx context.Context, cmd *cobra.Command, opts composeOptions, deps composeDeps) error {
	composition, actual, _, err := loadFleetState(ctx, opts, deps)
	if err != nil {
		return err
	}

	actions := fleet.Reconcile(composition, actual)
	if opts.JSON {
		return printJSON(cmd, fleet.ActionsView(actions))
	}
	return printActionsHuman(cmd, actions)
}

func runComposeApply(ctx context.Context, cmd *cobra.Command, opts composeOptions, deps composeDeps) error {
	composition, actual, cli, err := loadFleetState(ctx, opts, deps)
	if err != nil {
		return err
	}

	actions := fleet.Reconcile(composition, actual)
	executor := fleet.Executor{}

	if !opts.Execute {
		if opts.JSON {
			payload := map[string]any{
				"execute": false,
				"actions": fleet.ActionsView(actions),
			}
			return printJSON(cmd, payload)
		}

		if _, err := fmt.Fprintln(cmd.OutOrStdout(), "Dry run (pass --execute to apply):"); err != nil {
			return err
		}
		for _, line := range executor.DryRun(actions) {
			if _, err := fmt.Fprintln(cmd.OutOrStdout(), line); err != nil {
				return err
			}
		}
		if len(actions) == 0 {
			_, err := fmt.Fprintln(cmd.OutOrStdout(), "Fleet already converged.")
			return err
		}
		return nil
	}

	executor.Runtime = newComposeRuntime(cli, opts.Org)
	if err := executor.Execute(ctx, actions); err != nil {
		return err
	}

	if opts.JSON {
		payload := map[string]any{
			"execute":  true,
			"executed": len(actions),
			"actions":  fleet.ActionsView(actions),
		}
		return printJSON(cmd, payload)
	}

	if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Executed %d action(s).\n", len(actions)); err != nil {
		return err
	}
	for _, action := range fleet.SortActions(actions) {
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "- %s\n", action.Description()); err != nil {
			return err
		}
	}
	if len(actions) == 0 {
		_, err := fmt.Fprintln(cmd.OutOrStdout(), "Fleet already converged.")
		return err
	}
	return nil
}

func runComposeStatus(ctx context.Context, cmd *cobra.Command, opts composeOptions, deps composeDeps) error {
	composition, actual, _, err := loadFleetState(ctx, opts, deps)
	if err != nil {
		return err
	}

	plan := fleet.BuildPlan(composition, actual)
	rows := buildStatusRows(composition, actual)

	if opts.JSON {
		payload := map[string]any{
			"composition": composition.Name,
			"desired":     len(composition.Sprites),
			"actual":      len(actual),
			"missing":     spriteNames(plan.Missing),
			"extra":       plan.Extra,
			"drift":       plan.Drift,
			"rows":        rows,
		}
		return contracts.WriteJSON(cmd.OutOrStdout(), "compose.status", payload)
	}

	if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Composition: %s\n", composition.Name); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Desired: %d\n", len(composition.Sprites)); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Actual:  %d\n", len(actual)); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Missing: %d  Extra: %d  Drift: %d\n\n", len(plan.Missing), len(plan.Extra), len(plan.Drift)); err != nil {
		return err
	}

	tw := tabwriter.NewWriter(cmd.OutOrStdout(), 2, 2, 2, ' ', 0)
	if _, err := fmt.Fprintln(tw, "SPRITE\tSTATE\tDESIRED_PERSONA\tACTUAL_PERSONA\tDESIRED_CONFIG\tACTUAL_CONFIG\tSTATUS"); err != nil {
		return err
	}
	for _, row := range rows {
		if _, err := fmt.Fprintf(
			tw,
			"%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
			row.Name,
			row.State,
			row.DesiredPersona,
			row.ActualPersona,
			row.DesiredConfig,
			row.ActualConfig,
			row.Status,
		); err != nil {
			return err
		}
	}
	return tw.Flush()
}

func loadFleetState(ctx context.Context, opts composeOptions, deps composeDeps) (fleet.Composition, []fleet.SpriteStatus, sprite.SpriteCLI, error) {
	composition, err := deps.parseComposition(opts.CompositionPath)
	if err != nil {
		return fleet.Composition{}, nil, nil, err
	}

	cli := deps.newCLI(opts.SpriteCLI, opts.Org)

	names, err := cli.List(ctx)
	if err != nil {
		return fleet.Composition{}, nil, nil, fmt.Errorf("listing sprites: %w", err)
	}
	return composition, namesToSpriteStatuses(names, composition), cli, nil
}

// namesToSpriteStatuses converts observed sprite names into SpriteStatus values,
// populating Persona and ConfigVersion from the composition for sprites that
// match a desired spec. Without this metadata, BuildPlan would detect false
// drift on every existing sprite and trigger non-idempotent updates.
func namesToSpriteStatuses(names []string, composition fleet.Composition) []fleet.SpriteStatus {
	desiredByName := make(map[string]fleet.SpriteSpec, len(composition.Sprites))
	for _, spec := range composition.Sprites {
		desiredByName[spec.Name] = spec
	}
	configVersion := ""
	if composition.Version > 0 {
		configVersion = strconv.Itoa(composition.Version)
	}

	statuses := make([]fleet.SpriteStatus, 0, len(names))
	for _, name := range names {
		s := fleet.SpriteStatus{
			Name:  name,
			State: sprite.StateIdle,
		}
		if spec, ok := desiredByName[name]; ok {
			s.Persona = spec.Persona.Name
			s.ConfigVersion = configVersion
		}
		statuses = append(statuses, s)
	}
	sort.Slice(statuses, func(i, j int) bool {
		return statuses[i].Name < statuses[j].Name
	})
	return statuses
}

func printJSON(cmd *cobra.Command, payload any) error {
	encoded, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintln(cmd.OutOrStdout(), string(encoded)); err != nil {
		return err
	}
	return nil
}

func printActionsHuman(cmd *cobra.Command, actions []fleet.Action) error {
	if len(actions) == 0 {
		_, err := fmt.Fprintln(cmd.OutOrStdout(), "Fleet already converged.")
		return err
	}

	tw := tabwriter.NewWriter(cmd.OutOrStdout(), 2, 2, 2, ' ', 0)
	if _, err := fmt.Fprintln(tw, "ACTION\tSPRITE\tDESCRIPTION"); err != nil {
		return err
	}
	for _, action := range fleet.SortActions(actions) {
		if _, err := fmt.Fprintf(tw, "%s\t%s\t%s\n", action.Kind(), action.SpriteName(), action.Description()); err != nil {
			return err
		}
	}
	return tw.Flush()
}

type composeRuntime struct {
	cli sprite.SpriteCLI
	org string
}

func newComposeRuntime(cli sprite.SpriteCLI, org string) *composeRuntime {
	return &composeRuntime{cli: cli, org: org}
}

func (r *composeRuntime) Provision(ctx context.Context, action fleet.ProvisionAction) error {
	return r.cli.Create(ctx, action.Sprite.Name, r.org)
}

func (r *composeRuntime) Teardown(ctx context.Context, action fleet.TeardownAction) error {
	if err := r.cli.Destroy(ctx, action.Name, r.org); err != nil && !isNotFoundError(err) {
		return err
	}
	return nil
}

func (r *composeRuntime) Update(ctx context.Context, action fleet.UpdateAction) error {
	// Destroy before recreating; tolerate not-found but propagate real failures.
	if err := r.cli.Destroy(ctx, action.Desired.Name, r.org); err != nil && !isNotFoundError(err) {
		return fmt.Errorf("destroying sprite %q before update: %w", action.Desired.Name, err)
	}
	return r.cli.Create(ctx, action.Desired.Name, r.org)
}

func (r *composeRuntime) Redispatch(ctx context.Context, action fleet.RedispatchAction) error {
	_, err := r.cli.Exec(ctx, action.Name, "echo redispatch-requested", nil)
	if isNotFoundError(err) {
		return nil
	}
	return err
}

type statusRow struct {
	Name           string `json:"name"`
	State          string `json:"state"`
	DesiredPersona string `json:"desired_persona,omitempty"`
	ActualPersona  string `json:"actual_persona,omitempty"`
	DesiredConfig  string `json:"desired_config,omitempty"`
	ActualConfig   string `json:"actual_config,omitempty"`
	Status         string `json:"status"`
}

func buildStatusRows(composition fleet.Composition, actual []fleet.SpriteStatus) []statusRow {
	actualByName := make(map[string]fleet.SpriteStatus, len(actual))
	for _, spriteStatus := range actual {
		actualByName[spriteStatus.Name] = spriteStatus
	}

	desiredConfig := ""
	if composition.Version > 0 {
		desiredConfig = strconv.Itoa(composition.Version)
	}

	rows := make([]statusRow, 0, len(composition.Sprites)+len(actual))
	desiredNames := make(map[string]struct{}, len(composition.Sprites))
	for _, desired := range composition.Sprites {
		desiredNames[desired.Name] = struct{}{}

		current, exists := actualByName[desired.Name]
		if !exists {
			rows = append(rows, statusRow{
				Name:           desired.Name,
				State:          string(sprite.StateDead),
				DesiredPersona: desired.Persona.Name,
				DesiredConfig:  desiredConfig,
				Status:         "missing",
			})
			continue
		}

		status := "ok"
		if strings.TrimSpace(current.Persona) != strings.TrimSpace(desired.Persona.Name) ||
			strings.TrimSpace(current.ConfigVersion) != strings.TrimSpace(desiredConfig) {
			status = "drift"
		}
		rows = append(rows, statusRow{
			Name:           desired.Name,
			State:          string(current.State),
			DesiredPersona: desired.Persona.Name,
			ActualPersona:  current.Persona,
			DesiredConfig:  desiredConfig,
			ActualConfig:   current.ConfigVersion,
			Status:         status,
		})
	}

	for _, status := range actual {
		if _, exists := desiredNames[status.Name]; exists {
			continue
		}
		rows = append(rows, statusRow{
			Name:          status.Name,
			State:         string(status.State),
			ActualPersona: status.Persona,
			ActualConfig:  status.ConfigVersion,
			Status:        "extra",
		})
	}

	sort.Slice(rows, func(i, j int) bool {
		return rows[i].Name < rows[j].Name
	})
	return rows
}

func spriteNames(specs []fleet.SpriteSpec) []string {
	names := make([]string, 0, len(specs))
	for _, spec := range specs {
		names = append(names, spec.Name)
	}
	sort.Strings(names)
	return names
}
