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
