package lifecycle

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"

	"github.com/misty-step/bitterblossom/internal/proxy"
	"github.com/misty-step/bitterblossom/internal/sprite"
)

// PushConfig uploads base config (CLAUDE.md, hooks, skills, commands, settings) to a sprite.
func PushConfig(ctx context.Context, cli sprite.SpriteCLI, cfg Config, spriteName, settingsPath string) error {
	if err := cli.UploadFile(
		ctx,
		spriteName,
		cfg.Org,
		filepath.Join(cfg.BaseDir, "CLAUDE.md"),
		path.Join(cfg.Workspace, "CLAUDE.md"),
	); err != nil {
		return err
	}
	if err := UploadDir(
		ctx,
		cli,
		cfg,
		spriteName,
		filepath.Join(cfg.BaseDir, "hooks"),
		path.Join(cfg.RemoteHome, ".claude", "hooks"),
	); err != nil {
		return err
	}
	if err := UploadDir(
		ctx,
		cli,
		cfg,
		spriteName,
		filepath.Join(cfg.BaseDir, "skills"),
		path.Join(cfg.RemoteHome, ".claude", "skills"),
	); err != nil {
		return err
	}
	if err := UploadDir(
		ctx,
		cli,
		cfg,
		spriteName,
		filepath.Join(cfg.BaseDir, "commands"),
		path.Join(cfg.RemoteHome, ".claude", "commands"),
	); err != nil {
		return err
	}
	if err := cli.UploadFile(
		ctx,
		spriteName,
		cfg.Org,
		settingsPath,
		path.Join(cfg.RemoteHome, ".claude", "settings.json"),
	); err != nil {
		return err
	}

	// Deploy sprite-agent if present in the repo.
	agentScript := filepath.Join(cfg.RootDir, "scripts", "sprite-agent.sh")
	if _, err := os.Stat(agentScript); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("stat agent script %q: %w", agentScript, err)
	} else if err == nil {
		agentDest := path.Join(cfg.RemoteHome, ".local", "bin", "sprite-agent")
		if _, err := cli.Exec(ctx, spriteName, "mkdir -p "+shellQuote(path.Dir(agentDest)), nil); err != nil {
			return fmt.Errorf("create agent dir: %w", err)
		}
		if err := cli.UploadFile(ctx, spriteName, cfg.Org, agentScript, agentDest); err != nil {
			return fmt.Errorf("upload agent script: %w", err)
		}
		if _, err := cli.Exec(ctx, spriteName, "chmod +x "+shellQuote(agentDest), nil); err != nil {
			return fmt.Errorf("chmod agent: %w", err)
		}
	}

	// Deploy anthropic proxy (embedded).
	if err := cli.Upload(ctx, spriteName, proxy.ProxyScriptPath, proxy.ProxyScript); err != nil {
		return fmt.Errorf("upload anthropic proxy: %w", err)
	}

	return nil
}

// UploadDir recursively uploads a local directory to a remote path on a sprite.
func UploadDir(ctx context.Context, cli sprite.SpriteCLI, cfg Config, spriteName, localDir, remoteDir string) error {
	if _, err := cli.Exec(ctx, spriteName, "mkdir -p "+shellQuote(remoteDir), nil); err != nil {
		return fmt.Errorf("create remote dir %q: %w", remoteDir, err)
	}

	files := make([]string, 0, 32)
	if err := filepath.WalkDir(localDir, func(entryPath string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		files = append(files, entryPath)
		return nil
	}); err != nil {
		return fmt.Errorf("walk local dir %q: %w", localDir, err)
	}
	sort.Strings(files)

	for _, localPath := range files {
		rel, err := filepath.Rel(localDir, localPath)
		if err != nil {
			return fmt.Errorf("relative path for %q: %w", localPath, err)
		}
		rel = filepath.ToSlash(rel)
		remotePath := path.Join(remoteDir, rel)
		remoteParent := path.Dir(remotePath)

		if _, err := cli.Exec(ctx, spriteName, "mkdir -p "+shellQuote(remoteParent), nil); err != nil {
			return fmt.Errorf("create remote dir %q: %w", remoteParent, err)
		}
		if err := cli.UploadFile(ctx, spriteName, cfg.Org, localPath, remotePath); err != nil {
			return err
		}
	}
	return nil
}
