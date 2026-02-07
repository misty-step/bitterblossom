package lib

import (
	"context"
	"fmt"
	"io/fs"
	"path/filepath"
	"sort"
)

func UploadFile(ctx context.Context, sprite *SpriteCLI, spriteName, localPath, remotePath string) error {
	_, err := sprite.ExecWithFile(ctx, spriteName, localPath, remotePath, true, "echo", fmt.Sprintf("uploaded: %s", remotePath))
	return err
}

func UploadDir(ctx context.Context, sprite *SpriteCLI, spriteName, localDir, remoteDir string) error {
	if _, err := sprite.Exec(ctx, spriteName, true, "mkdir", "-p", remoteDir); err != nil {
		return err
	}

	files := make([]string, 0)
	err := filepath.WalkDir(localDir, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		files = append(files, path)
		return nil
	})
	if err != nil {
		return fmt.Errorf("walk directory %s: %w", localDir, err)
	}
	sort.Strings(files)

	for _, file := range files {
		rel, err := filepath.Rel(localDir, file)
		if err != nil {
			return fmt.Errorf("derive relative path for %s: %w", file, err)
		}
		dest := filepath.ToSlash(filepath.Join(remoteDir, rel))
		parent := filepath.ToSlash(filepath.Dir(dest))
		if _, err := sprite.Exec(ctx, spriteName, true, "mkdir", "-p", parent); err != nil {
			return err
		}
		if err := UploadFile(ctx, sprite, spriteName, file, dest); err != nil {
			return err
		}
	}
	return nil
}

func PushConfig(ctx context.Context, sprite *SpriteCLI, paths Paths, spriteName, settingsPath string) error {
	if err := UploadFile(ctx, sprite, spriteName, filepath.Join(paths.BaseDir, "CLAUDE.md"), filepath.ToSlash(filepath.Join(DefaultRemoteHome, "workspace", "CLAUDE.md"))); err != nil {
		return err
	}
	if err := UploadDir(ctx, sprite, spriteName, filepath.Join(paths.BaseDir, "hooks"), filepath.ToSlash(filepath.Join(DefaultRemoteHome, ".claude", "hooks"))); err != nil {
		return err
	}
	if err := UploadDir(ctx, sprite, spriteName, filepath.Join(paths.BaseDir, "skills"), filepath.ToSlash(filepath.Join(DefaultRemoteHome, ".claude", "skills"))); err != nil {
		return err
	}
	if err := UploadDir(ctx, sprite, spriteName, filepath.Join(paths.BaseDir, "commands"), filepath.ToSlash(filepath.Join(DefaultRemoteHome, ".claude", "commands"))); err != nil {
		return err
	}
	if err := UploadFile(ctx, sprite, spriteName, settingsPath, filepath.ToSlash(filepath.Join(DefaultRemoteHome, ".claude", "settings.json"))); err != nil {
		return err
	}
	return nil
}
