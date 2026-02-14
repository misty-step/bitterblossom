package dispatch

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/misty-step/bitterblossom/pkg/fly"
)

func TestValidateNoDirectAnthropic_EmptyKey(t *testing.T) {
	env := map[string]string{"ANTHROPIC_API_KEY": ""}
	if err := ValidateNoDirectAnthropic(env, false); err != nil {
		t.Fatalf("expected no error for empty key, got: %v", err)
	}
}

func TestValidateNoDirectAnthropic_ProxyMode(t *testing.T) {
	env := map[string]string{"ANTHROPIC_API_KEY": "proxy-mode"}
	if err := ValidateNoDirectAnthropic(env, false); err != nil {
		t.Fatalf("expected no error for proxy-mode, got: %v", err)
	}
}

func TestValidateNoDirectAnthropic_Unset(t *testing.T) {
	env := map[string]string{}
	if err := ValidateNoDirectAnthropic(env, false); err != nil {
		t.Fatalf("expected no error for unset key, got: %v", err)
	}
}

func TestValidateNoDirectAnthropic_RealKey_Blocked(t *testing.T) {
	env := map[string]string{"ANTHROPIC_API_KEY": "sk-ant-api03-abcdef123456"}
	err := ValidateNoDirectAnthropic(env, false)
	if err == nil {
		t.Fatal("expected error for real sk-ant- key")
	}
	var keyErr *ErrDirectAnthropicKey
	if !errors.As(err, &keyErr) {
		t.Fatalf("expected ErrDirectAnthropicKey, got %T: %v", err, err)
	}
	if keyErr.KeyPrefix != "sk-ant-api03" {
		t.Fatalf("expected prefix 'sk-ant-api03', got %q", keyErr.KeyPrefix)
	}
}

func TestValidateNoDirectAnthropic_RealKey_AllowDirect(t *testing.T) {
	env := map[string]string{"ANTHROPIC_API_KEY": "sk-ant-api03-abcdef123456"}
	if err := ValidateNoDirectAnthropic(env, true); err != nil {
		t.Fatalf("expected no error with allowDirect=true, got: %v", err)
	}
}

func TestValidateNoDirectAnthropic_NonAnthropicKey(t *testing.T) {
	env := map[string]string{"ANTHROPIC_API_KEY": "some-other-value"}
	if err := ValidateNoDirectAnthropic(env, false); err != nil {
		t.Fatalf("expected no error for non-anthropic key, got: %v", err)
	}
}

func TestContainsSecret(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		input string
		want bool
	}{
		{"anthropic key", "FOO=sk-ant-api03-abcdef123456 bar", true},
		{"anthropic key inline", "export ANTHROPIC_API_KEY=sk-ant-api03-xyz", true},
		{"openrouter key", "sk-or-v1-deadbeef01234567 baz", true},
		{"github pat", "ghp_abcDEF1234567890abcDEF1234567890abcd", true},
		{"github pat short", "ghp_abcd is enough to match", true},
		{"false positive sk-ants", "sk-ants-are-cool", false},
		{"false positive sk-ant without api", "sk-ant- is not enough", false},
		{"false positive sk-or short", "sk-or-v1-abc is too short", false},
		{"false positive ghp short", "ghp_ alone", false},
		{"safe command sourcing env", "source /home/sprite/.env-proxy && run", false},
		{"safe bash expansion", "${OPENROUTER_API_KEY}", false},
		{"empty string", "", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ContainsSecret(tc.input)
			if got != tc.want {
				t.Fatalf("ContainsSecret(%q) = %v, want %v", tc.input, got, tc.want)
			}
		})
	}
}

func TestValidateCommandNoSecrets(t *testing.T) {
	t.Parallel()

	if err := ValidateCommandNoSecrets("echo hello", "test"); err != nil {
		t.Fatalf("expected no error for clean command, got: %v", err)
	}

	err := ValidateCommandNoSecrets("echo sk-ant-api03-abcdef123456", "start command")
	if err == nil {
		t.Fatal("expected error for command with secret")
	}
	var secretErr *ErrSecretInCommand
	if !errors.As(err, &secretErr) {
		t.Fatalf("expected *ErrSecretInCommand, got %T: %v", err, err)
	}
	if secretErr.Context != "start command" {
		t.Fatalf("context = %q, want %q", secretErr.Context, "start command")
	}
}

func TestRunBlocksDispatchWithRealAnthropicKey(t *testing.T) {
	t.Parallel()

	remote := &fakeRemote{
		execResponses: []string{"sk-ant-abc123"}, // printenv returns real key
	}
	flyClient := &fakeFly{
		listMachines: []fly.Machine{{Name: "fern", ID: "m1"}},
	}

	service, err := NewService(Config{
		Remote:    remote,
		Fly:       flyClient,
		App:       "bb-app",
		Workspace: "/home/sprite/workspace",
		Now:       func() time.Time { return time.Now() },
	})
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	_, runErr := service.Run(context.Background(), Request{
		Sprite:  "fern",
		Prompt:  "Fix tests",
		Execute: true,
	})
	if runErr == nil {
		t.Fatal("expected error, got nil")
	}
	var keyErr *ErrDirectAnthropicKey
	if !errors.As(runErr, &keyErr) {
		t.Fatalf("error = %v (%T), want *ErrDirectAnthropicKey", runErr, runErr)
	}
}

func TestRunAllowsDispatchWithEmptyKey(t *testing.T) {
	t.Parallel()

	remote := &fakeRemote{
		execResponses: []string{
			"",     // printenv returns empty
			"done", // oneshot agent
		},
	}
	flyClient := &fakeFly{
		listMachines: []fly.Machine{{Name: "fern", ID: "m1"}},
	}

	service, err := NewService(Config{
		Remote:    remote,
		Fly:       flyClient,
		App:       "bb-app",
		Workspace: "/home/sprite/workspace",
	})
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	result, runErr := service.Run(context.Background(), Request{
		Sprite:  "fern",
		Prompt:  "Fix tests",
		Execute: true,
	})
	if runErr != nil {
		t.Fatalf("Run() error = %v", runErr)
	}
	if result.State != StateCompleted {
		t.Fatalf("state = %q, want %q", result.State, StateCompleted)
	}
}

func TestRunAllowsDirectKeyWithEscapeHatch(t *testing.T) {
	t.Parallel()

	remote := &fakeRemote{
		execResponses: []string{"done"}, // no env check — straight to agent
	}
	flyClient := &fakeFly{
		listMachines: []fly.Machine{{Name: "fern", ID: "m1"}},
	}

	service, err := NewService(Config{
		Remote:    remote,
		Fly:       flyClient,
		App:       "bb-app",
		Workspace: "/home/sprite/workspace",
	})
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	result, runErr := service.Run(context.Background(), Request{
		Sprite:               "fern",
		Prompt:               "Fix tests",
		Execute:              true,
		AllowAnthropicDirect: true,
	})
	if runErr != nil {
		t.Fatalf("Run() error = %v", runErr)
	}
	if result.State != StateCompleted {
		t.Fatalf("state = %q, want %q", result.State, StateCompleted)
	}
	for _, call := range remote.execCalls {
		if strings.Contains(call.command, "printenv ANTHROPIC_API_KEY") {
			t.Fatal("escape hatch should skip env validation")
		}
	}
}

func TestRunAllowsProxyModeKey(t *testing.T) {
	t.Parallel()

	remote := &fakeRemote{
		execResponses: []string{
			"proxy-mode", // printenv returns proxy-mode
			"done",       // oneshot agent
		},
	}
	flyClient := &fakeFly{
		listMachines: []fly.Machine{{Name: "fern", ID: "m1"}},
	}

	service, err := NewService(Config{
		Remote:    remote,
		Fly:       flyClient,
		App:       "bb-app",
		Workspace: "/home/sprite/workspace",
	})
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	result, runErr := service.Run(context.Background(), Request{
		Sprite:  "fern",
		Prompt:  "Fix tests",
		Execute: true,
	})
	if runErr != nil {
		t.Fatalf("Run() error = %v", runErr)
	}
	if result.State != StateCompleted {
		t.Fatalf("state = %q, want %q", result.State, StateCompleted)
	}
}

// Validation Profile Tests

func TestValidationProfile_IsValid(t *testing.T) {
	t.Parallel()

	tests := []struct {
		profile ValidationProfile
		want    bool
	}{
		{ValidationProfileAdvisory, true},
		{ValidationProfileStrict, true},
		{ValidationProfileOff, true},
		{ValidationProfile("unknown"), false},
		{ValidationProfile(""), false},
	}

	for _, tc := range tests {
		t.Run(string(tc.profile), func(t *testing.T) {
			if got := tc.profile.IsValid(); got != tc.want {
				t.Errorf("IsValid() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestParseValidationProfile(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input    string
		expected ValidationProfile
	}{
		{"advisory", ValidationProfileAdvisory},
		{"ADVISORY", ValidationProfileAdvisory},
		{"Advisory", ValidationProfileAdvisory},
		{"strict", ValidationProfileStrict},
		{"STRICT", ValidationProfileStrict},
		{"Strict", ValidationProfileStrict},
		{"off", ValidationProfileOff},
		{"OFF", ValidationProfileOff},
		{"Off", ValidationProfileOff},
		{"", ValidationProfileAdvisory},
		{"unknown", ValidationProfileAdvisory},
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			result := ParseValidationProfile(tc.input)
			if result != tc.expected {
				t.Errorf("ParseValidationProfile(%q) = %q, want %q", tc.input, result, tc.expected)
			}
		})
	}
}

func TestSafetyValidator_ValidateSafety(t *testing.T) {
	t.Parallel()

	validator := DefaultSafetyValidator()

	tests := []struct {
		name     string
		req      Request
		wantSafe bool
	}{
		{
			name: "valid request",
			req: Request{
				Sprite: "test-sprite",
				Prompt: "fix the bug",
				Repo:   "misty-step/test",
			},
			wantSafe: true,
		},
		{
			name: "missing sprite",
			req: Request{
				Sprite: "",
				Prompt: "fix the bug",
				Repo:   "misty-step/test",
			},
			wantSafe: false,
		},
		{
			name: "invalid repo format",
			req: Request{
				Sprite: "test-sprite",
				Prompt: "fix the bug",
				Repo:   "invalid-repo-format",
			},
			wantSafe: false,
		},
		{
			name: "prompt with secret",
			req: Request{
				Sprite: "test-sprite",
				Prompt: "use key sk-ant-api03-abcdef123456",
				Repo:   "misty-step/test",
			},
			wantSafe: false,
		},
		{
			name:     "empty repo is valid",
			req:      Request{Sprite: "test-sprite", Prompt: "fix", Repo: ""},
			wantSafe: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := validator.ValidateSafety(tc.req)
			if result.Valid != tc.wantSafe {
				t.Errorf("ValidateSafety() valid = %v, want %v; errors: %v", result.Valid, tc.wantSafe, result.Errors)
			}
		})
	}
}

func TestSafetyValidator_ValidateSafetyWithEnv(t *testing.T) {
	t.Parallel()

	validator := DefaultSafetyValidator()

	tests := []struct {
		name        string
		req         Request
		env         map[string]string
		allowDirect bool
		wantSafe    bool
	}{
		{
			name:        "valid with proxy mode",
			req:         Request{Sprite: "test", Prompt: "fix", Repo: "misty-step/test"},
			env:         map[string]string{"ANTHROPIC_API_KEY": "proxy-mode"},
			allowDirect: false,
			wantSafe:    true,
		},
		{
			name:        "direct key blocked",
			req:         Request{Sprite: "test", Prompt: "fix", Repo: "misty-step/test"},
			env:         map[string]string{"ANTHROPIC_API_KEY": "sk-ant-api03-abcdef123456"},
			allowDirect: false,
			wantSafe:    false,
		},
		{
			name:        "direct key allowed with escape hatch",
			req:         Request{Sprite: "test", Prompt: "fix", Repo: "misty-step/test"},
			env:         map[string]string{"ANTHROPIC_API_KEY": "sk-ant-api03-abcdef123456"},
			allowDirect: true,
			wantSafe:    true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := validator.ValidateSafetyWithEnv(context.Background(), tc.req, tc.env, tc.allowDirect)
			if result.Valid != tc.wantSafe {
				t.Errorf("ValidateSafetyWithEnv() valid = %v, want %v; errors: %v", result.Valid, tc.wantSafe, result.Errors)
			}
		})
	}
}

func TestCombinedValidationResult_IsSafe(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		result CombinedValidationResult
		want   bool
	}{
		{
			name:   "safe and valid",
			result: CombinedValidationResult{Safety: SafetyCheckResult{Valid: true}},
			want:   true,
		},
		{
			name:   "unsafe with errors",
			result: CombinedValidationResult{Safety: SafetyCheckResult{Valid: false, Errors: []string{"error"}}},
			want:   false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.result.IsSafe(); got != tc.want {
				t.Errorf("IsSafe() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestCombinedValidationResult_IsPolicyCompliant(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		result  CombinedValidationResult
		profile ValidationProfile
		want    bool
	}{
		{
			name:    "off profile - always compliant",
			result:  CombinedValidationResult{Policy: PolicyCheckResult{Errors: []string{"error"}, Warnings: []string{"warning"}}},
			profile: ValidationProfileOff,
			want:    true,
		},
		{
			name:    "advisory - errors fail",
			result:  CombinedValidationResult{Policy: PolicyCheckResult{Errors: []string{"error"}}},
			profile: ValidationProfileAdvisory,
			want:    false,
		},
		{
			name:    "advisory - warnings ok",
			result:  CombinedValidationResult{Policy: PolicyCheckResult{Warnings: []string{"warning"}}},
			profile: ValidationProfileAdvisory,
			want:    true,
		},
		{
			name:    "strict - errors fail",
			result:  CombinedValidationResult{Policy: PolicyCheckResult{Errors: []string{"error"}}},
			profile: ValidationProfileStrict,
			want:    false,
		},
		{
			name:    "strict - warnings fail",
			result:  CombinedValidationResult{Policy: PolicyCheckResult{Warnings: []string{"warning"}}},
			profile: ValidationProfileStrict,
			want:    false,
		},
		{
			name:    "strict - clean passes",
			result:  CombinedValidationResult{Policy: PolicyCheckResult{}},
			profile: ValidationProfileStrict,
			want:    true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.result.IsPolicyCompliant(tc.profile); got != tc.want {
				t.Errorf("IsPolicyCompliant(%q) = %v, want %v", tc.profile, got, tc.want)
			}
		})
	}
}

func TestCombinedValidationResult_HasIssues(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		result CombinedValidationResult
		want   bool
	}{
		{
			name:   "no issues",
			result: CombinedValidationResult{Safety: SafetyCheckResult{Valid: true}},
			want:   false,
		},
		{
			name:   "safety errors",
			result: CombinedValidationResult{Safety: SafetyCheckResult{Errors: []string{"err"}}},
			want:   true,
		},
		{
			name:   "policy warnings only",
			result: CombinedValidationResult{Safety: SafetyCheckResult{Valid: true}, Policy: PolicyCheckResult{Warnings: []string{"warn"}}},
			want:   true,
		},
		{
			name:   "policy errors only",
			result: CombinedValidationResult{Safety: SafetyCheckResult{Valid: true}, Policy: PolicyCheckResult{Errors: []string{"err"}}},
			want:   true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.result.HasIssues(); got != tc.want {
				t.Errorf("HasIssues() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestCombinedValidationResult_ToError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		result    CombinedValidationResult
		profile   ValidationProfile
		wantErr   bool
		wantInErr string
	}{
		{
			name:    "valid result",
			result:  CombinedValidationResult{Safety: SafetyCheckResult{Valid: true}},
			profile: ValidationProfileAdvisory,
			wantErr: false,
		},
		{
			name:      "safety error",
			result:    CombinedValidationResult{Safety: SafetyCheckResult{Valid: false, Errors: []string{"sprite required"}}},
			profile:   ValidationProfileAdvisory,
			wantErr:   true,
			wantInErr: "Safety errors",
		},
		{
			name:    "advisory - warnings only - no error",
			result:  CombinedValidationResult{Safety: SafetyCheckResult{Valid: true}, Policy: PolicyCheckResult{Warnings: []string{"short description"}}},
			profile: ValidationProfileAdvisory,
			wantErr: false,
		},
		{
			name:    "advisory - policy error",
			result:  CombinedValidationResult{Safety: SafetyCheckResult{Valid: true}, Policy: PolicyCheckResult{Errors: []string{"missing label"}}},
			profile: ValidationProfileAdvisory,
			wantErr: true,
		},
		{
			name:    "strict - warnings fail",
			result:  CombinedValidationResult{Safety: SafetyCheckResult{Valid: true}, Policy: PolicyCheckResult{Warnings: []string{"short description"}}},
			profile: ValidationProfileStrict,
			wantErr: true,
		},
		{
			name:    "off - errors ignored",
			result:  CombinedValidationResult{Safety: SafetyCheckResult{Valid: true}, Policy: PolicyCheckResult{Errors: []string{"closed issue"}}},
			profile: ValidationProfileOff,
			wantErr: false,
		},
		{
			name:      "off - safety still enforced",
			result:    CombinedValidationResult{Safety: SafetyCheckResult{Valid: false, Errors: []string{"missing sprite"}}},
			profile:   ValidationProfileOff,
			wantErr:   true,
			wantInErr: "Safety errors",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.result.ToError(tc.profile)
			if (err != nil) != tc.wantErr {
				t.Errorf("ToError(%q) error = %v, wantErr %v", tc.profile, err, tc.wantErr)
			}
			if tc.wantInErr != "" && err != nil && !strings.Contains(err.Error(), tc.wantInErr) {
				t.Errorf("error should contain %q, got: %v", tc.wantInErr, err)
			}
		})
	}
}

func TestCombinedValidationResult_FormatReport(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		result  CombinedValidationResult
		profile ValidationProfile
		want    []string
		notWant []string
	}{
		{
			name:    "safety errors shown",
			result:  CombinedValidationResult{Safety: SafetyCheckResult{Errors: []string{"sprite required"}}},
			profile: ValidationProfileAdvisory,
			want:    []string{"Safety validation failed", "sprite required"},
		},
		{
			name:    "advisory warnings shown with warning marker",
			result:  CombinedValidationResult{Safety: SafetyCheckResult{Valid: true}, Policy: PolicyCheckResult{Warnings: []string{"short desc"}}},
			profile: ValidationProfileAdvisory,
			want:    []string{"Policy warnings:", "⚠ short desc"},
		},
		{
			name:    "strict warnings shown as errors",
			result:  CombinedValidationResult{Safety: SafetyCheckResult{Valid: true}, Policy: PolicyCheckResult{Warnings: []string{"short desc"}}},
			profile: ValidationProfileStrict,
			want:    []string{"strict mode", "✗ short desc"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			report := tc.result.FormatReport(tc.profile)
			for _, s := range tc.want {
				if !strings.Contains(report, s) {
					t.Errorf("FormatReport() should contain %q, got:\n%s", s, report)
				}
			}
			for _, s := range tc.notWant {
				if strings.Contains(report, s) {
					t.Errorf("FormatReport() should not contain %q, got:\n%s", s, report)
				}
			}
		})
	}
}
