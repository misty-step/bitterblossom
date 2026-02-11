package main

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/misty-step/bitterblossom/internal/lifecycle"
	"github.com/misty-step/bitterblossom/internal/registry"
	"github.com/misty-step/bitterblossom/internal/sprite"
)

func TestRemoveCmdNotInRegistry(t *testing.T) {
	t.Parallel()

	regPath := filepath.Join(t.TempDir(), "registry.toml")
	reg := &registry.Registry{Sprites: map[string]registry.SpriteEntry{}}
	if err := reg.Save(regPath); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	deps := removeDeps{
		registryPath: func() string { return regPath },
	}
	cmd := newRemoveCmdWithDeps(deps)
	cmd.SetArgs([]string{"nonexistent"})
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for sprite not in registry")
	}
	if !strings.Contains(err.Error(), "not found in registry") {
		t.Fatalf("error = %v, want 'not found in registry'", err)
	}
}

func TestRemoveCmdBusySprite(t *testing.T) {
	t.Parallel()

	regPath := filepath.Join(t.TempDir(), "registry.toml")
	reg := &registry.Registry{Sprites: map[string]registry.SpriteEntry{
		"fern": {MachineID: "m-123"},
	}}
	if err := reg.Save(regPath); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	deps := removeDeps{
		getwd:  func() (string, error) { return t.TempDir(), nil },
		newCLI: func(binary, org string) sprite.SpriteCLI { return nil },
		isBusy: func(ctx context.Context, cli sprite.SpriteCLI, name string) (bool, error) {
			return true, nil
		},
		registryPath: func() string { return regPath },
	}
	cmd := newRemoveCmdWithDeps(deps)
	cmd.SetArgs([]string{"fern"})
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for busy sprite")
	}
	if !strings.Contains(err.Error(), "is busy") {
		t.Fatalf("error = %v, want 'is busy'", err)
	}
}

func TestRemoveCmdForce(t *testing.T) {
	t.Parallel()

	regPath := filepath.Join(t.TempDir(), "registry.toml")
	reg := &registry.Registry{Sprites: map[string]registry.SpriteEntry{
		"fern": {MachineID: "m-123"},
	}}
	if err := reg.Save(regPath); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	tornDown := false
	deps := removeDeps{
		getwd:  func() (string, error) { return t.TempDir(), nil },
		newCLI: func(binary, org string) sprite.SpriteCLI { return nil },
		teardown: func(ctx context.Context, cli sprite.SpriteCLI, cfg lifecycle.Config, opts lifecycle.TeardownOpts) (lifecycle.TeardownResult, error) {
			tornDown = true
			return lifecycle.TeardownResult{Name: opts.Name}, nil
		},
		isBusy: func(ctx context.Context, cli sprite.SpriteCLI, name string) (bool, error) {
			return true, nil
		},
		registryPath: func() string { return regPath },
	}
	cmd := newRemoveCmdWithDeps(deps)
	cmd.SetArgs([]string{"fern", "--force"})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&bytes.Buffer{})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !tornDown {
		t.Fatal("expected teardown to be called")
	}

	var envelope map[string]any
	if err := json.Unmarshal(out.Bytes(), &envelope); err != nil {
		t.Fatalf("json unmarshal error = %v", err)
	}
	if envelope["command"] != "remove" {
		t.Fatalf("command = %v, want remove", envelope["command"])
	}
	data, ok := envelope["data"].(map[string]any)
	if !ok {
		t.Fatalf("data not a map: %v", envelope["data"])
	}
	if data["removed"] != true {
		t.Fatalf("removed = %v, want true", data["removed"])
	}

	// Verify registry updated
	updatedReg, err := registry.Load(regPath)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if _, exists := updatedReg.LookupMachine("fern"); exists {
		t.Fatal("fern should be removed from registry")
	}
}

func TestRemoveCmdRequiresArg(t *testing.T) {
	t.Parallel()

	deps := removeDeps{
		registryPath: func() string { return filepath.Join(t.TempDir(), "reg.toml") },
	}
	cmd := newRemoveCmdWithDeps(deps)
	cmd.SetArgs([]string{})
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})

	if err := cmd.Execute(); err == nil {
		t.Fatal("expected error for missing arg")
	}
}
