package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/misty-step/bitterblossom/internal/fleet"
	"github.com/misty-step/bitterblossom/internal/lifecycle"
	"github.com/misty-step/bitterblossom/internal/registry"
	"github.com/misty-step/bitterblossom/internal/sprite"
)

func TestFleetCmdListText(t *testing.T) {
	t.Parallel()

	mockReg := &registry.Registry{
		Sprites: map[string]registry.SpriteEntry{
			"bramble": {MachineID: "machine-123", CreatedAt: time.Now().Add(-24 * time.Hour)},
			"willow":  {MachineID: "machine-456", CreatedAt: time.Now().Add(-48 * time.Hour)},
		},
	}

	deps := fleetDeps{
		getwd: func() (string, error) { return t.TempDir(), nil },
		newCLI: func(string, string) sprite.SpriteCLI {
			return &sprite.MockSpriteCLI{
				ListFn: func(ctx context.Context) ([]string, error) {
					return []string{"bramble", "fern"}, nil // fern is orphaned
				},
			}
		},
		loadRegistry: func(path string) (*registry.Registry, error) {
			return mockReg, nil
		},
	}

	cmd := newFleetCmdWithDeps(deps)
	var out bytes.Buffer
	var errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs([]string{"--format", "text"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("cmd.Execute() error = %v", err)
	}

	text := out.String()

	// Check for header
	if !strings.Contains(text, "Bitterblossom Fleet") {
		t.Fatalf("expected fleet header, got: %q", text)
	}

	// Check for registered sprites
	if !strings.Contains(text, "bramble") {
		t.Fatalf("expected bramble in output, got: %q", text)
	}
	if !strings.Contains(text, "willow") {
		t.Fatalf("expected willow in output, got: %q", text)
	}

	// Check for orphaned sprite
	if !strings.Contains(text, "fern") {
		t.Fatalf("expected orphaned fern in output, got: %q", text)
	}

	// Check summary
	if !strings.Contains(text, "2 registered") {
		t.Fatalf("expected 2 registered in summary, got: %q", text)
	}
}

func TestFleetCmdListJSON(t *testing.T) {
	t.Parallel()

	createdAt := time.Now().Add(-24 * time.Hour)
	mockReg := &registry.Registry{
		Sprites: map[string]registry.SpriteEntry{
			"bramble": {MachineID: "machine-123", CreatedAt: createdAt},
		},
	}

	deps := fleetDeps{
		getwd: func() (string, error) { return t.TempDir(), nil },
		newCLI: func(string, string) sprite.SpriteCLI {
			return &sprite.MockSpriteCLI{
				ListFn: func(ctx context.Context) ([]string, error) {
					return []string{"bramble"}, nil
				},
			}
		},
		loadRegistry: func(path string) (*registry.Registry, error) {
			return mockReg, nil
		},
	}

	cmd := newFleetCmdWithDeps(deps)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"--format", "json"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("cmd.Execute() error = %v", err)
	}

	var payload struct {
		Command string      `json:"command"`
		Data    FleetStatus `json:"data"`
	}
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}

	if payload.Command != "fleet" {
		t.Fatalf("command = %q, want fleet", payload.Command)
	}

	if len(payload.Data.Sprites) != 1 {
		t.Fatalf("len(Sprites) = %d, want 1", len(payload.Data.Sprites))
	}

	if payload.Data.Sprites[0].Name != "bramble" {
		t.Fatalf("sprite name = %q, want bramble", payload.Data.Sprites[0].Name)
	}

	if payload.Data.Summary.Total != 1 {
		t.Fatalf("summary.total = %d, want 1", payload.Data.Summary.Total)
	}
}

func TestFleetCmdEmptyRegistry(t *testing.T) {
	t.Parallel()

	mockReg := &registry.Registry{
		Sprites: map[string]registry.SpriteEntry{},
	}

	deps := fleetDeps{
		getwd: func() (string, error) { return t.TempDir(), nil },
		newCLI: func(string, string) sprite.SpriteCLI {
			return &sprite.MockSpriteCLI{
				ListFn: func(ctx context.Context) ([]string, error) {
					return []string{}, nil
				},
			}
		},
		loadRegistry: func(path string) (*registry.Registry, error) {
			return mockReg, nil
		},
	}

	cmd := newFleetCmdWithDeps(deps)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"--format", "text"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("cmd.Execute() error = %v", err)
	}

	text := out.String()
	if !strings.Contains(text, "No sprites registered") {
		t.Fatalf("expected 'No sprites registered', got: %q", text)
	}
}

func TestFleetCmdSyncDryRun(t *testing.T) {
	t.Parallel()

	mockReg := &registry.Registry{
		Sprites: map[string]registry.SpriteEntry{
			"bramble": {MachineID: "machine-123", CreatedAt: time.Now()},
			"willow":  {MachineID: "", CreatedAt: time.Now()}, // Missing in Fly.io
		},
	}

	deps := fleetDeps{
		getwd: func() (string, error) { return t.TempDir(), nil },
		newCLI: func(string, string) sprite.SpriteCLI {
			return &sprite.MockSpriteCLI{
				ListFn: func(ctx context.Context) ([]string, error) {
					return []string{"bramble"}, nil // willow is missing
				},
			}
		},
		loadRegistry: func(path string) (*registry.Registry, error) {
			return mockReg, nil
		},
		renderSettings: func(settingsPath, authToken string) (string, error) {
			return "/tmp/settings.json", nil
		},
		getenv: func(key string) string {
			if key == "OPENROUTER_API_KEY" {
				return "test-key"
			}
			return ""
		},
	}

	cmd := newFleetCmdWithDeps(deps)
	var out bytes.Buffer
	var errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs([]string{"--sync", "--dry-run", "--format", "json"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("cmd.Execute() error = %v", err)
	}

	var payload struct {
		Command string `json:"command"`
		Data    struct {
			Before FleetStatus `json:"before"`
			Sync   SyncResult  `json:"sync"`
		} `json:"data"`
	}
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}

	if payload.Command != "fleet.sync" {
		t.Fatalf("command = %q, want fleet.sync", payload.Command)
	}

	if !payload.Data.Sync.DryRun {
		t.Fatal("expected dry_run to be true")
	}

	// willow should be in the created list
	if len(payload.Data.Sync.Created) != 1 || payload.Data.Sync.Created[0] != "willow" {
		t.Fatalf("expected willow in created list, got: %v", payload.Data.Sync.Created)
	}
}

func TestFleetCmdSyncCreatesMissing(t *testing.T) {
	t.Parallel()

	mockReg := &registry.Registry{
		Sprites: map[string]registry.SpriteEntry{
			"willow": {MachineID: "", CreatedAt: time.Now()},
		},
	}

	provisionCalled := false
	deps := fleetDeps{
		getwd: func() (string, error) { return t.TempDir(), nil },
		newCLI: func(string, string) sprite.SpriteCLI {
			return &sprite.MockSpriteCLI{
				ListFn: func(ctx context.Context) ([]string, error) {
					return []string{}, nil // Empty - willow is missing
				},
			}
		},
		loadRegistry: func(path string) (*registry.Registry, error) {
			return mockReg, nil
		},
		provision: func(ctx context.Context, cli sprite.SpriteCLI, cfg lifecycle.Config, opts lifecycle.ProvisionOpts) (lifecycle.ProvisionResult, error) {
			provisionCalled = true
			if opts.Name != "willow" {
				t.Fatalf("expected provision for willow, got %q", opts.Name)
			}
			return lifecycle.ProvisionResult{Name: "willow", Created: true}, nil
		},
		resolveGitHubAuth: func(spriteName string, getenv func(string) string) (lifecycle.GitHubAuth, error) {
			return lifecycle.GitHubAuth{User: "test", Email: "test@test.com", Token: "token"}, nil
		},
		renderSettings: func(settingsPath, authToken string) (string, error) {
			return "/tmp/settings.json", nil
		},
		getenv: func(key string) string {
			if key == "OPENROUTER_API_KEY" {
				return "test-key"
			}
			return ""
		},
	}

	cmd := newFleetCmdWithDeps(deps)
	var out bytes.Buffer
	var errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs([]string{"--sync", "--format", "text"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("cmd.Execute() error = %v", err)
	}

	if !provisionCalled {
		t.Fatal("expected provision to be called")
	}

	text := out.String()
	if !strings.Contains(text, "Created (1)") {
		t.Fatalf("expected 'Created (1)' in output, got: %q", text)
	}
}

func TestFleetCmdSyncPrune(t *testing.T) {
	t.Parallel()

	mockReg := &registry.Registry{
		Sprites: map[string]registry.SpriteEntry{
			"bramble": {MachineID: "machine-123", CreatedAt: time.Now()},
		},
	}

	// Simulate user typing "yes"
	stdin := strings.NewReader("yes\n")

	teardownCalled := false
	deps := fleetDeps{
		getwd: func() (string, error) { return t.TempDir(), nil },
		newCLI: func(string, string) sprite.SpriteCLI {
			return &sprite.MockSpriteCLI{
				ListFn: func(ctx context.Context) ([]string, error) {
					return []string{"bramble", "orphan-sprite"}, nil
				},
			}
		},
		loadRegistry: func(path string) (*registry.Registry, error) {
			return mockReg, nil
		},
		teardown: func(ctx context.Context, cli sprite.SpriteCLI, cfg lifecycle.Config, opts lifecycle.TeardownOpts) (lifecycle.TeardownResult, error) {
			teardownCalled = true
			if opts.Name != "orphan-sprite" {
				t.Fatalf("expected teardown for orphan-sprite, got %q", opts.Name)
			}
			return lifecycle.TeardownResult{Name: "orphan-sprite"}, nil
		},
		renderSettings: func(settingsPath, authToken string) (string, error) {
			return "/tmp/settings.json", nil
		},
		getenv: func(key string) string {
			if key == "OPENROUTER_API_KEY" {
				return "test-key"
			}
			return ""
		},
	}

	cmd := newFleetCmdWithDeps(deps)
	var out bytes.Buffer
	var errOut bytes.Buffer
	cmd.SetIn(stdin)
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs([]string{"--sync", "--prune", "--format", "text"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("cmd.Execute() error = %v", err)
	}

	if !teardownCalled {
		t.Fatal("expected teardown to be called")
	}

	text := out.String()
	if !strings.Contains(text, "Destroyed (1)") {
		t.Fatalf("expected 'Destroyed (1)' in output, got: %q", text)
	}
}

func TestFleetCmdInvalidFormat(t *testing.T) {
	t.Parallel()

	deps := fleetDeps{
		getwd:       func() (string, error) { return t.TempDir(), nil },
		loadRegistry: func(path string) (*registry.Registry, error) {
			return &registry.Registry{Sprites: map[string]registry.SpriteEntry{}}, nil
		},
	}

	cmd := newFleetCmdWithDeps(deps)
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

func TestFleetCmdRegistryLoadError(t *testing.T) {
	t.Parallel()

	deps := fleetDeps{
		getwd: func() (string, error) { return t.TempDir(), nil },
		loadRegistry: func(path string) (*registry.Registry, error) {
			return nil, errors.New("registry not found")
		},
	}

	cmd := newFleetCmdWithDeps(deps)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for registry load failure")
	}
	if !strings.Contains(err.Error(), "loading registry") {
		t.Fatalf("unexpected error message: %v", err)
	}
}

func TestFleetCmdListError(t *testing.T) {
	t.Parallel()

	mockReg := &registry.Registry{
		Sprites: map[string]registry.SpriteEntry{},
	}

	deps := fleetDeps{
		getwd: func() (string, error) { return t.TempDir(), nil },
		newCLI: func(string, string) sprite.SpriteCLI {
			return &sprite.MockSpriteCLI{
				ListFn: func(ctx context.Context) ([]string, error) {
					return nil, errors.New("fly.io connection failed")
				},
			}
		},
		loadRegistry: func(path string) (*registry.Registry, error) {
			return mockReg, nil
		},
	}

	cmd := newFleetCmdWithDeps(deps)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for list failure")
	}
	if !strings.Contains(err.Error(), "listing sprites") {
		t.Fatalf("unexpected error message: %v", err)
	}
}

func TestStatusWithEmoji(t *testing.T) {
	tests := []struct {
		status   string
		expected string
	}{
		{"running", "🟢 running"},
		{"stopped", "🔴 stopped"},
		{"not found", "⚪ not found"},
		{"orphaned", "🟠 orphaned"},
		{"unknown", "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.status, func(t *testing.T) {
			result := statusWithEmoji(tt.status)
			if result != tt.expected {
				t.Errorf("statusWithEmoji(%q) = %q, want %q", tt.status, result, tt.expected)
			}
		})
	}
}

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		d      time.Duration
		want   string
	}{
		{30 * time.Second, "30s"},
		{5 * time.Minute, "5m"},
		{2 * time.Hour, "2h"},
		{48 * time.Hour, "2d"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := formatDuration(tt.d)
			if got != tt.want {
				t.Errorf("formatDuration(%v) = %q, want %q", tt.d, got, tt.want)
			}
		})
	}
}

func TestConfirmDestruction(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{"yes", "yes\n", true},
		{"YES", "YES\n", false}, // case sensitive
		{"no", "no\n", false},
		{"empty", "\n", false},
		{"other", "destroy\n", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stdin := strings.NewReader(tt.input)
			var stdout bytes.Buffer
			result := confirmDestruction(stdin, &stdout, []string{"sprite1", "sprite2"})
			if result != tt.expected {
				t.Errorf("confirmDestruction() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestBuildFleetStatus(t *testing.T) {
	reg := &registry.Registry{
		Sprites: map[string]registry.SpriteEntry{
			"bramble": {MachineID: "m1", CreatedAt: time.Now()},
			"willow":  {MachineID: "m2", CreatedAt: time.Now()},
		},
	}

	// bramble exists, willow is missing, fern is orphaned
	actual := []string{"bramble", "fern"}

	status := buildFleetStatus(reg, actual)

	if status.Summary.Total != 2 {
		t.Errorf("Total = %d, want 2", status.Summary.Total)
	}
	if status.Summary.Running != 1 {
		t.Errorf("Running = %d, want 1", status.Summary.Running)
	}
	if status.Summary.NotFound != 1 {
		t.Errorf("NotFound = %d, want 1", status.Summary.NotFound)
	}
	if status.Summary.Orphaned != 1 {
		t.Errorf("Orphaned = %d, want 1", status.Summary.Orphaned)
	}

	// Check sprites are sorted
	if len(status.Sprites) != 3 {
		t.Errorf("len(Sprites) = %d, want 3", len(status.Sprites))
	}
}

func TestFleetCmdSyncWithComposition(t *testing.T) {
	t.Parallel()

	mockReg := &registry.Registry{
		Sprites: map[string]registry.SpriteEntry{
			"bramble": {MachineID: "", CreatedAt: time.Now()},
		},
	}

	mockComp := fleet.Composition{
		Name:    "test",
		Version: 1,
		Sprites: []fleet.SpriteSpec{
			{Name: "bramble", Definition: "./bramble.md"},
		},
	}

	provisionCalled := false
	deps := fleetDeps{
		getwd: func() (string, error) { return t.TempDir(), nil },
		newCLI: func(string, string) sprite.SpriteCLI {
			return &sprite.MockSpriteCLI{
				ListFn: func(ctx context.Context) ([]string, error) {
					return []string{}, nil
				},
			}
		},
		loadRegistry: func(path string) (*registry.Registry, error) {
			return mockReg, nil
		},
		parseComposition: func(path string) (fleet.Composition, error) {
			return mockComp, nil
		},
		provision: func(ctx context.Context, cli sprite.SpriteCLI, cfg lifecycle.Config, opts lifecycle.ProvisionOpts) (lifecycle.ProvisionResult, error) {
			provisionCalled = true
			return lifecycle.ProvisionResult{Name: "bramble", Created: true}, nil
		},
		resolveGitHubAuth: func(spriteName string, getenv func(string) string) (lifecycle.GitHubAuth, error) {
			return lifecycle.GitHubAuth{User: "test", Email: "test@test.com", Token: "token"}, nil
		},
		renderSettings: func(settingsPath, authToken string) (string, error) {
			return "/tmp/settings.json", nil
		},
		getenv: func(key string) string {
			if key == "OPENROUTER_API_KEY" {
				return "test-key"
			}
			return ""
		},
	}

	cmd := newFleetCmdWithDeps(deps)
	var out bytes.Buffer
	var errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs([]string{"--sync", "--composition", "test.yaml", "--format", "text"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("cmd.Execute() error = %v", err)
	}

	if !provisionCalled {
		t.Fatal("expected provision to be called")
	}
}

func TestFleetCmdSyncWithProvisionError(t *testing.T) {
	t.Parallel()

	mockReg := &registry.Registry{
		Sprites: map[string]registry.SpriteEntry{
			"bramble": {MachineID: "", CreatedAt: time.Now()},
		},
	}

	deps := fleetDeps{
		getwd: func() (string, error) { return t.TempDir(), nil },
		newCLI: func(string, string) sprite.SpriteCLI {
			return &sprite.MockSpriteCLI{
				ListFn: func(ctx context.Context) ([]string, error) {
					return []string{}, nil
				},
			}
		},
		loadRegistry: func(path string) (*registry.Registry, error) {
			return mockReg, nil
		},
		provision: func(ctx context.Context, cli sprite.SpriteCLI, cfg lifecycle.Config, opts lifecycle.ProvisionOpts) (lifecycle.ProvisionResult, error) {
			return lifecycle.ProvisionResult{}, errors.New("provision failed: network error")
		},
		resolveGitHubAuth: func(spriteName string, getenv func(string) string) (lifecycle.GitHubAuth, error) {
			return lifecycle.GitHubAuth{User: "test", Email: "test@test.com", Token: "token"}, nil
		},
		renderSettings: func(settingsPath, authToken string) (string, error) {
			return "/tmp/settings.json", nil
		},
		getenv: func(key string) string {
			if key == "OPENROUTER_API_KEY" {
				return "test-key"
			}
			return ""
		},
	}

	cmd := newFleetCmdWithDeps(deps)
	var out bytes.Buffer
	var errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs([]string{"--sync", "--format", "text"})

	// Should complete without error but report errors in output
	if err := cmd.Execute(); err != nil {
		t.Fatalf("cmd.Execute() error = %v", err)
	}

	output := out.String()
	if !strings.Contains(output, "Errors") {
		t.Fatalf("expected 'Errors' in output, got: %q", output)
	}
}
