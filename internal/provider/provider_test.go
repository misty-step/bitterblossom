package provider

import (
	"os"
	"testing"
)

func TestConfig_IsInherited(t *testing.T) {
	tests := []struct {
		name     string
		provider Provider
		want     bool
	}{
		{"empty provider", "", true},
		{"inherit provider", ProviderInherit, true},
		{"moonshot provider", ProviderMoonshot, false},
		{"openrouter kimi", ProviderOpenRouterKimi, false},
		{"openrouter claude", ProviderOpenRouterClaude, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := Config{Provider: tt.provider}
			if got := c.IsInherited(); got != tt.want {
				t.Errorf("IsInherited() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestConfig_Resolve(t *testing.T) {
	tests := []struct {
		name string
		cfg  Config
		want ResolvedConfig
	}{
		{
			name: "inherited uses defaults",
			cfg:  Config{Provider: ProviderInherit},
			want: ResolvedConfig{
				Provider: DefaultProvider,
				Model:    DefaultModel,
			},
		},
		{
			name: "empty config uses defaults",
			cfg:  Config{},
			want: ResolvedConfig{
				Provider: DefaultProvider,
				Model:    DefaultModel,
			},
		},
		{
			name: "moonshot with explicit model",
			cfg: Config{
				Provider: ProviderMoonshot,
				Model:    "kimi-k2.5",
			},
			want: ResolvedConfig{
				Provider: ProviderMoonshot,
				Model:    "kimi-k2.5",
			},
		},
		{
			name: "moonshot without model gets default",
			cfg: Config{
				Provider: ProviderMoonshot,
			},
			want: ResolvedConfig{
				Provider: ProviderMoonshot,
				Model:    ModelKimiK25,
			},
		},
		{
			name: "openrouter kimi without model gets default",
			cfg: Config{
				Provider: ProviderOpenRouterKimi,
			},
			want: ResolvedConfig{
				Provider: ProviderOpenRouterKimi,
				Model:    ModelOpenRouterKimiK25,
			},
		},
		{
			name: "openrouter claude without model gets default",
			cfg: Config{
				Provider: ProviderOpenRouterClaude,
			},
			want: ResolvedConfig{
				Provider: ProviderOpenRouterClaude,
				Model:    ModelClaudeOpus4,
			},
		},
		{
			name: "openrouter claude with explicit model",
			cfg: Config{
				Provider: ProviderOpenRouterClaude,
				Model:    "anthropic/claude-sonnet-4",
			},
			want: ResolvedConfig{
				Provider: ProviderOpenRouterClaude,
				Model:    "anthropic/claude-sonnet-4",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.cfg.Resolve()
			if got.Provider != tt.want.Provider {
				t.Errorf("Resolve().Provider = %v, want %v", got.Provider, tt.want.Provider)
			}
			if got.Model != tt.want.Model {
				t.Errorf("Resolve().Model = %v, want %v", got.Model, tt.want.Model)
			}
		})
	}
}

func TestResolvedConfig_EnvironmentVars(t *testing.T) {
	tests := []struct {
		name      string
		cfg       ResolvedConfig
		authToken string
		wantKeys  []string
		checkVals map[string]string
	}{
		{
			name: "moonshot provider",
			cfg: ResolvedConfig{
				Provider: ProviderMoonshot,
				Model:    ModelKimiK25,
			},
			authToken: "test-token",
			wantKeys: []string{
				"ANTHROPIC_BASE_URL",
				"ANTHROPIC_MODEL",
				"ANTHROPIC_DEFAULT_OPUS_MODEL",
				"ANTHROPIC_DEFAULT_SONNET_MODEL",
				"ANTHROPIC_DEFAULT_HAIKU_MODEL",
				"CLAUDE_CODE_SUBAGENT_MODEL",
				"ANTHROPIC_AUTH_TOKEN",
				"CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC",
			},
			checkVals: map[string]string{
				"ANTHROPIC_BASE_URL": "https://api.moonshot.ai/anthropic",
				"ANTHROPIC_MODEL":    ModelKimiK25,
			},
		},
		{
			name: "openrouter kimi",
			cfg: ResolvedConfig{
				Provider: ProviderOpenRouterKimi,
				Model:    ModelOpenRouterKimiK25,
			},
			authToken: "test-token",
			wantKeys: []string{
				"ANTHROPIC_BASE_URL",
				"ANTHROPIC_MODEL",
				"ANTHROPIC_DEFAULT_OPUS_MODEL",
				"ANTHROPIC_DEFAULT_SONNET_MODEL",
				"ANTHROPIC_DEFAULT_HAIKU_MODEL",
				"CLAUDE_CODE_SUBAGENT_MODEL",
				"ANTHROPIC_AUTH_TOKEN",
				"OPENROUTER_API_KEY",
				"CLAUDE_CODE_OPENROUTER_COMPAT",
			},
			checkVals: map[string]string{
				"ANTHROPIC_BASE_URL":            "https://openrouter.ai/api",
				"ANTHROPIC_MODEL":               ModelOpenRouterKimiK25,
				"CLAUDE_CODE_OPENROUTER_COMPAT": "1",
			},
		},
		{
			name: "openrouter kimi with model needing prefix",
			cfg: ResolvedConfig{
				Provider: ProviderOpenRouterKimi,
				Model:    ModelKimiK25, // missing moonshotai/ prefix
			},
			authToken: "test-token",
			checkVals: map[string]string{
				"ANTHROPIC_MODEL": "moonshotai/kimi-k2.5",
			},
		},
		{
			name: "openrouter claude",
			cfg: ResolvedConfig{
				Provider: ProviderOpenRouterClaude,
				Model:    ModelClaudeOpus4,
			},
			authToken: "test-token",
			wantKeys: []string{
				"ANTHROPIC_BASE_URL",
				"ANTHROPIC_MODEL",
				"ANTHROPIC_DEFAULT_OPUS_MODEL",
				"ANTHROPIC_DEFAULT_SONNET_MODEL",
				"ANTHROPIC_DEFAULT_HAIKU_MODEL",
				"CLAUDE_CODE_SUBAGENT_MODEL",
				"ANTHROPIC_AUTH_TOKEN",
				"OPENROUTER_API_KEY",
				"CLAUDE_CODE_OPENROUTER_COMPAT",
			},
			checkVals: map[string]string{
				"ANTHROPIC_BASE_URL":            "https://openrouter.ai/api",
				"ANTHROPIC_MODEL":               ModelClaudeOpus4,
				"CLAUDE_CODE_OPENROUTER_COMPAT": "1",
			},
		},
		{
			name: "openrouter claude with model needing prefix",
			cfg: ResolvedConfig{
				Provider: ProviderOpenRouterClaude,
				Model:    "claude-sonnet-4", // missing anthropic/ prefix
			},
			authToken: "test-token",
			checkVals: map[string]string{
				"ANTHROPIC_MODEL": "anthropic/claude-sonnet-4",
			},
		},
		{
			name: "without auth token",
			cfg: ResolvedConfig{
				Provider: ProviderMoonshot,
				Model:    ModelKimiK25,
			},
			authToken: "",
			wantKeys: []string{
				"ANTHROPIC_BASE_URL",
				"ANTHROPIC_MODEL",
			},
			checkVals: map[string]string{
				"ANTHROPIC_MODEL": ModelKimiK25,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := tt.cfg.EnvironmentVars(tt.authToken)

			// Check required keys exist
			for _, key := range tt.wantKeys {
				if _, ok := env[key]; !ok {
					t.Errorf("missing expected key: %s", key)
				}
			}

			// Check specific values
			for key, want := range tt.checkVals {
				if got := env[key]; got != want {
					t.Errorf("env[%s] = %q, want %q", key, got, want)
				}
			}

			// Verify auth token is set correctly when provided
			if tt.authToken != "" {
				if env["ANTHROPIC_AUTH_TOKEN"] != tt.authToken {
					t.Errorf("ANTHROPIC_AUTH_TOKEN = %q, want %q", env["ANTHROPIC_AUTH_TOKEN"], tt.authToken)
				}
			}
		})
	}
}

func TestConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		cfg     Config
		wantErr bool
	}{
		{
			name:    "valid inherited",
			cfg:     Config{Provider: ProviderInherit},
			wantErr: false,
		},
		{
			name:    "valid moonshot",
			cfg:     Config{Provider: ProviderMoonshot, Model: ModelKimiK25},
			wantErr: false,
		},
		{
			name:    "valid openrouter kimi",
			cfg:     Config{Provider: ProviderOpenRouterKimi, Model: ModelOpenRouterKimiK25},
			wantErr: false,
		},
		{
			name:    "valid openrouter claude",
			cfg:     Config{Provider: ProviderOpenRouterClaude, Model: ModelClaudeOpus4},
			wantErr: false,
		},
		{
			name:    "empty config is valid (inherit)",
			cfg:     Config{},
			wantErr: false,
		},
		{
			name:    "invalid provider",
			cfg:     Config{Provider: "invalid-provider"},
			wantErr: true,
		},
		{
			name:    "invalid model with spaces",
			cfg:     Config{Provider: ProviderMoonshot, Model: "model with spaces"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestParseProvider(t *testing.T) {
	tests := []struct {
		input   string
		want    Provider
		wantErr bool
	}{
		{"moonshot", ProviderMoonshot, false},
		{"kimi", ProviderMoonshot, false},
		{"openrouter-kimi", ProviderOpenRouterKimi, false},
		{"openrouter/kimi", ProviderOpenRouterKimi, false},
		{"openrouter-claude", ProviderOpenRouterClaude, false},
		{"openrouter/claude", ProviderOpenRouterClaude, false},
		{"claude", ProviderOpenRouterClaude, false},
		{"inherit", ProviderInherit, false},
		{"", ProviderInherit, false},
		{"unknown", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := ParseProvider(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseProvider(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("ParseProvider(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestGetAuthToken(t *testing.T) {
	// Save and restore env vars
	origOpenRouter := os.Getenv("OPENROUTER_API_KEY")
	origAnthropic := os.Getenv("ANTHROPIC_AUTH_TOKEN")
	defer func() {
		_ = os.Setenv("OPENROUTER_API_KEY", origOpenRouter)
		_ = os.Setenv("ANTHROPIC_AUTH_TOKEN", origAnthropic)
	}()

	tests := []struct {
		name       string
		openRouter string
		anthropic  string
		provider   Provider
		want       string
	}{
		{
			name:       "openrouter key for openrouter provider",
			openRouter: "openrouter-token",
			anthropic:  "anthropic-token",
			provider:   ProviderOpenRouterClaude,
			want:       "openrouter-token",
		},
		{
			name:       "fallback to anthropic token",
			openRouter: "",
			anthropic:  "anthropic-token",
			provider:   ProviderOpenRouterKimi,
			want:       "anthropic-token",
		},
		{
			name:       "moonshot uses anthropic token",
			openRouter: "",
			anthropic:  "moonshot-token",
			provider:   ProviderMoonshot,
			want:       "moonshot-token",
		},
		{
			name:       "no token returns empty",
			openRouter: "",
			anthropic:  "",
			provider:   ProviderMoonshot,
			want:       "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_ = os.Setenv("OPENROUTER_API_KEY", tt.openRouter)
			_ = os.Setenv("ANTHROPIC_AUTH_TOKEN", tt.anthropic)

			got := GetAuthToken(tt.provider)
			if got != tt.want {
				t.Errorf("GetAuthToken(%v) = %q, want %q", tt.provider, got, tt.want)
			}
		})
	}
}

func TestResolveAuthToken(t *testing.T) {
	tests := []struct {
		name     string
		provider Provider
		env      map[string]string
		want     string
	}{
		{
			name:     "openrouter provider prefers openrouter key",
			provider: ProviderOpenRouterKimi,
			env: map[string]string{
				"OPENROUTER_API_KEY":   "openrouter-token",
				"ANTHROPIC_AUTH_TOKEN": "anthropic-token",
			},
			want: "openrouter-token",
		},
		{
			name:     "moonshot provider ignores openrouter key",
			provider: ProviderMoonshot,
			env: map[string]string{
				"OPENROUTER_API_KEY":   "openrouter-token",
				"ANTHROPIC_AUTH_TOKEN": "anthropic-token",
			},
			want: "anthropic-token",
		},
		{
			name:     "inherit uses canonical default provider",
			provider: ProviderInherit,
			env: map[string]string{
				"OPENROUTER_API_KEY": "openrouter-token",
			},
			want: "openrouter-token",
		},
		{
			name:     "missing token",
			provider: ProviderOpenRouterKimi,
			env:      map[string]string{},
			want:     "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			getenv := func(key string) string {
				return tt.env[key]
			}
			got := ResolveAuthToken(tt.provider, getenv)
			if got != tt.want {
				t.Fatalf("ResolveAuthToken(%v) = %q, want %q", tt.provider, got, tt.want)
			}
		})
	}

	if got := ResolveAuthToken(ProviderOpenRouterKimi, nil); got != "" {
		t.Fatalf("ResolveAuthToken(nil getenv) = %q, want empty string", got)
	}
}

func TestAvailableProviders(t *testing.T) {
	providers := AvailableProviders()
	expected := []string{"moonshot-anthropic", "moonshot", "openrouter-kimi", "openrouter-claude", "inherit"}

	if len(providers) != len(expected) {
		t.Errorf("AvailableProviders() returned %d providers, want %d", len(providers), len(expected))
	}

	for i, p := range expected {
		if i >= len(providers) || providers[i] != p {
			t.Errorf("AvailableProviders()[%d] = %q, want %q", i, providers[i], p)
		}
	}
}
