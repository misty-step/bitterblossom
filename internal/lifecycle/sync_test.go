package lifecycle

import (
	"context"
	"strings"
	"testing"

	"github.com/misty-step/bitterblossom/internal/sprite"
)

func TestSyncHappyPath(t *testing.T) {
	t.Parallel()

	fx := newFixture(t, "bramble")
	uploaded := make([]string, 0, 8)
	contentUploaded := make([]string, 0, 4)

	cli := &sprite.MockSpriteCLI{
		ListFn: func(context.Context) ([]string, error) {
			return []string{"bramble"}, nil
		},
		ExecFn: func(context.Context, string, string, []byte) (string, error) {
			return "", nil
		},
		UploadFileFn: func(_ context.Context, _ string, _ string, _ string, remotePath string) error {
			uploaded = append(uploaded, remotePath)
			return nil
		},
		UploadFn: func(_ context.Context, _ string, remotePath string, _ []byte) error {
			contentUploaded = append(contentUploaded, remotePath)
			return nil
		},
	}

	if err := Sync(context.Background(), cli, fx.cfg, SyncOpts{
		Name:         "bramble",
		SettingsPath: fx.settingsPath,
		BaseOnly:     false,
	}); err != nil {
		t.Fatalf("Sync() error = %v", err)
	}
	if !containsString(uploaded, "/home/sprite/workspace/PERSONA.md") {
		t.Fatalf("persona upload missing: %#v", uploaded)
	}
	if !containsString(uploaded, "/home/sprite/.local/bin/sprite-agent") {
		t.Fatalf("agent upload missing: %#v", uploaded)
	}
	if !containsString(contentUploaded, "/home/sprite/anthropic-proxy.mjs") {
		t.Fatalf("proxy upload missing: %#v", contentUploaded)
	}
}

func TestSyncSpriteDoesNotExist(t *testing.T) {
	t.Parallel()

	fx := newFixture(t, "bramble")
	cli := &sprite.MockSpriteCLI{
		ListFn: func(context.Context) ([]string, error) {
			return []string{}, nil
		},
	}

	if err := Sync(context.Background(), cli, fx.cfg, SyncOpts{
		Name:         "bramble",
		SettingsPath: fx.settingsPath,
	}); err == nil {
		t.Fatal("expected error for missing sprite")
	}
}

func TestSyncBaseOnlySkipsPersonaUpload(t *testing.T) {
	t.Parallel()

	fx := newFixture(t, "bramble")
	uploaded := make([]string, 0, 8)

	cli := &sprite.MockSpriteCLI{
		ListFn: func(context.Context) ([]string, error) {
			return []string{"bramble"}, nil
		},
		ExecFn: func(context.Context, string, string, []byte) (string, error) {
			return "", nil
		},
		UploadFileFn: func(_ context.Context, _ string, _ string, _ string, remotePath string) error {
			uploaded = append(uploaded, remotePath)
			return nil
		},
		UploadFn: func(_ context.Context, _ string, _ string, _ []byte) error {
			return nil
		},
	}

	if err := Sync(context.Background(), cli, fx.cfg, SyncOpts{
		Name:         "bramble",
		SettingsPath: fx.settingsPath,
		BaseOnly:     true,
	}); err != nil {
		t.Fatalf("Sync() error = %v", err)
	}
	for _, path := range uploaded {
		if strings.Contains(path, "PERSONA.md") {
			t.Fatalf("persona upload should be skipped in base-only mode: %#v", uploaded)
		}
	}
}
