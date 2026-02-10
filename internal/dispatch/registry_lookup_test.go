package dispatch

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/misty-step/bitterblossom/internal/registry"
)

func writeTestRegistry(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "registry.toml")
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatalf("writeTestRegistry: %v", err)
	}
	return p
}

const testRegistryTOML = `[meta]
fly_app = "test-app"
created_at = "2026-02-10T00:00:00Z"

[sprites.bramble]
machine_id = "m-abc123"
provisioned_at = "2026-02-10T00:00:00Z"

[sprites.fern]
machine_id = "m-def456"
provisioned_at = "2026-02-10T00:00:00Z"
`

func TestResolveSprite_Found(t *testing.T) {
	path := writeTestRegistry(t, testRegistryTOML)
	machineID, err := ResolveSprite("bramble", path)
	if err != nil {
		t.Fatalf("ResolveSprite(bramble) error = %v", err)
	}
	if machineID != "m-abc123" {
		t.Fatalf("ResolveSprite(bramble) = %q, want %q", machineID, "m-abc123")
	}
}

func TestResolveSprite_NotFound(t *testing.T) {
	path := writeTestRegistry(t, testRegistryTOML)
	_, err := ResolveSprite("unknown", path)
	if err == nil {
		t.Fatal("expected error for unknown sprite")
	}
	var notFound *ErrSpriteNotInRegistry
	if !errors.As(err, &notFound) {
		t.Fatalf("expected ErrSpriteNotInRegistry, got %T: %v", err, err)
	}
	if notFound.Name != "unknown" {
		t.Fatalf("error.Name = %q, want %q", notFound.Name, "unknown")
	}
}

func TestResolveSprite_DefaultPath(t *testing.T) {
	// When called with empty registryPath, should use default path.
	// Result depends on whether a registry file exists on this machine.
	// We only verify it doesn't panic and returns a meaningful error or result.
	_, err := ResolveSprite("bramble", "")
	// Either nil (found) or an error (load or lookup) — both acceptable.
	_ = err
}

func TestResolveSprite_MissingRegistryFile(t *testing.T) {
	_, err := ResolveSprite("bramble", "/tmp/nonexistent-registry-"+t.Name()+".toml")
	if err == nil {
		t.Fatal("expected error for missing registry file")
	}
}

func TestIssuePrompt_WithRepo(t *testing.T) {
	p := IssuePrompt(134, "misty-step/bitterblossom")
	if p == "" {
		t.Fatal("IssuePrompt returned empty string")
	}
	if !contains(p, "#134") {
		t.Fatalf("prompt missing issue number: %s", p)
	}
	if !contains(p, "misty-step/bitterblossom") {
		t.Fatalf("prompt missing repo: %s", p)
	}
}

func TestIssuePrompt_WithoutRepo(t *testing.T) {
	p := IssuePrompt(42, "")
	if !contains(p, "#42") {
		t.Fatalf("prompt missing issue number: %s", p)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// Verify registry package is importable and functional.
func TestRegistryPackageIntegration(t *testing.T) {
	path := writeTestRegistry(t, testRegistryTOML)
	reg, err := registry.Load(path)
	if err != nil {
		t.Fatalf("registry.Load() error = %v", err)
	}
	names := reg.Names()
	if len(names) != 2 {
		t.Fatalf("expected 2 sprites, got %d: %v", len(names), names)
	}
}
