package lifecycle

import (
	"context"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/misty-step/bitterblossom/internal/sprite"
)

// SyncOpts configures sync for one sprite.
type SyncOpts struct {
	Name         string
	SettingsPath string
	BaseOnly     bool
	AgentScript  string // Path to sprite-agent.sh (optional, defaults to scripts/sprite-agent.sh)
}

// Sync uploads base config, persona definition, and agent script to one sprite.
func Sync(ctx context.Context, cli sprite.SpriteCLI, cfg Config, opts SyncOpts) error {
	if err := requireConfig(cfg); err != nil {
		return err
	}

	name := strings.TrimSpace(opts.Name)
	if err := ValidateSpriteName(name); err != nil {
		return err
	}

	exists, err := spriteExists(ctx, cli, name)
	if err != nil {
		return fmt.Errorf("check sprite existence %q: %w", name, err)
	}
	if !exists {
		return fmt.Errorf("sprite %q does not exist; run provision first", name)
	}

	if err := PushConfig(ctx, cli, cfg, name, opts.SettingsPath); err != nil {
		return err
	}

	// Upload agent script if available
	agentScript := strings.TrimSpace(opts.AgentScript)
	if agentScript == "" {
		agentScript = filepath.Join(cfg.RootDir, "scripts", "sprite-agent.sh")
	}
	if _, err := os.Stat(agentScript); err == nil {
		if err := cli.UploadFile(ctx, name, cfg.Org, agentScript, "/tmp/sprite-agent.sh"); err != nil {
			return fmt.Errorf("upload agent script for %q: %w", name, err)
		}
		// Install the agent script to the proper location
		if _, err := cli.Exec(ctx, name, "mkdir -p $HOME/.local/bin && cp /tmp/sprite-agent.sh $HOME/.local/bin/sprite-agent && chmod +x $HOME/.local/bin/sprite-agent", nil); err != nil {
			return fmt.Errorf("install agent script for %q: %w", name, err)
		}
	}

	if opts.BaseOnly {
		return nil
	}

	definition := spriteDefinitionPath(cfg, name)
	if _, err := os.Stat(definition); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("stat persona definition %q: %w", definition, err)
	}
	return cli.UploadFile(ctx, name, cfg.Org, definition, path.Join(cfg.Workspace, "PERSONA.md"))
}
