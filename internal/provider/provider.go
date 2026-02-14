// Package provider abstracts LLM provider configurations for Bitterblossom sprites.
//
// This package provides a unified interface for configuring different LLM providers
// while keeping one canonical default profile for sprite runtime.
package provider

import (
	"fmt"
	"os"
	"strings"
)

// Provider identifies the LLM provider to use.
type Provider string

const (
	// ProviderMoonshotAnthropic uses Moonshot AI's Anthropic-compatible endpoint.
	// This is retained for compatibility with legacy compositions.
	ProviderMoonshotAnthropic Provider = "moonshot-anthropic"

	// ProviderMoonshot uses Moonshot AI (Kimi models) via their native Anthropic-compatible API.
	// This is the legacy provider for backward compatibility.
	ProviderMoonshot Provider = "moonshot"

	// ProviderOpenRouterKimi uses Kimi models via OpenRouter's unified API.
	ProviderOpenRouterKimi Provider = "openrouter-kimi"

	// ProviderOpenRouterClaude uses Claude models via OpenRouter's unified API.
	ProviderOpenRouterClaude Provider = "openrouter-claude"

	// ProviderProxy uses a local Node.js proxy to translate Anthropic Messages API
	// to OpenAI Chat Completions API. This enables Claude Code to use non-Anthropic
	// models (Kimi K2.5, GLM 4.7, etc.) via OpenRouter.
	ProviderProxy Provider = "proxy"

	// ProviderInherit means "use the base configuration" (default behavior).
	ProviderInherit Provider = "inherit"
)

// Model identifiers for known providers.
const (
	// MiniMax models
	ModelMiniMaxM25            = "minimax-m2.5"
	ModelOpenRouterMiniMaxM25  = "minimax/minimax-m2.5"

	// Kimi models (legacy — Kimi K2.5 doesn't produce tool calls reliably)
	ModelKimiK25             = "kimi-k2.5"
	ModelKimiK2ThinkingTurbo = "kimi-k2-thinking-turbo"
	ModelOpenRouterKimiK25   = "moonshotai/kimi-k2.5"

	// Claude models via OpenRouter
	ModelClaudeOpus4   = "anthropic/claude-opus-4"
	ModelClaudeSonnet4 = "anthropic/claude-sonnet-4"
	ModelClaudeHaiku4  = "anthropic/claude-haiku-4"
)

// Default provider and model for canonical runtime operation.
const (
	DefaultProvider = ProviderProxy
	DefaultModel    = ModelOpenRouterMiniMaxM25
)

// Config holds provider-specific configuration for a sprite.
type Config struct {
	// Provider identifies which provider to use.
	// Use ProviderInherit to use base settings (default).
	Provider Provider `json:"provider,omitempty" yaml:"provider,omitempty"`

	// Model specifies the model identifier.
	// For OpenRouter, this should include the provider prefix (e.g., "anthropic/claude-opus-4").
	Model string `json:"model,omitempty" yaml:"model,omitempty"`

	// Environment variables to set for this provider.
	// These override the base settings.json env vars.
	Environment map[string]string `json:"environment,omitempty" yaml:"environment,omitempty"`
}

// IsInherited reports whether this config should inherit from base settings.
func (c Config) IsInherited() bool {
	return c.Provider == "" || c.Provider == ProviderInherit
}

// Resolve returns the effective provider configuration.
// If inherited, it returns the default provider config.
func (c Config) Resolve() ResolvedConfig {
	if c.IsInherited() {
		return ResolvedConfig{
			Provider: DefaultProvider,
			Model:    DefaultModel,
		}
	}

	provider := c.Provider
	model := c.Model

	// Set defaults based on provider if model not specified
	if model == "" {
		switch provider {
		case ProviderMoonshotAnthropic:
			model = ModelKimiK2ThinkingTurbo
		case ProviderMoonshot:
			model = ModelKimiK25
		case ProviderOpenRouterKimi:
			model = ModelOpenRouterKimiK25
		case ProviderProxy:
			model = DefaultModel
		case ProviderOpenRouterClaude:
			model = ModelClaudeOpus4
		}
	}

	return ResolvedConfig{
		Provider:    provider,
		Model:       model,
		Environment: c.Environment,
	}
}

// ResolvedConfig is a fully resolved provider configuration.
type ResolvedConfig struct {
	Provider    Provider
	Model       string
	Environment map[string]string
}

// EnvironmentVars returns the environment variables for Claude Code based on the provider.
// The authToken is injected at runtime from environment variables.
// Custom environment variables from the config take precedence over provider defaults.
func (r ResolvedConfig) EnvironmentVars(authToken string) map[string]string {
	env := make(map[string]string)

	switch r.Provider {
	case ProviderMoonshotAnthropic:
		// Moonshot Anthropic endpoint (preferred/default)
		env["ANTHROPIC_BASE_URL"] = "https://api.moonshot.ai/anthropic"
		env["ANTHROPIC_MODEL"] = r.Model
		env["ANTHROPIC_SMALL_FAST_MODEL"] = r.Model
		env["ANTHROPIC_DEFAULT_OPUS_MODEL"] = r.Model
		env["ANTHROPIC_DEFAULT_SONNET_MODEL"] = r.Model
		env["ANTHROPIC_DEFAULT_HAIKU_MODEL"] = r.Model
		env["CLAUDE_CODE_SUBAGENT_MODEL"] = r.Model
		if authToken != "" {
			env["ANTHROPIC_AUTH_TOKEN"] = authToken
		}

	case ProviderMoonshot:
		// Native Moonshot API (legacy, backward compatible)
		env["ANTHROPIC_BASE_URL"] = "https://api.moonshot.ai/anthropic"
		env["ANTHROPIC_MODEL"] = r.Model
		env["ANTHROPIC_SMALL_FAST_MODEL"] = r.Model
		env["ANTHROPIC_DEFAULT_OPUS_MODEL"] = r.Model
		env["ANTHROPIC_DEFAULT_SONNET_MODEL"] = r.Model
		env["ANTHROPIC_DEFAULT_HAIKU_MODEL"] = r.Model
		env["CLAUDE_CODE_SUBAGENT_MODEL"] = r.Model
		if authToken != "" {
			env["ANTHROPIC_AUTH_TOKEN"] = authToken
		}

	case ProviderOpenRouterKimi:
		// Kimi via OpenRouter
		// Claude Code appends /v1/messages?beta=true internally.
		// Base URL must not include /v1 or requests become /v1/v1/... and 404.
		env["ANTHROPIC_BASE_URL"] = "https://openrouter.ai/api"
		model := r.Model
		if !strings.Contains(model, "/") {
			model = "moonshotai/" + model
		}
		env["ANTHROPIC_MODEL"] = model
		env["ANTHROPIC_SMALL_FAST_MODEL"] = model
		env["ANTHROPIC_DEFAULT_OPUS_MODEL"] = model
		env["ANTHROPIC_DEFAULT_SONNET_MODEL"] = model
		env["ANTHROPIC_DEFAULT_HAIKU_MODEL"] = model
		env["CLAUDE_CODE_SUBAGENT_MODEL"] = model
		// OpenRouter uses OPENROUTER_API_KEY, but we can also use ANTHROPIC_AUTH_TOKEN
		// Claude Code will use ANTHROPIC_AUTH_TOKEN if set
		if authToken != "" {
			env["ANTHROPIC_AUTH_TOKEN"] = authToken
			env["OPENROUTER_API_KEY"] = authToken
		}
		// Tell Claude Code this is an OpenRouter endpoint
		env["CLAUDE_CODE_OPENROUTER_COMPAT"] = "1"

	case ProviderOpenRouterClaude:
		// Claude via OpenRouter
		// Claude Code appends /v1/messages?beta=true internally.
		// Base URL must not include /v1 or requests become /v1/v1/... and 404.
		env["ANTHROPIC_BASE_URL"] = "https://openrouter.ai/api"
		// OpenRouter model format: provider/model (e.g., "anthropic/claude-opus-4")
		model := r.Model
		if !strings.Contains(model, "/") {
			// Assume anthropic provider if not specified
			model = "anthropic/" + model
		}
		env["ANTHROPIC_MODEL"] = model
		env["ANTHROPIC_SMALL_FAST_MODEL"] = model
		env["ANTHROPIC_DEFAULT_OPUS_MODEL"] = model
		env["ANTHROPIC_DEFAULT_SONNET_MODEL"] = model
		env["ANTHROPIC_DEFAULT_HAIKU_MODEL"] = model
		env["CLAUDE_CODE_SUBAGENT_MODEL"] = model
		if authToken != "" {
			env["ANTHROPIC_AUTH_TOKEN"] = authToken
			env["OPENROUTER_API_KEY"] = authToken
		}
		env["CLAUDE_CODE_OPENROUTER_COMPAT"] = "1"

	case ProviderProxy:
		// Proxy provider: local Node.js proxy translates Anthropic API to OpenAI
		// The proxy runs on localhost:4000 and forwards to OpenRouter
		// Security note: The proxy binds to localhost without authentication.
		// This is acceptable for single-tenant sprites but should be documented
		// for multi-tenant deployments where any local process could access the proxy.
		env["ANTHROPIC_BASE_URL"] = "http://127.0.0.1:4000"
		env["ANTHROPIC_API_KEY"] = "proxy-mode"
		// The proxy handles auth via OPENROUTER_API_KEY env var on the sprite
		// Claude Code doesn't need to know the real API key
		model := r.Model
		if model == "" {
			model = DefaultModel
		}
		env["ANTHROPIC_MODEL"] = model
		env["ANTHROPIC_SMALL_FAST_MODEL"] = model
		env["CLAUDE_CODE_SUBAGENT_MODEL"] = model
	}

	// Common settings for all providers
	env["CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC"] = "1"
	env["API_TIMEOUT_MS"] = "600000"

	// Apply custom environment variables last so they override provider defaults
	for k, v := range r.Environment {
		env[k] = v
	}

	return env
}

// Validate checks if the provider configuration is valid.
func (c Config) Validate() error {
	if c.IsInherited() {
		return nil
	}

	switch c.Provider {
	case ProviderMoonshotAnthropic, ProviderMoonshot, ProviderOpenRouterKimi, ProviderOpenRouterClaude, ProviderProxy, ProviderInherit:
		// Valid
	default:
		return fmt.Errorf("invalid provider: %q (valid: moonshot-anthropic, moonshot, openrouter-kimi, openrouter-claude, proxy, inherit)", c.Provider)
	}

	if c.Model != "" {
		// Basic validation - could be expanded
		if strings.Contains(c.Model, " ") {
			return fmt.Errorf("invalid model: contains spaces")
		}
	}

	return nil
}

// ParseProvider parses a provider string into a Provider type.
func ParseProvider(s string) (Provider, error) {
	s = strings.ToLower(strings.TrimSpace(s))
	switch s {
	case "moonshot-anthropic", "moonshotanthropic":
		return ProviderMoonshotAnthropic, nil
	case "moonshot", "kimi":
		return ProviderMoonshot, nil
	case "openrouter-kimi", "openrouter/kimi":
		return ProviderOpenRouterKimi, nil
	case "openrouter-claude", "openrouter/claude", "claude":
		return ProviderOpenRouterClaude, nil
	case "proxy":
		return ProviderProxy, nil
	case "inherit", "":
		return ProviderInherit, nil
	default:
		return "", fmt.Errorf("unknown provider: %q", s)
	}
}

// AvailableProviders returns a list of valid provider identifiers.
func AvailableProviders() []string {
	return []string{
		string(ProviderProxy),
		string(ProviderMoonshotAnthropic),
		string(ProviderMoonshot),
		string(ProviderOpenRouterKimi),
		string(ProviderOpenRouterClaude),
		string(ProviderInherit),
	}
}

// AvailableModels returns a map of provider to available models.
func AvailableModels() map[string][]string {
	return map[string][]string{
		string(ProviderProxy): {
			ModelOpenRouterMiniMaxM25,
			ModelOpenRouterKimiK25,
		},
		string(ProviderMoonshotAnthropic): {
			ModelKimiK2ThinkingTurbo,
		},
		string(ProviderMoonshot): {
			ModelKimiK25,
		},
		string(ProviderOpenRouterKimi): {
			ModelOpenRouterKimiK25,
			ModelKimiK25,
		},
		string(ProviderOpenRouterClaude): {
			ModelClaudeOpus4,
			ModelClaudeSonnet4,
			ModelClaudeHaiku4,
		},
	}
}

// GetAuthToken retrieves the appropriate auth token from environment variables.
// It checks provider-specific variables first, then falls back to generic ones.
func GetAuthToken(provider Provider) string {
	return ResolveAuthToken(provider, os.Getenv)
}

// ResolveAuthToken retrieves auth token for a provider using the supplied getenv function.
// OpenRouter providers prefer OPENROUTER_API_KEY and fall back to ANTHROPIC_AUTH_TOKEN.
// Moonshot providers only use ANTHROPIC_AUTH_TOKEN.
func ResolveAuthToken(provider Provider, getenv func(string) string) string {
	if getenv == nil {
		return ""
	}

	get := func(key string) string {
		return strings.TrimSpace(getenv(key))
	}

	if provider == "" || provider == ProviderInherit {
		provider = DefaultProvider
	}

	// Check provider-specific env vars first
	switch provider {
	case ProviderOpenRouterKimi, ProviderOpenRouterClaude, ProviderProxy:
		// OpenRouter providers and proxy: check OPENROUTER_API_KEY first, then fall back to ANTHROPIC_AUTH_TOKEN
		if token := get("OPENROUTER_API_KEY"); token != "" {
			return token
		}
		if token := get("ANTHROPIC_AUTH_TOKEN"); token != "" {
			return token
		}
	case ProviderMoonshotAnthropic, ProviderMoonshot:
		// Moonshot providers: ONLY check ANTHROPIC_AUTH_TOKEN
		if token := get("ANTHROPIC_AUTH_TOKEN"); token != "" {
			return token
		}
	}

	return ""
}
