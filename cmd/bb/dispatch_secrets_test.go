package main

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	dispatchsvc "github.com/misty-step/bitterblossom/internal/dispatch"
	"github.com/misty-step/bitterblossom/pkg/fly"
)

func TestDispatchCommand_SecretFlag(t *testing.T) {
	// Cannot use t.Parallel() — t.Setenv modifies process environment.
	t.Setenv("GITHUB_TOKEN", "ghp-test")
	t.Setenv("OPENROUTER_API_KEY", "or-test")
	t.Setenv("MY_SECRET_VAR", "resolved-env-value")

	var capturedEnvVars map[string]string
	runner := &fakeDispatchRunner{
		result: dispatchsvc.Result{
			Executed: true,
			State:    dispatchsvc.StateCompleted,
			Plan: dispatchsvc.Plan{
				Sprite: "bramble",
				Mode:   "execute",
				Steps:  []dispatchsvc.PlanStep{{Kind: dispatchsvc.StepStartAgent, Description: "start"}},
			},
		},
	}

	deps := dispatchDeps{
		readFile: func(string) ([]byte, error) { return nil, nil },
		newFlyClient: func(token, apiURL string) (fly.MachineClient, error) {
			return fakeFlyClient{}, nil
		},
		newRemote: func(binary, org string) *spriteCLIRemote {
			return &spriteCLIRemote{}
		},
		newService: func(cfg dispatchsvc.Config) (dispatchRunner, error) {
			capturedEnvVars = cfg.EnvVars
			return runner, nil
		},
		pollSprite: func(ctx context.Context, remote *spriteCLIRemote, sprite string, timeout time.Duration, progress func(string)) (*waitResult, error) {
			return nil, nil
		},
	}

	cmd := newDispatchCmdWithDeps(deps)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{
		"bramble",
		"Fix flaky tests",
		"--execute",
		"--secret", "API_KEY=direct-secret-value",
		"--secret", "CONFIG_VAR=${env:MY_SECRET_VAR}",
		"--app", "bb-app",
		"--token", "tok",
	})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("cmd.Execute() error = %v", err)
	}

	// Check that secrets were resolved and injected into env vars
	if capturedEnvVars == nil {
		t.Fatal("expected env vars to be captured")
	}
	if capturedEnvVars["API_KEY"] != "direct-secret-value" {
		t.Errorf("API_KEY = %q, want direct-secret-value", capturedEnvVars["API_KEY"])
	}
	if capturedEnvVars["CONFIG_VAR"] != "resolved-env-value" {
		t.Errorf("CONFIG_VAR = %q, want resolved-env-value", capturedEnvVars["CONFIG_VAR"])
	}

	// Check standard auth vars are still present
	if capturedEnvVars["GITHUB_TOKEN"] != "ghp-test" {
		t.Errorf("GITHUB_TOKEN = %q, want ghp-test", capturedEnvVars["GITHUB_TOKEN"])
	}
	if capturedEnvVars["OPENROUTER_API_KEY"] != "or-test" {
		t.Errorf("OPENROUTER_API_KEY = %q, want or-test", capturedEnvVars["OPENROUTER_API_KEY"])
	}
}

func TestDispatchCommand_SecretFlag_InvalidFormat(t *testing.T) {
	// Cannot use t.Parallel() — t.Setenv modifies process environment.
	t.Setenv("GITHUB_TOKEN", "ghp-test")
	t.Setenv("OPENROUTER_API_KEY", "or-test")

	deps := dispatchDeps{
		readFile: func(string) ([]byte, error) { return nil, nil },
		newFlyClient: func(token, apiURL string) (fly.MachineClient, error) {
			return fakeFlyClient{}, nil
		},
		newRemote: func(binary, org string) *spriteCLIRemote {
			return &spriteCLIRemote{}
		},
		newService: func(cfg dispatchsvc.Config) (dispatchRunner, error) {
			t.Fatal("newService should not be called with invalid secret flag")
			return nil, nil
		},
	}

	cmd := newDispatchCmdWithDeps(deps)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{
		"bramble",
		"Fix tests",
		"--execute",
		"--secret", "INVALID_NO_EQUALS",
		"--app", "bb-app",
		"--token", "tok",
	})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for invalid secret flag format")
	}
	if !strings.Contains(err.Error(), "secret") {
		t.Errorf("expected error to mention 'secret', got: %v", err)
	}
}

func TestDispatchCommand_SecretFlag_UnsetEnvVar(t *testing.T) {
	// Cannot use t.Parallel() — t.Setenv modifies process environment.
	t.Setenv("GITHUB_TOKEN", "ghp-test")
	t.Setenv("OPENROUTER_API_KEY", "or-test")
	// Ensure UNSET_VAR is not set
	t.Setenv("UNSET_VAR", "")

	deps := dispatchDeps{
		readFile: func(string) ([]byte, error) { return nil, nil },
		newFlyClient: func(token, apiURL string) (fly.MachineClient, error) {
			return fakeFlyClient{}, nil
		},
		newRemote: func(binary, org string) *spriteCLIRemote {
			return &spriteCLIRemote{}
		},
		newService: func(cfg dispatchsvc.Config) (dispatchRunner, error) {
			t.Fatal("newService should not be called with unresolved secret")
			return nil, nil
		},
	}

	cmd := newDispatchCmdWithDeps(deps)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{
		"bramble",
		"Fix tests",
		"--execute",
		"--secret", "API_KEY=${env:UNSET_VAR}",
		"--app", "bb-app",
		"--token", "tok",
	})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for unset env var in secret flag")
	}
	if !strings.Contains(err.Error(), "UNSET_VAR") {
		t.Errorf("expected error to mention 'UNSET_VAR', got: %v", err)
	}
}