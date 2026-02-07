package lib

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestParseCompositionSprites(t *testing.T) {
	tests := []struct {
		name    string
		yaml    string
		want    []string
		wantErr bool
	}{
		{
			name: "valid",
			yaml: "version: 1\nsprites:\n  bramble:\n    preference: x\n  thorn:\n    preference: y\n",
			want: []string{"bramble", "thorn"},
		},
		{
			name:    "missing_sprites_section",
			yaml:    "version: 1\nname: x\n",
			wantErr: true,
		},
		{
			name:    "duplicate",
			yaml:    "sprites:\n  bramble:\n    a: 1\n  bramble:\n    a: 2\n",
			wantErr: true,
		},
		{
			name:    "no_sprites",
			yaml:    "sprites:\n  \n",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseCompositionSprites(strings.NewReader(tt.yaml))
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !reflect.DeepEqual(tt.want, got) {
				t.Fatalf("want %v, got %v", tt.want, got)
			}
		})
	}
}

func TestResolveCompositionPath(t *testing.T) {
	root := t.TempDir()
	comps := filepath.Join(root, "compositions")
	if err := os.MkdirAll(comps, 0o755); err != nil {
		t.Fatalf("mkdir compositions: %v", err)
	}
	if err := os.WriteFile(filepath.Join(comps, "v1.yaml"), []byte("sprites:\n  bramble:\n"), 0o644); err != nil {
		t.Fatalf("write composition: %v", err)
	}

	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{name: "relative_ok", input: "compositions/v1.yaml"},
		{name: "absolute_ok", input: filepath.Join(comps, "v1.yaml")},
		{name: "escape_blocked", input: "../outside.yaml", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ResolveCompositionPath(root, tt.input)
			if tt.wantErr && err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestFallbackSpriteNames(t *testing.T) {
	root := t.TempDir()
	paths := Paths{SpritesDir: filepath.Join(root, "sprites")}
	if err := os.MkdirAll(paths.SpritesDir, 0o755); err != nil {
		t.Fatalf("mkdir sprites: %v", err)
	}
	if err := os.WriteFile(filepath.Join(paths.SpritesDir, "thorn.md"), []byte("x"), 0o644); err != nil {
		t.Fatalf("write thorn: %v", err)
	}
	if err := os.WriteFile(filepath.Join(paths.SpritesDir, "bramble.md"), []byte("x"), 0o644); err != nil {
		t.Fatalf("write bramble: %v", err)
	}

	names, err := FallbackSpriteNames(paths)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{"bramble", "thorn"}
	if !reflect.DeepEqual(want, names) {
		t.Fatalf("want %v, got %v", want, names)
	}
}

func TestCompositionSpritesFallbackNonStrict(t *testing.T) {
	root := t.TempDir()
	paths := Paths{
		Root:       root,
		CompsDir:   filepath.Join(root, "compositions"),
		SpritesDir: filepath.Join(root, "sprites"),
	}
	if err := os.MkdirAll(paths.CompsDir, 0o755); err != nil {
		t.Fatalf("mkdir comps: %v", err)
	}
	if err := os.MkdirAll(paths.SpritesDir, 0o755); err != nil {
		t.Fatalf("mkdir sprites: %v", err)
	}
	if err := os.WriteFile(filepath.Join(paths.SpritesDir, "thorn.md"), []byte("x"), 0o644); err != nil {
		t.Fatalf("write sprite: %v", err)
	}
	sprites, _, err := CompositionSprites(paths, "compositions/missing.yaml", false)
	if err != nil {
		t.Fatalf("expected fallback success, got %v", err)
	}
	if len(sprites) != 1 || sprites[0] != "thorn" {
		t.Fatalf("unexpected fallback sprites: %v", sprites)
	}
}

func TestCompositionSpritesStrictFails(t *testing.T) {
	root := t.TempDir()
	paths := Paths{
		Root:       root,
		CompsDir:   filepath.Join(root, "compositions"),
		SpritesDir: filepath.Join(root, "sprites"),
	}
	if err := os.MkdirAll(paths.CompsDir, 0o755); err != nil {
		t.Fatalf("mkdir comps: %v", err)
	}
	if err := os.MkdirAll(paths.SpritesDir, 0o755); err != nil {
		t.Fatalf("mkdir sprites: %v", err)
	}
	if _, _, err := CompositionSprites(paths, "compositions/missing.yaml", true); err == nil {
		t.Fatalf("expected strict mode error")
	}
}
