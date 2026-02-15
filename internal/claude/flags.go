// Package claude provides shared constants and utilities for Claude Code invocation.
package claude

import (
	"fmt"
	"strings"
)

// RequiredFlags returns the list of required flags for sprite agent execution.
// These flags are mandatory because:
// - --dangerously-skip-permissions: prevents blocking on permissions prompts in headless sprites
// - --permission-mode bypassPermissions: alternative way to skip permissions
// - --verbose: enables structured output needed for progress parsing
// - --output-format stream-json: enables JSON streaming for real-time progress updates
var RequiredFlags = []string{
	"--dangerously-skip-permissions",
	"--permission-mode",
	"bypassPermissions",
	"--verbose",
	"--output-format",
	"stream-json",
}

// FlagSet returns RequiredFlags as a space-separated string for shell invocation.
func FlagSet() string {
	return strings.Join(RequiredFlags, " ")
}

// FlagSetWithPrefix returns RequiredFlags prefixed with "-p " (prompt mode).
func FlagSetWithPrefix() string {
	return "-p " + FlagSet()
}

// ShellExport returns a shell script that exports BB_CLAUDE_FLAGS and BB_CLAUDE_FLAGS_WITH_PROMPT.
// This can be sourced by shell scripts to use the same flags as the Go code.
// Derives values from RequiredFlags so there is exactly one source of truth.
func ShellExport() string {
	return fmt.Sprintf(`# Auto-generated from internal/claude/flags.go - do not edit
BB_CLAUDE_FLAGS="%s"
BB_CLAUDE_FLAGS_WITH_PROMPT="-p $BB_CLAUDE_FLAGS"
export BB_CLAUDE_FLAGS BB_CLAUDE_FLAGS_WITH_PROMPT
`, FlagSet())
}

// HasRequiredFlag checks if the given flag is in RequiredFlags.
// This provides a structured way to validate flags without string ordering dependencies.
func HasRequiredFlag(flag string) bool {
	for _, f := range RequiredFlags {
		if f == flag {
			return true
		}
	}
	return false
}

// ValidateFlags checks that all RequiredFlags are present in the given slice.
// Returns nil if valid, or an error listing missing flags.
func ValidateFlags(flags []string) error {
	present := make(map[string]bool)
	for _, f := range flags {
		present[f] = true
	}

	var missing []string
	for _, required := range RequiredFlags {
		if !present[required] {
			missing = append(missing, required)
		}
	}

	if len(missing) > 0 {
		return &ValidationError{Missing: missing}
	}
	return nil
}

// ValidationError represents missing required flags.
type ValidationError struct {
	Missing []string
}

func (e *ValidationError) Error() string {
	return "missing required Claude flags: " + strings.Join(e.Missing, ", ")
}
