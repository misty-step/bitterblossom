package sync

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/misty-step/bitterblossom/internal/lib"
)

type Service struct {
	Logger          *slog.Logger
	Sprite          *lib.SpriteCLI
	Paths           lib.Paths
	CompositionPath string
}

func NewService(logger *slog.Logger, sprite *lib.SpriteCLI, paths lib.Paths, compositionPath string) *Service {
	if strings.TrimSpace(compositionPath) == "" {
		compositionPath = filepath.Join(paths.Root, lib.DefaultComposition)
	}
	return &Service{
		Logger:          logger,
		Sprite:          sprite,
		Paths:           paths,
		CompositionPath: compositionPath,
	}
}

func (s *Service) PrepareRenderedSettings(anthropicToken string) (settingsPath string, cleanup func() error, err error) {
	rendered, err := lib.PrepareSettings(s.Paths.BaseSettingsPath(), anthropicToken)
	if err != nil {
		return "", nil, err
	}
	return rendered.Path(), rendered.Cleanup, nil
}

func (s *Service) ResolveTargets(explicit []string) ([]string, string, error) {
	if len(explicit) > 0 {
		return explicit, s.CompositionPath, nil
	}
	sprites, resolvedPath, err := lib.CompositionSprites(s.Paths, s.CompositionPath, true)
	if err != nil {
		return nil, "", err
	}
	return sprites, resolvedPath, nil
}

func (s *Service) SyncSprite(ctx context.Context, spriteName, settingsPath string, baseOnly bool) error {
	if err := lib.ValidateSpriteName(spriteName); err != nil {
		return err
	}
	exists, err := s.Sprite.Exists(ctx, spriteName)
	if err != nil {
		return err
	}
	if !exists {
		return fmt.Errorf("sprite %q does not exist; run provision first", spriteName)
	}

	if err := lib.PushConfig(ctx, s.Sprite, s.Paths, spriteName, settingsPath); err != nil {
		return err
	}

	if baseOnly {
		return nil
	}
	definition := filepath.Join(s.Paths.SpritesDir, spriteName+".md")
	if _, err := os.Stat(definition); err != nil {
		if os.IsNotExist(err) {
			if s.Logger != nil {
				s.Logger.InfoContext(ctx, "persona definition missing, skipping", "sprite", spriteName, "definition", definition)
			}
			return nil
		}
		return err
	}

	return lib.UploadFile(ctx, s.Sprite, spriteName, definition, filepath.ToSlash(filepath.Join(lib.DefaultRemoteHome, "workspace", "PERSONA.md")))
}
