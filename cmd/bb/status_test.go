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

func TestSpriteStateLabel(t *testing.T) {
	tests := []struct {
		name     string
		item     lifecycle.SpriteStatus
		expected string
	}{
		{
			name:     "idle without probe shows unverified",
			item:     lifecycle.SpriteStatus{Name: "s1", State: lifecycle.StateIdle, Status: "warm"},
			expected: "🟢 idle ⚠ unverified",
		},
		{
			name:     "busy without probe shows unverified",
			item:     lifecycle.SpriteStatus{Name: "s1", State: lifecycle.StateBusy, Status: "running"},
			expected: "🔴 busy ⚠ unverified",
		},
		{
			name:     "probed and reachable",
			item:     lifecycle.SpriteStatus{Name: "s1", State: lifecycle.StateIdle, Status: "warm", Probed: true, Reachable: true},
			expected: "🟢 idle ✓ reachable",
		},
		{
			name:     "probed but unreachable",
			item:     lifecycle.SpriteStatus{Name: "s1", State: lifecycle.StateIdle, Status: "warm", Probed: true, Reachable: false},
			expected: "🟢 idle ✗ unreachable",
		},
		{
			name:     "offline doesn't show unverified",
			item:     lifecycle.SpriteStatus{Name: "s1", State: lifecycle.StateOffline, Status: "stopped"},
			expected: "⚫ offline",
		},
		{
			name:     "idle with stale and unverified",
			item:     lifecycle.SpriteStatus{Name: "s1", State: lifecycle.StateIdle, Status: "warm", Stale: true},
			expected: "🟢 idle ⚠ stale ⚠ unverified",
		},
		{
			name:     "probed and reachable with stale",
			item:     lifecycle.SpriteStatus{Name: "s1", State: lifecycle.StateIdle, Status: "warm", Probed: true, Reachable: true, Stale: true},
			expected: "🟢 idle ⚠ stale ✓ reachable",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := spriteStateLabel(tt.item)
			if result != tt.expected {
				t.Errorf("spriteStateLabel() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestIsRunningStatus(t *testing.T) {
	tests := []struct {
		state    string
		expected bool
	}{
		{"idle", true},
		{"busy", true},
		{"operational", true},
		{"IDLE", true},
		{"Busy", true},
		{"offline", false},
		{"stopped", false},
		{"unknown", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.state, func(t *testing.T) {
			result := isRunningStatus(tt.state)
			if result != tt.expected {
				t.Errorf("isRunningStatus(%q) = %v, want %v", tt.state, result, tt.expected)
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

func TestFormatSpriteStatus(t *testing.T) {
	tests := []struct {
		name        string
		status      string
		currentTask *lifecycle.TaskInfo
		want        string
	}{
		{
			name:        "running with task shows running",
			status:      "running",
			currentTask: &lifecycle.TaskInfo{Description: "Implement feature"},
			want:        "running",
		},
		{
			name:        "running without task shows warm",
			status:      "running",
			currentTask: nil,
			want:        "warm",
		},
		{
			name:        "warm status unchanged",
			status:      "warm",
			currentTask: nil,
			want:        "warm",
		},
		{
			name:        "stopped status unchanged",
			status:      "stopped",
			currentTask: nil,
			want:        "stopped",
		},
		{
			name:        "error status unchanged",
			status:      "error",
			currentTask: &lifecycle.TaskInfo{Description: "Failed task"},
			want:        "error",
		},
		{
			name:        "starting status unchanged even without task",
			status:      "starting",
			currentTask: nil,
			want:        "starting",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			item := lifecycle.SpriteStatus{
				Status:      tt.status,
				CurrentTask: tt.currentTask,
			}
			got := formatSpriteStatus(item)
			if got != tt.want {
				t.Errorf("formatSpriteStatus() = %q, want %q", got, tt.want)
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

func TestStatusCmdSpriteTimeout(t *testing.T) {
	t.Parallel()

	// Simulate a timeout by returning context.DeadlineExceeded
	deps := statusDeps{
		getwd:  func() (string, error) { return t.TempDir(), nil },
		newCLI: func(string, string) sprite.SpriteCLI { return &sprite.MockSpriteCLI{} },
		fleetOverview: func(context.Context, sprite.SpriteCLI, lifecycle.Config, string, lifecycle.FleetOverviewOpts) (lifecycle.FleetStatus, error) {
			t.Fatal("fleetOverview should not be called for sprite detail")
			return lifecycle.FleetStatus{}, nil
		},
		spriteDetail: func(ctx context.Context, cli sprite.SpriteCLI, cfg lifecycle.Config, name string) (lifecycle.SpriteDetailResult, error) {
			return lifecycle.SpriteDetailResult{}, context.DeadlineExceeded
		},
	}

	cmd := newStatusCmdWithDeps(deps)
	var out bytes.Buffer
	var errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs([]string{"--format", "text", "--sprite-timeout", "100ms", "fern"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for timeout")
	}
	if !strings.Contains(err.Error(), "unreachable") {
		t.Fatalf("expected error to contain 'unreachable', got: %v", err)
	}
	if !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("expected error to contain 'timed out', got: %v", err)
	}
	if !strings.Contains(err.Error(), "100ms") {
		t.Fatalf("expected error to contain timeout duration, got: %v", err)
	}
}

func TestStatusCmdSpriteTimeoutJSON(t *testing.T) {
	t.Parallel()

	// Simulate a timeout with JSON format
	deps := statusDeps{
		getwd:  func() (string, error) { return t.TempDir(), nil },
		newCLI: func(string, string) sprite.SpriteCLI { return &sprite.MockSpriteCLI{} },
		fleetOverview: func(context.Context, sprite.SpriteCLI, lifecycle.Config, string, lifecycle.FleetOverviewOpts) (lifecycle.FleetStatus, error) {
			t.Fatal("fleetOverview should not be called for sprite detail")
			return lifecycle.FleetStatus{}, nil
		},
		spriteDetail: func(ctx context.Context, cli sprite.SpriteCLI, cfg lifecycle.Config, name string) (lifecycle.SpriteDetailResult, error) {
			return lifecycle.SpriteDetailResult{}, context.DeadlineExceeded
		},
	}

	cmd := newStatusCmdWithDeps(deps)
	var out bytes.Buffer
	var errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs([]string{"--format", "json", "--sprite-timeout", "250ms", "thorn"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for timeout")
	}
	if !strings.Contains(err.Error(), "unreachable") {
		t.Fatalf("expected error to contain 'unreachable', got: %v", err)
	}
}
