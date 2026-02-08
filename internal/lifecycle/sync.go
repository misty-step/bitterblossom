package lifecycle

import (
	"context"
	"fmt"
	"os"
	"path"
	"strings"

	"github.com/misty-step/bitterblossom/internal/sprite"
)

// SyncOpts configures sync for one sprite.
type SyncOpts struct {
	Name         string
	SettingsPath string
	BaseOnly     bool
}

// Sync uploads base config, and optionally persona definition, to one sprite.
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

	if opts.BaseOnly {
		return nil
	}

	definition := spriteDefinitionPath(cfg, name)
	if _, err := os.Stat(definition); err != nil {
		return nil
	}
	return cli.UploadFile(ctx, name, cfg.Org, definition, path.Join(cfg.Workspace, "PERSONA.md"))
}
