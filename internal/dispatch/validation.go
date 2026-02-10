package dispatch

import (
	"fmt"
	"strings"
)

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
