package lifecycle

import (
	"context"
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
	Name    string `json:"name"`
	Created bool   `json:"created"` // false when sprite already existed
}

// ProvisionOpts configures provisioning for one sprite.
type ProvisionOpts struct {
	Name             string
	CompositionLabel string
	SettingsPath     string
	GitHubAuth       GitHubAuth
	BootstrapScript  string
	AgentScript      string
}

// Provision creates or refreshes a sprite and pushes base/persona/bootstrap config.
func Provision(ctx context.Context, cli sprite.SpriteCLI, cfg Config, opts ProvisionOpts) (ProvisionResult, error) {
	if err := requireConfig(cfg); err != nil {
		return ProvisionResult{}, err
	}

	name := strings.TrimSpace(opts.Name)
	if err := ValidateSpriteName(name); err != nil {
		return ProvisionResult{}, err
	}
	definition := spriteDefinitionPath(cfg, name)
	if _, err := os.Stat(definition); err != nil {
		return ProvisionResult{}, fmt.Errorf("no sprite definition found at %s: %w", definition, err)
	}

	exists, err := spriteExists(ctx, cli, name)
	if err != nil {
		return ProvisionResult{}, fmt.Errorf("check sprite existence %q: %w", name, err)
	}

	created := false
	if !exists {
		if err := cli.Create(ctx, name, cfg.Org); err != nil {
			return ProvisionResult{}, err
		}
		created = true
	}

	if _, err := cli.Exec(ctx, name, "mkdir -p "+shellQuote(cfg.Workspace), nil); err != nil {
		return ProvisionResult{}, fmt.Errorf("setup workspace for %q: %w", name, err)
	}

	if err := PushConfig(ctx, cli, cfg, name, opts.SettingsPath); err != nil {
		return ProvisionResult{}, err
	}

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

	if err := cli.UploadFile(ctx, name, cfg.Org, bootstrapScript, "/tmp/sprite-bootstrap.sh"); err != nil {
		return ProvisionResult{}, err
	}
	if err := cli.UploadFile(ctx, name, cfg.Org, agentScript, "/tmp/sprite-agent.sh"); err != nil {
		return ProvisionResult{}, err
	}
	if _, err := cli.Exec(ctx, name, "bash /tmp/sprite-bootstrap.sh --agent-source /tmp/sprite-agent.sh", nil); err != nil {
		return ProvisionResult{}, fmt.Errorf("run bootstrap for %q: %w", name, err)
	}

	// Preserve shell behavior: checkpoint failure is non-fatal.
	_ = cli.CheckpointCreate(ctx, name, cfg.Org)

	return ProvisionResult{Name: name, Created: created}, nil
}
