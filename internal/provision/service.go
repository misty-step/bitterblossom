package provision

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/misty-step/bitterblossom/internal/lib"
)

const verifyRepo = "https://github.com/misty-step/cerberus.git"

// Service provisions sprites and bootstraps their workspace/runtime config.
type Service struct {
	Logger          *slog.Logger
	Sprite          *lib.SpriteCLI
	Runner          lib.Runner
	Paths           lib.Paths
	CompositionPath string
	DryRun          bool
}

func NewService(logger *slog.Logger, sprite *lib.SpriteCLI, runner lib.Runner, paths lib.Paths, compositionPath string, dryRun bool) *Service {
	if strings.TrimSpace(compositionPath) == "" {
		compositionPath = filepath.Join(paths.Root, lib.DefaultComposition)
	}
	return &Service{
		Logger:          logger,
		Sprite:          sprite,
		Runner:          runner,
		Paths:           paths,
		CompositionPath: compositionPath,
		DryRun:          dryRun,
	}
}

func (s *Service) ResolveTargets(ctx context.Context, all bool, explicit []string) ([]string, string, error) {
	if all && len(explicit) > 0 {
		return nil, "", &lib.ValidationError{Field: "targets", Message: "use either --all or explicit sprite names"}
	}
	if !all && len(explicit) == 0 {
		return nil, "", &lib.ValidationError{Field: "targets", Message: "provide sprite names or pass --all"}
	}
	if !all {
		return explicit, s.CompositionPath, nil
	}

	sprites, resolvedPath, err := lib.CompositionSprites(s.Paths, s.CompositionPath, true)
	if err != nil {
		return nil, "", err
	}
	return sprites, resolvedPath, nil
}

func (s *Service) PrepareRenderedSettings(anthropicToken string) (settingsPath string, cleanup func() error, err error) {
	rendered, err := lib.PrepareSettings(s.Paths.BaseSettingsPath(), anthropicToken)
	if err != nil {
		return "", nil, err
	}
	return rendered.Path(), rendered.Cleanup, nil
}

func (s *Service) ProvisionSprite(ctx context.Context, name, settingsPath, compositionPath, githubToken string) error {
	if err := lib.ValidateSpriteName(name); err != nil {
		return err
	}
	definition := filepath.Join(s.Paths.SpritesDir, name+".md")
	if _, err := filepath.Abs(definition); err != nil {
		return fmt.Errorf("resolve sprite definition path: %w", err)
	}
	if _, err := os.Stat(definition); err != nil {
		return fmt.Errorf("sprite definition not found at %s", definition)
	}

	exists, err := s.Sprite.Exists(ctx, name)
	if err != nil {
		return err
	}
	if !exists {
		if err := s.Sprite.Create(ctx, name); err != nil {
			return err
		}
	}

	compositionLabel := strings.TrimSuffix(filepath.Base(compositionPath), filepath.Ext(compositionPath))
	return s.BootstrapSprite(ctx, name, definition, settingsPath, compositionLabel, githubToken)
}

// BootstrapSprite performs idempotent sprite environment setup.
func (s *Service) BootstrapSprite(ctx context.Context, name, definitionPath, settingsPath, compositionLabel, githubToken string) error {
	if err := lib.ValidateSpriteName(name); err != nil {
		return err
	}
	if strings.TrimSpace(definitionPath) == "" {
		return &lib.ValidationError{Field: "definition", Message: "is required"}
	}
	if strings.TrimSpace(settingsPath) == "" {
		return &lib.ValidationError{Field: "settings", Message: "is required"}
	}

	workspace := filepath.ToSlash(filepath.Join(lib.DefaultRemoteHome, "workspace"))
	if _, err := s.Sprite.Exec(ctx, name, true, "mkdir", "-p", workspace); err != nil {
		return err
	}

	if err := lib.PushConfig(ctx, s.Sprite, s.Paths, name, settingsPath); err != nil {
		return err
	}

	if err := lib.UploadFile(ctx, s.Sprite, name, definitionPath, filepath.ToSlash(filepath.Join(workspace, "PERSONA.md"))); err != nil {
		return err
	}

	timestamp := time.Now().UTC().Format(time.RFC3339)
	memory := fmt.Sprintf(`# MEMORY.md - %s

Sprite: %s
Provisioned: %s
Composition: %s

## Learnings

_No observations yet. Update after completing work._
`, name, name, timestamp, compositionLabel)
	memoryCmd := fmt.Sprintf("cat > %s/MEMORY.md <<'MEMEOF'\n%s\nMEMEOF", workspace, memory)
	if _, err := s.Sprite.Exec(ctx, name, true, "bash", "-c", memoryCmd); err != nil {
		return err
	}

	token, err := s.resolveGitHubToken(ctx, githubToken)
	if err != nil {
		return err
	}

	configureCmd := fmt.Sprintf("git config --global user.name %q && git config --global user.email %q && git config --global credential.helper store && echo %q > /home/sprite/.git-credentials && echo 'Git credentials configured for kaylee-mistystep'", name+" (bitterblossom sprite)", "kaylee@mistystep.io", "https://kaylee-mistystep:"+token+"@github.com")
	if _, err := s.Sprite.Exec(ctx, name, true, "bash", "-c", configureCmd); err != nil {
		return err
	}
	if s.DryRun {
		return nil
	}

	verifyCmd := fmt.Sprintf("cd /tmp && rm -rf _git_test && mkdir _git_test && cd _git_test && git init -q && git remote add origin %s && git ls-remote origin HEAD >/dev/null 2>&1 && echo GIT_AUTH_OK || echo GIT_AUTH_FAIL", verifyRepo)
	verifyResult, err := s.Sprite.Exec(ctx, name, false, "bash", "-c", verifyCmd)
	if err != nil {
		return err
	}
	if !strings.Contains(verifyResult.Stdout, "GIT_AUTH_OK") {
		return fmt.Errorf("git auth verification failed on sprite %q", name)
	}

	if err := s.Sprite.CheckpointCreate(ctx, name); err != nil && s.Logger != nil {
		s.Logger.WarnContext(ctx, "checkpoint creation skipped", "sprite", name, "error", err)
	}
	return nil
}

func (s *Service) resolveGitHubToken(ctx context.Context, provided string) (string, error) {
	trimmed := strings.TrimSpace(provided)
	if trimmed != "" {
		return trimmed, nil
	}
	if s.DryRun {
		return "__DRY_RUN_TOKEN__", nil
	}
	if s.Runner == nil {
		return "", &lib.ValidationError{Field: "GITHUB_TOKEN", Message: "is required (runner unavailable for gh auth token fallback)"}
	}

	result, err := s.Runner.Run(ctx, lib.RunRequest{Cmd: "gh", Args: []string{"auth", "token"}})
	if err != nil {
		return "", &lib.ValidationError{Field: "GITHUB_TOKEN", Message: "not set and gh CLI is not authenticated"}
	}
	fromCLI := strings.TrimSpace(result.Stdout)
	if fromCLI == "" {
		return "", &lib.ValidationError{Field: "GITHUB_TOKEN", Message: "not set and gh CLI did not return a token"}
	}
	return fromCLI, nil
}
