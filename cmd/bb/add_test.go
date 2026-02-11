package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/misty-step/bitterblossom/internal/lifecycle"
	"github.com/misty-step/bitterblossom/internal/registry"
	"github.com/misty-step/bitterblossom/internal/sprite"
)

func TestPickNewNames_Custom(t *testing.T) {
	t.Parallel()

	reg := &registry.Registry{Sprites: map[string]registry.SpriteEntry{}}
	names, err := pickNewNames(reg, "custom-sprite", 1)
	if err != nil {
		t.Fatalf("pickNewNames() error = %v", err)
	}
	if len(names) != 1 || names[0] != "custom-sprite" {
		t.Fatalf("names = %v, want [custom-sprite]", names)
	}
}

func TestPickNewNames_CustomDuplicate(t *testing.T) {
	t.Parallel()

	reg := &registry.Registry{Sprites: map[string]registry.SpriteEntry{
		"custom-sprite": {},
	}}
	_, err := pickNewNames(reg, "custom-sprite", 1)
	if err == nil {
		t.Fatal("expected error for duplicate name")
	}
}

func TestPickNewNames_AutoFromPool(t *testing.T) {
	t.Parallel()

	reg := &registry.Registry{Sprites: map[string]registry.SpriteEntry{
		"bramble": {},
		"fern":    {},
	}}
	names, err := pickNewNames(reg, "", 3)
	if err != nil {
		t.Fatalf("pickNewNames() error = %v", err)
	}
	if len(names) != 3 {
		t.Fatalf("got %d names, want 3", len(names))
	}
	if names[0] != "moss" || names[1] != "thistle" || names[2] != "ivy" {
		t.Fatalf("names = %v, want [moss thistle ivy]", names)
	}
}

func TestPickNewNames_SkipsExisting(t *testing.T) {
	t.Parallel()

	reg := &registry.Registry{Sprites: map[string]registry.SpriteEntry{
		"bramble": {},
		"moss":    {},
	}}
	names, err := pickNewNames(reg, "", 2)
	if err != nil {
		t.Fatalf("pickNewNames() error = %v", err)
	}
	if names[0] != "fern" || names[1] != "thistle" {
		t.Fatalf("names = %v, want [fern thistle]", names)
	}
}

func TestAddCmdValidation(t *testing.T) {
	t.Parallel()

	deps := addDeps{
		registryPath: func() string { return filepath.Join(t.TempDir(), "reg.toml") },
	}
	cmd := newAddCmdWithDeps(deps)
	cmd.SetArgs([]string{"--name", "foo", "--count", "3"})
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})

	if err := cmd.Execute(); err == nil {
		t.Fatal("expected error for --name with --count > 1")
	}
}

func TestAddCmdWithMockProvision(t *testing.T) {
	t.Parallel()

	regPath := filepath.Join(t.TempDir(), "registry.toml")
	reg := &registry.Registry{Sprites: map[string]registry.SpriteEntry{}}
	if err := reg.Save(regPath); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	provisioned := make([]string, 0)
	deps := addDeps{
		getwd: func() (string, error) { return t.TempDir(), nil },
		getenv: func(key string) string {
			if key == "OPENROUTER_API_KEY" {
				return "test-key"
			}
			return ""
		},
		newCLI: func(binary, org string) sprite.SpriteCLI { return nil },
		resolveGitHubAuth: func(name string, getenv func(string) string) (lifecycle.GitHubAuth, error) {
			return lifecycle.GitHubAuth{}, nil
		},
		renderSettings: func(settingsPath, authToken string) (string, error) {
			tmp := filepath.Join(t.TempDir(), "settings.json")
			if err := os.WriteFile(tmp, []byte("{}"), 0o644); err != nil {
				return "", err
			}
			return tmp, nil
		},
		provision: func(ctx context.Context, cli sprite.SpriteCLI, cfg lifecycle.Config, opts lifecycle.ProvisionOpts) (lifecycle.ProvisionResult, error) {
			provisioned = append(provisioned, opts.Name)
			return lifecycle.ProvisionResult{Name: opts.Name, MachineID: "m-" + opts.Name, Created: true}, nil
		},
		registryPath: func() string { return regPath },
	}

	cmd := newAddCmdWithDeps(deps)
	cmd.SetArgs([]string{"--count", "2"})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&bytes.Buffer{})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if len(provisioned) != 2 {
		t.Fatalf("provisioned %d sprites, want 2", len(provisioned))
	}
	if provisioned[0] != "bramble" || provisioned[1] != "fern" {
		t.Fatalf("provisioned = %v, want [bramble fern]", provisioned)
	}

	var envelope map[string]any
	if err := json.Unmarshal(out.Bytes(), &envelope); err != nil {
		t.Fatalf("json unmarshal error = %v", err)
	}
	if envelope["command"] != "add" {
		t.Fatalf("command = %v, want add", envelope["command"])
	}

	// Verify registry was updated with correct machine IDs
	updatedReg, err := registry.Load(regPath)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if updatedReg.Count() != 2 {
		t.Fatalf("registry count = %d, want 2", updatedReg.Count())
	}
	for _, name := range []string{"bramble", "fern"} {
		machineID, exists := updatedReg.LookupMachine(name)
		if !exists {
			t.Fatalf("sprite %q not found in registry", name)
		}
		wantID := "m-" + name
		if machineID != wantID {
			t.Fatalf("sprite %q machine_id = %q, want %q", name, machineID, wantID)
		}
	}
}
