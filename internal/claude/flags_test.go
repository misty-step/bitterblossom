package claude

import (
	"strings"
	"testing"
)

func TestRequiredFlags(t *testing.T) {
	if len(RequiredFlags) == 0 {
		t.Error("RequiredFlags should not be empty")
	}

	// Check all expected flags are present
	expected := []string{
		"--dangerously-skip-permissions",
		"--permission-mode",
		"bypassPermissions",
		"--verbose",
		"--output-format",
		"stream-json",
	}

	for _, exp := range expected {
		found := false
		for _, f := range RequiredFlags {
			if f == exp {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Expected flag %q not found in RequiredFlags", exp)
		}
	}
}

func TestFlagSet(t *testing.T) {
	flagSet := FlagSet()
	if flagSet == "" {
		t.Error("FlagSet should not be empty")
	}

	for _, f := range RequiredFlags {
		if !strings.Contains(flagSet, f) {
			t.Errorf("FlagSet missing %q", f)
		}
	}
}

func TestFlagSetWithPrefix(t *testing.T) {
	flagSet := FlagSetWithPrefix()
	if !strings.HasPrefix(flagSet, "-p ") {
		t.Error("FlagSetWithPrefix should start with -p")
	}

	for _, f := range RequiredFlags {
		if !strings.Contains(flagSet, f) {
			t.Errorf("FlagSetWithPrefix missing %q", f)
		}
	}
}

func TestShellExport(t *testing.T) {
	export := ShellExport()

	// Must contain every flag from RequiredFlags (derived, not hardcoded)
	for _, f := range RequiredFlags {
		if !strings.Contains(export, f) {
			t.Errorf("ShellExport missing flag %q", f)
		}
	}

	// Must set BB_CLAUDE_FLAGS and export it
	if !strings.Contains(export, "BB_CLAUDE_FLAGS=") {
		t.Error("ShellExport must set BB_CLAUDE_FLAGS")
	}
	if !strings.Contains(export, "export BB_CLAUDE_FLAGS") {
		t.Error("ShellExport must export BB_CLAUDE_FLAGS")
	}

	// Verify it uses FlagSet() output (single source of truth)
	if !strings.Contains(export, FlagSet()) {
		t.Error("ShellExport must derive flags from FlagSet()")
	}
}

func TestHasRequiredFlag(t *testing.T) {
	tests := []struct {
		flag string
		want bool
	}{
		{"--dangerously-skip-permissions", true},
		{"--verbose", true},
		{"--output-format", true},
		{"stream-json", true},
		{"--unknown-flag", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.flag, func(t *testing.T) {
			if got := HasRequiredFlag(tt.flag); got != tt.want {
				t.Errorf("HasRequiredFlag(%q) = %v, want %v", tt.flag, got, tt.want)
			}
		})
	}
}

func TestValidateFlags(t *testing.T) {
	// Valid case - all flags present
	err := ValidateFlags(RequiredFlags)
	if err != nil {
		t.Errorf("ValidateFlags with all flags should pass, got: %v", err)
	}

	// Invalid case - missing flags
	missingFlags := []string{"--dangerously-skip-permissions"}
	err = ValidateFlags(missingFlags)
	if err == nil {
		t.Error("ValidateFlags with missing flags should return error")
	}

	// Empty flags
	err = ValidateFlags([]string{})
	if err == nil {
		t.Error("ValidateFlags with empty flags should return error")
	}
}
