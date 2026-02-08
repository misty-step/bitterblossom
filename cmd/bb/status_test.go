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
		fleetOverview: func(context.Context, sprite.SpriteCLI, lifecycle.Config, string) (lifecycle.FleetStatus, error) {
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
	cmd.SetOut(&out)
	cmd.SetErr(&out)
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
}

func TestStatusCmdSpriteText(t *testing.T) {
	t.Parallel()

	deps := statusDeps{
		getwd:  func() (string, error) { return t.TempDir(), nil },
		newCLI: func(string, string) sprite.SpriteCLI { return &sprite.MockSpriteCLI{} },
		fleetOverview: func(context.Context, sprite.SpriteCLI, lifecycle.Config, string) (lifecycle.FleetStatus, error) {
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
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"--format", "text", "bramble"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("cmd.Execute() error = %v", err)
	}
	text := out.String()
	if !strings.Contains(text, "=== Sprite: bramble ===") {
		t.Fatalf("unexpected text output: %q", text)
	}
}
