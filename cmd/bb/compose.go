package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/misty-step/bitterblossom/internal/contracts"
	"github.com/misty-step/bitterblossom/internal/fleet"
	"github.com/misty-step/bitterblossom/internal/sprite"
	"github.com/misty-step/bitterblossom/pkg/fly"
	"github.com/spf13/cobra"
)

type composeOptions struct {
	CompositionPath string
	App             string
	Token           string
	JSON            bool
	Execute         bool
	APIURL          string
}

type composeDeps struct {
	parseComposition func(path string) (fleet.Composition, error)
	newClient        func(token, apiURL string) (fly.MachineClient, error)
}

func defaultComposeDeps() composeDeps {
	return composeDeps{
		parseComposition: fleet.ParseComposition,
		newClient: func(token, apiURL string) (fly.MachineClient, error) {
			return fly.NewClient(token, fly.WithBaseURL(apiURL))
		},
	}
}

func newComposeCmd() *cobra.Command {
	return newComposeCmdWithDeps(defaultComposeDeps())
}

func newComposeCmdWithDeps(deps composeDeps) *cobra.Command {
	opts := composeOptions{
		CompositionPath: "compositions/v1.yaml",
		App:             strings.TrimSpace(os.Getenv("FLY_APP")),
		Token:           defaultFlyToken(),
		APIURL:          fly.DefaultBaseURL,
	}

	cmd := &cobra.Command{
		Use:   "compose",
		Short: "Composition-driven fleet reconciliation",
	}

	cmd.PersistentFlags().StringVar(&opts.CompositionPath, "composition", opts.CompositionPath, "Path to composition YAML")
	cmd.PersistentFlags().StringVar(&opts.App, "app", opts.App, "Sprites app name")
	cmd.PersistentFlags().StringVar(&opts.Token, "token", opts.Token, "API token (or FLY_API_TOKEN)")
	cmd.PersistentFlags().StringVar(&opts.APIURL, "api-url", opts.APIURL, "Sprites API base URL")
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
	composition, actual, client, err := loadFleetState(ctx, opts, deps)
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

	executor.Runtime = newComposeRuntime(opts.App, client, actual)
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

func loadFleetState(ctx context.Context, opts composeOptions, deps composeDeps) (fleet.Composition, []fleet.SpriteStatus, fly.MachineClient, error) {
	composition, err := deps.parseComposition(opts.CompositionPath)
	if err != nil {
		return fleet.Composition{}, nil, nil, err
	}

	appMissing := strings.TrimSpace(opts.App) == ""
	tokenMissing := strings.TrimSpace(opts.Token) == ""
	if appMissing || tokenMissing {
		return fleet.Composition{}, nil, nil, errors.New("Error: FLY_APP and FLY_API_TOKEN are required for sprite operations.\n  export FLY_APP=your-app\n  export FLY_API_TOKEN=your-token")
	}

	client, err := deps.newClient(opts.Token, opts.APIURL)
	if err != nil {
		return fleet.Composition{}, nil, nil, err
	}

	machines, err := client.List(ctx, opts.App)
	if err != nil {
		return fleet.Composition{}, nil, nil, err
	}
	return composition, machinesToSpriteStatuses(machines), client, nil
}

func machinesToSpriteStatuses(machines []fly.Machine) []fleet.SpriteStatus {
	statuses := make([]fleet.SpriteStatus, 0, len(machines))
	for _, machine := range machines {
		statuses = append(statuses, fleet.SpriteStatus{
			Name:          machine.Name,
			MachineID:     machine.ID,
			State:         mapMachineState(machine.State),
			Persona:       machine.Metadata["persona"],
			ConfigVersion: machine.Metadata["config_version"],
		})
	}
	sort.Slice(statuses, func(i, j int) bool {
		if statuses[i].Name != statuses[j].Name {
			return statuses[i].Name < statuses[j].Name
		}
		return statuses[i].MachineID < statuses[j].MachineID
	})
	return statuses
}

func mapMachineState(state string) sprite.State {
	switch strings.ToLower(strings.TrimSpace(state)) {
	case "started", "running":
		return sprite.StateWorking
	case "stopped", "suspended", "idle":
		return sprite.StateIdle
	case "failed", "error", "dead":
		return sprite.StateDead
	case "blocked", "stuck":
		return sprite.StateBlocked
	case "done":
		return sprite.StateDone
	case "provisioned":
		return sprite.StateProvisioned
	default:
		return sprite.StateIdle
	}
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

func defaultFlyToken() string {
	if token := strings.TrimSpace(os.Getenv("FLY_API_TOKEN")); token != "" {
		return token
	}
	return strings.TrimSpace(os.Getenv("FLY_TOKEN"))
}

type composeRuntime struct {
	app        string
	client     fly.MachineClient
	machineIDs map[string]string
}

func newComposeRuntime(app string, client fly.MachineClient, actual []fleet.SpriteStatus) *composeRuntime {
	machineIDs := make(map[string]string, len(actual))
	for _, sprite := range actual {
		if sprite.MachineID == "" {
			continue
		}
		machineIDs[sprite.Name] = sprite.MachineID
	}
	return &composeRuntime{app: app, client: client, machineIDs: machineIDs}
}

func (r *composeRuntime) Provision(ctx context.Context, action fleet.ProvisionAction) error {
	if _, exists := r.machineIDs[action.Sprite.Name]; exists {
		return nil
	}

	machine, err := r.client.Create(ctx, fly.CreateRequest{
		App:  r.app,
		Name: action.Sprite.Name,
		Metadata: map[string]string{
			"persona":        action.Sprite.Persona.Name,
			"config_version": action.ConfigVersion,
		},
	})
	if err != nil {
		return err
	}
	r.machineIDs[action.Sprite.Name] = machine.ID
	return nil
}

func (r *composeRuntime) Teardown(ctx context.Context, action fleet.TeardownAction) error {
	machineID := action.MachineID
	if machineID == "" {
		machineID = r.machineIDs[action.Name]
	}
	if machineID == "" {
		return nil
	}

	if err := r.client.Destroy(ctx, r.app, machineID); err != nil && !isNotFound(err) {
		return err
	}
	delete(r.machineIDs, action.Name)
	return nil
}

func (r *composeRuntime) Update(ctx context.Context, action fleet.UpdateAction) error {
	if machineID := r.machineIDs[action.Desired.Name]; machineID != "" {
		if err := r.client.Destroy(ctx, r.app, machineID); err != nil && !isNotFound(err) {
			return err
		}
		delete(r.machineIDs, action.Desired.Name)
	}

	machine, err := r.client.Create(ctx, fly.CreateRequest{
		App:  r.app,
		Name: action.Desired.Name,
		Metadata: map[string]string{
			"persona":        action.Desired.Persona.Name,
			"config_version": action.DesiredConfig,
		},
	})
	if err != nil {
		return err
	}
	r.machineIDs[action.Desired.Name] = machine.ID
	return nil
}

func (r *composeRuntime) Redispatch(ctx context.Context, action fleet.RedispatchAction) error {
	machineID := r.machineIDs[action.Name]
	if machineID == "" {
		return nil
	}

	_, err := r.client.Exec(ctx, r.app, machineID, fly.ExecRequest{
		Command: []string{"/bin/sh", "-lc", "echo redispatch-requested"},
	})
	if isNotFound(err) {
		delete(r.machineIDs, action.Name)
		return nil
	}
	return err
}

func isNotFound(err error) bool {
	if err == nil {
		return false
	}
	var apiErr fly.APIError
	if !errors.As(err, &apiErr) {
		return false
	}
	return apiErr.StatusCode == http.StatusNotFound
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
