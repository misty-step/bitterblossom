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
		State:    "completed",
		Task:     "Implement feature",
		Complete: true,
		PRURL:    "https://github.com/misty-step/bitterblossom/pull/123",
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

	// Should succeed with graceful degradation (just show dispatch result)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("cmd.Execute() error = %v", err)
	}

	// Should have warning in stderr
	errStr := errOut.String()
	if !strings.Contains(errStr, "polling failed") {
		t.Fatalf("expected warning about polling failure, got: %s", errStr)
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
		State:    "completed",
		Complete: true,
		PRURL:    "https://github.com/misty-step/bitterblossom/pull/123",
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

	remote := &spriteCLIRemote{binary: "sprite"}

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
