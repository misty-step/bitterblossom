package main

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/misty-step/bitterblossom/internal/lifecycle"
	"github.com/misty-step/bitterblossom/internal/sprite"
)

func TestStatusCmdFleetJSONDefault(t *testing.T) {
	t.Parallel()

	deps := statusDeps{
		getwd:  func() (string, error) { return t.TempDir(), nil },
		newCLI: func(string, string) sprite.SpriteCLI { return &sprite.MockSpriteCLI{} },
		fleetOverview: func(context.Context, sprite.SpriteCLI, lifecycle.Config, string, lifecycle.FleetOverviewOpts) (lifecycle.FleetStatus, error) {
			return lifecycle.FleetStatus{
				Sprites: []lifecycle.SpriteStatus{
					{Name: "bramble", Status: "running", State: lifecycle.StateIdle},
				},
				Summary: lifecycle.FleetSummary{Total: 1, Idle: 1},
			}, nil
		},
		spriteDetail: func(context.Context, sprite.SpriteCLI, lifecycle.Config, string) (lifecycle.SpriteDetailResult, error) {
			t.Fatal("spriteDetail should not be called for fleet mode")
			return lifecycle.SpriteDetailResult{}, nil
		},
	}

	cmd := newStatusCmdWithDeps(deps)
	var out bytes.Buffer
	var errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs([]string{"--format", "json"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("cmd.Execute() error = %v", err)
	}

	var payload struct {
		Command string                `json:"command"`
		Data    lifecycle.FleetStatus `json:"data"`
	}
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if payload.Command != "status.fleet" {
		t.Fatalf("command = %q, want status.fleet", payload.Command)
	}
	if len(payload.Data.Sprites) != 1 {
		t.Fatalf("len(Sprites) = %d, want 1", len(payload.Data.Sprites))
	}
	if payload.Data.Summary.Total != 1 {
		t.Fatalf("summary.total = %d, want 1", payload.Data.Summary.Total)
	}
}

func TestStatusCmdFleetText(t *testing.T) {
	t.Parallel()

	deps := statusDeps{
		getwd:  func() (string, error) { return t.TempDir(), nil },
		newCLI: func(string, string) sprite.SpriteCLI { return &sprite.MockSpriteCLI{} },
		fleetOverview: func(context.Context, sprite.SpriteCLI, lifecycle.Config, string, lifecycle.FleetOverviewOpts) (lifecycle.FleetStatus, error) {
			return lifecycle.FleetStatus{
				Sprites: []lifecycle.SpriteStatus{
					{Name: "bramble", Status: "running", State: lifecycle.StateIdle, Uptime: "2h30m"},
					{Name: "thorn", Status: "running", State: lifecycle.StateBusy, CurrentTask: &lifecycle.TaskInfo{Description: "Implement feature X"}},
				},
				Composition: []lifecycle.CompositionEntry{
					{Name: "bramble", Provisioned: true},
					{Name: "thorn", Provisioned: true},
				},
				Summary: lifecycle.FleetSummary{Total: 2, Idle: 1, Busy: 1},
			}, nil
		},
		spriteDetail: func(context.Context, sprite.SpriteCLI, lifecycle.Config, string) (lifecycle.SpriteDetailResult, error) {
			t.Fatal("spriteDetail should not be called for fleet mode")
			return lifecycle.SpriteDetailResult{}, nil
		},
	}

	cmd := newStatusCmdWithDeps(deps)
	var out bytes.Buffer
	var errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs([]string{"--format", "text"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("cmd.Execute() error = %v", err)
	}
	text := out.String()

	// Check for fleet summary
	if !strings.Contains(text, "Fleet Summary: 2 total") {
		t.Fatalf("expected fleet summary, got: %q", text)
	}

	// Check for sprite states
	if !strings.Contains(text, "bramble") {
		t.Fatalf("expected bramble in output, got: %q", text)
	}
	if !strings.Contains(text, "thorn") {
		t.Fatalf("expected thorn in output, got: %q", text)
	}

	// Check for composition entries
	if !strings.Contains(text, "Composition sprites") {
		t.Fatalf("expected composition header, got: %q", text)
	}
}

func TestStatusCmdSpriteJSON(t *testing.T) {
	t.Parallel()

	startedAt := time.Date(2026, 2, 10, 14, 0, 0, 0, time.UTC)
	deps := statusDeps{
		getwd:  func() (string, error) { return t.TempDir(), nil },
		newCLI: func(string, string) sprite.SpriteCLI { return &sprite.MockSpriteCLI{} },
		fleetOverview: func(context.Context, sprite.SpriteCLI, lifecycle.Config, string, lifecycle.FleetOverviewOpts) (lifecycle.FleetStatus, error) {
			t.Fatal("fleetOverview should not be called for sprite detail")
			return lifecycle.FleetStatus{}, nil
		},
		spriteDetail: func(context.Context, sprite.SpriteCLI, lifecycle.Config, string) (lifecycle.SpriteDetailResult, error) {
			return lifecycle.SpriteDetailResult{
				Name:      "bramble",
				State:     lifecycle.StateBusy,
				Workspace: "workspace",
				Memory:    "memory",
				Checkpoints: "cp-1",
				CurrentTask: &lifecycle.TaskInfo{
					ID:          "task-123",
					Description: "Implement feature",
					StartedAt:   &startedAt,
				},
				Uptime: "3h45m",
			}, nil
		},
	}

	cmd := newStatusCmdWithDeps(deps)
	var out bytes.Buffer
	var errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs([]string{"--format", "json", "bramble"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("cmd.Execute() error = %v", err)
	}

	var payload struct {
		Command string                      `json:"command"`
		Data    lifecycle.SpriteDetailResult `json:"data"`
	}
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if payload.Command != "status.sprite" {
		t.Fatalf("command = %q, want status.sprite", payload.Command)
	}
	if payload.Data.Name != "bramble" {
		t.Fatalf("name = %q, want bramble", payload.Data.Name)
	}
	if payload.Data.CurrentTask == nil {
		t.Fatalf("expected current task, got nil")
	}
	if payload.Data.CurrentTask.ID != "task-123" {
		t.Fatalf("task ID = %q, want task-123", payload.Data.CurrentTask.ID)
	}
}

func TestStatusCmdSpriteText(t *testing.T) {
	t.Parallel()

	deps := statusDeps{
		getwd:  func() (string, error) { return t.TempDir(), nil },
		newCLI: func(string, string) sprite.SpriteCLI { return &sprite.MockSpriteCLI{} },
		fleetOverview: func(context.Context, sprite.SpriteCLI, lifecycle.Config, string, lifecycle.FleetOverviewOpts) (lifecycle.FleetStatus, error) {
			t.Fatal("fleetOverview should not be called for sprite detail")
			return lifecycle.FleetStatus{}, nil
		},
		spriteDetail: func(context.Context, sprite.SpriteCLI, lifecycle.Config, string) (lifecycle.SpriteDetailResult, error) {
			return lifecycle.SpriteDetailResult{
				Name:        "bramble",
				State:       lifecycle.StateIdle,
				Workspace:   "workspace",
				Memory:      "memory",
				Checkpoints: "cp-1",
			}, nil
		},
	}

	cmd := newStatusCmdWithDeps(deps)
	var out bytes.Buffer
	var errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs([]string{"--format", "text", "bramble"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("cmd.Execute() error = %v", err)
	}
	text := out.String()
	if !strings.Contains(text, "=== Sprite: bramble ===") {
		t.Fatalf("unexpected text output: %q", text)
	}
	if !strings.Contains(text, "State:") {
		t.Fatalf("expected state in output: %q", text)
	}
}

func TestStatusCmdInvalidFormat(t *testing.T) {
	t.Parallel()

	deps := statusDeps{
		getwd:       func() (string, error) { return t.TempDir(), nil },
		newCLI:      func(string, string) sprite.SpriteCLI { return &sprite.MockSpriteCLI{} },
		fleetOverview: func(context.Context, sprite.SpriteCLI, lifecycle.Config, string, lifecycle.FleetOverviewOpts) (lifecycle.FleetStatus, error) {
			return lifecycle.FleetStatus{}, nil
		},
		spriteDetail: func(context.Context, sprite.SpriteCLI, lifecycle.Config, string) (lifecycle.SpriteDetailResult, error) {
			return lifecycle.SpriteDetailResult{}, nil
		},
	}

	cmd := newStatusCmdWithDeps(deps)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"--format", "xml"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for invalid format")
	}
	if !strings.Contains(err.Error(), "--format must be json or text") {
		t.Fatalf("unexpected error message: %v", err)
	}
}

func TestStatusCmdTooManyArgs(t *testing.T) {
	t.Parallel()

	deps := statusDeps{
		getwd:  func() (string, error) { return t.TempDir(), nil },
		newCLI: func(string, string) sprite.SpriteCLI { return &sprite.MockSpriteCLI{} },
		fleetOverview: func(context.Context, sprite.SpriteCLI, lifecycle.Config, string, lifecycle.FleetOverviewOpts) (lifecycle.FleetStatus, error) {
			return lifecycle.FleetStatus{}, nil
		},
		spriteDetail: func(context.Context, sprite.SpriteCLI, lifecycle.Config, string) (lifecycle.SpriteDetailResult, error) {
			return lifecycle.SpriteDetailResult{}, nil
		},
	}

	cmd := newStatusCmdWithDeps(deps)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"sprite1", "sprite2"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for too many args")
	}
	if !strings.Contains(err.Error(), "only one sprite name can be provided") {
		t.Fatalf("unexpected error message: %v", err)
	}
}

func TestStateWithEmoji(t *testing.T) {
	tests := []struct {
		state    lifecycle.SpriteState
		expected string
	}{
		{lifecycle.StateIdle, "🟢 idle"},
		{lifecycle.StateBusy, "🔴 busy"},
		{lifecycle.StateOffline, "⚫ offline"},
		{lifecycle.StateOperational, "🟢 operational"},
		{lifecycle.StateUnknown, "⚪ unknown"},
		{"custom", "custom"},
	}

	for _, tt := range tests {
		t.Run(string(tt.state), func(t *testing.T) {
			result := stateWithEmoji(tt.state)
			if result != tt.expected {
				t.Errorf("stateWithEmoji(%q) = %q, want %q", tt.state, result, tt.expected)
			}
		})
	}
}

func TestTruncateString(t *testing.T) {
	tests := []struct {
		input  string
		maxLen int
		want   string
	}{
		{"hello", 10, "hello"},
		{"hello world", 8, "hello..."},
		{"hello", 3, "hel"},
		{"", 5, ""},
		{"ab", 5, "ab"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := truncateString(tt.input, tt.maxLen)
			if got != tt.want {
				t.Errorf("truncateString(%q, %d) = %q, want %q", tt.input, tt.maxLen, got, tt.want)
			}
		})
	}
}

func TestWatchModeInvalidInterval(t *testing.T) {
	t.Parallel()

	deps := statusDeps{
		getwd:  func() (string, error) { return t.TempDir(), nil },
		newCLI: func(string, string) sprite.SpriteCLI { return &sprite.MockSpriteCLI{} },
		fleetOverview: func(context.Context, sprite.SpriteCLI, lifecycle.Config, string, lifecycle.FleetOverviewOpts) (lifecycle.FleetStatus, error) {
			return lifecycle.FleetStatus{}, nil
		},
		spriteDetail: func(context.Context, sprite.SpriteCLI, lifecycle.Config, string) (lifecycle.SpriteDetailResult, error) {
			return lifecycle.SpriteDetailResult{}, nil
		},
	}

	cmd := newStatusCmdWithDeps(deps)
	var out bytes.Buffer
	var errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs([]string{"--watch", "--watch-interval", "0s"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for invalid watch interval")
	}
	if !strings.Contains(err.Error(), "--watch-interval must be positive") {
		t.Fatalf("unexpected error message: %v", err)
	}
}

func TestWatchModeNegativeInterval(t *testing.T) {
	t.Parallel()

	deps := statusDeps{
		getwd:  func() (string, error) { return t.TempDir(), nil },
		newCLI: func(string, string) sprite.SpriteCLI { return &sprite.MockSpriteCLI{} },
		fleetOverview: func(context.Context, sprite.SpriteCLI, lifecycle.Config, string, lifecycle.FleetOverviewOpts) (lifecycle.FleetStatus, error) {
			return lifecycle.FleetStatus{}, nil
		},
		spriteDetail: func(context.Context, sprite.SpriteCLI, lifecycle.Config, string) (lifecycle.SpriteDetailResult, error) {
			return lifecycle.SpriteDetailResult{}, nil
		},
	}

	cmd := newStatusCmdWithDeps(deps)
	var out bytes.Buffer
	var errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs([]string{"--watch", "--watch-interval", "-1s"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for negative watch interval")
	}
	if !strings.Contains(err.Error(), "--watch-interval must be positive") {
		t.Fatalf("unexpected error message: %v", err)
	}
}
