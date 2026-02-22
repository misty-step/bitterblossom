package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// setupSpritesDir creates a temp directory with sprite persona files and
// changes the working directory to it so resolvePersona can find sprites/.
func setupSpritesDir(t *testing.T, files []string) string {
	t.Helper()
	dir := t.TempDir()
	spritesDir := filepath.Join(dir, "sprites")
	if err := os.MkdirAll(spritesDir, 0755); err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		path := filepath.Join(spritesDir, f)
		if err := os.WriteFile(path, []byte("# persona: "+f), 0644); err != nil {
			t.Fatal(err)
		}
	}
	orig, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd failed: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(orig) })
	return dir
}

func TestResolvePersonaExactMatch(t *testing.T) {
	setupSpritesDir(t, []string{"bramble.md", "willow.md"})

	got, err := resolvePersona("bramble", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "sprites/bramble.md" {
		t.Errorf("got %q, want %q", got, "sprites/bramble.md")
	}
}

func TestResolvePersonaFallbackWhenNoExactMatch(t *testing.T) {
	setupSpritesDir(t, []string{"bramble.md", "willow.md"})

	got, err := resolvePersona("e2e-0219123628", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should fall back to first available (alphabetical: bramble.md)
	if got != "sprites/bramble.md" {
		t.Errorf("got %q, want %q", got, "sprites/bramble.md")
	}
}

func TestResolvePersonaExplicitPersonaFlag(t *testing.T) {
	setupSpritesDir(t, []string{"bramble.md", "willow.md"})

	got, err := resolvePersona("e2e-0219123628", "willow")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "sprites/willow.md" {
		t.Errorf("got %q, want %q", got, "sprites/willow.md")
	}
}

func TestResolvePersonaExplicitPersonaWithExtension(t *testing.T) {
	setupSpritesDir(t, []string{"bramble.md"})

	got, err := resolvePersona("worker-1", "bramble.md")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "sprites/bramble.md" {
		t.Errorf("got %q, want %q", got, "sprites/bramble.md")
	}
}

func TestResolvePersonaExplicitDirectPath(t *testing.T) {
	dir := t.TempDir()
	personaPath := filepath.Join(dir, "custom-persona.md")
	if err := os.WriteFile(personaPath, []byte("# custom"), 0644); err != nil {
		t.Fatal(err)
	}
	// No sprites/ dir needed — direct path resolves first
	setupSpritesDir(t, nil)

	got, err := resolvePersona("worker-1", personaPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != personaPath {
		t.Errorf("got %q, want %q", got, personaPath)
	}
}

func TestResolvePersonaErrorWhenNoPersonasAvailable(t *testing.T) {
	setupSpritesDir(t, nil) // empty sprites/

	_, err := resolvePersona("e2e-0219123628", "")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "--persona") {
		t.Errorf("error should mention --persona flag, got: %v", err)
	}
}

func TestResolvePersonaErrorWhenExplicitPersonaNotFound(t *testing.T) {
	setupSpritesDir(t, []string{"bramble.md"})

	_, err := resolvePersona("worker-1", "nonexistent")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "nonexistent") {
		t.Errorf("error should mention the missing persona name, got: %v", err)
	}
}

func TestResolvePersonaErrorWhenPersonaContainsSlashButNotFound(t *testing.T) {
	setupSpritesDir(t, []string{"bramble.md"})

	// A path-style persona that doesn't exist should fail cleanly without
	// building a garbled double-prefixed candidate like sprites/sprites/...
	_, err := resolvePersona("worker-1", "sprites/nonexistent")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	// Error should reference the exact path the user supplied
	if !strings.Contains(err.Error(), "sprites/nonexistent") {
		t.Errorf("error should mention the persona path, got: %v", err)
	}
	// Should NOT expose a double-prefixed path
	if strings.Contains(err.Error(), "sprites/sprites/") {
		t.Errorf("error must not expose a double-prefixed path, got: %v", err)
	}
}
