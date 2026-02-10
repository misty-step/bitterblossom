package dispatch

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/misty-step/bitterblossom/pkg/fly"
)

func TestValidateNoDirectAnthropic(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		key     string
		wantErr bool
	}{
		{name: "empty key is allowed", key: "", wantErr: false},
		{name: "proxy-mode is allowed", key: "proxy-mode", wantErr: false},
		{name: "arbitrary non-sk-ant value is allowed", key: "some-other-value", wantErr: false},
		{name: "sk-ant prefix is blocked", key: "sk-ant-abc123", wantErr: true},
		{name: "sk-ant with whitespace is blocked", key: "  sk-ant-abc123  ", wantErr: true},
		{name: "sk-ant- exact prefix required", key: "sk-antenna", wantErr: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := ValidateNoDirectAnthropic(tc.key)
			if tc.wantErr && err == nil {
				t.Fatal("expected error, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tc.wantErr && !errors.Is(err, ErrAnthropicKeyDetected) {
				t.Fatalf("error = %v, want ErrAnthropicKeyDetected", err)
			}
		})
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
	if !errors.Is(runErr, ErrAnthropicKeyDetected) {
		t.Fatalf("error = %v, want ErrAnthropicKeyDetected", runErr)
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
	// Should not have called printenv
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
