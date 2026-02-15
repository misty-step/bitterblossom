package signals

import (
	"strings"
	"testing"
)

func TestAll(t *testing.T) {
	all := All()
	if len(all) != 4 {
		t.Errorf("All() returned %d items, want 4", len(all))
	}

	expected := map[string]bool{
		TaskComplete:   false,
		TaskCompleteMD: false,
		Blocked:        false,
		BlockedLegacy:  false,
	}

	for _, item := range all {
		if _, ok := expected[item]; !ok {
			t.Errorf("All() contains unexpected item: %q", item)
		}
		expected[item] = true
	}

	for name, found := range expected {
		if !found {
			t.Errorf("All() missing expected item: %q", name)
		}
	}
}

func TestCleanScript(t *testing.T) {
	tests := []struct {
		workspace string
		wantParts []string
	}{
		{
			workspace: "/home/sprite/workspace",
			wantParts: []string{
				"rm -f",
				"/home/sprite/workspace/TASK_COMPLETE",
				"/home/sprite/workspace/TASK_COMPLETE.md",
				"/home/sprite/workspace/BLOCKED.md",
				"/home/sprite/workspace/BLOCKED",
				"/home/sprite/workspace/PR_URL",
			},
		},
		{
			workspace: "/tmp/test space",
			wantParts: []string{
				"rm -f",
				"'/tmp/test space/TASK_COMPLETE'",
			},
		},
		{
			workspace: "",
			wantParts: []string{
				"rm -f",
				"TASK_COMPLETE", // filepath.Join("", "TASK_COMPLETE") = "TASK_COMPLETE"
			},
		},
	}

	for _, tt := range tests {
		t.Run("workspace="+tt.workspace, func(t *testing.T) {
			got := CleanScript(tt.workspace)
			for _, part := range tt.wantParts {
				if !strings.Contains(got, part) {
					t.Errorf("CleanScript() missing expected part %q in:\n%s", part, got)
				}
			}
		})
	}
}

func TestCleanOnlySignalsScript(t *testing.T) {
	script := CleanOnlySignalsScript("/workspace")

	// Should contain signal files
	want := []string{
		"TASK_COMPLETE",
		"TASK_COMPLETE.md",
		"BLOCKED.md",
		"BLOCKED",
	}
	for _, w := range want {
		if !strings.Contains(script, w) {
			t.Errorf("CleanOnlySignalsScript() missing %q", w)
		}
	}

	// Should NOT contain PR_URL
	if strings.Contains(script, "PR_URL") {
		t.Error("CleanOnlySignalsScript() should not contain PR_URL")
	}
}

func TestDetectCompleteScript(t *testing.T) {
	tests := []struct {
		workspace string
		checks    []string
	}{
		{
			workspace: "/home/sprite/workspace",
			checks: []string{
				"HAS_COMPLETE=no",
				"HAS_COMPLETE=yes",
				"TASK_COMPLETE",
				"TASK_COMPLETE.md",
				"-f",
				"/home/sprite/workspace/TASK_COMPLETE",
			},
		},
		{
			workspace: "/path with spaces",
			checks: []string{
				"'/path with spaces/TASK_COMPLETE'", // Full path quoted
			},
		},
	}

	for _, tt := range tests {
		t.Run("workspace="+tt.workspace, func(t *testing.T) {
			got := DetectCompleteScript(tt.workspace)
			for _, check := range tt.checks {
				if !strings.Contains(got, check) {
					t.Errorf("DetectCompleteScript() missing %q in:\n%s", check, got)
				}
			}
		})
	}
}

func TestDetectBlockedScript(t *testing.T) {
	tests := []struct {
		workspace string
		checks    []string
	}{
		{
			workspace: "/home/sprite/workspace",
			checks: []string{
				"HAS_BLOCKED=no",
				"HAS_BLOCKED=yes",
				"BLOCKED.md",
				"BLOCKED_SUMMARY",
				"head -5",
				"/home/sprite/workspace/BLOCKED.md",
			},
		},
	}

	for _, tt := range tests {
		t.Run("workspace="+tt.workspace, func(t *testing.T) {
			got := DetectBlockedScript(tt.workspace)
			for _, check := range tt.checks {
				if !strings.Contains(got, check) {
					t.Errorf("DetectBlockedScript() missing %q in:\n%s", check, got)
				}
			}
		})
	}
}

func TestExtractPRURLScript(t *testing.T) {
	tests := []struct {
		workspace string
		checks    []string
	}{
		{
			workspace: "/home/sprite/workspace",
			checks: []string{
				"PR_URL=",
				"PR_URL",
				"TASK_COMPLETE",
				"TASK_COMPLETE.md",
				"github.com",
				"pull/[0-9]+",
			},
		},
	}

	for _, tt := range tests {
		t.Run("workspace="+tt.workspace, func(t *testing.T) {
			got := ExtractPRURLScript(tt.workspace)
			for _, check := range tt.checks {
				if !strings.Contains(got, check) {
					t.Errorf("ExtractPRURLScript() missing %q in:\n%s", check, got)
				}
			}
		})
	}
}

func TestVarAssignments(t *testing.T) {
	script := VarAssignments("/workspace")

	expectedVars := []string{
		"SIGNAL_TASK_COMPLETE=",
		"SIGNAL_TASK_COMPLETE_MD=",
		"SIGNAL_BLOCKED=",
		"SIGNAL_BLOCKED_LEGACY=",
	}

	lines := strings.Split(script, "\n")
	if len(lines) < 4 {
		t.Errorf("VarAssignments() returned %d lines, want at least 4", len(lines))
	}

	for _, expected := range expectedVars {
		found := false
		for _, line := range lines {
			if strings.Contains(line, expected) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("VarAssignments() missing variable %q", expected)
		}
	}
}

func TestConstants(t *testing.T) {
	if TaskComplete != "TASK_COMPLETE" {
		t.Errorf("TaskComplete = %q, want TASK_COMPLETE", TaskComplete)
	}
	if TaskCompleteMD != "TASK_COMPLETE.md" {
		t.Errorf("TaskCompleteMD = %q, want TASK_COMPLETE.md", TaskCompleteMD)
	}
	if Blocked != "BLOCKED.md" {
		t.Errorf("Blocked = %q, want BLOCKED.md", Blocked)
	}
	if BlockedLegacy != "BLOCKED" {
		t.Errorf("BlockedLegacy = %q, want BLOCKED", BlockedLegacy)
	}
}

func TestScriptsAreValidShell(t *testing.T) {
	// Ensure scripts don't have obvious syntax issues
	workspace := "/home/sprite/workspace"

	scripts := map[string]string{
		"CleanScript":            CleanScript(workspace),
		"CleanOnlySignalsScript": CleanOnlySignalsScript(workspace),
		"DetectCompleteScript":   DetectCompleteScript(workspace),
		"DetectBlockedScript":    DetectBlockedScript(workspace),
		"ExtractPRURLScript":     ExtractPRURLScript(workspace),
	}

	for name, script := range scripts {
		t.Run(name, func(t *testing.T) {
			// Basic sanity checks
			if script == "" {
				t.Error("script is empty")
			}

			// Should not have unquoted special characters
			if strings.Contains(script, "; ;") {
				t.Error("script has double semicolon")
			}

			// Should properly quote workspace
			if strings.Contains(workspace, " ") {
				if !strings.Contains(script, `"`+workspace+`"`) {
					t.Error("workspace with spaces not properly quoted")
				}
			}
		})
	}
}

// BenchmarkScriptGeneration measures the performance of generating shell scripts.
// These should be very fast since they're just string concatenation.
func BenchmarkCleanScript(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = CleanScript("/home/sprite/workspace")
	}
}

func BenchmarkDetectCompleteScript(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = DetectCompleteScript("/home/sprite/workspace")
	}
}
