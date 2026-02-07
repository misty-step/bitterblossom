package fleet

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestCompositionSprites(t *testing.T) {
	path := filepath.Join(t.TempDir(), "v1.yaml")
	content := "version: 1\nsprites:\n  thorn:\n    role: qa\n  fern:\n    role: ops\nother: value\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := CompositionSprites(path)
	if err != nil {
		t.Fatalf("CompositionSprites error: %v", err)
	}
	want := []string{"thorn", "fern"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestFallbackSpriteNames(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "thorn.md"), []byte("x"), 0o644)
	_ = os.WriteFile(filepath.Join(dir, "fern.md"), []byte("x"), 0o644)
	got, err := FallbackSpriteNames(dir)
	if err != nil {
		t.Fatalf("FallbackSpriteNames error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 names, got %d", len(got))
	}
}
