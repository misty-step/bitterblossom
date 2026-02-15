// Package signals provides centralized knowledge of signal file operations.
//
// Signal files are used by agents to indicate task completion or blocking states.
// This package eliminates duplication across 7+ locations that previously had
// hardcoded signal filename knowledge.
//
// The recent TASK_COMPLETE filename mismatch bug (PR #282) happened precisely
// because of this scattered duplication. This package is the fix.
package signals

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/misty-step/bitterblossom/internal/shellutil"
)

// Signal file names written by agents to indicate task completion or blocking.
// Both extensions are checked because agents may write either variant.
const (
	// TaskComplete is the legacy signal file name (no extension).
	TaskComplete = "TASK_COMPLETE"
	// TaskCompleteMD is the markdown variant signal file name.
	TaskCompleteMD = "TASK_COMPLETE.md"
	// Blocked is the blocked state signal file name.
	Blocked = "BLOCKED.md"
	// BlockedLegacy is the legacy blocked signal file name (no extension).
	BlockedLegacy = "BLOCKED"
)

// All returns all signal file names for iteration or cleanup.
func All() []string {
	return []string{TaskComplete, TaskCompleteMD, Blocked, BlockedLegacy}
}

// CleanScript returns a shell fragment to remove stale signal files.
// This should be called before starting a new agent run.
func CleanScript(workspace string) string {
	files := make([]string, 0, len(All())+1)
	for _, f := range All() {
		files = append(files, shellutil.Quote(filepath.Join(workspace, f)))
	}
	// Also remove PR_URL to prevent stale URLs from causing false positive
	// completion detection (see PR #318).
	files = append(files, shellutil.Quote(filepath.Join(workspace, "PR_URL")))
	return fmt.Sprintf("rm -f %s", strings.Join(files, " "))
}

// CleanOnlySignalsScript returns a shell fragment to remove only signal files
// (not PR_URL). Used when cleaning signals but preserving PR_URL for
// work delta tracking.
func CleanOnlySignalsScript(workspace string) string {
	files := make([]string, 0, len(All()))
	for _, f := range All() {
		files = append(files, shellutil.Quote(filepath.Join(workspace, f)))
	}
	return fmt.Sprintf("rm -f %s", strings.Join(files, " "))
}

// DetectCompleteScript returns a shell fragment that exits 0 if task
// completion signal is detected. Sets HAS_COMPLETE=yes environment variable.
func DetectCompleteScript(workspace string) string {
	return fmt.Sprintf(
		`HAS_COMPLETE=no; { [ -f %[1]s ] || [ -f %[2]s ]; } && HAS_COMPLETE=yes`,
		shellutil.Quote(filepath.Join(workspace, TaskComplete)),
		shellutil.Quote(filepath.Join(workspace, TaskCompleteMD)),
	)
}

// DetectBlockedScript returns a shell fragment that exits 0 if blocked
// signal is detected. Sets HAS_BLOCKED=yes and BLOCKED_SUMMARY environment variables.
func DetectBlockedScript(workspace string) string {
	blockedPath := shellutil.Quote(filepath.Join(workspace, Blocked))
	return fmt.Sprintf(
		`HAS_BLOCKED=no; [ -f %[1]s ] && HAS_BLOCKED=yes && BLOCKED_SUMMARY="$(head -5 %[1]s 2>/dev/null | tr '\n' ' ' | sed 's/[[:space:]]\+/ /g')"`,
		blockedPath,
	)
}

// ExtractPRURLScript returns a shell fragment that extracts the PR URL.
// Checks dedicated PR_URL file first, then falls back to scanning signal files.
// Sets PR_URL environment variable.
func ExtractPRURLScript(workspace string) string {
	ws := shellutil.Quote(workspace)
	return fmt.Sprintf(
		`PR_URL=""; if [ -f %[1]s/PR_URL ]; then PR_URL="$(cat %[1]s/PR_URL | tr -d '[:space:]')"; else for f in %[2]s %[3]s; do [ -f "$f" ] && PR_URL="$(grep -oE 'https://github.com/[^/]+/[^/]+/pull/[0-9]+' "$f" 2>/dev/null | tr -d '[:space:]' || true)" && break; done; fi`,
		ws,
		shellutil.Quote(filepath.Join(workspace, TaskComplete)),
		shellutil.Quote(filepath.Join(workspace, TaskCompleteMD)),
	)
}

// VarAssignments returns shell variable assignments for signal file paths.
// Use this to export variables for use in shell scripts.
func VarAssignments(workspace string) string {
	var builder strings.Builder
	fmt.Fprintf(&builder, "SIGNAL_TASK_COMPLETE=%s\n", shellutil.Quote(filepath.Join(workspace, TaskComplete)))
	fmt.Fprintf(&builder, "SIGNAL_TASK_COMPLETE_MD=%s\n", shellutil.Quote(filepath.Join(workspace, TaskCompleteMD)))
	fmt.Fprintf(&builder, "SIGNAL_BLOCKED=%s\n", shellutil.Quote(filepath.Join(workspace, Blocked)))
	fmt.Fprintf(&builder, "SIGNAL_BLOCKED_LEGACY=%s\n", shellutil.Quote(filepath.Join(workspace, BlockedLegacy)))
	return builder.String()
}
