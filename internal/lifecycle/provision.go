package lifecycle

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/misty-step/bitterblossom/internal/sprite"
)

const gitAuthProbeRepository = "https://github.com/misty-step/cerberus.git"

// ProvisionResult reports provisioning outcome for one sprite.
type ProvisionResult struct {
	Name      string `json:"name"`
	MachineID string `json:"machine_id,omitempty"` // Fly machine ID, resolved best-effort
	Created   bool   `json:"created"`              // false when sprite already existed
}

// ProvisionStage identifies a single provisioning step.
type ProvisionStage string

const (
	ProvisionStageValidate         ProvisionStage = "validate"
	ProvisionStageCheckExists      ProvisionStage = "check_exists"
	ProvisionStageCreate           ProvisionStage = "create"
	ProvisionStagePrepareWorkspace ProvisionStage = "prepare_workspace"
	ProvisionStagePushConfig       ProvisionStage = "push_config"
	ProvisionStageUploadPersona    ProvisionStage = "upload_persona"
	ProvisionStageWriteMemory      ProvisionStage = "write_memory"
	ProvisionStageConfigureGit     ProvisionStage = "configure_git"
	ProvisionStageVerifyGit        ProvisionStage = "verify_git"
	ProvisionStageUploadBootstrap  ProvisionStage = "upload_bootstrap"
	ProvisionStageUploadAgent      ProvisionStage = "upload_agent"
	ProvisionStageUploadProxy      ProvisionStage = "upload_proxy"
	ProvisionStageRunBootstrap     ProvisionStage = "run_bootstrap"
	ProvisionStageCheckpoint       ProvisionStage = "checkpoint"
	ProvisionStageComplete         ProvisionStage = "complete"
)

// ProvisionProgress reports incremental status for one sprite provision run.
type ProvisionProgress struct {
	Name    string         `json:"name"`
	Stage   ProvisionStage `json:"stage"`
	Message string         `json:"message"`
}

// ProvisionOpts configures provisioning for one sprite.
type ProvisionOpts struct {
	Name             string
	CompositionLabel string
	SettingsPath     string
	GitHubAuth       GitHubAuth
	BootstrapScript  string
	AgentScript      string
	Progress         func(ProvisionProgress)
}

func emitProvisionProgress(name string, progressFn func(ProvisionProgress), stage ProvisionStage, message string) {
	if progressFn == nil {
		return
	}
	progressFn(ProvisionProgress{
		Name:    name,
		Stage:   stage,
		Message: message,
	})
}

// Provision creates or refreshes a sprite and pushes base/persona/bootstrap config.
func Provision(ctx context.Context, cli sprite.SpriteCLI, cfg Config, opts ProvisionOpts) (ProvisionResult, error) {
	if err := requireConfig(cfg); err != nil {
		return ProvisionResult{}, err
	}

	name := strings.TrimSpace(opts.Name)
	emitProvisionProgress(name, opts.Progress, ProvisionStageValidate, "validating sprite definition")
	if err := ValidateSpriteName(name); err != nil {
		return ProvisionResult{}, err
	}
	definition := spriteDefinitionPath(cfg, name)
	if _, err := os.Stat(definition); err != nil {
		return ProvisionResult{}, fmt.Errorf("no sprite definition found at %s: %w", definition, err)
	}

	emitProvisionProgress(name, opts.Progress, ProvisionStageCheckExists, "checking if sprite already exists")
	exists, err := spriteExists(ctx, cli, name)
	if err != nil {
		return ProvisionResult{}, fmt.Errorf("check sprite existence %q: %w", name, err)
	}

	created := false
	if !exists {
		emitProvisionProgress(name, opts.Progress, ProvisionStageCreate, "creating sprite")
		if err := cli.Create(ctx, name, cfg.Org); err != nil {
			return ProvisionResult{}, err
		}
		created = true
	}

	emitProvisionProgress(name, opts.Progress, ProvisionStagePrepareWorkspace, "preparing workspace")
	if _, err := cli.Exec(ctx, name, "mkdir -p "+shellQuote(cfg.Workspace), nil); err != nil {
		return ProvisionResult{}, fmt.Errorf("setup workspace for %q: %w", name, err)
	}

	emitProvisionProgress(name, opts.Progress, ProvisionStagePushConfig, "uploading base config")
	if err := PushConfig(ctx, cli, cfg, name, opts.SettingsPath); err != nil {
		return ProvisionResult{}, err
	}

	emitProvisionProgress(name, opts.Progress, ProvisionStageUploadPersona, "uploading persona")
	if err := cli.UploadFile(ctx, name, cfg.Org, definition, path.Join(cfg.Workspace, "PERSONA.md")); err != nil {
		return ProvisionResult{}, err
	}

	compositionLabel := strings.TrimSpace(opts.CompositionLabel)
	if compositionLabel == "" {
		compositionLabel = "unknown"
	}
	timestamp := time.Now().UTC().Format(time.RFC3339)
	memory := fmt.Sprintf(
		"# MEMORY.md — %s\n\n"+
			"Sprite: %s\n"+
			"Provisioned: %s\n"+
			"Composition: %s\n\n"+
			"## Learnings\n\n"+
			"_No observations yet. Update after completing work._\n",
		name,
		name,
		timestamp,
		compositionLabel,
	)
	emitProvisionProgress(name, opts.Progress, ProvisionStageWriteMemory, "writing MEMORY.md")
	if _, err := cli.Exec(ctx, name, "cat > "+shellQuote(path.Join(cfg.Workspace, "MEMORY.md")), []byte(memory)); err != nil {
		return ProvisionResult{}, fmt.Errorf("write initial MEMORY.md for %q: %w", name, err)
	}

	auth := opts.GitHubAuth
	if strings.TrimSpace(auth.User) == "" || strings.TrimSpace(auth.Email) == "" || strings.TrimSpace(auth.Token) == "" {
		return ProvisionResult{}, fmt.Errorf("GitHub auth is incomplete for sprite %q", name)
	}

	credentials := fmt.Sprintf("https://%s:%s@github.com", auth.User, auth.Token)
	gitIdentity := fmt.Sprintf("%s (%s sprite)", name, auth.User)
	configureGitCommand := strings.Join([]string{
		"git config --global user.name " + shellQuote(gitIdentity),
		"git config --global user.email " + shellQuote(auth.Email),
		"git config --global credential.helper store",
		"printf '%s\\n' " + shellQuote(credentials) + " > " + shellQuote(path.Join(cfg.RemoteHome, ".git-credentials")),
		"echo " + shellQuote("Git credentials configured for "+auth.User),
	}, " && ")
	emitProvisionProgress(name, opts.Progress, ProvisionStageConfigureGit, "configuring git credentials")
	if _, err := cli.Exec(ctx, name, configureGitCommand, nil); err != nil {
		return ProvisionResult{}, fmt.Errorf("configure git credentials for %q: %w", name, err)
	}

	verifyGitAuthCommand := strings.Join([]string{
		"cd /tmp",
		"rm -rf _git_test",
		"mkdir _git_test",
		"cd _git_test",
		"git init -q",
		"git remote add origin " + shellQuote(gitAuthProbeRepository),
		"git ls-remote origin HEAD >/dev/null 2>&1 && echo GIT_AUTH_OK || echo GIT_AUTH_FAIL",
	}, " && ")
	emitProvisionProgress(name, opts.Progress, ProvisionStageVerifyGit, "verifying git auth")
	verifyOutput, err := cli.Exec(ctx, name, verifyGitAuthCommand, nil)
	if err != nil {
		return ProvisionResult{}, fmt.Errorf("verify git auth for %q: %w", name, err)
	}
	if !strings.Contains(verifyOutput, "GIT_AUTH_OK") {
		return ProvisionResult{}, fmt.Errorf("git auth verification failed for sprite %q", name)
	}

	bootstrapScript := strings.TrimSpace(opts.BootstrapScript)
	if bootstrapScript == "" {
		bootstrapScript = filepath.Join(cfg.RootDir, "scripts", "sprite-bootstrap.sh")
	}
	agentScript := strings.TrimSpace(opts.AgentScript)
	if agentScript == "" {
		agentScript = filepath.Join(cfg.RootDir, "scripts", "sprite-agent.sh")
	}
	if _, err := os.Stat(bootstrapScript); err != nil {
		return ProvisionResult{}, fmt.Errorf("missing bootstrap script %q: %w", bootstrapScript, err)
	}
	if _, err := os.Stat(agentScript); err != nil {
		return ProvisionResult{}, fmt.Errorf("missing agent script %q: %w", agentScript, err)
	}

	emitProvisionProgress(name, opts.Progress, ProvisionStageUploadBootstrap, "uploading bootstrap script")
	if err := cli.UploadFile(ctx, name, cfg.Org, bootstrapScript, "/tmp/sprite-bootstrap.sh"); err != nil {
		return ProvisionResult{}, err
	}
	emitProvisionProgress(name, opts.Progress, ProvisionStageUploadAgent, "uploading agent script")
	if err := cli.UploadFile(ctx, name, cfg.Org, agentScript, "/tmp/sprite-agent.sh"); err != nil {
		return ProvisionResult{}, err
	}

	// Note: anthropic proxy is uploaded by PushConfig() above, no need to duplicate here

	emitProvisionProgress(name, opts.Progress, ProvisionStageRunBootstrap, "running bootstrap")
	if _, err := cli.Exec(ctx, name, "bash /tmp/sprite-bootstrap.sh --agent-source /tmp/sprite-agent.sh", nil); err != nil {
		return ProvisionResult{}, fmt.Errorf("run bootstrap for %q: %w", name, err)
	}

	// Preserve shell behavior: checkpoint failure is non-fatal.
	emitProvisionProgress(name, opts.Progress, ProvisionStageCheckpoint, "creating checkpoint")
	_ = cli.CheckpointCreate(ctx, name, cfg.Org)

	// Best-effort machine ID resolution via sprite API.
	machineID := resolveMachineID(ctx, cli, cfg.Org, name)

	emitProvisionProgress(name, opts.Progress, ProvisionStageComplete, "provision complete")
	return ProvisionResult{Name: name, MachineID: machineID, Created: created}, nil
}

// resolveMachineID queries the sprite API for the underlying Fly machine ID.
// Returns empty string on any failure (best-effort, non-fatal).
func resolveMachineID(ctx context.Context, cli sprite.SpriteCLI, org, name string) string {
	raw, err := cli.APISprite(ctx, org, name, "/")
	if err != nil {
		return ""
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return ""
	}
	// Try common field names for machine ID.
	for _, key := range []string{"machine_id", "id"} {
		if v, ok := payload[key].(string); ok && v != "" {
			return v
		}
	}
	return ""
}
