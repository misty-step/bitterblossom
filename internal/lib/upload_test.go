package lib

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestUploadDirUploadsEachFile(t *testing.T) {
	root := t.TempDir()
	localDir := filepath.Join(root, "hooks")
	if err := os.MkdirAll(filepath.Join(localDir, "nested"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(localDir, "a.txt"), []byte("a"), 0o644); err != nil {
		t.Fatalf("write a: %v", err)
	}
	if err := os.WriteFile(filepath.Join(localDir, "nested", "b.txt"), []byte("b"), 0o644); err != nil {
		t.Fatalf("write b: %v", err)
	}

	runner := &mockRunner{}
	sprite := NewSpriteCLI(runner, "sprite", "misty-step")
	if err := UploadDir(context.Background(), sprite, "bramble", localDir, "/home/sprite/.claude/hooks"); err != nil {
		t.Fatalf("upload dir: %v", err)
	}

	reqs := runner.Requests()
	if len(reqs) == 0 {
		t.Fatalf("expected commands, got none")
	}
	var uploadCount int
	for _, req := range reqs {
		if strings.Contains(strings.Join(req.Args, " "), "-file") {
			uploadCount++
		}
	}
	if uploadCount != 2 {
		t.Fatalf("expected 2 file uploads, got %d", uploadCount)
	}
}

func TestPushConfig(t *testing.T) {
	root := t.TempDir()
	paths := Paths{BaseDir: filepath.Join(root, "base")}
	for _, dir := range []string{"hooks", "skills", "commands"} {
		if err := os.MkdirAll(filepath.Join(paths.BaseDir, dir), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
		if err := os.WriteFile(filepath.Join(paths.BaseDir, dir, "x.txt"), []byte("x"), 0o644); err != nil {
			t.Fatalf("write %s: %v", dir, err)
		}
	}
	if err := os.WriteFile(filepath.Join(paths.BaseDir, "CLAUDE.md"), []byte("base"), 0o644); err != nil {
		t.Fatalf("write CLAUDE.md: %v", err)
	}
	settings := filepath.Join(root, "settings.json")
	if err := os.WriteFile(settings, []byte("{}"), 0o644); err != nil {
		t.Fatalf("write settings: %v", err)
	}

	runner := &mockRunner{}
	sprite := NewSpriteCLI(runner, "sprite", "misty-step")
	if err := PushConfig(context.Background(), sprite, paths, "thorn", settings); err != nil {
		t.Fatalf("push config: %v", err)
	}

	reqs := runner.Requests()
	if len(reqs) < 5 {
		t.Fatalf("expected multiple commands, got %d", len(reqs))
	}
}
