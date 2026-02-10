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
		"--dangerously-skip-permissions",
		"--verbose",
		"--output-format stream-json",
	}
	if !strings.Contains(startCommand, "claude -p") {
		missing = append(missing, "claude -p")
	}
	for _, needle := range required {
		if !strings.Contains(startCommand, needle) {
			missing = append(missing, needle)
		}
	}

	if len(missing) == 0 {
		return nil
	}
	return &InvariantViolation{
		Context: "oneshot dispatch start command",
		Missing: missing,
	}
}
