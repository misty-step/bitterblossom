package main

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

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
				Sprites: []lifecycle.SpriteStatus{{Name: "bramble", Status: "running"}},
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
	cmd.SetArgs(nil)

	if err := cmd.Execute(); err != nil {
		t.Fatalf("cmd.Execute() error = %v", err)
	}

	var payload struct {
		Command string `json:"command"`
	}
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if payload.Command != "status.fleet" {
		t.Fatalf("command = %q, want status.fleet", payload.Command)
	}
	if !strings.Contains(errOut.String(), "status: fetching fleet overview") {
		t.Fatalf("expected status progress output, got %q", errOut.String())
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
	if !strings.Contains(errOut.String(), "status: fetching detail for bramble") {
		t.Fatalf("expected detail progress output, got %q", errOut.String())
	}
}

func TestStatusCmdFleetTextWithTask(t *testing.T) {
	t.Parallel()

	deps := statusDeps{
		getwd:  func() (string, error) { return t.TempDir(), nil },
		newCLI: func(string, string) sprite.SpriteCLI { return &sprite.MockSpriteCLI{} },
		fleetOverview: func(context.Context, sprite.SpriteCLI, lifecycle.Config, string, lifecycle.FleetOverviewOpts) (lifecycle.FleetStatus, error) {
			return lifecycle.FleetStatus{
				Sprites: []lifecycle.SpriteStatus{
					{Name: "willow", Status: "working", Task: "Fix memory leak"},
					{Name: "oak", Status: "idle", Task: ""},
				},
				Composition:         []lifecycle.CompositionEntry{{Name: "willow", Provisioned: true}},
				Orphans:             []lifecycle.SpriteStatus{},
				CheckpointsIncluded: false,
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
	if !strings.Contains(text, "SPRITE") || !strings.Contains(text, "STATUS") || !strings.Contains(text, "TASK") {
		t.Fatalf("expected table header with TASK column, got: %q", text)
	}
	if !strings.Contains(text, "Fix memory leak") {
		t.Fatalf("expected task description in output, got: %q", text)
	}
	if !strings.Contains(text, "(none)") {
		t.Fatalf("expected '(none)' for empty task, got: %q", text)
	}
}

func TestStatusCmdFleetJSONWithTask(t *testing.T) {
	t.Parallel()

	deps := statusDeps{
		getwd:  func() (string, error) { return t.TempDir(), nil },
		newCLI: func(string, string) sprite.SpriteCLI { return &sprite.MockSpriteCLI{} },
		fleetOverview: func(context.Context, sprite.SpriteCLI, lifecycle.Config, string, lifecycle.FleetOverviewOpts) (lifecycle.FleetStatus, error) {
			return lifecycle.FleetStatus{
				Sprites: []lifecycle.SpriteStatus{
					{Name: "pine", Status: "working", Task: "Implement dark mode"},
				},
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
	cmd.SetArgs(nil)

	if err := cmd.Execute(); err != nil {
		t.Fatalf("cmd.Execute() error = %v", err)
	}

	var payload struct {
		Data struct {
			Sprites []lifecycle.SpriteStatus `json:"sprites"`
		} `json:"data"`
	}
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if len(payload.Data.Sprites) != 1 {
		t.Fatalf("sprites count = %d, want 1", len(payload.Data.Sprites))
	}
	if payload.Data.Sprites[0].Task != "Implement dark mode" {
		t.Fatalf("sprite.Task = %q, want 'Implement dark mode'", payload.Data.Sprites[0].Task)
	}
}
