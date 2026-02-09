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

func TestProvisionCmdWiringSingleSprite(t *testing.T) {
	t.Parallel()

	rootDir := t.TempDir()
	rendered := filepath.Join(t.TempDir(), "rendered-settings.json")
	calls := make([]lifecycle.ProvisionOpts, 0, 2)

	deps := provisionDeps{
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
		resolveGitHubAuth: func(string, func(string) string) (lifecycle.GitHubAuth, error) {
			return lifecycle.GitHubAuth{User: "u", Email: "e", Token: "t"}, nil
		},
		renderSettings: func(string, string) (string, error) { return rendered, nil },
		provision: func(_ context.Context, _ sprite.SpriteCLI, _ lifecycle.Config, opts lifecycle.ProvisionOpts) (lifecycle.ProvisionResult, error) {
			calls = append(calls, opts)
			return lifecycle.ProvisionResult{Name: opts.Name, Created: true}, nil
		},
	}

	cmd := newProvisionCmdWithDeps(deps)
	var out bytes.Buffer
	var errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs([]string{"--composition", "compositions/v2.yaml", "willow"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("cmd.Execute() error = %v", err)
	}
	if len(calls) != 1 {
		t.Fatalf("provision calls = %d, want 1", len(calls))
	}
	if calls[0].Name != "willow" {
		t.Fatalf("provision name = %q, want willow", calls[0].Name)
	}
	if calls[0].CompositionLabel != "v2" {
		t.Fatalf("composition label = %q, want v2", calls[0].CompositionLabel)
	}
	if !strings.Contains(errOut.String(), "starting provisioning") {
		t.Fatalf("expected progress output, got %q", errOut.String())
	}

	var payload struct {
		Command string `json:"command"`
	}
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if payload.Command != "provision" {
		t.Fatalf("command = %q, want provision", payload.Command)
	}
}

func TestProvisionCmdAllUsesComposition(t *testing.T) {
	t.Parallel()

	rootDir := t.TempDir()
	rendered := filepath.Join(t.TempDir(), "rendered-settings.json")
	callNames := make([]string, 0, 2)

	deps := provisionDeps{
		getwd: func() (string, error) { return rootDir, nil },
		getenv: func(key string) string {
			if key == "OPENROUTER_API_KEY" {
				return "token"
			}
			return ""
		},
		newCLI:             func(string, string) sprite.SpriteCLI { return &sprite.MockSpriteCLI{} },
		resolveComposition: func(string) ([]string, error) { return []string{"bramble", "thorn"}, nil },
		resolveGitHubAuth: func(string, func(string) string) (lifecycle.GitHubAuth, error) {
			return lifecycle.GitHubAuth{User: "u", Email: "e", Token: "t"}, nil
		},
		renderSettings: func(string, string) (string, error) { return rendered, nil },
		provision: func(_ context.Context, _ sprite.SpriteCLI, _ lifecycle.Config, opts lifecycle.ProvisionOpts) (lifecycle.ProvisionResult, error) {
			callNames = append(callNames, opts.Name)
			return lifecycle.ProvisionResult{Name: opts.Name, Created: true}, nil
		},
	}

	cmd := newProvisionCmdWithDeps(deps)
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"--all"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("cmd.Execute() error = %v", err)
	}
	if len(callNames) != 2 {
		t.Fatalf("provision call count = %d, want 2", len(callNames))
	}
}

func TestProvisionCmdAcceptsLegacyAuthFallback(t *testing.T) {
	t.Parallel()

	rootDir := t.TempDir()
	rendered := filepath.Join(t.TempDir(), "rendered-settings.json")

	deps := provisionDeps{
		getwd: func() (string, error) { return rootDir, nil },
		getenv: func(key string) string {
			if key == "ANTHROPIC_AUTH_TOKEN" {
				return "legacy-token"
			}
			return ""
		},
		newCLI:             func(string, string) sprite.SpriteCLI { return &sprite.MockSpriteCLI{} },
		resolveComposition: func(string) ([]string, error) { return []string{"willow"}, nil },
		resolveGitHubAuth: func(string, func(string) string) (lifecycle.GitHubAuth, error) {
			return lifecycle.GitHubAuth{User: "u", Email: "e", Token: "t"}, nil
		},
		renderSettings: func(_ string, token string) (string, error) {
			if token != "legacy-token" {
				t.Fatalf("renderSettings token = %q, want legacy-token", token)
			}
			return rendered, nil
		},
		provision: func(_ context.Context, _ sprite.SpriteCLI, _ lifecycle.Config, opts lifecycle.ProvisionOpts) (lifecycle.ProvisionResult, error) {
			return lifecycle.ProvisionResult{Name: opts.Name, Created: true}, nil
		},
	}

	cmd := newProvisionCmdWithDeps(deps)
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"willow"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("cmd.Execute() error = %v", err)
	}
}

func TestProvisionCmdFailsWithoutCanonicalAuth(t *testing.T) {
	t.Parallel()

	rootDir := t.TempDir()

	deps := provisionDeps{
		getwd:              func() (string, error) { return rootDir, nil },
		getenv:             func(string) string { return "" },
		newCLI:             func(string, string) sprite.SpriteCLI { return &sprite.MockSpriteCLI{} },
		resolveComposition: func(string) ([]string, error) { return []string{"willow"}, nil },
		resolveGitHubAuth: func(string, func(string) string) (lifecycle.GitHubAuth, error) {
			return lifecycle.GitHubAuth{User: "u", Email: "e", Token: "t"}, nil
		},
		renderSettings: func(string, string) (string, error) {
			t.Fatal("renderSettings should not be called when auth is missing")
			return "", nil
		},
		provision: func(_ context.Context, _ sprite.SpriteCLI, _ lifecycle.Config, _ lifecycle.ProvisionOpts) (lifecycle.ProvisionResult, error) {
			t.Fatal("provision should not be called when auth is missing")
			return lifecycle.ProvisionResult{}, nil
		},
	}

	cmd := newProvisionCmdWithDeps(deps)
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
