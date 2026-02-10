package main

import (
	"bytes"
	"context"
	"strings"
	"testing"

	dispatchsvc "github.com/misty-step/bitterblossom/internal/dispatch"
	"github.com/misty-step/bitterblossom/pkg/fly"
)

type fakeDispatchRunner struct {
	lastReq dispatchsvc.Request
	result  dispatchsvc.Result
	err     error
}

func (f *fakeDispatchRunner) Run(_ context.Context, req dispatchsvc.Request) (dispatchsvc.Result, error) {
	f.lastReq = req
	return f.result, f.err
}

type fakeFlyClient struct{}

func (fakeFlyClient) Create(context.Context, fly.CreateRequest) (fly.Machine, error) {
	return fly.Machine{}, nil
}
func (fakeFlyClient) Destroy(context.Context, string, string) error { return nil }
func (fakeFlyClient) List(context.Context, string) ([]fly.Machine, error) {
	return nil, nil
}
func (fakeFlyClient) Status(context.Context, string, string) (fly.Machine, error) {
	return fly.Machine{}, nil
}
func (fakeFlyClient) Exec(context.Context, string, string, fly.ExecRequest) (fly.ExecResult, error) {
	return fly.ExecResult{}, nil
}

func TestDispatchCommandJSONOutput(t *testing.T) {
	t.Parallel()

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
			return runner, nil
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
		"--json",
		"--app", "bb-app",
		"--token", "tok",
	})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("cmd.Execute() error = %v", err)
	}

	if !strings.Contains(out.String(), `"executed": true`) {
		t.Fatalf("output = %q, expected json payload", out.String())
	}
	if runner.lastReq.Sprite != "bramble" {
		t.Fatalf("runner.lastReq.Sprite = %q, want bramble", runner.lastReq.Sprite)
	}
	if runner.lastReq.Prompt != "Fix flaky tests" {
		t.Fatalf("runner.lastReq.Prompt = %q, want provided prompt", runner.lastReq.Prompt)
	}
	if !runner.lastReq.Execute {
		t.Fatalf("runner.lastReq.Execute = false, want true")
	}
}

func TestDispatchCommandMissingFLY_APP(t *testing.T) {
	t.Parallel()

	deps := dispatchDeps{
		readFile: func(string) ([]byte, error) { return nil, nil },
		newFlyClient: func(token, apiURL string) (fly.MachineClient, error) {
			return fakeFlyClient{}, nil
		},
		newRemote: func(binary, org string) *spriteCLIRemote {
			return &spriteCLIRemote{}
		},
		newService: func(cfg dispatchsvc.Config) (dispatchRunner, error) {
			return &fakeDispatchRunner{}, nil
		},
	}

	cmd := newDispatchCmdWithDeps(deps)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{
		"bramble",
		"test prompt",
		"--token", "tok",
	})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for missing FLY_APP, got nil")
	}
	if !strings.Contains(err.Error(), "FLY_APP environment variable is required") {
		t.Fatalf("error = %q, expected FLY_APP error message", err.Error())
	}
	if !strings.Contains(err.Error(), "export FLY_APP=sprites-main") {
		t.Fatalf("error = %q, expected example export command", err.Error())
	}
}

func TestDispatchCommandMissingFLY_API_TOKEN(t *testing.T) {
	t.Parallel()

	deps := dispatchDeps{
		readFile: func(string) ([]byte, error) { return nil, nil },
		newFlyClient: func(token, apiURL string) (fly.MachineClient, error) {
			return fakeFlyClient{}, nil
		},
		newRemote: func(binary, org string) *spriteCLIRemote {
			return &spriteCLIRemote{}
		},
		newService: func(cfg dispatchsvc.Config) (dispatchRunner, error) {
			return &fakeDispatchRunner{}, nil
		},
	}

	cmd := newDispatchCmdWithDeps(deps)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{
		"bramble",
		"test prompt",
		"--app", "bb-app",
	})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for missing FLY_API_TOKEN, got nil")
	}
	if !strings.Contains(err.Error(), "FLY_API_TOKEN environment variable is required") {
		t.Fatalf("error = %q, expected FLY_API_TOKEN error message", err.Error())
	}
	if !strings.Contains(err.Error(), "fly.io/user/personal_access_tokens") {
		t.Fatalf("error = %q, expected URL to token page", err.Error())
	}
}

func TestDispatchCommandUsesPromptFile(t *testing.T) {
	t.Parallel()

	runner := &fakeDispatchRunner{
		result: dispatchsvc.Result{
			Executed: false,
			State:    dispatchsvc.StatePending,
			Plan: dispatchsvc.Plan{
				Sprite: "fern",
				Mode:   "dry-run",
			},
		},
	}

	deps := dispatchDeps{
		readFile: func(path string) ([]byte, error) {
			if path != "prompt.md" {
				t.Fatalf("unexpected prompt file path: %s", path)
			}
			return []byte("Prompt from file"), nil
		},
		newFlyClient: func(token, apiURL string) (fly.MachineClient, error) {
			return fakeFlyClient{}, nil
		},
		newRemote: func(binary, org string) *spriteCLIRemote {
			return &spriteCLIRemote{}
		},
		newService: func(cfg dispatchsvc.Config) (dispatchRunner, error) {
			return runner, nil
		},
	}

	cmd := newDispatchCmdWithDeps(deps)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{
		"fern",
		"--file", "prompt.md",
		"--app", "bb-app",
		"--token", "tok",
	})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("cmd.Execute() error = %v", err)
	}
	if runner.lastReq.Prompt != "Prompt from file" {
		t.Fatalf("runner.lastReq.Prompt = %q, want prompt file content", runner.lastReq.Prompt)
	}
	if runner.lastReq.Execute {
		t.Fatalf("runner.lastReq.Execute = true, want dry-run")
	}
}
