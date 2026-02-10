package dispatch

import (
	"fmt"
	"strings"
)

// InvariantViolation indicates a non-negotiable dispatch invariant was not met.
// These are enforced by code to prevent silent hangs on sprites.
type InvariantViolation struct {
	Context   string
	Missing   []string
	Forbidden []string
}

func (e *InvariantViolation) Error() string {
	parts := make([]string, 0, 2)
	if len(e.Missing) > 0 {
		parts = append(parts, "missing: "+strings.Join(e.Missing, ", "))
	}
	if len(e.Forbidden) > 0 {
		parts = append(parts, "forbidden: "+strings.Join(e.Forbidden, ", "))
	}
	if len(parts) == 0 {
		return "dispatch: invariant violation"
	}
	if strings.TrimSpace(e.Context) == "" {
		return "dispatch: invariant violation (" + strings.Join(parts, "; ") + ")"
	}
	return fmt.Sprintf("dispatch: invariant violation (%s) [%s]", strings.Join(parts, "; "), e.Context)
}

func requireOneShotInvariants(startCommand string) error {
	missing := make([]string, 0, 4)

	required := []string{
		"claude -p",
		"--dangerously-skip-permissions",
		"--verbose --output-format stream-json",
	}
	for _, needle := range required {
		if !strings.Contains(startCommand, needle) {
			missing = append(missing, needle)
		}
	}

	forbidden := make([]string, 0, 1)
	if strings.Contains(startCommand, "export ANTHROPIC_API_KEY=") {
		forbidden = append(forbidden, "export ANTHROPIC_API_KEY=")
	}
	if strings.Contains(startCommand, "export ANTHROPIC_BASE_URL=") && !strings.Contains(startCommand, "export ANTHROPIC_AUTH_TOKEN=") {
		missing = append(missing, "export ANTHROPIC_AUTH_TOKEN=")
	}

	if len(missing) == 0 && len(forbidden) == 0 {
		return nil
	}
	return &InvariantViolation{
		Context:   "oneshot dispatch start command",
		Missing:   missing,
		Forbidden: forbidden,
	}
}
