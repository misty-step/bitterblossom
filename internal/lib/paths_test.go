package lib

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNewPathsAndResolveRoot(t *testing.T) {
	root := t.TempDir()
	paths, err := NewPaths(root)
	if err != nil {
		t.Fatalf("new paths: %v", err)
	}
	if paths.Root == "" || paths.BaseDir == "" || paths.SpritesDir == "" {
		t.Fatalf("expected populated paths: %+v", paths)
	}

	resolved, err := ResolveRoot(root)
	if err != nil {
		t.Fatalf("resolve root: %v", err)
	}
	if resolved == "" {
		t.Fatalf("expected resolved root")
	}
}

func TestResolveRootError(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing")
	if _, err := os.Stat(missing); !os.IsNotExist(err) {
		t.Fatalf("expected missing path")
	}
	if _, err := ResolveRoot(missing); err == nil {
		t.Fatalf("expected resolve root error")
	}
}
