package dispatch

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/misty-step/bitterblossom/internal/registry"
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
	listSprites   []string
	listErr       error
}

func (f *fakeRemote) Exec(_ context.Context, sprite, command string, stdin []byte) (string, error) {
	return f.ExecWithEnv(context.Background(), sprite, command, stdin, nil)
}

func (f *fakeRemote) ExecWithEnv(_ context.Context, sprite, command string, stdin []byte, env map[string]string) (string, error) {
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

func (f *fakeRemote) List(_ context.Context) ([]string, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	return f.listSprites, nil
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
	if len(result.Plan.Steps) != 7 {
		t.Fatalf("len(plan.steps) = %d, want 7", len(result.Plan.Steps))
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
			"",          // clean signals
			"",          // setup repo
			"PID: 4242", // start ralph
		},
		listSprites: []string{},
	}
	flyClient := &fakeFly{}
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
	if !strings.Contains(remote.execCalls[1].command, "rm -f") {
		t.Fatalf("expected clean signals command, got %q", remote.execCalls[1].command)
	}
	if !strings.Contains(remote.execCalls[2].command, "gh repo clone") {
		t.Fatalf("expected repo setup command, got %q", remote.execCalls[2].command)
	}
	if !strings.Contains(remote.execCalls[3].command, "sprite-agent") {
		t.Fatalf("expected ralph start command, got %q", remote.execCalls[3].command)
	}
	if !strings.Contains(remote.execCalls[3].command, "BB_CLAUDE_FLAGS") {
		t.Fatalf("expected ralph start to pass BB_CLAUDE_FLAGS to sprite-agent, got %q", remote.execCalls[3].command)
	}
	if !strings.Contains(remote.execCalls[3].command, "MAX_TOKENS=200000") {
		t.Fatalf("expected ralph start to pass MAX_TOKENS, got %q", remote.execCalls[3].command)
	}
	if !strings.Contains(remote.execCalls[3].command, "MAX_TIME_SEC=1800") {
		t.Fatalf("expected ralph start to pass MAX_TIME_SEC, got %q", remote.execCalls[3].command)
	}
	if !strings.Contains(remote.execCalls[3].command, "--dangerously-skip-permissions") {
		t.Fatalf("expected ralph start BB_CLAUDE_FLAGS to include dangerously-skip-permissions, got %q", remote.execCalls[3].command)
	}
	if !strings.Contains(remote.execCalls[3].command, "--output-format stream-json") {
		t.Fatalf("expected ralph start BB_CLAUDE_FLAGS to include stream-json output, got %q", remote.execCalls[3].command)
	}
}

func TestRunExecuteOneShotCompletes(t *testing.T) {
	remote := &fakeRemote{
		execResponses: []string{
			"",     // validate env
			"",     // clean signals
			"done", // oneshot agent
		},
		listSprites: []string{"willow"},
	}
	flyClient := &fakeFly{}

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
	if len(remote.execCalls) != 3 {
		t.Fatalf("exec calls = %d, want 3", len(remote.execCalls))
	}
	if !strings.Contains(remote.execCalls[0].command, "printenv ANTHROPIC_API_KEY") {
		t.Fatalf("expected env validation command, got %q", remote.execCalls[0].command)
	}
	if !strings.Contains(remote.execCalls[1].command, "rm -f") {
		t.Fatalf("expected clean signals command, got %q", remote.execCalls[1].command)
	}
	if !strings.Contains(remote.execCalls[2].command, "claude -p") {
		t.Fatalf("expected claude command, got %q", remote.execCalls[2].command)
	}
	if !strings.Contains(remote.execCalls[2].command, "--dangerously-skip-permissions") {
		t.Fatalf("expected claude command to include dangerously-skip-permissions, got %q", remote.execCalls[2].command)
	}
	if !strings.Contains(remote.execCalls[2].command, "--verbose --output-format stream-json") {
		t.Fatalf("expected claude command to include verbose stream-json output, got %q", remote.execCalls[2].command)
	}
}

func TestRunCleanSignalsBeforePromptUpload(t *testing.T) {
	remote := &fakeRemote{
		execResponses: []string{
			"",     // validate env
			"",     // clean signals
			"done", // oneshot agent
		},
		listSprites: []string{"willow"},
	}
	flyClient := &fakeFly{}

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

	// Verify that clean signals exec happens before prompt upload
	if len(remote.execCalls) < 2 {
		t.Fatalf("expected at least 2 exec calls, got %d", len(remote.execCalls))
	}

	// Find the dedicated cleanSignals call. It's a standalone rm -f that only
	// removes signal files (not PID files — those are needed by start scripts).
	cleanSignalsIdx := -1

	for i, call := range remote.execCalls {
		if strings.Contains(call.command, "rm -f") && strings.Contains(call.command, "TASK_COMPLETE") && !strings.Contains(call.command, "agent.pid") {
			cleanSignalsIdx = i
			break
		}
	}

	if cleanSignalsIdx == -1 {
		t.Fatal("expected clean signals exec call not found")
	}

	if len(remote.uploads) != 2 {
		t.Fatalf("upload calls = %d, want 2", len(remote.uploads))
	}

	// Verify the first upload is the prompt (after clean signals)
	if remote.uploads[0].path != "/home/sprite/workspace/.dispatch-prompt.md" {
		t.Fatalf("expected prompt upload first, got %q", remote.uploads[0].path)
	}

	// Verify clean signals removes signal files but NOT PID files
	cleanSignalsCmd := remote.execCalls[cleanSignalsIdx].command
	expectedFiles := []string{
		"TASK_COMPLETE",
		"TASK_COMPLETE.md",
		"BLOCKED.md",
		"BLOCKED",
	}
	for _, file := range expectedFiles {
		if !strings.Contains(cleanSignalsCmd, file) {
			t.Errorf("clean signals command missing %q: %q", file, cleanSignalsCmd)
		}
	}
	// PID files must NOT be in cleanSignals — start scripts need them to kill stale processes
	for _, file := range []string{"agent.pid", "ralph.pid"} {
		if strings.Contains(cleanSignalsCmd, file) {
			t.Errorf("clean signals should not remove %q (needed by start scripts): %q", file, cleanSignalsCmd)
		}
	}
}

func TestRunExecuteWithSkillsUploadsAndInjectsPrompt(t *testing.T) {
	skillRoot := t.TempDir()

	dispatchSkill := filepath.Join(skillRoot, "dispatch-loop")
	if err := os.MkdirAll(filepath.Join(dispatchSkill, "references"), 0o755); err != nil {
		t.Fatalf("mkdir dispatch skill: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dispatchSkill, "SKILL.md"), []byte("# Dispatch Loop\n"), 0o644); err != nil {
		t.Fatalf("write dispatch SKILL.md: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dispatchSkill, "references", "examples.md"), []byte("example"), 0o644); err != nil {
		t.Fatalf("write dispatch references: %v", err)
	}

	statusSkill := filepath.Join(skillRoot, "status-ops")
	if err := os.MkdirAll(statusSkill, 0o755); err != nil {
		t.Fatalf("mkdir status skill: %v", err)
	}
	if err := os.WriteFile(filepath.Join(statusSkill, "SKILL.md"), []byte("# Status Ops\n"), 0o644); err != nil {
		t.Fatalf("write status SKILL.md: %v", err)
	}

	remote := &fakeRemote{
		execResponses: []string{
			"",     // validate env
			"",     // clean signals
			"done", // oneshot agent
		},
		listSprites: []string{"willow"},
	}
	flyClient := &fakeFly{}

	service, err := NewService(Config{
		Remote:               remote,
		Fly:                  flyClient,
		App:                  "bb-app",
		Workspace:            "/home/sprite/workspace",
		MaxConcurrentUploads: 1, // sequential to avoid data race on fakeRemote
	})
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	result, err := service.Run(context.Background(), Request{
		Sprite:  "willow",
		Prompt:  "Implement issue #252",
		Execute: true,
		Skills: []string{
			dispatchSkill,
			filepath.Join(statusSkill, "SKILL.md"),
		},
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if result.State != StateCompleted {
		t.Fatalf("state = %q, want %q", result.State, StateCompleted)
	}

	uploadByPath := map[string]string{}
	for _, call := range remote.uploads {
		uploadByPath[call.path] = call.body
	}

	// Skill directories are uploaded into /skills/<name>/...
	if _, ok := uploadByPath["/home/sprite/workspace/skills/dispatch-loop/SKILL.md"]; !ok {
		t.Fatalf("missing uploaded dispatch skill SKILL.md")
	}
	if _, ok := uploadByPath["/home/sprite/workspace/skills/dispatch-loop/references/examples.md"]; !ok {
		t.Fatalf("missing uploaded dispatch skill reference file")
	}
	if _, ok := uploadByPath["/home/sprite/workspace/skills/status-ops/SKILL.md"]; !ok {
		t.Fatalf("missing uploaded status skill SKILL.md")
	}

	// Prompt includes explicit skill instructions.
	promptBody, ok := uploadByPath["/home/sprite/workspace/.dispatch-prompt.md"]
	if !ok {
		t.Fatalf("missing uploaded prompt at .dispatch-prompt.md")
	}
	if !strings.Contains(promptBody, "Follow the skill at ./skills/dispatch-loop/SKILL.md") {
		t.Fatalf("prompt missing dispatch-loop skill instruction: %q", promptBody)
	}
	if !strings.Contains(promptBody, "Follow the skill at ./skills/status-ops/SKILL.md") {
		t.Fatalf("prompt missing status-ops skill instruction: %q", promptBody)
	}

	hasSkillStep := false
	for _, step := range result.Plan.Steps {
		if step.Kind == StepUploadSkills {
			hasSkillStep = true
			break
		}
	}
	if !hasSkillStep {
		t.Fatalf("plan missing StepUploadSkills when skills are provided")
	}
}

func TestRunValidationFailsForMissingSkillPath(t *testing.T) {
	service, err := NewService(Config{
		Remote: &fakeRemote{},
		Fly:    &fakeFly{},
		App:    "bb-app",
	})
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	_, runErr := service.Run(context.Background(), Request{
		Sprite: "bramble",
		Prompt: "Fix tests",
		Skills: []string{"/definitely/not/a/real/skill/path"},
	})
	if runErr == nil {
		t.Fatal("expected error for missing skill path")
	}
	if !strings.Contains(runErr.Error(), "skill") {
		t.Fatalf("error = %v, want mention of skill path", runErr)
	}
}

func TestRunValidationFailsForSymlinkedSkillFile(t *testing.T) {
	skillRoot := t.TempDir()
	skillDir := filepath.Join(skillRoot, "malicious-skill")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatalf("mkdir skill dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("# Skill\n"), 0o644); err != nil {
		t.Fatalf("write SKILL.md: %v", err)
	}

	target := filepath.Join(skillRoot, "outside-secret.txt")
	if err := os.WriteFile(target, []byte("secret"), 0o644); err != nil {
		t.Fatalf("write target: %v", err)
	}
	linkPath := filepath.Join(skillDir, "stolen.txt")
	if err := os.Symlink(target, linkPath); err != nil {
		t.Skipf("symlink unsupported in test environment: %v", err)
	}

	service, err := NewService(Config{
		Remote: &fakeRemote{},
		Fly:    &fakeFly{},
		App:    "bb-app",
	})
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	_, runErr := service.Run(context.Background(), Request{
		Sprite: "bramble",
		Prompt: "Fix tests",
		Skills: []string{skillDir},
	})
	if runErr == nil {
		t.Fatal("expected error for symlinked skill file")
	}
	if !strings.Contains(strings.ToLower(runErr.Error()), "symlink") {
		t.Fatalf("error = %v, want symlink rejection", runErr)
	}
}

func TestUploadSkillsRejectsSymlinkFiles(t *testing.T) {
	tmp := t.TempDir()
	target := filepath.Join(tmp, "target.txt")
	if err := os.WriteFile(target, []byte("secret"), 0o644); err != nil {
		t.Fatalf("write target: %v", err)
	}
	linkPath := filepath.Join(tmp, "link.txt")
	if err := os.Symlink(target, linkPath); err != nil {
		t.Skipf("symlink unsupported in test environment: %v", err)
	}

	remote := &fakeRemote{}
	service, err := NewService(Config{
		Remote: remote,
		Fly:    &fakeFly{},
		App:    "bb-app",
	})
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	err = service.uploadSkills(context.Background(), "bramble", []preparedSkill{
		{
			Name: "test",
			Files: []skillFile{
				{
					LocalPath:  linkPath,
					RemotePath: "/home/sprite/workspace/skills/test/link.txt",
				},
			},
		},
	})
	if err == nil {
		t.Fatal("expected symlink upload rejection")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "non-symlink") {
		t.Fatalf("error = %v, want non-symlink validation error", err)
	}
	if len(remote.uploads) != 0 {
		t.Fatalf("unexpected uploads for rejected symlink: %d", len(remote.uploads))
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
			remote:  &fakeRemote{execErrs: []error{nil, nil, errors.New("setup failed")}, listSprites: []string{"fern"}},
			fly:     &fakeFly{},
			wantErr: "dispatch: setup repo",
		},
		{
			name: "upload prompt failure returns failed state",
			req: Request{
				Sprite:  "fern",
				Prompt:  "Fix tests",
				Execute: true,
			},
			remote:  &fakeRemote{uploadErrs: []error{errors.New("prompt upload failed")}, listSprites: []string{"fern"}},
			fly:     &fakeFly{},
			wantErr: "dispatch: upload prompt",
		},
		{
			name: "upload status failure returns failed state",
			req: Request{
				Sprite:  "fern",
				Prompt:  "Fix tests",
				Execute: true,
			},
			remote:  &fakeRemote{uploadErrs: []error{nil, errors.New("status upload failed")}, listSprites: []string{"fern"}},
			fly:     &fakeFly{},
			wantErr: "dispatch: upload status",
		},
		{
			name: "start agent failure returns failed state",
			req: Request{
				Sprite:  "fern",
				Prompt:  "Fix tests",
				Execute: true,
			},
			remote:  &fakeRemote{execErrs: []error{nil, nil, errors.New("start failed")}, listSprites: []string{"fern"}},
			fly:     &fakeFly{},
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

func TestRegisterSpritePreservesAssignmentFields(t *testing.T) {
	registryPath := writeTestRegistry(t, `[sprites.fern]
machine_id = "m-old"
created_at = "2026-02-10T00:00:00Z"
assigned_issue = 186
assigned_repo = "misty-step/bitterblossom"
assigned_at = "2026-02-10T00:01:00Z"
`)

	remote := &fakeRemote{}
	flyClient := &fakeFly{}
	service, err := NewService(Config{
		Remote:       remote,
		Fly:          flyClient,
		App:          "bb-app",
		RegistryPath: registryPath,
	})
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	if err := service.registerSprite(context.Background(), "fern", "m-new"); err != nil {
		t.Fatalf("registerSprite() error = %v", err)
	}

	reg, err := registry.Load(registryPath)
	if err != nil {
		t.Fatalf("registry.Load() error = %v", err)
	}
	entry := reg.Sprites["fern"]
	if entry.MachineID != "m-new" {
		t.Fatalf("MachineID = %q, want %q", entry.MachineID, "m-new")
	}
	if entry.AssignedIssue != 186 {
		t.Fatalf("AssignedIssue = %d, want 186", entry.AssignedIssue)
	}
	if entry.AssignedRepo != "misty-step/bitterblossom" {
		t.Fatalf("AssignedRepo = %q, want %q", entry.AssignedRepo, "misty-step/bitterblossom")
	}
	if entry.AssignedAt.IsZero() {
		t.Fatalf("AssignedAt is zero, want preserved")
	}
}

func TestRunExecuteUsesNameNotMachineIDForRemoteOps(t *testing.T) {
	registryPath := writeTestRegistry(t, `[sprites.fern]
machine_id = "m-def456"
`)

	remote := &fakeRemote{
		execResponses: []string{
			"",     // validate env
			"",     // clean signals
			"done", // oneshot
		},
	}
	flyClient := &fakeFly{}

	service, err := NewService(Config{
		Remote:       remote,
		Fly:          flyClient,
		App:          "bb-app",
		Workspace:    "/home/sprite/workspace",
		RegistryPath: registryPath,
	})
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	result, err := service.Run(context.Background(), Request{
		Sprite:  "fern",
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
	// Remote ops (exec/upload) should use sprite name, not machine ID.
	// sprite exec -s expects names, not Fly machine IDs.
	if len(remote.execCalls) != 3 {
		t.Fatalf("exec calls = %d, want 3", len(remote.execCalls))
	}
	if remote.execCalls[0].sprite != "fern" {
		t.Fatalf("exec sprite = %q, want name %q (not machine ID)", remote.execCalls[0].sprite, "fern")
	}
	if len(remote.uploads) != 2 {
		t.Fatalf("upload calls = %d, want 2", len(remote.uploads))
	}
	if remote.uploads[0].sprite != "fern" {
		t.Fatalf("upload sprite = %q, want name %q (not machine ID)", remote.uploads[0].sprite, "fern")
	}
}

func TestRunExecuteProvisionUsesCreatedMachineIDWhenRegistryEnabled(t *testing.T) {
	registryPath := writeTestRegistry(t, `[meta]
app = "bb-app"
`)

	remote := &fakeRemote{
		execResponses: []string{
			"",     // validate env
			"",     // clean signals
			"done", // oneshot
		},
	}
	flyClient := &fakeFly{}

	service, err := NewService(Config{
		Remote:       remote,
		Fly:          flyClient,
		App:          "bb-app",
		Workspace:    "/home/sprite/workspace",
		RegistryPath: registryPath,
	})
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	result, err := service.Run(context.Background(), Request{
		Sprite:  "fern",
		Prompt:  "Generate release notes",
		Execute: true,
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if !result.Provisioned {
		t.Fatalf("Provisioned = %v, want true", result.Provisioned)
	}
	if len(flyClient.createReqs) != 1 {
		t.Fatalf("create calls = %d, want 1", len(flyClient.createReqs))
	}
	if len(remote.execCalls) != 3 {
		t.Fatalf("exec calls = %d, want 3", len(remote.execCalls))
	}
	if remote.execCalls[0].sprite != "fern" {
		t.Fatalf("exec sprite = %q, want name %q (not machine ID)", remote.execCalls[0].sprite, "fern")
	}

	machineID, lookupErr := ResolveSprite("fern", registryPath)
	if lookupErr != nil {
		t.Fatalf("ResolveSprite(fern) error = %v", lookupErr)
	}
	if machineID != "m-created" {
		t.Fatalf("ResolveSprite(fern) = %q, want %q", machineID, "m-created")
	}
}

func TestRunExecuteWithOpenRouterKey_EnsuresProxy(t *testing.T) {
	// Test that when OPENROUTER_API_KEY is provided, the proxy is ensured
	remote := &fakeRemote{
		execResponses: []string{
			"",     // validate env
			"",     // clean signals
			"000",  // proxy health check (not running)
			"",     // kill existing process on port (cleanup)
			"",     // mkdir -p
			"",     // write API key file
			"",     // start proxy
			"200",  // proxy health check (now running)
			"done", // oneshot agent
		},
	}
	flyClient := &fakeFly{}

	service, err := NewService(Config{
		Remote:    remote,
		Fly:       flyClient,
		App:       "bb-app",
		Workspace: "/home/sprite/workspace",
		EnvVars: map[string]string{
			"OPENROUTER_API_KEY": "test-api-key",
		},
	})
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	// Set a timeout that accommodates the 1s polling interval.
	service.proxyLifecycle.SetTimeout(3 * time.Second)

	result, err := service.Run(context.Background(), Request{
		Sprite:  "fern",
		Prompt:  "Generate release notes",
		Execute: true,
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	// Oneshot mode transitions to completed immediately after starting agent
	if result.State != StateCompleted {
		t.Fatalf("state = %q, want %q", result.State, StateCompleted)
	}

	// Check that plan includes ensure_proxy step
	hasProxyStep := false
	for _, step := range result.Plan.Steps {
		if step.Kind == StepEnsureProxy {
			hasProxyStep = true
			break
		}
	}
	if !hasProxyStep {
		t.Error("expected plan to include StepEnsureProxy")
	}

	// Check that the agent exec call happened
	if len(remote.execCalls) < 1 {
		t.Fatal("expected at least 1 exec call")
	}
}

func TestRunExecuteWithoutOpenRouterKey_SkipsProxy(t *testing.T) {
	// Test that when OPENROUTER_API_KEY is not provided, the proxy step is skipped
	remote := &fakeRemote{
		execResponses: []string{
			"",     // validate env
			"done", // oneshot agent
		},
	}
	flyClient := &fakeFly{}

	service, err := NewService(Config{
		Remote:    remote,
		Fly:       flyClient,
		App:       "bb-app",
		Workspace: "/home/sprite/workspace",
		EnvVars:   map[string]string{}, // No OPENROUTER_API_KEY
	})
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	result, err := service.Run(context.Background(), Request{
		Sprite:  "fern",
		Prompt:  "Generate release notes",
		Execute: true,
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	// Oneshot mode transitions to completed immediately after starting agent
	if result.State != StateCompleted {
		t.Fatalf("state = %q, want %q", result.State, StateCompleted)
	}

	// Check that plan does NOT include ensure_proxy step
	hasProxyStep := false
	for _, step := range result.Plan.Steps {
		if step.Kind == StepEnsureProxy {
			hasProxyStep = true
			break
		}
	}
	if hasProxyStep {
		t.Error("expected plan to NOT include StepEnsureProxy when no OPENROUTER_API_KEY")
	}
}

func TestBuildOneShotScriptCleansStatusFiles(t *testing.T) {
	t.Parallel()

	script := buildOneShotScript("/home/sprite/workspace", "/home/sprite/workspace/prompt.md", "/home/sprite/workspace/logs/oneshot.log")

	// Must contain cleanup of TASK_COMPLETE and BLOCKED.md
	if !strings.Contains(script, "rm -f TASK_COMPLETE TASK_COMPLETE.md BLOCKED.md") {
		t.Errorf("buildOneShotScript missing cleanup of TASK_COMPLETE and BLOCKED.md")
	}

	// Cleanup must be early in the script (before proxy startup)
	lines := strings.Split(script, "\n")
	cleanupIdx, proxyIdx := -1, -1
	for i, line := range lines {
		if strings.Contains(line, "rm -f TASK_COMPLETE TASK_COMPLETE.md BLOCKED.md") {
			cleanupIdx = i
		}
		if strings.Contains(line, "Start anthropic proxy") {
			proxyIdx = i
		}
	}
	if cleanupIdx == -1 {
		t.Fatal("cleanup command not found in script")
	}
	if proxyIdx == -1 {
		t.Fatal("proxy comment not found in script")
	}
	if cleanupIdx >= proxyIdx {
		t.Errorf("cleanup (line %d) must come before proxy startup (line %d)", cleanupIdx, proxyIdx)
	}
}

func TestBuildOneShotScriptCapturesOutput(t *testing.T) {
	t.Parallel()

	logPath := "/home/sprite/workspace/logs/oneshot-20260212-120000.log"
	script := buildOneShotScript("/home/sprite/workspace", "/home/sprite/workspace/prompt.md", logPath)

	// Must create logs directory before cd (path is quoted by shellutil.Quote)
	if !strings.Contains(script, "mkdir -p '/home/sprite/workspace/logs'") {
		t.Errorf("buildOneShotScript missing logs directory creation")
	}

	// Must use tee to capture output to log file (without -a; truncated each dispatch)
	if !strings.Contains(script, "| tee ") {
		t.Errorf("buildOneShotScript missing tee for output capture")
	}

	// Must capture exit code
	if !strings.Contains(script, "EXIT_CODE=$?") {
		t.Errorf("buildOneShotScript missing exit code capture")
	}

	// Log path must appear in script (quoted)
	if !strings.Contains(script, "'"+logPath+"'") {
		t.Errorf("buildOneShotScript does not contain log path")
	}
}

func TestBuildScriptMkdirBeforeCD(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		script string
	}{
		{
			name:   "buildSetupRepoScript",
			script: buildSetupRepoScript("/home/sprite/workspace", "https://github.com/misty-step/bb.git", "bb"),
		},
		{
			name:   "buildOneShotScript",
			script: buildOneShotScript("/home/sprite/workspace", "/home/sprite/workspace/bb/prompt.md", "/home/sprite/workspace/logs/oneshot.log"),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			lines := strings.Split(tc.script, "\n")

			mkdirIdx, cdIdx := -1, -1
			for i, line := range lines {
				if strings.HasPrefix(line, "mkdir -p ") {
					mkdirIdx = i
				}
				if strings.HasPrefix(line, "cd ") && cdIdx == -1 {
					cdIdx = i
				}
			}

			if mkdirIdx == -1 {
				t.Fatal("script missing mkdir -p")
			}
			if cdIdx == -1 {
				t.Fatal("script missing cd")
			}
			if mkdirIdx >= cdIdx {
				t.Fatalf("mkdir (line %d) must come before cd (line %d)", mkdirIdx, cdIdx)
			}
		})
	}
}

func TestBuildOneShotScriptCapturesLogs(t *testing.T) {
	t.Parallel()

	logPath := "/home/sprite/workspace/logs/agent-oneshot.log"
	script := buildOneShotScript("/home/sprite/workspace", "/home/sprite/workspace/.dispatch-prompt.md", logPath)

	// Must create log directory
	if !strings.Contains(script, "mkdir -p '/home/sprite/workspace/logs'") {
		t.Error("script must create logs directory")
	}

	// Must write timestamp header to log file
	if !strings.Contains(script, "[oneshot] starting at") {
		t.Error("script must write start timestamp to log")
	}

	// Must use tee (without -a) to truncate and capture output each dispatch
	if !strings.Contains(script, `| tee "$AGENT_LOG"`) {
		t.Error("script must pipe output to tee for log capture")
	}

	// Must redirect stderr to stdout so errors are captured
	if !strings.Contains(script, "2>&1 | tee") {
		t.Error("script must redirect stderr to stdout for error capture")
	}
}

func TestBuildSetupRepoScriptResetsGitState(t *testing.T) {
	t.Parallel()

	script := buildSetupRepoScript("/workspace", "https://github.com/org/repo.git", "repo")

	// Must reset working tree before fetching
	required := []string{
		"git checkout -- .",
		"git clean -fd",
		"DEFAULT_BRANCH=",
		"git fetch origin",
		"git reset --hard",
	}
	for _, needle := range required {
		if !strings.Contains(script, needle) {
			t.Errorf("script missing %q", needle)
		}
	}
}

func TestBuildSetupRepoScriptResetUsesActualBranch(t *testing.T) {
	t.Parallel()

	script := buildSetupRepoScript("/workspace", "https://github.com/org/repo.git", "repo")

	// After checkout fallback, the reset target must derive from the
	// actually checked-out branch (via rev-parse), not $DEFAULT_BRANCH.
	if !strings.Contains(script, `git rev-parse --abbrev-ref HEAD`) {
		t.Error("script must derive reset target from actual HEAD branch")
	}
	if strings.Contains(script, `origin/$DEFAULT_BRANCH`) {
		t.Error("git reset --hard must NOT use DEFAULT_BRANCH (stale after checkout fallback)")
	}
}

func TestBuildSetupRepoScriptFreshClone(t *testing.T) {
	t.Parallel()

	script := buildSetupRepoScript("/workspace", "https://github.com/org/repo.git", "repo")

	// Fresh clone path must include both gh and git fallback
	if !strings.Contains(script, "gh repo clone") {
		t.Error("script missing gh repo clone for fresh clone path")
	}
	if !strings.Contains(script, "git clone") {
		t.Error("script missing git clone fallback")
	}
}

func TestBuildSetupRepoScriptProgressIndicators(t *testing.T) {
	t.Parallel()

	script := buildSetupRepoScript("/workspace", "https://github.com/org/repo.git", "repo")

	// Must show progress message for existing repo (pull path)
	if !strings.Contains(script, "[setup] pulling latest for") {
		t.Error("script missing progress message for existing repo")
	}

	// Must show progress message for fresh clone
	if !strings.Contains(script, "[setup] cloning") {
		t.Error("script missing progress message for fresh clone")
	}
	if !strings.Contains(script, "(first time, may take a few minutes)") {
		t.Error("script missing 'first time' hint for cold start")
	}

	// Must track timing
	if !strings.Contains(script, "START_TIME=$(date +%s)") {
		t.Error("script missing START_TIME")
	}
	if !strings.Contains(script, "END_TIME=$(date +%s)") {
		t.Error("script missing END_TIME")
	}

	// Must show completion with elapsed time
	if !strings.Contains(script, "[setup] repo ready (${ELAPSED}s)") {
		t.Error("script missing completion message with elapsed time")
	}
}

func TestResolveSkillMountsEnforcesMaxMounts(t *testing.T) {
	// Create temp skill directories
	skillRoot := t.TempDir()
	skills := make([]string, DefaultMaxSkillMounts+2)
	for i := 0; i < DefaultMaxSkillMounts+2; i++ {
		skillDir := filepath.Join(skillRoot, fmt.Sprintf("test-skill-%d", i))
		if err := os.MkdirAll(skillDir, 0o755); err != nil {
			t.Fatalf("mkdir skill dir: %v", err)
		}
		if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("# Skill\n"), 0o644); err != nil {
			t.Fatalf("write SKILL.md: %v", err)
		}
		skills[i] = skillDir
	}

	// Should fail with too many mounts
	_, err := resolveSkillMounts(skills, "/home/sprite/workspace")
	if err == nil {
		t.Fatal("expected error for too many skill mounts")
	}
	if !strings.Contains(err.Error(), "too many --skill mounts") {
		t.Fatalf("error = %v, want mention of too many mounts", err)
	}

	// Should succeed with exactly MaxMounts
	_, err = resolveSkillMounts(skills[:DefaultMaxSkillMounts], "/home/sprite/workspace")
	if err != nil {
		t.Fatalf("unexpected error for max mounts: %v", err)
	}
}

func TestResolveSkillMountsEnforcesMaxFilesPerSkill(t *testing.T) {
	skillRoot := t.TempDir()
	skillDir := filepath.Join(skillRoot, "test-skill")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatalf("mkdir skill dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("# Skill\n"), 0o644); err != nil {
		t.Fatalf("write SKILL.md: %v", err)
	}

	// Create files exceeding the limit
	for i := 0; i < DefaultMaxFilesPerSkill+1; i++ {
		if err := os.WriteFile(filepath.Join(skillDir, fmt.Sprintf("file-%d.txt", i)), []byte("content\n"), 0o644); err != nil {
			t.Fatalf("write file: %v", err)
		}
	}

	_, err := resolveSkillMounts([]string{skillDir}, "/home/sprite/workspace")
	if err == nil {
		t.Fatal("expected error for too many files")
	}
	if !strings.Contains(err.Error(), "files") {
		t.Fatalf("error = %v, want mention of file count", err)
	}
}

func TestResolveSkillMountsEnforcesMaxBytesPerSkill(t *testing.T) {
	skillRoot := t.TempDir()
	skillDir := filepath.Join(skillRoot, "test-skill")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatalf("mkdir skill dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("# Skill\n"), 0o644); err != nil {
		t.Fatalf("write SKILL.md: %v", err)
	}

	// Create a file that exceeds the total bytes limit
	largeContent := make([]byte, DefaultMaxBytesPerSkill+1)
	if err := os.WriteFile(filepath.Join(skillDir, "large-file.bin"), largeContent, 0o644); err != nil {
		t.Fatalf("write large file: %v", err)
	}

	_, err := resolveSkillMounts([]string{skillDir}, "/home/sprite/workspace")
	if err == nil {
		t.Fatal("expected error for exceeding total bytes")
	}
	if !strings.Contains(err.Error(), "size") {
		t.Fatalf("error = %v, want mention of size limit", err)
	}
}

func TestResolveSkillMountsEnforcesMaxFileSize(t *testing.T) {
	skillRoot := t.TempDir()
	skillDir := filepath.Join(skillRoot, "test-skill")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatalf("mkdir skill dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("# Skill\n"), 0o644); err != nil {
		t.Fatalf("write SKILL.md: %v", err)
	}

	// Create a file that exceeds the individual file size limit
	largeContent := make([]byte, DefaultMaxFileSize+1)
	if err := os.WriteFile(filepath.Join(skillDir, "large-file.bin"), largeContent, 0o644); err != nil {
		t.Fatalf("write large file: %v", err)
	}

	_, err := resolveSkillMounts([]string{skillDir}, "/home/sprite/workspace")
	if err == nil {
		t.Fatal("expected error for exceeding max file size")
	}
	if !strings.Contains(err.Error(), "max file size") {
		t.Fatalf("error = %v, want mention of max file size", err)
	}
}

func TestResolveSkillMountsEnforcesSkillNamePattern(t *testing.T) {
	skillRoot := t.TempDir()

	// Invalid skill names that don't match skillNamePattern
	// Note: "has/slash" is excluded because it can't be created as a directory name on most filesystems
	invalidNames := []string{"UPPERCASE", "mixedCase", "123-starts-with-number", "has_underscore", "has.space"}

	for _, name := range invalidNames {
		skillDir := filepath.Join(skillRoot, name)
		if err := os.MkdirAll(skillDir, 0o755); err != nil {
			t.Fatalf("mkdir skill dir %q: %v", name, err)
		}
		if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("# Skill\n"), 0o644); err != nil {
			t.Fatalf("write SKILL.md: %v", err)
		}

		_, err := resolveSkillMounts([]string{skillDir}, "/home/sprite/workspace")
		if err == nil {
			t.Fatalf("expected error for invalid skill name %q", name)
		}
		if !strings.Contains(err.Error(), "invalid skill directory name") {
			t.Fatalf("error = %v, want mention of invalid skill name", err)
		}
	}

	// Valid skill names that match skillNamePattern
	validNames := []string{"valid-skill", "skill123", "a-b-c", "x1", "test"}

	for _, name := range validNames {
		skillDir := filepath.Join(skillRoot, name)
		if err := os.MkdirAll(skillDir, 0o755); err != nil {
			t.Fatalf("mkdir skill dir %q: %v", name, err)
		}
		if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("# Skill\n"), 0o644); err != nil {
			t.Fatalf("write SKILL.md: %v", err)
		}

		_, err := resolveSkillMounts([]string{skillDir}, "/home/sprite/workspace")
		if err != nil {
			t.Fatalf("unexpected error for valid skill name %q: %v", name, err)
		}
	}
}

func TestResolveSkillMountsDetectsCanonicalPathDuplicates(t *testing.T) {
	skillRoot := t.TempDir()
	skillDir := filepath.Join(skillRoot, "test-skill")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatalf("mkdir skill dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("# Skill\n"), 0o644); err != nil {
		t.Fatalf("write SKILL.md: %v", err)
	}

	// Create a subdirectory with a symlink to the same skill
	subDir := filepath.Join(skillRoot, "subdir")
	if err := os.MkdirAll(subDir, 0o755); err != nil {
		t.Fatalf("mkdir subdir: %v", err)
	}
	linkPath := filepath.Join(subDir, "linked-skill")
	if err := os.Symlink(skillDir, linkPath); err != nil {
		t.Skipf("symlink unsupported in test environment: %v", err)
	}

	// Try to mount both the original and the symlink
	_, err := resolveSkillMounts([]string{skillDir, linkPath}, "/home/sprite/workspace")
	if err == nil {
		t.Fatal("expected error for duplicate skill via canonical path")
	}
	if !strings.Contains(err.Error(), "already mounted") && !strings.Contains(err.Error(), "canonical path") {
		t.Fatalf("error = %v, want mention of canonical path duplicate", err)
	}
}

func TestResolveSkillMountsAcceptsCustomLimits(t *testing.T) {
	skillRoot := t.TempDir()
	skillDir := filepath.Join(skillRoot, "test-skill")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatalf("mkdir skill dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("# Skill\n"), 0o644); err != nil {
		t.Fatalf("write SKILL.md: %v", err)
	}

	// Create files that would exceed default limits but pass custom limits
	for i := 0; i < 5; i++ {
		if err := os.WriteFile(filepath.Join(skillDir, fmt.Sprintf("file-%d.txt", i)), []byte("content\n"), 0o644); err != nil {
			t.Fatalf("write file: %v", err)
		}
	}

	// Should fail with very restrictive limits
	strictLimits := resolveSkillLimits{
		MaxMounts:        1,
		MaxFilesPerSkill: 2,
		MaxBytesPerSkill: 100,
		MaxFileSize:      50,
	}

	_, err := resolveSkillMountsWithLimits([]string{skillDir}, "/home/sprite/workspace", strictLimits)
	if err == nil {
		t.Fatal("expected error with strict limits")
	}

	// Should succeed with generous limits
	generousLimits := resolveSkillLimits{
		MaxMounts:        10,
		MaxFilesPerSkill: 100,
		MaxBytesPerSkill: 1024 * 1024,
		MaxFileSize:      1024 * 1024,
	}

	_, err = resolveSkillMountsWithLimits([]string{skillDir}, "/home/sprite/workspace", generousLimits)
	if err != nil {
		t.Fatalf("unexpected error with generous limits: %v", err)
	}
}

func TestScaffoldUploadsBaseFiles(t *testing.T) {
	// Create a temp scaffold directory
	scaffoldDir := t.TempDir()

	// Create base/CLAUDE.md
	if err := os.WriteFile(filepath.Join(scaffoldDir, "CLAUDE.md"), []byte("# Base CLAUDE"), 0o644); err != nil {
		t.Fatalf("write CLAUDE.md: %v", err)
	}

	// Create base/settings.json
	if err := os.WriteFile(filepath.Join(scaffoldDir, "settings.json"), []byte(`{"key":"val"}`), 0o644); err != nil {
		t.Fatalf("write settings.json: %v", err)
	}

	// Create base/hooks/
	if err := os.MkdirAll(filepath.Join(scaffoldDir, "hooks"), 0o755); err != nil {
		t.Fatalf("mkdir hooks: %v", err)
	}
	if err := os.WriteFile(filepath.Join(scaffoldDir, "hooks", "guard.py"), []byte("# hook"), 0o644); err != nil {
		t.Fatalf("write guard.py: %v", err)
	}

	// Create sprites/ dir (sibling of scaffold dir)
	spritesDir := filepath.Join(filepath.Dir(scaffoldDir), "sprites")
	if err := os.MkdirAll(spritesDir, 0o755); err != nil {
		t.Fatalf("mkdir sprites: %v", err)
	}
	if err := os.WriteFile(filepath.Join(spritesDir, "fern.md"), []byte("# Fern persona"), 0o644); err != nil {
		t.Fatalf("write fern.md: %v", err)
	}

	remote := &fakeRemote{
		execResponses: []string{
			"",     // validate env
			"",     // clean signals
			"",     // MEMORY.md init
			"",     // LEARNINGS.md init
			"done", // oneshot agent
		},
		listSprites: []string{"fern"},
	}

	service, err := NewService(Config{
		Remote:      remote,
		Fly:         &fakeFly{},
		App:         "bb-app",
		Workspace:   "/home/sprite/workspace",
		ScaffoldDir: scaffoldDir,
	})
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	result, err := service.Run(context.Background(), Request{
		Sprite:  "fern",
		Prompt:  "Test scaffolding",
		Execute: true,
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.State != StateCompleted {
		t.Fatalf("state = %q, want %q", result.State, StateCompleted)
	}

	// Check uploads include CLAUDE.md, settings.json, hook, and PERSONA.md
	uploadPaths := make(map[string]bool)
	for _, u := range remote.uploads {
		uploadPaths[u.path] = true
	}

	want := []string{
		"/home/sprite/workspace/CLAUDE.md",
		"/home/sprite/workspace/.claude/settings.json",
		"/home/sprite/workspace/.claude/hooks/guard.py",
		"/home/sprite/workspace/PERSONA.md",
	}
	for _, p := range want {
		if !uploadPaths[p] {
			t.Errorf("missing upload: %s", p)
		}
	}

	// Check plan includes scaffold step
	hasScaffold := false
	for _, step := range result.Plan.Steps {
		if step.Kind == StepUploadScaffold {
			hasScaffold = true
		}
	}
	if !hasScaffold {
		t.Error("plan missing StepUploadScaffold")
	}
}
