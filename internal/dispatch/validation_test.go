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
