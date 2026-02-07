package config

import (
	"path/filepath"
	"testing"
)

func TestLoadComposition(t *testing.T) {
	path := filepath.Join("..", "..", "compositions", "v1.yaml")
	composition, err := LoadComposition(path)
	if err != nil {
		t.Fatalf("LoadComposition() error = %v", err)
	}
	if composition.Name == "" {
		t.Fatal("expected composition name")
	}
	names := SpriteNames(composition)
	if len(names) == 0 {
		t.Fatal("expected at least one sprite")
	}
	if names[0] != "bramble" {
		t.Fatalf("unexpected sorted first sprite: %q", names[0])
	}
}
