package lifecycle

import (
	"context"
	"strings"
	"testing"

	"github.com/misty-step/bitterblossom/internal/sprite"
)

func TestProvisionHappyPath(t *testing.T) {
	t.Parallel()

	fx := newFixture(t, "bramble")
	var createCalled bool
	var checkpointCalled bool
	uploaded := make([]string, 0, 8)
	execCommands := make([]string, 0, 16)

	cli := &sprite.MockSpriteCLI{
		ListFn: func(context.Context) ([]string, error) {
			return []string{}, nil
		},
		CreateFn: func(context.Context, string, string) error {
			createCalled = true
			return nil
		},
		ExecFn: func(_ context.Context, _ string, command string, _ []byte) (string, error) {
			execCommands = append(execCommands, command)
			if strings.Contains(command, "git ls-remote") {
				return "GIT_AUTH_OK\n", nil
			}
			return "", nil
		},
		UploadFileFn: func(_ context.Context, _ string, _ string, _ string, remotePath string) error {
			uploaded = append(uploaded, remotePath)
			return nil
		},
		CheckpointCreateFn: func(context.Context, string, string) error {
			checkpointCalled = true
			return nil
		},
	}

	result, err := Provision(context.Background(), cli, fx.cfg, ProvisionOpts{
		Name:             "bramble",
		CompositionLabel: "v1",
		SettingsPath:     fx.settingsPath,
		GitHubAuth: GitHubAuth{
			User:  "sprite-user",
			Email: "sprite@example.com",
			Token: "sprite-token",
		},
		BootstrapScript: fx.rootDir + "/scripts/sprite-bootstrap.sh",
		AgentScript:     fx.rootDir + "/scripts/sprite-agent.sh",
	})
	if err != nil {
		t.Fatalf("Provision() error = %v", err)
	}
	if result.Name != "bramble" || !result.Created {
		t.Fatalf("unexpected result: %+v", result)
	}
	if !createCalled {
		t.Fatal("expected create to be called")
	}
	if !checkpointCalled {
		t.Fatal("expected checkpoint create to be called")
	}
	if !containsString(uploaded, "/home/sprite/workspace/PERSONA.md") {
		t.Fatalf("persona upload missing: %#v", uploaded)
	}
	if !containsAny(execCommands, "bash /tmp/sprite-bootstrap.sh --agent-source /tmp/sprite-agent.sh") {
		t.Fatalf("bootstrap command missing: %#v", execCommands)
	}
}

func TestProvisionSpriteAlreadyExistsSkipsCreate(t *testing.T) {
	t.Parallel()

	fx := newFixture(t, "bramble")
	var createCalled bool
	cli := &sprite.MockSpriteCLI{
		ListFn: func(context.Context) ([]string, error) {
			return []string{"bramble"}, nil
		},
		CreateFn: func(context.Context, string, string) error {
			createCalled = true
			return nil
		},
		ExecFn: func(_ context.Context, _ string, command string, _ []byte) (string, error) {
			if strings.Contains(command, "git ls-remote") {
				return "GIT_AUTH_OK\n", nil
			}
			return "", nil
		},
		UploadFileFn:       func(context.Context, string, string, string, string) error { return nil },
		CheckpointCreateFn: func(context.Context, string, string) error { return nil },
	}

	result, err := Provision(context.Background(), cli, fx.cfg, ProvisionOpts{
		Name:             "bramble",
		CompositionLabel: "v1",
		SettingsPath:     fx.settingsPath,
		GitHubAuth: GitHubAuth{
			User:  "sprite-user",
			Email: "sprite@example.com",
			Token: "sprite-token",
		},
		BootstrapScript: fx.rootDir + "/scripts/sprite-bootstrap.sh",
		AgentScript:     fx.rootDir + "/scripts/sprite-agent.sh",
	})
	if err != nil {
		t.Fatalf("Provision() error = %v", err)
	}
	if result.Created {
		t.Fatalf("expected Created=false, got %+v", result)
	}
	if createCalled {
		t.Fatal("create should be skipped for existing sprite")
	}
}

func TestProvisionReportsProgressStages(t *testing.T) {
	t.Parallel()

	fx := newFixture(t, "bramble")
	stages := make([]ProvisionStage, 0, 16)
	cli := &sprite.MockSpriteCLI{
		ListFn: func(context.Context) ([]string, error) {
			return []string{}, nil
		},
		CreateFn: func(context.Context, string, string) error {
			return nil
		},
		ExecFn: func(_ context.Context, _ string, command string, _ []byte) (string, error) {
			if strings.Contains(command, "git ls-remote") {
				return "GIT_AUTH_OK\n", nil
			}
			return "", nil
		},
		UploadFileFn:       func(context.Context, string, string, string, string) error { return nil },
		CheckpointCreateFn: func(context.Context, string, string) error { return nil },
	}

	_, err := Provision(context.Background(), cli, fx.cfg, ProvisionOpts{
		Name:             "bramble",
		CompositionLabel: "v1",
		SettingsPath:     fx.settingsPath,
		GitHubAuth: GitHubAuth{
			User:  "sprite-user",
			Email: "sprite@example.com",
			Token: "sprite-token",
		},
		BootstrapScript: fx.rootDir + "/scripts/sprite-bootstrap.sh",
		AgentScript:     fx.rootDir + "/scripts/sprite-agent.sh",
		Progress: func(progress ProvisionProgress) {
			stages = append(stages, progress.Stage)
		},
	})
	if err != nil {
		t.Fatalf("Provision() error = %v", err)
	}

	expected := []ProvisionStage{
		ProvisionStageValidate,
		ProvisionStageCheckExists,
		ProvisionStageCreate,
		ProvisionStagePrepareWorkspace,
		ProvisionStagePushConfig,
		ProvisionStageUploadPersona,
		ProvisionStageWriteMemory,
		ProvisionStageConfigureGit,
		ProvisionStageVerifyGit,
		ProvisionStageUploadBootstrap,
		ProvisionStageUploadAgent,
		ProvisionStageRunBootstrap,
		ProvisionStageCheckpoint,
		ProvisionStageComplete,
	}
	if len(stages) < len(expected) {
		t.Fatalf("expected at least %d progress stages, got %d (%v)", len(expected), len(stages), stages)
	}
	for i, stage := range expected {
		if stages[i] != stage {
			t.Fatalf("stage[%d] = %q, want %q (all=%v)", i, stages[i], stage, stages)
		}
	}
}

func TestProvisionInvalidSpriteName(t *testing.T) {
	t.Parallel()

	fx := newFixture(t, "bramble")
	_, err := Provision(context.Background(), &sprite.MockSpriteCLI{}, fx.cfg, ProvisionOpts{
		Name:         "Bramble",
		SettingsPath: fx.settingsPath,
	})
	if err == nil {
		t.Fatal("expected validation error")
	}
}

func TestProvisionGitHubAuthFailure(t *testing.T) {
	t.Parallel()

	fx := newFixture(t, "bramble")
	cli := &sprite.MockSpriteCLI{
		ListFn:             func(context.Context) ([]string, error) { return []string{}, nil },
		CreateFn:           func(context.Context, string, string) error { return nil },
		ExecFn:             func(context.Context, string, string, []byte) (string, error) { return "", nil },
		UploadFileFn:       func(context.Context, string, string, string, string) error { return nil },
		CheckpointCreateFn: func(context.Context, string, string) error { return nil },
	}

	_, err := Provision(context.Background(), cli, fx.cfg, ProvisionOpts{
		Name:             "bramble",
		CompositionLabel: "v1",
		SettingsPath:     fx.settingsPath,
		GitHubAuth: GitHubAuth{
			User:  "sprite-user",
			Email: "sprite@example.com",
			Token: "",
		},
		BootstrapScript: fx.rootDir + "/scripts/sprite-bootstrap.sh",
		AgentScript:     fx.rootDir + "/scripts/sprite-agent.sh",
	})
	if err == nil {
		t.Fatal("expected GitHub auth error")
	}
}

func TestProvisionGitAuthVerificationFailure(t *testing.T) {
	t.Parallel()

	fx := newFixture(t, "bramble")
	cli := &sprite.MockSpriteCLI{
		ListFn:       func(context.Context) ([]string, error) { return []string{}, nil },
		CreateFn:     func(context.Context, string, string) error { return nil },
		UploadFileFn: func(context.Context, string, string, string, string) error { return nil },
		ExecFn: func(_ context.Context, _ string, command string, _ []byte) (string, error) {
			if strings.Contains(command, "git ls-remote") {
				return "GIT_AUTH_FAIL\n", nil
			}
			return "", nil
		},
	}

	_, err := Provision(context.Background(), cli, fx.cfg, ProvisionOpts{
		Name:             "bramble",
		CompositionLabel: "v1",
		SettingsPath:     fx.settingsPath,
		GitHubAuth: GitHubAuth{
			User:  "sprite-user",
			Email: "sprite@example.com",
			Token: "sprite-token",
		},
		BootstrapScript: fx.rootDir + "/scripts/sprite-bootstrap.sh",
		AgentScript:     fx.rootDir + "/scripts/sprite-agent.sh",
	})
	if err == nil {
		t.Fatal("expected git auth verification failure")
	}
}

func containsAny(values []string, want string) bool {
	for _, value := range values {
		if strings.Contains(value, want) {
			return true
		}
	}
	return false
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
