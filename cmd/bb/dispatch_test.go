package main

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

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
	// Cannot use t.Parallel() — t.Setenv modifies process environment.
	t.Setenv("GITHUB_TOKEN", "ghp-test")
	t.Setenv("OPENROUTER_API_KEY", "or-test")

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

func TestDispatchCommandAllowsIssueWithoutPrompt(t *testing.T) {
	t.Parallel()

	runner := &fakeDispatchRunner{
		result: dispatchsvc.Result{
			Executed: false,
			State:    dispatchsvc.StatePending,
			Plan: dispatchsvc.Plan{
				Sprite: "bramble",
				Mode:   "dry-run",
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
		"--issue", "186",
		"--repo", "misty-step/bitterblossom",
		"--skip-validation",
		"--app", "bb-app",
		"--token", "tok",
	})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("cmd.Execute() error = %v", err)
	}
	if runner.lastReq.Sprite != "bramble" {
		t.Fatalf("runner.lastReq.Sprite = %q, want bramble", runner.lastReq.Sprite)
	}
	if runner.lastReq.Issue != 186 {
		t.Fatalf("runner.lastReq.Issue = %d, want 186", runner.lastReq.Issue)
	}
	if runner.lastReq.Repo != "misty-step/bitterblossom" {
		t.Fatalf("runner.lastReq.Repo = %q, want misty-step/bitterblossom", runner.lastReq.Repo)
	}
	if runner.lastReq.Prompt != "" {
		t.Fatalf("runner.lastReq.Prompt = %q, want empty (IssuePrompt synthesized in service)", runner.lastReq.Prompt)
	}
}

func TestDispatchCommandAutoAssignWithoutSprite(t *testing.T) {
	t.Parallel()

	runner := &fakeDispatchRunner{
		result: dispatchsvc.Result{
			Executed: false,
			State:    dispatchsvc.StatePending,
			Plan: dispatchsvc.Plan{
				Sprite: "moss",
				Mode:   "dry-run",
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
		selectSprite: func(ctx context.Context, remote *spriteCLIRemote, opts dispatchOptions) (string, error) {
			return "moss", nil
		},
	}

	cmd := newDispatchCmdWithDeps(deps)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{
		"--issue", "186",
		"--repo", "misty-step/bitterblossom",
		"--skip-validation",
		"--app", "bb-app",
		"--token", "tok",
	})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("cmd.Execute() error = %v", err)
	}
	if runner.lastReq.Sprite != "moss" {
		t.Fatalf("runner.lastReq.Sprite = %q, want moss", runner.lastReq.Sprite)
	}
}

func TestDispatchCommandWithoutFlyCredentials(t *testing.T) {
	t.Parallel()

	// Dispatch should succeed in dry-run mode without FLY_APP/FLY_API_TOKEN
	// since these are only needed for provisioning new sprites.
	runner := &fakeDispatchRunner{}
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
		"test prompt",
	})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("dry-run dispatch should succeed without Fly credentials, got: %v", err)
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
		pollSprite: func(ctx context.Context, remote *spriteCLIRemote, sprite string, timeout time.Duration, progress func(string)) (*waitResult, error) {
			return nil, nil
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

func TestDispatchCommandPassesSkills(t *testing.T) {
	t.Parallel()

	runner := &fakeDispatchRunner{
		result: dispatchsvc.Result{
			Executed: false,
			State:    dispatchsvc.StatePending,
			Plan: dispatchsvc.Plan{
				Sprite: "bramble",
				Mode:   "dry-run",
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
		"--skill", "base/skills/git-mastery",
		"--skill", "base/skills/testing-philosophy/SKILL.md",
		"--app", "bb-app",
		"--token", "tok",
	})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("cmd.Execute() error = %v", err)
	}

	if len(runner.lastReq.Skills) != 2 {
		t.Fatalf("runner.lastReq.Skills len = %d, want 2", len(runner.lastReq.Skills))
	}
	if runner.lastReq.Skills[0] != "base/skills/git-mastery" {
		t.Fatalf("runner.lastReq.Skills[0] = %q, want %q", runner.lastReq.Skills[0], "base/skills/git-mastery")
	}
	if runner.lastReq.Skills[1] != "base/skills/testing-philosophy/SKILL.md" {
		t.Fatalf("runner.lastReq.Skills[1] = %q, want %q", runner.lastReq.Skills[1], "base/skills/testing-philosophy/SKILL.md")
	}
}

func TestDispatchCommandWaitRequiresExecute(t *testing.T) {
	t.Parallel()

	deps := dispatchDeps{
		readFile:     func(string) ([]byte, error) { return nil, nil },
		newFlyClient: func(token, apiURL string) (fly.MachineClient, error) { return fakeFlyClient{}, nil },
		newRemote:    func(binary, org string) *spriteCLIRemote { return &spriteCLIRemote{} },
		newService:   func(cfg dispatchsvc.Config) (dispatchRunner, error) { return &fakeDispatchRunner{}, nil },
		pollSprite: func(ctx context.Context, remote *spriteCLIRemote, sprite string, timeout time.Duration, progress func(string)) (*waitResult, error) {
			return nil, nil
		},
	}

	cmd := newDispatchCmdWithDeps(deps)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{
		"fern",
		"Fix bug",
		"--wait", // --wait without --execute should fail
		"--app", "bb-app",
		"--token", "tok",
	})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for --wait without --execute, got nil")
	}
	if !strings.Contains(err.Error(), "--wait requires --execute") {
		t.Fatalf("expected error about --wait requiring --execute, got: %v", err)
	}
}

func TestDispatchCommandWithWait(t *testing.T) {
	// Cannot use t.Parallel() — t.Setenv modifies process environment.
	t.Setenv("GITHUB_TOKEN", "ghp-test")
	t.Setenv("OPENROUTER_API_KEY", "or-test")

	runner := &fakeDispatchRunner{
		result: dispatchsvc.Result{
			Executed: true,
			State:    dispatchsvc.StateRunning,
			AgentPID: 12345,
			Plan: dispatchsvc.Plan{
				Sprite: "moss",
				Mode:   "execute",
				Steps:  []dispatchsvc.PlanStep{{Kind: dispatchsvc.StepStartAgent, Description: "start"}},
			},
		},
	}

	expectedResult := &waitResult{
		State:      "completed",
		Task:       "Implement feature",
		Complete:   true,
		PRURL:      "https://github.com/misty-step/bitterblossom/pull/123",
		HasChanges: true,
		Commits:    1,
		PRs:        1,
	}

	pollCalled := false
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
		pollSprite: func(ctx context.Context, remote *spriteCLIRemote, sprite string, timeout time.Duration, progress func(string)) (*waitResult, error) {
			pollCalled = true
			if sprite != "moss" {
				t.Errorf("expected sprite 'moss', got %q", sprite)
			}
			return expectedResult, nil
		},
	}

	cmd := newDispatchCmdWithDeps(deps)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{
		"moss",
		"Implement feature",
		"--execute",
		"--wait",
		"--timeout", "5m",
		"--app", "bb-app",
		"--token", "tok",
	})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("cmd.Execute() error = %v", err)
	}

	if !pollCalled {
		t.Fatal("expected pollSprite to be called with --wait flag")
	}

	output := out.String()
	if !strings.Contains(output, "COMPLETE") {
		t.Fatalf("expected output to contain completion status, got: %s", output)
	}
	if !strings.Contains(output, "https://github.com/misty-step/bitterblossom/pull/123") {
		t.Fatalf("expected output to contain PR URL, got: %s", output)
	}
}

func TestDispatchCommandWaitWithPollingError(t *testing.T) {
	// Cannot use t.Parallel() — t.Setenv modifies process environment.
	t.Setenv("GITHUB_TOKEN", "ghp-test")
	t.Setenv("OPENROUTER_API_KEY", "or-test")

	runner := &fakeDispatchRunner{
		result: dispatchsvc.Result{
			Executed: true,
			State:    dispatchsvc.StateRunning,
			AgentPID: 12345,
			Plan: dispatchsvc.Plan{
				Sprite: "moss",
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
		pollSprite: func(ctx context.Context, remote *spriteCLIRemote, sprite string, timeout time.Duration, progress func(string)) (*waitResult, error) {
			return nil, errors.New("polling failed")
		},
	}

	cmd := newDispatchCmdWithDeps(deps)
	var out bytes.Buffer
	var errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs([]string{
		"moss",
		"Implement feature",
		"--execute",
		"--wait",
		"--app", "bb-app",
		"--token", "tok",
	})

	// Should return exit code 1 for polling/infrastructure failure
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for polling failure, got nil")
	}
	var coded *exitError
	if !errors.As(err, &coded) {
		t.Fatalf("expected exitError, got %T: %v", err, err)
	}
	if coded.Code != exitCodeDispatchFailure {
		t.Fatalf("expected exit code %d, got %d", exitCodeDispatchFailure, coded.Code)
	}

	// Should have warning in stderr
	errStr := errOut.String()
	if !strings.Contains(errStr, "polling failed") {
		t.Fatalf("expected warning about polling failure, got: %s", errStr)
	}
}

func TestDispatchCommandWaitWithContextDeadlineExceeded(t *testing.T) {
	// Cannot use t.Parallel() — t.Setenv modifies process environment.
	t.Setenv("GITHUB_TOKEN", "ghp-test")
	t.Setenv("OPENROUTER_API_KEY", "or-test")

	runner := &fakeDispatchRunner{
		result: dispatchsvc.Result{
			Executed: true,
			State:    dispatchsvc.StateRunning,
			Plan: dispatchsvc.Plan{
				Sprite: "moss",
				Mode:   "execute",
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
		pollSprite: func(ctx context.Context, remote *spriteCLIRemote, sprite string, timeout time.Duration, progress func(string)) (*waitResult, error) {
			// Simulate context.DeadlineExceeded - happens during initial delay
			return nil, context.DeadlineExceeded
		},
	}

	cmd := newDispatchCmdWithDeps(deps)
	var out bytes.Buffer
	var errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs([]string{
		"moss",
		"Implement feature",
		"--execute",
		"--wait",
		"--app", "bb-app",
		"--token", "tok",
	})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for context.DeadlineExceeded, got nil")
	}
	var coded *exitError
	if !errors.As(err, &coded) {
		t.Fatalf("expected exitError, got %T: %v", err, err)
	}
	// Should return exit code 124 (timeout), NOT 1 (dispatch failure)
	if coded.Code != exitCodeTimeout {
		t.Fatalf("expected exit code %d (timeout), got %d (dispatch failure)", exitCodeTimeout, coded.Code)
	}
}

func TestDispatchCommandWaitJSONOutput(t *testing.T) {
	// Cannot use t.Parallel() — t.Setenv modifies process environment.
	t.Setenv("GITHUB_TOKEN", "ghp-test")
	t.Setenv("OPENROUTER_API_KEY", "or-test")

	runner := &fakeDispatchRunner{
		result: dispatchsvc.Result{
			Executed: true,
			State:    dispatchsvc.StateRunning,
			Plan: dispatchsvc.Plan{
				Sprite: "moss",
				Mode:   "execute",
			},
		},
	}

	expectedResult := &waitResult{
		State:      "completed",
		Complete:   true,
		PRURL:      "https://github.com/misty-step/bitterblossom/pull/123",
		HasChanges: true,
		Commits:    1,
		PRs:        1,
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
		pollSprite: func(ctx context.Context, remote *spriteCLIRemote, sprite string, timeout time.Duration, progress func(string)) (*waitResult, error) {
			return expectedResult, nil
		},
	}

	cmd := newDispatchCmdWithDeps(deps)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{
		"moss",
		"Implement feature",
		"--execute",
		"--wait",
		"--json",
		"--app", "bb-app",
		"--token", "tok",
	})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("cmd.Execute() error = %v", err)
	}

	output := out.String()
	if !strings.Contains(output, `"wait"`) {
		t.Fatalf("expected JSON output to contain wait result, got: %s", output)
	}
	if !strings.Contains(output, `"pr_url"`) {
		t.Fatalf("expected JSON output to contain pr_url, got: %s", output)
	}
}

func TestParseStatusCheckOutput(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		output   string
		wantDone bool
		wantRes  *waitResult
	}{
		{
			name: "completed task",
			output: "__STATUS_JSON__{\"repo\":\"misty-step/bitterblossom\",\"started\":\"2024-01-15T10:00:00Z\",\"task\":\"Fix bug\"}\n" +
				"__AGENT_STATE__dead\n" +
				"__HAS_COMPLETE__yes\n" +
				"__HAS_BLOCKED__no\n" +
				"__BLOCKED_B64__\n" +
				"__PR_URL__https://github.com/misty-step/bitterblossom/pull/42\n",
			wantDone: true,
			wantRes: &waitResult{
				State:    "completed",
				Task:     "Fix bug",
				Repo:     "misty-step/bitterblossom",
				Started:  "2024-01-15T10:00:00Z",
				PRURL:    "https://github.com/misty-step/bitterblossom/pull/42",
				Complete: true,
			},
		},
		{
			name: "blocked task",
			output: "__STATUS_JSON__{\"task\":\"Refactor auth\"}\n" +
				"__AGENT_STATE__dead\n" +
				"__HAS_COMPLETE__no\n" +
				"__HAS_BLOCKED__yes\n" +
				"__BLOCKED_B64__QmxvY2tlZDogbmVlZCBwZXJtaXNzaW9ucw==\n" + // "Blocked: need permissions"
				"__PR_URL__\n",
			wantDone: true,
			wantRes: &waitResult{
				State:         "blocked",
				Task:          "Refactor auth",
				Blocked:       true,
				BlockedReason: "Blocked: need permissions",
				Complete:      true,
			},
		},
		{
			name: "running task",
			output: "__STATUS_JSON__{\"task\":\"Implement feature\"}\n" +
				"__AGENT_STATE__alive\n" +
				"__HAS_COMPLETE__no\n" +
				"__HAS_BLOCKED__no\n" +
				"__BLOCKED_B64__\n" +
				"__PR_URL__\n",
			wantDone: false,
			wantRes: &waitResult{
				State:    "running",
				Task:     "Implement feature",
				Complete: false,
			},
		},
		{
			name: "idle - no task",
			output: "__STATUS_JSON__{}\n" +
				"__AGENT_STATE__dead\n" +
				"__HAS_COMPLETE__no\n" +
				"__HAS_BLOCKED__no\n" +
				"__BLOCKED_B64__\n" +
				"__PR_URL__\n",
			wantDone: false,
			wantRes: &waitResult{
				State:    "idle",
				Complete: false,
			},
		},
		{
			name: "fallback completion - agent dead with PR URL but no TASK_COMPLETE",
			output: "__STATUS_JSON__{\"repo\":\"misty-step/bitterblossom\",\"started\":\"2024-01-15T10:00:00Z\",\"task\":\"Fix bug\"}\n" +
				"__AGENT_STATE__dead\n" +
				"__HAS_COMPLETE__no\n" +
				"__HAS_BLOCKED__no\n" +
				"__BLOCKED_B64__\n" +
				"__PR_URL__https://github.com/misty-step/bitterblossom/pull/42\n",
			wantDone: true,
			wantRes: &waitResult{
				State:    "completed",
				Task:     "Fix bug",
				Repo:     "misty-step/bitterblossom",
				Started:  "2024-01-15T10:00:00Z",
				PRURL:    "https://github.com/misty-step/bitterblossom/pull/42",
				Complete: true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, done, err := parseStatusCheckOutput(tt.output)
			if err != nil {
				t.Fatalf("parseStatusCheckOutput() error = %v", err)
			}
			if done != tt.wantDone {
				t.Errorf("done = %v, want %v", done, tt.wantDone)
			}
			if res.State != tt.wantRes.State {
				t.Errorf("State = %q, want %q", res.State, tt.wantRes.State)
			}
			if res.Task != tt.wantRes.Task {
				t.Errorf("Task = %q, want %q", res.Task, tt.wantRes.Task)
			}
			if res.Complete != tt.wantRes.Complete {
				t.Errorf("Complete = %v, want %v", res.Complete, tt.wantRes.Complete)
			}
			if res.Blocked != tt.wantRes.Blocked {
				t.Errorf("Blocked = %v, want %v", res.Blocked, tt.wantRes.Blocked)
			}
			if res.PRURL != tt.wantRes.PRURL {
				t.Errorf("PRURL = %q, want %q", res.PRURL, tt.wantRes.PRURL)
			}
			if res.Repo != tt.wantRes.Repo {
				t.Errorf("Repo = %q, want %q", res.Repo, tt.wantRes.Repo)
			}
		})
	}
}

func TestBuildStatusCheckScript(t *testing.T) {
	t.Parallel()

	script := buildStatusCheckScript("/home/sprite/workspace")

	// Check for expected components
	expectedComponents := []string{
		"STATUS_JSON",
		"AGENT_STATE",
		"HAS_COMPLETE",
		"HAS_BLOCKED",
		"BLOCKED_B64",
		"PR_URL",
		"TASK_COMPLETE",
		"TASK_COMPLETE.md",
		"BLOCKED.md",
	}

	for _, component := range expectedComponents {
		if !strings.Contains(script, component) {
			t.Errorf("script missing expected component: %s", component)
		}
	}
}

func TestDispatchCollectsGHToken(t *testing.T) {
	var capturedEnvVars map[string]string
	runner := &fakeDispatchRunner{
		result: dispatchsvc.Result{
			Executed: false,
			State:    dispatchsvc.StatePending,
			Plan:     dispatchsvc.Plan{Sprite: "bramble", Mode: "dry-run"},
		},
	}

	deps := dispatchDeps{
		readFile:     func(string) ([]byte, error) { return nil, nil },
		newFlyClient: func(token, apiURL string) (fly.MachineClient, error) { return fakeFlyClient{}, nil },
		newRemote:    func(binary, org string) *spriteCLIRemote { return &spriteCLIRemote{} },
		newService: func(cfg dispatchsvc.Config) (dispatchRunner, error) {
			capturedEnvVars = cfg.EnvVars
			return runner, nil
		},
	}

	t.Setenv("GH_TOKEN", "gh-test-token")
	t.Setenv("GITHUB_TOKEN", "github-test-token")

	cmd := newDispatchCmdWithDeps(deps)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{
		"bramble", "Fix tests",
		"--app", "bb-app",
		"--token", "tok",
	})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("cmd.Execute() error = %v", err)
	}
	if capturedEnvVars == nil {
		t.Fatal("newService was never called")
	}
	if v := capturedEnvVars["GH_TOKEN"]; v != "gh-test-token" {
		t.Fatalf("GH_TOKEN = %q, want %q", v, "gh-test-token")
	}
	if v := capturedEnvVars["GITHUB_TOKEN"]; v != "github-test-token" {
		t.Fatalf("GITHUB_TOKEN = %q, want %q", v, "github-test-token")
	}
}

func TestDispatchStripsAnthropicAPIKey(t *testing.T) {
	// Cannot use t.Parallel() — t.Setenv modifies process environment.

	var capturedEnvVars map[string]string
	runner := &fakeDispatchRunner{
		result: dispatchsvc.Result{
			Executed: false,
			State:    dispatchsvc.StatePending,
			Plan:     dispatchsvc.Plan{Sprite: "bramble", Mode: "dry-run"},
		},
	}

	deps := dispatchDeps{
		readFile:     func(string) ([]byte, error) { return nil, nil },
		newFlyClient: func(token, apiURL string) (fly.MachineClient, error) { return fakeFlyClient{}, nil },
		newRemote:    func(binary, org string) *spriteCLIRemote { return &spriteCLIRemote{} },
		newService: func(cfg dispatchsvc.Config) (dispatchRunner, error) {
			capturedEnvVars = cfg.EnvVars
			return runner, nil
		},
	}

	// Set ANTHROPIC_API_KEY like Claude Code would
	t.Setenv("ANTHROPIC_API_KEY", "sk-ant-api03-test-key")
	// Also set OpenRouter key so dispatch has a valid LLM key
	t.Setenv("OPENROUTER_API_KEY", "sk-or-v1-test-key")
	t.Setenv("GITHUB_TOKEN", "ghp-test-token")

	cmd := newDispatchCmdWithDeps(deps)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{
		"bramble", "Fix tests",
		"--app", "bb-app",
		"--token", "tok",
	})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("cmd.Execute() error = %v", err)
	}
	if capturedEnvVars == nil {
		t.Fatal("newService was never called")
	}
	// ANTHROPIC_API_KEY should be stripped to prevent bypassing the proxy
	if _, exists := capturedEnvVars["ANTHROPIC_API_KEY"]; exists {
		t.Fatal("ANTHROPIC_API_KEY should be stripped from envVars to prevent proxy bypass")
	}
	// OpenRouter key should still be present
	if v := capturedEnvVars["OPENROUTER_API_KEY"]; v != "sk-or-v1-test-key" {
		t.Fatalf("OPENROUTER_API_KEY = %q, want %q", v, "sk-or-v1-test-key")
	}
}

func TestDispatchExecuteRequiresGitHubToken(t *testing.T) {
	// Cannot use t.Parallel() — t.Setenv modifies process environment.

	// Clear all tokens so validation fires.
	t.Setenv("GH_TOKEN", "")
	t.Setenv("GITHUB_TOKEN", "")
	// Set an LLM key so only the GitHub token check fails.
	t.Setenv("OPENROUTER_API_KEY", "test-key")

	deps := dispatchDeps{
		readFile: func(string) ([]byte, error) { return nil, nil },
		newFlyClient: func(token, apiURL string) (fly.MachineClient, error) {
			return fakeFlyClient{}, nil
		},
		newRemote: func(binary, org string) *spriteCLIRemote {
			return &spriteCLIRemote{}
		},
		newService: func(cfg dispatchsvc.Config) (dispatchRunner, error) {
			t.Fatal("newService should not be called when credentials are missing")
			return nil, nil
		},
	}

	cmd := newDispatchCmdWithDeps(deps)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{
		"bramble", "Fix tests",
		"--execute",
		"--app", "bb-app",
		"--token", "fly-tok",
	})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error when GITHUB_TOKEN is missing in --execute mode")
	}
	if !strings.Contains(err.Error(), "no GitHub token found") {
		t.Fatalf("expected 'no GitHub token found' error, got: %v", err)
	}
	if !strings.Contains(err.Error(), "gh auth token") {
		t.Fatalf("expected remediation hint with 'gh auth token', got: %v", err)
	}
}

func TestDispatchExecuteRequiresLLMKey(t *testing.T) {
	// Cannot use t.Parallel() — t.Setenv modifies process environment.

	// Clear all LLM keys.
	for _, key := range []string{
		"OPENROUTER_API_KEY", "ANTHROPIC_AUTH_TOKEN", "ANTHROPIC_API_KEY",
		"MOONSHOT_AI_API_KEY", "XAI_API_KEY", "GEMINI_API_KEY", "OPENAI_API_KEY",
	} {
		t.Setenv(key, "")
	}
	// Set a GitHub token so only the LLM key check fails.
	t.Setenv("GITHUB_TOKEN", "ghp-test")

	deps := dispatchDeps{
		readFile: func(string) ([]byte, error) { return nil, nil },
		newFlyClient: func(token, apiURL string) (fly.MachineClient, error) {
			return fakeFlyClient{}, nil
		},
		newRemote: func(binary, org string) *spriteCLIRemote {
			return &spriteCLIRemote{}
		},
		newService: func(cfg dispatchsvc.Config) (dispatchRunner, error) {
			t.Fatal("newService should not be called when credentials are missing")
			return nil, nil
		},
	}

	cmd := newDispatchCmdWithDeps(deps)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{
		"bramble", "Fix tests",
		"--execute",
		"--app", "bb-app",
		"--token", "fly-tok",
	})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error when all LLM keys are missing in --execute mode")
	}
	if !strings.Contains(err.Error(), "no LLM API key found") {
		t.Fatalf("expected 'no LLM API key found' error, got: %v", err)
	}
}

func TestDispatchDryRunSkipsCredentialValidation(t *testing.T) {
	// Cannot use t.Parallel() — t.Setenv modifies process environment.

	// Clear everything — dry-run should not care.
	t.Setenv("GH_TOKEN", "")
	t.Setenv("GITHUB_TOKEN", "")
	t.Setenv("OPENROUTER_API_KEY", "")
	t.Setenv("ANTHROPIC_API_KEY", "")

	runner := &fakeDispatchRunner{}
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
	cmd.SetArgs([]string{"bramble", "test prompt"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("dry-run should not validate credentials, got: %v", err)
	}
}

func TestSelectSpriteFromRegistryMissingFile(t *testing.T) {
	t.Parallel()

	remote := newSpriteCLIRemote("sprite", "")

	testCases := []struct {
		name          string
		opts          dispatchOptions
		expectedInErr string
	}{
		{
			name: "with issue",
			opts: dispatchOptions{
				Issue: 42,
				Repo:  "misty-step/bitterblossom",
			},
			expectedInErr: "bb dispatch <sprite> --issue 42",
		},
		{
			name: "with file",
			opts: dispatchOptions{
				PromptFile: "prompt.md",
				Repo:       "misty-step/bitterblossom",
			},
			expectedInErr: "bb dispatch <sprite> --file <path>",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			opts := tc.opts
			opts.RegistryPath = t.TempDir() + "/nonexistent-registry.toml"

			_, err := selectSpriteFromRegistry(context.Background(), remote, opts)
			if err == nil {
				t.Fatal("expected error for missing registry file")
			}
			if !strings.Contains(err.Error(), "registry not found") {
				t.Fatalf("expected 'registry not found' error, got: %v", err)
			}
			if !strings.Contains(err.Error(), "bb add") {
				t.Fatalf("expected guidance about 'bb add', got: %v", err)
			}
			if !strings.Contains(err.Error(), tc.expectedInErr) {
				t.Fatalf("error message should contain %q, but it was: %v", tc.expectedInErr, err)
			}
		})
	}
}

func TestDispatchCommandStreamLogsRequiresWait(t *testing.T) {
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
	}

	cmd := newDispatchCmdWithDeps(deps)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{
		"moss",
		"Implement feature",
		"--execute",
		"--stream-logs", // Without --wait
		"--app", "bb-app",
		"--token", "tok",
	})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error when --stream-logs is used without --wait")
	}
	if !strings.Contains(err.Error(), "--stream-logs requires --wait") {
		t.Fatalf("expected '--stream-logs requires --wait' error, got: %v", err)
	}
}

func TestFormatStatusLine(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		result      *waitResult
		contains    []string
		notContains []string
	}{
		{
			name: "running with task and runtime",
			result: &waitResult{
				State:   "running",
				Task:    "Fix authentication bug",
				Runtime: "5m30s",
			},
			contains: []string{"🔄 Running", "task: Fix authentication bug", "runtime: 5m30s"},
		},
		{
			name: "completed with PR",
			result: &waitResult{
				State:    "completed",
				Task:     "Add new feature",
				Runtime:  "10m0s",
				Complete: true,
				PRURL:    "https://github.com/misty-step/bitterblossom/pull/42",
			},
			contains: []string{"✅ Completed", "task: Add new feature", "runtime: 10m0s"},
		},
		{
			name: "blocked with reason",
			result: &waitResult{
				State:         "blocked",
				Task:          "Update dependencies",
				Blocked:       true,
				BlockedReason: "Missing API credentials for external service",
			},
			contains: []string{"🚫 Blocked", "task: Update dependencies", "reason: Missing API credentials"},
		},
		{
			name: "idle state",
			result: &waitResult{
				State: "idle",
			},
			contains: []string{"💤 Idle"},
		},
		{
			name: "long task truncated",
			result: &waitResult{
				State:   "running",
				Task:    "This is a very long task description that should be truncated to fit within the display limits",
				Runtime: "2m0s",
			},
			contains:    []string{"🔄 Running", "task: This is a very long task description ...", "runtime: 2m0s"},
			notContains: []string{"truncated to fit within the display limits"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := formatStatusLine(tc.result)
			for _, want := range tc.contains {
				if !strings.Contains(got, want) {
					t.Errorf("formatStatusLine() = %q, should contain %q", got, want)
				}
			}
			for _, reject := range tc.notContains {
				if strings.Contains(got, reject) {
					t.Errorf("formatStatusLine() = %q, should NOT contain %q", got, reject)
				}
			}
		})
	}
}

func TestDispatchCommandWaitExitCodes(t *testing.T) {
	// Cannot use t.Parallel() — t.Setenv modifies process environment.
	tests := []struct {
		name         string
		waitResult   *waitResult
		wantExitCode int
		wantErr      bool
	}{
		{
			name: "completed state returns exit 0",
			waitResult: &waitResult{
				State:      "completed",
				Complete:   true,
				PRURL:      "https://github.com/misty-step/bitterblossom/pull/123",
				HasChanges: true,
				Commits:    1,
				PRs:        1,
			},
			wantExitCode: exitCodeSuccess,
			wantErr:      false,
		},
		{
			name: "blocked state returns exit 0",
			waitResult: &waitResult{
				State:         "blocked",
				Complete:      true,
				Blocked:       true,
				BlockedReason: "needs permissions",
			},
			wantExitCode: exitCodeSuccess,
			wantErr:      false,
		},
		{
			name: "timeout state returns exit 124",
			waitResult: &waitResult{
				State: "timeout",
				Error: "polling timed out",
			},
			wantExitCode: exitCodeTimeout,
			wantErr:      true,
		},
		{
			name: "idle state returns exit 2 (no signals)",
			waitResult: &waitResult{
				State:    "idle",
				Complete: false,
			},
			wantExitCode: exitCodeAgentNoSignals,
			wantErr:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("GITHUB_TOKEN", "ghp-test")
			t.Setenv("OPENROUTER_API_KEY", "or-test")

			runner := &fakeDispatchRunner{
				result: dispatchsvc.Result{
					Executed: true,
					State:    dispatchsvc.StateRunning,
					Plan: dispatchsvc.Plan{
						Sprite: "moss",
						Mode:   "execute",
					},
				},
			}

			deps := dispatchDeps{
				readFile:     func(string) ([]byte, error) { return nil, nil },
				newFlyClient: func(token, apiURL string) (fly.MachineClient, error) { return fakeFlyClient{}, nil },
				newRemote:    func(binary, org string) *spriteCLIRemote { return &spriteCLIRemote{} },
				newService:   func(cfg dispatchsvc.Config) (dispatchRunner, error) { return runner, nil },
				pollSprite: func(ctx context.Context, remote *spriteCLIRemote, sprite string, timeout time.Duration, progress func(string)) (*waitResult, error) {
					return tt.waitResult, nil
				},
			}

			cmd := newDispatchCmdWithDeps(deps)
			var out bytes.Buffer
			var errOut bytes.Buffer
			cmd.SetOut(&out)
			cmd.SetErr(&errOut)
			cmd.SetArgs([]string{
				"moss",
				"Test task",
				"--execute",
				"--wait",
				"--app", "bb-app",
				"--token", "tok",
			})

			err := cmd.Execute()
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error with exit code %d, got nil", tt.wantExitCode)
				}
				var coded *exitError
				if !errors.As(err, &coded) {
					t.Fatalf("expected exitError, got %T: %v", err, err)
				}
				if coded.Code != tt.wantExitCode {
					t.Fatalf("expected exit code %d, got %d", tt.wantExitCode, coded.Code)
				}
			} else {
				if err != nil {
					t.Fatalf("expected no error (exit 0), got: %v", err)
				}
			}
		})
	}
}

func TestWaitExitError(t *testing.T) {
	tests := []struct {
		name         string
		waitRes      *waitResult
		wantErr      bool
		wantExitCode int
	}{
		{
			name:         "completed with changes returns nil (exit 0)",
			waitRes:      &waitResult{State: "completed", Complete: true, HasChanges: true},
			wantErr:      false,
			wantExitCode: exitCodeSuccess,
		},
		{
			name:         "completed without changes returns exit 3",
			waitRes:      &waitResult{State: "completed", Complete: true, HasChanges: false},
			wantErr:      true,
			wantExitCode: exitCodeNoNewWork,
		},
		{
			name:         "completed with dirty files returns exit 4",
			waitRes:      &waitResult{State: "completed", Complete: true, HasChanges: false, DirtyFiles: 2},
			wantErr:      true,
			wantExitCode: exitCodePartialWork,
		},
		{
			name:         "blocked returns nil (exit 0)",
			waitRes:      &waitResult{State: "blocked", Complete: true, Blocked: true},
			wantErr:      false,
			wantExitCode: exitCodeSuccess,
		},
		{
			name:         "timeout returns exit 124",
			waitRes:      &waitResult{State: "timeout", Error: "polling timed out"},
			wantErr:      true,
			wantExitCode: exitCodeTimeout,
		},
		{
			name:         "idle returns exit 2",
			waitRes:      &waitResult{State: "idle", Complete: false},
			wantErr:      true,
			wantExitCode: exitCodeAgentNoSignals,
		},
		{
			name:         "running returns exit 2 (unexpected state)",
			waitRes:      &waitResult{State: "running"},
			wantErr:      true,
			wantExitCode: exitCodeAgentNoSignals,
		},
		{
			name:         "nil waitResult returns error",
			waitRes:      nil,
			wantErr:      true,
			wantExitCode: exitCodeDispatchFailure,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := waitExitError(tt.waitRes)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error with exit code %d, got nil", tt.wantExitCode)
				}
				var coded *exitError
				if !errors.As(err, &coded) {
					t.Fatalf("expected exitError, got %T: %v", err, err)
				}
				if coded.Code != tt.wantExitCode {
					t.Fatalf("expected exit code %d, got %d", tt.wantExitCode, coded.Code)
				}
			} else {
				if err != nil {
					t.Fatalf("expected nil error (exit 0), got: %v", err)
				}
			}
		})
	}
}

// TestDispatchWaitSkipsPollingWhenOneshotCompleted verifies that when an oneshot
// dispatch completes successfully (StateCompleted) and --wait is set, the polling
// loop is skipped entirely because the local state machine already knows the task
// is done. This addresses issue #293.
func TestDispatchWaitSkipsPollingWhenOneshotCompleted(t *testing.T) {
	// Cannot use t.Parallel() — t.Setenv modifies process environment.
	t.Setenv("GITHUB_TOKEN", "ghp-test")
	t.Setenv("OPENROUTER_API_KEY", "or-test")

	runner := &fakeDispatchRunner{
		result: dispatchsvc.Result{
			Executed: true,
			State:    dispatchsvc.StateCompleted, // Key: oneshot already completed
			Plan: dispatchsvc.Plan{
				Sprite: "moss",
				Mode:   "execute",
				Steps:  []dispatchsvc.PlanStep{{Kind: dispatchsvc.StepStartAgent, Description: "start"}},
			},
			Work: dispatchsvc.WorkDelta{
				HasChanges: true,
				Commits:    1,
				PRs:        1,
			},
		},
	}

	pollCalled := false
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
		pollSprite: func(ctx context.Context, remote *spriteCLIRemote, sprite string, timeout time.Duration, progress func(string)) (*waitResult, error) {
			pollCalled = true
			return nil, errors.New("polling should not be called for oneshot completed state")
		},
	}

	cmd := newDispatchCmdWithDeps(deps)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{
		"moss",
		"Implement feature",
		"--execute",
		"--wait",
		"--timeout", "5m",
		"--app", "bb-app",
		"--token", "tok",
	})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("cmd.Execute() error = %v", err)
	}

	if pollCalled {
		t.Fatal("expected pollSprite to be SKIPPED when oneshot dispatch completes with StateCompleted")
	}

	output := out.String()
	if !strings.Contains(output, "COMPLETE") {
		t.Fatalf("expected output to contain COMPLETE status, got: %s", output)
	}
}

// TestDispatchWaitPartialWorkDirtyFiles verifies that when an oneshot dispatch
// completes with uncommitted changes (DirtyFiles > 0), the output shows PARTIAL
// status and exits with code 4.
func TestDispatchWaitPartialWorkDirtyFiles(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "ghp-test")
	t.Setenv("OPENROUTER_API_KEY", "or-test")

	runner := &fakeDispatchRunner{
		result: dispatchsvc.Result{
			Executed: true,
			State:    dispatchsvc.StateCompleted,
			Plan: dispatchsvc.Plan{
				Sprite: "bramble",
				Mode:   "execute",
				Steps:  []dispatchsvc.PlanStep{{Kind: dispatchsvc.StepStartAgent, Description: "start"}},
			},
			Work: dispatchsvc.WorkDelta{
				HasChanges: false,
				Commits:    0,
				PRs:        0,
				DirtyFiles: 3,
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
		pollSprite: func(ctx context.Context, remote *spriteCLIRemote, sprite string, timeout time.Duration, progress func(string)) (*waitResult, error) {
			t.Fatal("pollSprite should not be called for oneshot completed state")
			return nil, nil
		},
	}

	cmd := newDispatchCmdWithDeps(deps)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{
		"bramble",
		"Fix issue",
		"--execute",
		"--wait",
		"--timeout", "5m",
		"--app", "bb-app",
		"--token", "tok",
	})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for partial work exit code")
	}
	var coded *exitError
	if !errors.As(err, &coded) {
		t.Fatalf("expected exitError, got %T: %v", err, err)
	}
	if coded.Code != exitCodePartialWork {
		t.Fatalf("expected exit code %d, got %d", exitCodePartialWork, coded.Code)
	}

	output := out.String()
	if !strings.Contains(output, "PARTIAL") {
		t.Fatalf("expected output to contain PARTIAL status, got: %s", output)
	}
	if !strings.Contains(output, "3 file(s)") {
		t.Fatalf("expected output to contain dirty file count, got: %s", output)
	}
}

// TestDispatchWaitPollsWhenRalphMode verifies that when Ralph mode is used,
// polling still occurs even if the exec returns, because the agent may continue
// running in the background.
func TestDispatchWaitPollsWhenRalphMode(t *testing.T) {
	// Cannot use t.Parallel() — t.Setenv modifies process environment.
	t.Setenv("GITHUB_TOKEN", "ghp-test")
	t.Setenv("OPENROUTER_API_KEY", "or-test")

	runner := &fakeDispatchRunner{
		result: dispatchsvc.Result{
			Executed: true,
			State:    dispatchsvc.StateRunning, // Ralph mode: agent may still be running
			Plan: dispatchsvc.Plan{
				Sprite: "moss",
				Mode:   "execute",
				Steps:  []dispatchsvc.PlanStep{{Kind: dispatchsvc.StepStartAgent, Description: "start"}},
			},
		},
	}

	pollCalled := false
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
		pollSprite: func(ctx context.Context, remote *spriteCLIRemote, sprite string, timeout time.Duration, progress func(string)) (*waitResult, error) {
			pollCalled = true
			return &waitResult{
				State:      "completed",
				Complete:   true,
				HasChanges: true,
				Commits:    1,
				PRs:        1,
			}, nil
		},
	}

	cmd := newDispatchCmdWithDeps(deps)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{
		"moss",
		"Implement feature",
		"--execute",
		"--wait",
		"--ralph", // Ralph mode: polling should still happen
		"--timeout", "5m",
		"--app", "bb-app",
		"--token", "tok",
	})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("cmd.Execute() error = %v", err)
	}

	if !pollCalled {
		t.Fatal("expected pollSprite to be called for Ralph mode even if StateRunning")
	}
}

// TestDispatchWaitWorkDeltaVerificationFailed verifies that when work delta
// calculation fails (e.g., I/O timeout), the output shows VERIFICATION FAILED
// status and exits with code 5 (not code 3 which would indicate "no new work").
// This addresses issue #356.
func TestDispatchWaitWorkDeltaVerificationFailed(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "ghp-test")
	t.Setenv("OPENROUTER_API_KEY", "or-test")

	runner := &fakeDispatchRunner{
		result: dispatchsvc.Result{
			Executed: true,
			State:    dispatchsvc.StateCompleted,
			Plan: dispatchsvc.Plan{
				Sprite: "fern",
				Mode:   "execute",
				Steps:  []dispatchsvc.PlanStep{{Kind: dispatchsvc.StepStartAgent, Description: "start"}},
			},
			Work: dispatchsvc.WorkDelta{
				VerificationFailed: true,
				VerificationError:  "capture post-exec HEAD SHA: exec timeout: i/o timeout",
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
		pollSprite: func(ctx context.Context, remote *spriteCLIRemote, sprite string, timeout time.Duration, progress func(string)) (*waitResult, error) {
			t.Fatal("pollSprite should not be called for oneshot completed state")
			return nil, nil
		},
	}

	cmd := newDispatchCmdWithDeps(deps)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{
		"fern",
		"Fix issue",
		"--execute",
		"--wait",
		"--timeout", "5m",
		"--app", "bb-app",
		"--token", "tok",
	})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for work delta verification failure, got nil")
	}

	// Verify exit code 5 (not 3 which would mean "no new work")
	exitErr, ok := err.(*exitError)
	if !ok {
		t.Fatalf("expected exitError, got: %T", err)
	}
	if exitErr.Code != 5 {
		t.Fatalf("expected exit code 5 (work delta verification failed), got: %d", exitErr.Code)
	}

	output := out.String()
	if !strings.Contains(output, "VERIFICATION FAILED") {
		t.Fatalf("expected output to contain 'VERIFICATION FAILED', got: %s", output)
	}
	if !strings.Contains(output, "i/o timeout") {
		t.Fatalf("expected output to contain error message 'i/o timeout', got: %s", output)
	}
	// Ensure we don't show "no new work" which would be misleading
	if strings.Contains(output, "no new work") {
		t.Fatalf("output should NOT contain 'no new work' for verification failure, got: %s", output)
	}
}
