package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolvePersonaFile_UsesSpriteMatchFirst(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "sprites", "fresh.md"), "fresh")
	writeFile(t, filepath.Join(root, "sprites", "bramble.md"), "bramble")

	got, fallback, err := resolvePersonaFileFromRoot(root, "fresh", "")
	if err != nil {
		t.Fatalf("resolvePersonaFile() error = %v", err)
	}
	if fallback {
		t.Fatal("fallback should be false when sprite persona exists")
	}
	want := filepath.Join(root, "sprites", "fresh.md")
	if got != want {
		t.Fatalf("persona path = %q, want %q", got, want)
	}
}

func TestResolvePersonaFile_FallsBackToBramble(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "sprites", "bramble.md"), "bramble")

	got, fallback, err := resolvePersonaFileFromRoot(root, "e2e-123", "")
	if err != nil {
		t.Fatalf("resolvePersonaFile() error = %v", err)
	}
	if !fallback {
		t.Fatal("fallback should be true when sprite persona is missing")
	}
	want := filepath.Join(root, "sprites", "bramble.md")
	if got != want {
		t.Fatalf("persona path = %q, want %q", got, want)
	}
}

func TestResolvePersonaFile_ExplicitPersonaName(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "sprites", "fern.md"), "fern")

	got, fallback, err := resolvePersonaFileFromRoot(root, "unused", "fern")
	if err != nil {
		t.Fatalf("resolvePersonaFile() error = %v", err)
	}
	if fallback {
		t.Fatal("explicit persona should not be treated as fallback")
	}
	want := filepath.Join(root, "sprites", "fern.md")
	if got != want {
		t.Fatalf("persona path = %q, want %q", got, want)
	}
}

func TestResolvePersonaFile_ExplicitPersonaPath(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "sprites", "moss.md"), "moss")

	personaPath := filepath.Join(root, "sprites", "moss.md")
	got, fallback, err := resolvePersonaFileFromRoot(root, "unused", personaPath)
	if err != nil {
		t.Fatalf("resolvePersonaFile() error = %v", err)
	}
	if fallback {
		t.Fatal("explicit persona path should not be treated as fallback")
	}
	if got != personaPath {
		t.Fatalf("persona path = %q, want %q", got, personaPath)
	}
}

func TestResolvePersonaFile_ExplicitPersonaMissing(t *testing.T) {
	root := t.TempDir()
	_, _, err := resolvePersonaFileFromRoot(root, "unused", "missing")
	if err == nil {
		t.Fatal("expected error for missing explicit persona")
	}
}

func writeFile(t *testing.T, path, contents string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("os.MkdirAll(%q) error = %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(contents), 0644); err != nil {
		t.Fatalf("os.WriteFile(%q) error = %v", path, err)
	}
}
