package fleet

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadCompositionFromRepositoryFixture(t *testing.T) {
	t.Parallel()

	path := filepath.Clean(filepath.Join("..", "..", "compositions", "v1.yaml"))
	composition, err := LoadComposition(path)
	if err != nil {
		t.Fatalf("LoadComposition: %v", err)
	}

	if composition.Name == "" {
		t.Fatalf("expected name")
	}
	if composition.Version != 1 {
		t.Fatalf("expected version 1, got %d", composition.Version)
	}
	if len(composition.Sprites) == 0 {
		t.Fatalf("expected sprites")
	}
	if composition.Sprites[0].Persona.Name == "" {
		t.Fatalf("expected persona name")
	}
}

func TestLoadCompositionDetectsDuplicateNames(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	compositionsDir := filepath.Join(root, "compositions")
	spritesDir := filepath.Join(root, "sprites")
	if err := os.MkdirAll(compositionsDir, 0o755); err != nil {
		t.Fatalf("mkdir compositions: %v", err)
	}
	if err := os.MkdirAll(spritesDir, 0o755); err != nil {
		t.Fatalf("mkdir sprites: %v", err)
	}
	if err := os.WriteFile(filepath.Join(spritesDir, "bramble.md"), []byte("# bramble"), 0o644); err != nil {
		t.Fatalf("write persona: %v", err)
	}

	compositionPath := filepath.Join(compositionsDir, "dup.yaml")
	content := `
version: 1
name: "dup"
sprites:
  bramble:
    definition: sprites/bramble.md
  bramble:
    definition: sprites/bramble.md
`
	if err := os.WriteFile(compositionPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write composition: %v", err)
	}

	_, err := LoadCompositionWithSprites(compositionPath, spritesDir)
	if err == nil {
		t.Fatalf("expected duplicate error")
	}
	if !strings.Contains(err.Error(), "duplicate sprite name") {
		t.Fatalf("expected duplicate error message, got %v", err)
	}
}

func TestLoadCompositionValidatesPersona(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	compositionsDir := filepath.Join(root, "compositions")
	spritesDir := filepath.Join(root, "sprites")
	definitionsDir := filepath.Join(root, "definitions")
	if err := os.MkdirAll(compositionsDir, 0o755); err != nil {
		t.Fatalf("mkdir compositions: %v", err)
	}
	if err := os.MkdirAll(spritesDir, 0o755); err != nil {
		t.Fatalf("mkdir sprites: %v", err)
	}
	if err := os.MkdirAll(definitionsDir, 0o755); err != nil {
		t.Fatalf("mkdir definitions: %v", err)
	}
	if err := os.WriteFile(filepath.Join(spritesDir, "bramble.md"), []byte("# bramble"), 0o644); err != nil {
		t.Fatalf("write known persona: %v", err)
	}
	if err := os.WriteFile(filepath.Join(definitionsDir, "unknown.md"), []byte("# unknown"), 0o644); err != nil {
		t.Fatalf("write unknown definition: %v", err)
	}

	compositionPath := filepath.Join(compositionsDir, "invalid.yaml")
	content := `
version: 1
name: "invalid"
sprites:
  bramble:
    definition: ../definitions/unknown.md
`
	if err := os.WriteFile(compositionPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write composition: %v", err)
	}

	_, err := LoadCompositionWithSprites(compositionPath, spritesDir)
	if err == nil {
		t.Fatalf("expected persona validation error")
	}
	if !strings.Contains(err.Error(), "unknown persona") {
		t.Fatalf("expected unknown persona error, got %v", err)
	}
}

func TestLoadCompositionsDirectory(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	compositionsDir := filepath.Join(root, "compositions")
	spritesDir := filepath.Join(root, "sprites")
	if err := os.MkdirAll(compositionsDir, 0o755); err != nil {
		t.Fatalf("mkdir compositions: %v", err)
	}
	if err := os.MkdirAll(spritesDir, 0o755); err != nil {
		t.Fatalf("mkdir sprites: %v", err)
	}
	if err := os.WriteFile(filepath.Join(spritesDir, "bramble.md"), []byte("# bramble"), 0o644); err != nil {
		t.Fatalf("write persona: %v", err)
	}

	alpha := `
version: 1
name: "alpha"
sprites:
  bramble:
    definition: ../sprites/bramble.md
`
	beta := `
version: 1
name: "beta"
sprites:
  bramble:
    definition: ../sprites/bramble.md
`
	if err := os.WriteFile(filepath.Join(compositionsDir, "b.yaml"), []byte(beta), 0o644); err != nil {
		t.Fatalf("write b.yaml: %v", err)
	}
	if err := os.WriteFile(filepath.Join(compositionsDir, "a.yaml"), []byte(alpha), 0o644); err != nil {
		t.Fatalf("write a.yaml: %v", err)
	}

	compositions, err := LoadCompositions(compositionsDir)
	if err != nil {
		t.Fatalf("LoadCompositions: %v", err)
	}
	if len(compositions) != 2 {
		t.Fatalf("expected 2 compositions, got %d", len(compositions))
	}
	if compositions[0].Name != "alpha" || compositions[1].Name != "beta" {
		t.Fatalf("expected sorted compositions, got %q, %q", compositions[0].Name, compositions[1].Name)
	}
}
