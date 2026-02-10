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

func TestDispatchCommandWithTaskFlag(t *testing.T) {
	t.Parallel()

	runner := &fakeDispatchRunner{
		result: dispatchsvc.Result{
			Executed: false,
			State:    dispatchsvc.StatePending,
			Plan: dispatchsvc.Plan{
				Sprite: "oak",
				Mode:   "dry-run",
			},
			Task: "Fix authentication bug",
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
		"oak",
		"Detailed prompt about fixing auth bug in the login flow",
		"--task", "Fix authentication bug",
		"--app", "bb-app",
		"--token", "tok",
	})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("cmd.Execute() error = %v", err)
	}
	if runner.lastReq.Task != "Fix authentication bug" {
		t.Fatalf("runner.lastReq.Task = %q, want 'Fix authentication bug'", runner.lastReq.Task)
	}
	if runner.lastReq.Prompt != "Detailed prompt about fixing auth bug in the login flow" {
		t.Fatalf("runner.lastReq.Prompt = %q, want full prompt", runner.lastReq.Prompt)
	}
}
