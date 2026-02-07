package teardown

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/misty-step/bitterblossom/internal/lib"
)

var ErrAborted = errors.New("operation aborted")

type ConfirmFunc func(prompt string) (bool, error)

type Service struct {
	Logger *slog.Logger
	Sprite *lib.SpriteCLI
	Paths  lib.Paths
	DryRun bool
}

func NewService(logger *slog.Logger, sprite *lib.SpriteCLI, paths lib.Paths, dryRun bool) *Service {
	return &Service{Logger: logger, Sprite: sprite, Paths: paths, DryRun: dryRun}
}

func (s *Service) TeardownSprite(ctx context.Context, spriteName string, force bool, confirm ConfirmFunc) (string, error) {
	if err := lib.ValidateSpriteName(spriteName); err != nil {
		return "", err
	}
	exists, err := s.Sprite.Exists(ctx, spriteName)
	if err != nil {
		return "", err
	}
	if !exists {
		return "", fmt.Errorf("sprite %q does not exist", spriteName)
	}

	if !force {
		if confirm == nil {
			return "", &lib.ValidationError{Field: "confirm", Message: "confirmation handler is required when --force is not set"}
		}
		ok, err := confirm(fmt.Sprintf("This will destroy sprite %q and its disk. Continue? [y/N] ", spriteName))
		if err != nil {
			return "", err
		}
		if !ok {
			return "", ErrAborted
		}
	}

	timestamp := time.Now().UTC().Format("20060102T150405Z")
	archivePath := filepath.Join(s.Paths.ArchivesDir, fmt.Sprintf("%s-%s", spriteName, timestamp))
	if !s.DryRun {
		if err := os.MkdirAll(archivePath, 0o755); err != nil {
			return "", fmt.Errorf("create archive directory %s: %w", archivePath, err)
		}
	}

	if err := s.exportMemory(ctx, spriteName, archivePath); err != nil && s.Logger != nil {
		s.Logger.WarnContext(ctx, "memory export skipped", "sprite", spriteName, "error", err)
	}
	if err := s.exportWorkspaceClaude(ctx, spriteName, archivePath); err != nil && s.Logger != nil {
		s.Logger.WarnContext(ctx, "workspace CLAUDE.md export skipped", "sprite", spriteName, "error", err)
	}
	if err := s.exportSettings(ctx, spriteName, archivePath); err != nil && s.Logger != nil {
		s.Logger.WarnContext(ctx, "settings export skipped", "sprite", spriteName, "error", err)
	}

	if err := s.Sprite.CheckpointCreate(ctx, spriteName); err != nil && s.Logger != nil {
		s.Logger.WarnContext(ctx, "final checkpoint failed (continuing)", "sprite", spriteName, "error", err)
	}

	if err := s.Sprite.Destroy(ctx, spriteName, true); err != nil {
		return archivePath, err
	}
	return archivePath, nil
}

func (s *Service) exportMemory(ctx context.Context, spriteName, archivePath string) error {
	result, err := s.Sprite.Exec(ctx, spriteName, false, "cat", filepath.ToSlash(filepath.Join(lib.DefaultRemoteHome, "workspace", "MEMORY.md")))
	if err != nil {
		return err
	}
	if strings.TrimSpace(result.Stdout) == "" {
		return errors.New("empty MEMORY.md")
	}
	if s.DryRun {
		return nil
	}
	return os.WriteFile(filepath.Join(archivePath, "MEMORY.md"), []byte(result.Stdout), 0o644)
}

func (s *Service) exportWorkspaceClaude(ctx context.Context, spriteName, archivePath string) error {
	result, err := s.Sprite.Exec(ctx, spriteName, false, "cat", filepath.ToSlash(filepath.Join(lib.DefaultRemoteHome, "workspace", "CLAUDE.md")))
	if err != nil {
		return err
	}
	if strings.TrimSpace(result.Stdout) == "" {
		return errors.New("empty workspace CLAUDE.md")
	}
	if s.DryRun {
		return nil
	}
	return os.WriteFile(filepath.Join(archivePath, "CLAUDE.md"), []byte(result.Stdout), 0o644)
}

func (s *Service) exportSettings(ctx context.Context, spriteName, archivePath string) error {
	result, err := s.Sprite.Exec(ctx, spriteName, false, "cat", filepath.ToSlash(filepath.Join(lib.DefaultRemoteHome, ".claude", "settings.json")))
	if err != nil {
		return err
	}
	redacted, err := RedactSettingsJSON([]byte(result.Stdout))
	if err != nil {
		return err
	}
	if s.DryRun {
		return nil
	}
	return os.WriteFile(filepath.Join(archivePath, "settings.json"), redacted, 0o644)
}

func RedactSettingsJSON(raw []byte) ([]byte, error) {
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, err
	}
	env, ok := payload["env"].(map[string]any)
	if ok {
		if _, hasToken := env["ANTHROPIC_AUTH_TOKEN"]; hasToken {
			env["ANTHROPIC_AUTH_TOKEN"] = "__REDACTED__"
		}
	}
	out, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return nil, err
	}
	out = append(out, '\n')
	return out, nil
}
