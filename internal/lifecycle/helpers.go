package lifecycle

import (
	"context"
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"github.com/misty-step/bitterblossom/internal/sprite"
)

func spriteExists(ctx context.Context, cli sprite.SpriteCLI, name string) (bool, error) {
	names, err := cli.List(ctx)
	if err != nil {
		return false, err
	}
	return slices.Contains(names, name), nil
}

func spriteDefinitionPath(cfg Config, spriteName string) string {
	return filepath.Join(cfg.SpritesDir, spriteName+".md")
}

func requireConfig(cfg Config) error {
	if strings.TrimSpace(cfg.Org) == "" {
		return fmt.Errorf("org is required")
	}
	if strings.TrimSpace(cfg.RemoteHome) == "" {
		return fmt.Errorf("remote home is required")
	}
	if strings.TrimSpace(cfg.Workspace) == "" {
		return fmt.Errorf("workspace is required")
	}
	if strings.TrimSpace(cfg.BaseDir) == "" {
		return fmt.Errorf("base dir is required")
	}
	if strings.TrimSpace(cfg.SpritesDir) == "" {
		return fmt.Errorf("sprites dir is required")
	}
	if strings.TrimSpace(cfg.RootDir) == "" {
		return fmt.Errorf("root dir is required")
	}
	return nil
}
