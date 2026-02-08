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

// TeardownResult describes a completed teardown archive.
type TeardownResult struct {
	Name        string `json:"name"`
	ArchivePath string `json:"archive_path"`
}

// TeardownOpts configures teardown behavior.
type TeardownOpts struct {
	Name       string
	ArchiveDir string
	Force      bool // CLI handles confirmation; this flag is informational.
}

// Teardown exports learnings and destroys one sprite.
func Teardown(ctx context.Context, cli sprite.SpriteCLI, cfg Config, opts TeardownOpts) (TeardownResult, error) {
	if err := requireConfig(cfg); err != nil {
		return TeardownResult{}, err
	}

	name := strings.TrimSpace(opts.Name)
	if err := ValidateSpriteName(name); err != nil {
		return TeardownResult{}, err
	}

	exists, err := spriteExists(ctx, cli, name)
	if err != nil {
		return TeardownResult{}, fmt.Errorf("check sprite existence %q: %w", name, err)
	}
	if !exists {
		return TeardownResult{}, fmt.Errorf("sprite %q does not exist", name)
	}

	archiveDir := strings.TrimSpace(opts.ArchiveDir)
	if archiveDir == "" {
		return TeardownResult{}, fmt.Errorf("archive dir is required")
	}

	timestamp := time.Now().UTC().Format("20060102T150405Z")
	archivePath := filepath.Join(archiveDir, name+"-"+timestamp)
	if err := os.MkdirAll(archivePath, 0o700); err != nil {
		return TeardownResult{}, fmt.Errorf("create archive path %q: %w", archivePath, err)
	}

	// Exports are non-fatal: sprite may not have these files yet (e.g. fresh provision).
	_ = exportRemoteFile(ctx, cli, name, path.Join(cfg.Workspace, "MEMORY.md"), filepath.Join(archivePath, "MEMORY.md"))
	_ = exportRemoteFile(ctx, cli, name, path.Join(cfg.Workspace, "CLAUDE.md"), filepath.Join(archivePath, "CLAUDE.md"))
	_ = exportSettings(ctx, cli, name, path.Join(cfg.RemoteHome, ".claude", "settings.json"), filepath.Join(archivePath, "settings.json"))

	// Preserve shell behavior: final checkpoint failure is non-fatal.
	_ = cli.CheckpointCreate(ctx, name, cfg.Org)

	if err := cli.Destroy(ctx, name, cfg.Org); err != nil {
		return TeardownResult{}, err
	}

	return TeardownResult{
		Name:        name,
		ArchivePath: archivePath,
	}, nil
}

func exportRemoteFile(ctx context.Context, cli sprite.SpriteCLI, spriteName, remotePath, localPath string) error {
	content, err := cli.Exec(ctx, spriteName, "cat "+shellQuote(remotePath), nil)
	if err != nil {
		return err
	}
	return os.WriteFile(localPath, []byte(content), 0o600)
}

func exportSettings(ctx context.Context, cli sprite.SpriteCLI, spriteName, remotePath, localPath string) error {
	content, err := cli.Exec(ctx, spriteName, "cat "+shellQuote(remotePath), nil)
	if err != nil {
		return err
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(content), &payload); err != nil {
		return err
	}

	if envAny, ok := payload["env"]; ok {
		if env, ok := envAny.(map[string]any); ok {
			if _, exists := env["ANTHROPIC_AUTH_TOKEN"]; exists {
				env["ANTHROPIC_AUTH_TOKEN"] = "__REDACTED__"
			}
		}
	}

	encoded, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return err
	}
	encoded = append(encoded, '\n')
	return os.WriteFile(localPath, encoded, 0o600)
}
