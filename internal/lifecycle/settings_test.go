package lifecycle

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/misty-step/bitterblossom/internal/provider"
)

func TestRenderSettingsInjectsToken(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	source := filepath.Join(dir, "settings.json")
	if err := os.WriteFile(source, []byte(`{"env":{"EXISTING":"1"}}`), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	rendered, err := RenderSettings(source, "test-token")
	if err != nil {
		t.Fatalf("RenderSettings() error = %v", err)
	}
	defer func() {
		_ = os.Remove(rendered)
	}()

	raw, err := os.ReadFile(rendered)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	env, ok := payload["env"].(map[string]any)
	if !ok {
		t.Fatal("env is not a map")
	}
	// Proxy provider uses local proxy - no ANTHROPIC_AUTH_TOKEN or OPENROUTER_API_KEY in settings
	if env["ANTHROPIC_API_KEY"] != "proxy-mode" {
		t.Fatalf("ANTHROPIC_API_KEY = %v, want proxy-mode", env["ANTHROPIC_API_KEY"])
	}
	if env["ANTHROPIC_BASE_URL"] != "http://127.0.0.1:4000" {
		t.Fatalf("ANTHROPIC_BASE_URL = %v, want http://127.0.0.1:4000", env["ANTHROPIC_BASE_URL"])
	}
	if env["ANTHROPIC_MODEL"] != provider.ModelOpenRouterKimiK25 {
		t.Fatalf("ANTHROPIC_MODEL = %v, want %s", env["ANTHROPIC_MODEL"], provider.ModelOpenRouterKimiK25)
	}
}

func TestRenderSettingsMissingToken(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	source := filepath.Join(dir, "settings.json")
	if err := os.WriteFile(source, []byte(`{"env":{}}`), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	if _, err := RenderSettings(source, ""); err == nil {
		t.Fatal("expected missing token error")
	}
}

func TestRenderSettingsWithProvider(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	source := filepath.Join(dir, "settings.json")
	baseSettings := `{
		"env": {
			"EXISTING": "1",
			"ANTHROPIC_BASE_URL": "https://default.example.com"
		}
	}`
	if err := os.WriteFile(source, []byte(baseSettings), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	tests := []struct {
		name         string
		provider     provider.Config
		wantProvider string
		wantModel    string
		wantBaseURL  string
	}{
		{
			name:         "moonshot provider",
			provider:     provider.Config{Provider: provider.ProviderMoonshot, Model: provider.ModelKimiK25},
			wantProvider: "moonshot",
			wantModel:    "kimi-k2.5",
			wantBaseURL:  "https://api.moonshot.ai/anthropic",
		},
		{
			name:         "openrouter kimi",
			provider:     provider.Config{Provider: provider.ProviderOpenRouterKimi, Model: provider.ModelOpenRouterKimiK25},
			wantProvider: "openrouter-kimi",
			wantModel:    provider.ModelOpenRouterKimiK25,
			wantBaseURL:  "https://openrouter.ai/api",
		},
		{
			name:         "openrouter claude",
			provider:     provider.Config{Provider: provider.ProviderOpenRouterClaude, Model: provider.ModelClaudeOpus4},
			wantProvider: "openrouter-claude",
			wantModel:    provider.ModelClaudeOpus4,
			wantBaseURL:  "https://openrouter.ai/api",
		},
		{
			name:         "inherited uses proxy provider",
			provider:     provider.Config{Provider: provider.ProviderInherit},
			wantProvider: "proxy",
			wantModel:    provider.ModelOpenRouterKimiK25,
			wantBaseURL:  "http://127.0.0.1:4000",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rendered, err := RenderSettingsWithProvider(source, "test-token", tt.provider)
			if err != nil {
				t.Fatalf("RenderSettingsWithProvider() error = %v", err)
			}
			defer func() {
				_ = os.Remove(rendered)
			}()

			raw, err := os.ReadFile(rendered)
			if err != nil {
				t.Fatalf("ReadFile() error = %v", err)
			}

			var payload map[string]any
			if err := json.Unmarshal(raw, &payload); err != nil {
				t.Fatalf("json.Unmarshal() error = %v", err)
			}

			env, ok := payload["env"].(map[string]any)
			if !ok {
				t.Fatal("env is not a map")
			}

			// Check base URL is overridden correctly
			if got := env["ANTHROPIC_BASE_URL"]; got != tt.wantBaseURL {
				t.Errorf("ANTHROPIC_BASE_URL = %v, want %v", got, tt.wantBaseURL)
			}

			// Check model is set
			if got := env["ANTHROPIC_MODEL"]; got != tt.wantModel {
				t.Errorf("ANTHROPIC_MODEL = %v, want %v", got, tt.wantModel)
			}

			// Verify that provider-specific env vars are present
			switch tt.provider.Provider {
			case provider.ProviderOpenRouterKimi, provider.ProviderOpenRouterClaude:
				if _, ok := env["OPENROUTER_API_KEY"]; !ok {
					t.Error("expected OPENROUTER_API_KEY to be set")
				}
				if _, ok := env["CLAUDE_CODE_OPENROUTER_COMPAT"]; !ok {
					t.Error("expected CLAUDE_CODE_OPENROUTER_COMPAT to be set")
				}
				// Check token is set
				if env["ANTHROPIC_AUTH_TOKEN"] != "test-token" {
					t.Errorf("token = %v, want test-token", env["ANTHROPIC_AUTH_TOKEN"])
				}
			case provider.ProviderMoonshot, provider.ProviderMoonshotAnthropic:
				if _, ok := env["CLAUDE_CODE_OPENROUTER_COMPAT"]; ok {
					t.Error("did not expect CLAUDE_CODE_OPENROUTER_COMPAT for moonshot providers")
				}
				// Check token is set
				if env["ANTHROPIC_AUTH_TOKEN"] != "test-token" {
					t.Errorf("token = %v, want test-token", env["ANTHROPIC_AUTH_TOKEN"])
				}
			case provider.ProviderInherit:
				// Inherited now uses proxy provider
				if env["ANTHROPIC_API_KEY"] != "proxy-mode" {
					t.Errorf("ANTHROPIC_API_KEY = %v, want proxy-mode", env["ANTHROPIC_API_KEY"])
				}
			}
		})
	}
}

func TestRenderSettingsWithCustomEnvVars(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	source := filepath.Join(dir, "settings.json")
	if err := os.WriteFile(source, []byte(`{"env":{}}`), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	cfg := provider.Config{
		Provider: provider.ProviderMoonshot,
		Model:    provider.ModelKimiK25,
		Environment: map[string]string{
			"CUSTOM_VAR":     "custom_value",
			"API_TIMEOUT_MS": "1200000",
		},
	}

	rendered, err := RenderSettingsWithProvider(source, "test-token", cfg)
	if err != nil {
		t.Fatalf("RenderSettingsWithProvider() error = %v", err)
	}
	defer func() {
		_ = os.Remove(rendered)
	}()

	raw, err := os.ReadFile(rendered)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}

	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}

	env, ok := payload["env"].(map[string]any)
	if !ok {
		t.Fatal("env is not a map")
	}

	// Check custom vars are set
	if env["CUSTOM_VAR"] != "custom_value" {
		t.Errorf("CUSTOM_VAR = %v, want custom_value", env["CUSTOM_VAR"])
	}

	// Check that custom vars can override defaults
	if env["API_TIMEOUT_MS"] != "1200000" {
		t.Errorf("API_TIMEOUT_MS = %v, want 1200000", env["API_TIMEOUT_MS"])
	}
}
