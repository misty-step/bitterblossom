package dispatch

import (
	"errors"
	"fmt"
	"strings"
)

// ErrAnthropicKeyDetected indicates a real Anthropic API key was found on a sprite.
var ErrAnthropicKeyDetected = errors.New("dispatch: ANTHROPIC_API_KEY contains direct key")

// ValidateNoDirectAnthropic rejects keys with the sk-ant- prefix that would
// bypass the proxy and incur direct Anthropic billing.
func ValidateNoDirectAnthropic(key string) error {
	if strings.HasPrefix(strings.TrimSpace(key), "sk-ant-") {
		return fmt.Errorf("%w — this would bypass the proxy and use expensive Anthropic credits; unset ANTHROPIC_API_KEY on the sprite or use --allow-anthropic-direct", ErrAnthropicKeyDetected)
	}
	return nil
}
