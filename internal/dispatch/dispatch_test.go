package dispatch

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/misty-step/bitterblossom/pkg/fly"
)

type execCall struct {
	sprite  string
	command string
	stdin   string
}

type uploadCall struct {
	sprite string
	path   string
	body   string
}

type fakeRemote struct {
	execCalls     []execCall
	execResponses []string
	execErrs      []error
	uploads       []uploadCall
	uploadErrs    []error
	uploadErr     error
}

func (f *fakeRemote) Exec(_ context.Context, sprite, command string, stdin []byte) (string, error) {
	f.execCalls = append(f.execCalls, execCall{
		sprite:  sprite,
		command: command,
		stdin:   string(stdin),
	})
	index := len(f.execCalls) - 1
	var output string
	if index < len(f.execResponses) {
		output = f.execResponses[index]
	}
	var err error
	if index < len(f.execErrs) {
		err = f.execErrs[index]
	}
	return output, err
}

func (f *fakeRemote) Upload(_ context.Context, sprite, remotePath string, content []byte) error {
	f.uploads = append(f.uploads, uploadCall{
		sprite: sprite,
		path:   remotePath,
		body:   string(content),
	})
	index := len(f.uploads) - 1
	if index < len(f.uploadErrs) {
		return f.uploadErrs[index]
	}
	return f.uploadErr
}

type fakeFly struct {
	listMachines []fly.Machine
	listErr      error
	createReqs   []fly.CreateRequest
	createErr    error
}

func (f *fakeFly) Create(_ context.Context, req fly.CreateRequest) (fly.Machine, error) {
	f.createReqs = append(f.createReqs, req)
	if f.createErr != nil {
		return fly.Machine{}, f.createErr
	}
	return fly.Machine{ID: "m-created", Name: req.Name}, nil
}

func (f *fakeFly) Destroy(context.Context, string, string) error {
	return errors.New("not implemented")
}

func (f *fakeFly) List(context.Context, string) ([]fly.Machine, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	result := make([]fly.Machine, len(f.listMachines))
	copy(result, f.listMachines)
	return result, nil
}

func (f *fakeFly) Status(context.Context, string, string) (fly.Machine, error) {
	return fly.Machine{}, errors.New("not implemented")
}

func (f *fakeFly) Exec(context.Context, string, string, fly.ExecRequest) (fly.ExecResult, error) {
	return fly.ExecResult{}, errors.New("not implemented")
}

func TestRunDryRunBuildsPlanWithoutSideEffects(t *testing.T) {
	remote := &fakeRemote{}
	flyClient := &fakeFly{listMachines: []fly.Machine{}}
	now := time.Date(2026, time.February, 8, 12, 0, 0, 0, time.UTC)

	service, err := NewService(Config{
		Remote:    remote,
		Fly:       flyClient,
		App:       "bb-app",
		Workspace: "/home/sprite/workspace",
		Now:       func() time.Time { return now },
	})
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	result, err := service.Run(context.Background(), Request{
		Sprite:  "bramble",
		Prompt:  "Fix flaky auth tests",
		Repo:    "misty-step/heartbeat",
		Ralph:   true,
		Execute: false,
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if result.Executed {
		t.Fatalf("Executed = %v, want false", result.Executed)
	}
	if len(result.Plan.Steps) != 6 {
		t.Fatalf("len(plan.steps) = %d, want 6", len(result.Plan.Steps))
	}
	if len(flyClient.createReqs) != 0 {
		t.Fatalf("unexpected create calls: %d", len(flyClient.createReqs))
	}
	if len(remote.execCalls) != 0 {
		t.Fatalf("unexpected exec calls: %d", len(remote.execCalls))
	}
	if len(remote.uploads) != 0 {
		t.Fatalf("unexpected uploads: %d", len(remote.uploads))
	}
}

func TestRunExecuteProvisionAndStartRalph(t *testing.T) {
	remote := &fakeRemote{
		execResponses: []string{
			"",          // validate env (empty key = ok)
			"",          // setup repo
			"PID: 4242", // start ralph
		},
	}
	flyClient := &fakeFly{listMachines: []fly.Machine{}}
	now := time.Date(2026, time.February, 8, 12, 0, 0, 0, time.UTC)

	service, err := NewService(Config{
		Remote:    remote,
		Fly:       flyClient,
		App:       "bb-app",
		Workspace: "/home/sprite/workspace",
		Now:       func() time.Time { return now },
	})
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	result, err := service.Run(context.Background(), Request{
		Sprite:  "fern",
		Prompt:  "Implement webhook retries",
		Repo:    "misty-step/heartbeat",
		Ralph:   true,
		Execute: true,
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if !result.Executed {
		t.Fatalf("Executed = %v, want true", result.Executed)
	}
	if !result.Provisioned {
		t.Fatalf("Provisioned = %v, want true", result.Provisioned)
	}
	if result.State != StateRunning {
		t.Fatalf("state = %q, want %q", result.State, StateRunning)
	}
	if result.AgentPID != 4242 {
		t.Fatalf("AgentPID = %d, want 4242", result.AgentPID)
	}
	if len(flyClient.createReqs) != 1 {
		t.Fatalf("create calls = %d, want 1", len(flyClient.createReqs))
	}
	if flyClient.createReqs[0].Name != "fern" {
		t.Fatalf("create name = %q, want fern", flyClient.createReqs[0].Name)
	}
	if len(remote.uploads) != 2 {
		t.Fatalf("upload calls = %d, want 2", len(remote.uploads))
	}
	if remote.uploads[0].path != "/home/sprite/workspace/PROMPT.md" {
		t.Fatalf("prompt path = %q, want PROMPT.md", remote.uploads[0].path)
	}
	if !strings.Contains(remote.uploads[0].body, "Implement webhook retries") {
		t.Fatalf("prompt upload missing task text: %q", remote.uploads[0].body)
	}
	if !strings.Contains(remote.execCalls[0].command, "printenv ANTHROPIC_API_KEY") {
		t.Fatalf("expected env validation command, got %q", remote.execCalls[0].command)
	}
	if !strings.Contains(remote.execCalls[1].command, "gh repo clone") {
		t.Fatalf("expected repo setup command, got %q", remote.execCalls[1].command)
	}
	if !strings.Contains(remote.execCalls[2].command, "sprite-agent") {
		t.Fatalf("expected ralph start command, got %q", remote.execCalls[2].command)
	}
	if !strings.Contains(remote.execCalls[2].command, "BB_CLAUDE_FLAGS") {
		t.Fatalf("expected ralph start to pass BB_CLAUDE_FLAGS to sprite-agent, got %q", remote.execCalls[2].command)
	}
	if !strings.Contains(remote.execCalls[2].command, "--dangerously-skip-permissions") {
		t.Fatalf("expected ralph start BB_CLAUDE_FLAGS to include dangerously-skip-permissions, got %q", remote.execCalls[2].command)
	}
	if !strings.Contains(remote.execCalls[2].command, "--output-format stream-json") {
		t.Fatalf("expected ralph start BB_CLAUDE_FLAGS to include stream-json output, got %q", remote.execCalls[2].command)
	}
}

func TestRunExecuteOneShotCompletes(t *testing.T) {
	remote := &fakeRemote{
		execResponses: []string{
			"",     // validate env
			"done", // oneshot agent
		},
	}
	flyClient := &fakeFly{
		listMachines: []fly.Machine{{Name: "willow", ID: "m1"}},
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

	result, err := service.Run(context.Background(), Request{
		Sprite:  "willow",
		Prompt:  "Generate release notes",
		Execute: true,
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if result.State != StateCompleted {
		t.Fatalf("state = %q, want %q", result.State, StateCompleted)
	}
	if len(flyClient.createReqs) != 0 {
		t.Fatalf("unexpected create calls: %d", len(flyClient.createReqs))
	}
	if len(remote.uploads) != 2 {
		t.Fatalf("upload calls = %d, want 2", len(remote.uploads))
	}
	if remote.uploads[0].path != "/home/sprite/workspace/.dispatch-prompt.md" {
		t.Fatalf("oneshot prompt path = %q", remote.uploads[0].path)
	}
	if len(remote.execCalls) != 2 {
		t.Fatalf("exec calls = %d, want 2", len(remote.execCalls))
	}
	if !strings.Contains(remote.execCalls[0].command, "printenv ANTHROPIC_API_KEY") {
		t.Fatalf("expected env validation command, got %q", remote.execCalls[0].command)
	}
	if !strings.Contains(remote.execCalls[1].command, "claude -p") {
		t.Fatalf("expected claude command, got %q", remote.execCalls[1].command)
	}
	if !strings.Contains(remote.execCalls[1].command, "--dangerously-skip-permissions") {
		t.Fatalf("expected claude command to include dangerously-skip-permissions, got %q", remote.execCalls[1].command)
	}
	if !strings.Contains(remote.execCalls[1].command, "--verbose --output-format stream-json") {
		t.Fatalf("expected claude command to include verbose stream-json output, got %q", remote.execCalls[1].command)
	}
}

func TestRunExecuteErrorsPreserveFailedState(t *testing.T) {
	now := time.Date(2026, time.February, 8, 12, 0, 0, 0, time.UTC)

	cases := []struct {
		name    string
		req     Request
		remote  *fakeRemote
		fly     *fakeFly
		wantErr string
	}{
		{
			name: "provision failure returns failed state",
			req: Request{
				Sprite:  "fern",
				Prompt:  "Fix tests",
				Execute: true,
			},
			remote:  &fakeRemote{},
			fly:     &fakeFly{createErr: errors.New("provision failed")},
			wantErr: "dispatch: provision sprite",
		},
		{
			name: "setup repo failure returns failed state",
			req: Request{
				Sprite:  "fern",
				Prompt:  "Fix tests",
				Repo:    "misty-step/heartbeat",
				Execute: true,
			},
			remote:  &fakeRemote{execErrs: []error{nil, errors.New("setup failed")}},
			fly:     &fakeFly{listMachines: []fly.Machine{{Name: "fern", ID: "m1"}}},
			wantErr: "dispatch: setup repo",
		},
		{
			name: "upload prompt failure returns failed state",
			req: Request{
				Sprite:  "fern",
				Prompt:  "Fix tests",
				Execute: true,
			},
			remote:  &fakeRemote{uploadErrs: []error{errors.New("prompt upload failed")}},
			fly:     &fakeFly{listMachines: []fly.Machine{{Name: "fern", ID: "m1"}}},
			wantErr: "dispatch: upload prompt",
		},
		{
			name: "upload status failure returns failed state",
			req: Request{
				Sprite:  "fern",
				Prompt:  "Fix tests",
				Execute: true,
			},
			remote:  &fakeRemote{uploadErrs: []error{nil, errors.New("status upload failed")}},
			fly:     &fakeFly{listMachines: []fly.Machine{{Name: "fern", ID: "m1"}}},
			wantErr: "dispatch: upload status",
		},
		{
			name: "start agent failure returns failed state",
			req: Request{
				Sprite:  "fern",
				Prompt:  "Fix tests",
				Execute: true,
			},
			remote:  &fakeRemote{execErrs: []error{nil, errors.New("start failed")}},
			fly:     &fakeFly{listMachines: []fly.Machine{{Name: "fern", ID: "m1"}}},
			wantErr: "dispatch: start agent",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			service, err := NewService(Config{
				Remote:    tc.remote,
				Fly:       tc.fly,
				App:       "bb-app",
				Workspace: "/home/sprite/workspace",
				Now:       func() time.Time { return now },
			})
			if err != nil {
				t.Fatalf("NewService() error = %v", err)
			}

			result, runErr := service.Run(context.Background(), tc.req)
			if runErr == nil {
				t.Fatalf("Run() error = nil, want non-nil")
			}
			if !strings.Contains(runErr.Error(), tc.wantErr) {
				t.Fatalf("Run() error = %v, want contains %q", runErr, tc.wantErr)
			}
			if result.State != StateFailed {
				t.Fatalf("state = %q, want %q", result.State, StateFailed)
			}
		})
	}
}

func TestRunValidation(t *testing.T) {
	service, err := NewService(Config{
		Remote: &fakeRemote{},
		Fly:    &fakeFly{},
		App:    "bb-app",
	})
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	cases := []struct {
		name string
		req  Request
	}{
		{
			name: "missing prompt",
			req: Request{
				Sprite: "bramble",
			},
		},
		{
			name: "invalid sprite",
			req: Request{
				Sprite: "Bad Sprite",
				Prompt: "Fix tests",
			},
		},
		{
			name: "invalid repo",
			req: Request{
				Sprite: "bramble",
				Prompt: "Fix tests",
				Repo:   "https://github.com/",
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, runErr := service.Run(context.Background(), tc.req)
			if runErr == nil {
				t.Fatalf("expected error for case %q", tc.name)
			}
		})
	}
}
