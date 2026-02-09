package lifecycle

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/misty-step/bitterblossom/internal/sprite"
)

func TestTeardownHappyPath(t *testing.T) {
	t.Parallel()

	fx := newFixture(t, "bramble")
	var checkpointCalled bool
	var destroyCalled bool

	cli := &sprite.MockSpriteCLI{
		ListFn: func(context.Context) ([]string, error) {
			return []string{"bramble"}, nil
		},
		ExecFn: func(_ context.Context, _ string, command string, _ []byte) (string, error) {
			switch command {
			case "cat '/home/sprite/workspace/MEMORY.md'":
				return "memory content", nil
			case "cat '/home/sprite/workspace/CLAUDE.md'":
				return "claude content", nil
			case "cat '/home/sprite/.claude/settings.json'":
				return `{"env":{"ANTHROPIC_AUTH_TOKEN":"secret","OPENROUTER_API_KEY":"openrouter-secret","X":"1"}}`, nil
			default:
				return "", nil
			}
		},
		CheckpointCreateFn: func(context.Context, string, string) error {
			checkpointCalled = true
			return nil
		},
		DestroyFn: func(context.Context, string, string) error {
			destroyCalled = true
			return nil
		},
	}

	archiveDir := filepath.Join(fx.rootDir, "archives")
	result, err := Teardown(context.Background(), cli, fx.cfg, TeardownOpts{
		Name:       "bramble",
		ArchiveDir: archiveDir,
		Force:      true,
	})
	if err != nil {
		t.Fatalf("Teardown() error = %v", err)
	}
	if result.Name != "bramble" {
		t.Fatalf("unexpected result: %+v", result)
	}
	if !checkpointCalled {
		t.Fatal("expected final checkpoint call")
	}
	if !destroyCalled {
		t.Fatal("expected destroy call")
	}

	settingsPath := filepath.Join(result.ArchivePath, "settings.json")
	settingsRaw, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("ReadFile(settings) error = %v", err)
	}
	if string(settingsRaw) == "" || !containsAny([]string{string(settingsRaw)}, "__REDACTED__") {
		t.Fatalf("expected redacted token, got %q", string(settingsRaw))
	}
	if containsAny([]string{string(settingsRaw)}, "secret") || containsAny([]string{string(settingsRaw)}, "openrouter-secret") {
		t.Fatalf("expected no raw secret values, got %q", string(settingsRaw))
	}
}

func TestTeardownMissingSprite(t *testing.T) {
	t.Parallel()

	fx := newFixture(t, "bramble")
	cli := &sprite.MockSpriteCLI{
		ListFn: func(context.Context) ([]string, error) {
			return []string{}, nil
		},
	}

	if _, err := Teardown(context.Background(), cli, fx.cfg, TeardownOpts{
		Name:       "bramble",
		ArchiveDir: filepath.Join(fx.rootDir, "archives"),
	}); err == nil {
		t.Fatal("expected error for missing sprite")
	}
}

func TestTeardownExportFailureContinues(t *testing.T) {
	t.Parallel()

	fx := newFixture(t, "bramble")
	var destroyCalled bool

	cli := &sprite.MockSpriteCLI{
		ListFn: func(context.Context) ([]string, error) {
			return []string{"bramble"}, nil
		},
		ExecFn: func(context.Context, string, string, []byte) (string, error) {
			return "", errors.New("missing")
		},
		CheckpointCreateFn: func(context.Context, string, string) error {
			return errors.New("checkpoint failed")
		},
		DestroyFn: func(context.Context, string, string) error {
			destroyCalled = true
			return nil
		},
	}

	result, err := Teardown(context.Background(), cli, fx.cfg, TeardownOpts{
		Name:       "bramble",
		ArchiveDir: filepath.Join(fx.rootDir, "archives"),
		Force:      true,
	})
	if err != nil {
		t.Fatalf("Teardown() error = %v", err)
	}
	if result.ArchivePath == "" {
		t.Fatalf("expected archive path in result: %+v", result)
	}
	if !destroyCalled {
		t.Fatal("destroy should run even when exports fail")
	}
}
