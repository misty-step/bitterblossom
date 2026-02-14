package claude

import (
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
	
	// Should contain all required flags
	for _, f := range RequiredFlags {
		if !contains(flagSet, f) {
			t.Errorf("FlagSet missing %q", f)
		}
	}
}

func TestFlagSetWithPrefix(t *testing.T) {
	flagSet := FlagSetWithPrefix()
	if !contains(flagSet, "-p ") {
		t.Error("FlagSetWithPrefix should start with -p ")
	}
	
	// Should contain all required flags
	for _, f := range RequiredFlags {
		if !contains(flagSet, f) {
			t.Errorf("FlagSetWithPrefix missing %q", f)
		}
	}
}

func TestHasRequiredFlag(t *testing.T) {
	tests := []struct {
		flag    string
		want    bool
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

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsAt(s, substr))
}

func containsAt(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
