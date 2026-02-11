package dispatch

import (
	"fmt"
	"regexp"
	"strings"
)

// secretPatterns matches known credential prefixes that must never appear
// in command-line arguments (visible via ps aux / Fly dashboard).
//
// Each pattern requires the prefix plus enough key-like characters to
// distinguish real keys from innocent strings like "sk-ants-are-cool".
var secretPatterns = []*regexp.Regexp{
	regexp.MustCompile(`sk-ant-api[a-zA-Z0-9]`), // Anthropic API keys
	regexp.MustCompile(`sk-or-v1-[a-f0-9]{8}`),   // OpenRouter API keys
	regexp.MustCompile(`ghp_[A-Za-z0-9]{4}`),      // GitHub personal access tokens
}

// ContainsSecret reports whether s contains a string matching known API key
// or token patterns. Use this to validate that constructed shell commands
// do not leak credentials into process argument lists.
func ContainsSecret(s string) bool {
	for _, pat := range secretPatterns {
		if pat.MatchString(s) {
			return true
		}
	}
	return false
}

// ErrSecretInCommand indicates that a constructed command contains what looks
// like a credential, which would be visible in process listings.
type ErrSecretInCommand struct {
	Context string
}

func (e *ErrSecretInCommand) Error() string {
	return fmt.Sprintf("dispatch: credential detected in %s — secrets must be passed via env files, not command arguments", e.Context)
}

// ErrDirectAnthropicKey indicates that a real Anthropic API key was detected
// in the dispatch environment, which would bypass the proxy and use expensive
// Anthropic credits directly.
type ErrDirectAnthropicKey struct {
	KeyPrefix string
}

func (e *ErrDirectAnthropicKey) Error() string {
	return fmt.Sprintf(
		"ANTHROPIC_API_KEY contains a real key (%s...) — this would bypass the proxy "+
			"and use expensive Anthropic credits. Set ANTHROPIC_API_KEY=\"\" and use "+
			"ANTHROPIC_AUTH_TOKEN for OpenRouter instead",
		e.KeyPrefix,
	)
}

// ValidateNoDirectAnthropic checks that the dispatch environment does not contain
// a real Anthropic API key that would bypass the proxy.
//
// Acceptable values for ANTHROPIC_API_KEY: empty string, "proxy-mode", unset.
// If the key starts with "sk-ant-", dispatch is blocked unless allowDirect is true.
func ValidateNoDirectAnthropic(env map[string]string, allowDirect bool) error {
	key, exists := env["ANTHROPIC_API_KEY"]
	if !exists {
		return nil
	}
	if key == "" || key == "proxy-mode" {
		return nil
	}
	if strings.HasPrefix(key, "sk-ant-") && !allowDirect {
		prefix := key
		if len(prefix) > 12 {
			prefix = prefix[:12]
		}
		return &ErrDirectAnthropicKey{KeyPrefix: prefix}
	}
	return nil
}

// ValidateCommandNoSecrets checks that a constructed command string does not
// contain credentials. Returns an error if secrets are detected.
func ValidateCommandNoSecrets(command, context string) error {
	if ContainsSecret(command) {
		return &ErrSecretInCommand{Context: context}
	}
	return nil
}
