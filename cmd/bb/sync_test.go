package main

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/misty-step/bitterblossom/internal/lifecycle"
	"github.com/misty-step/bitterblossom/internal/sprite"
)

func TestSyncCmdWiring(t *testing.T) {
	t.Parallel()

	rootDir := t.TempDir()
	rendered := filepath.Join(t.TempDir(), "rendered-settings.json")
	calls := make([]lifecycle.SyncOpts, 0, 2)

	deps := syncDeps{
		getwd: func() (string, error) { return rootDir, nil },
		getenv: func(key string) string {
			if key == "OPENROUTER_API_KEY" {
				return "token"
			}
			return ""
		},
		newCLI: func(string, string) sprite.SpriteCLI { return &sprite.MockSpriteCLI{} },
		resolveComposition: func(string) ([]string, error) {
			t.Fatal("resolveComposition should not be called for explicit target")
			return nil, nil
		},
		renderSettings: func(string, string) (string, error) { return rendered, nil },
		sync: func(_ context.Context, _ sprite.SpriteCLI, _ lifecycle.Config, opts lifecycle.SyncOpts) error {
			calls = append(calls, opts)
			return nil
		},
	}

	cmd := newSyncCmdWithDeps(deps)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"--base-only", "willow"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("cmd.Execute() error = %v", err)
	}
	if len(calls) != 1 {
		t.Fatalf("sync calls = %d, want 1", len(calls))
	}
	if calls[0].Name != "willow" || !calls[0].BaseOnly {
		t.Fatalf("unexpected call opts: %+v", calls[0])
	}

	var payload struct {
		Command string `json:"command"`
	}
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if payload.Command != "sync" {
		t.Fatalf("command = %q, want sync", payload.Command)
	}
}

func TestSyncCmdNoArgsUsesComposition(t *testing.T) {
	t.Parallel()

	rootDir := t.TempDir()
	rendered := filepath.Join(t.TempDir(), "rendered-settings.json")
	callNames := make([]string, 0, 2)

	deps := syncDeps{
		getwd: func() (string, error) { return rootDir, nil },
		getenv: func(key string) string {
			if key == "OPENROUTER_API_KEY" {
				return "token"
			}
			return ""
		},
		newCLI:             func(string, string) sprite.SpriteCLI { return &sprite.MockSpriteCLI{} },
		resolveComposition: func(string) ([]string, error) { return []string{"bramble", "thorn"}, nil },
		renderSettings:     func(string, string) (string, error) { return rendered, nil },
		sync: func(_ context.Context, _ sprite.SpriteCLI, _ lifecycle.Config, opts lifecycle.SyncOpts) error {
			callNames = append(callNames, opts.Name)
			return nil
		},
	}

	cmd := newSyncCmdWithDeps(deps)
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs(nil)

	if err := cmd.Execute(); err != nil {
		t.Fatalf("cmd.Execute() error = %v", err)
	}
	if len(callNames) != 2 {
		t.Fatalf("sync call count = %d, want 2", len(callNames))
	}
}

func TestSyncCmdFailsWithoutCanonicalAuth(t *testing.T) {
	t.Parallel()

	rootDir := t.TempDir()

	deps := syncDeps{
		getwd:              func() (string, error) { return rootDir, nil },
		getenv:             func(string) string { return "" },
		newCLI:             func(string, string) sprite.SpriteCLI { return &sprite.MockSpriteCLI{} },
		resolveComposition: func(string) ([]string, error) { return []string{"willow"}, nil },
		renderSettings: func(string, string) (string, error) {
			t.Fatal("renderSettings should not be called when auth is missing")
			return "", nil
		},
		sync: func(_ context.Context, _ sprite.SpriteCLI, _ lifecycle.Config, _ lifecycle.SyncOpts) error {
			t.Fatal("sync should not be called when auth is missing")
			return nil
		},
	}

	cmd := newSyncCmdWithDeps(deps)
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"willow"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for missing canonical auth")
	}
	if !strings.Contains(err.Error(), "OPENROUTER_API_KEY") {
		t.Fatalf("expected OPENROUTER_API_KEY guidance, got: %v", err)
	}
}
