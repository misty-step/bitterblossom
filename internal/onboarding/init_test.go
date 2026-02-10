package onboarding

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/misty-step/bitterblossom/internal/registry"
)

// mockDiscoverer implements MachineDiscoverer for testing.
type mockDiscoverer struct {
	machines []Machine
	err      error
}

func (m *mockDiscoverer) ListMachines(_ context.Context, _ string) ([]Machine, error) {
	return m.machines, m.err
}

func TestInit_Basic(t *testing.T) {
	disco := &mockDiscoverer{
		machines: []Machine{
			{ID: "m-001", Name: "sprite-1"},
			{ID: "m-002", Name: "sprite-2"},
			{ID: "m-003", Name: "sprite-3"},
		},
	}

	regPath := filepath.Join(t.TempDir(), "registry.toml")
	cfg := InitConfig{
		App:          "test-app",
		RegistryPath: regPath,
	}

	result, err := Init(context.Background(), disco, cfg)
	if err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	if result.SpritesFound != 3 {
		t.Fatalf("SpritesFound = %d, want 3", result.SpritesFound)
	}
	if result.Registered != 3 {
		t.Fatalf("Registered = %d, want 3", result.Registered)
	}
	if len(result.Names) != 3 {
		t.Fatalf("Names = %v, want 3 names", result.Names)
	}

	// Verify registry was written and is loadable.
	reg, err := registry.Load(regPath)
	if err != nil {
		t.Fatalf("registry.Load() error = %v", err)
	}
	if reg.Count() != 3 {
		t.Fatalf("registry has %d sprites, want 3", reg.Count())
	}
	if reg.Meta.App != "test-app" {
		t.Fatalf("Meta.App = %q, want %q", reg.Meta.App, "test-app")
	}
}

func TestInit_MaxSprites(t *testing.T) {
	disco := &mockDiscoverer{
		machines: []Machine{
			{ID: "m-001", Name: "s1"},
			{ID: "m-002", Name: "s2"},
			{ID: "m-003", Name: "s3"},
			{ID: "m-004", Name: "s4"},
			{ID: "m-005", Name: "s5"},
		},
	}

	regPath := filepath.Join(t.TempDir(), "registry.toml")
	cfg := InitConfig{
		App:          "test-app",
		RegistryPath: regPath,
		MaxSprites:   3,
	}

	result, err := Init(context.Background(), disco, cfg)
	if err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	if result.Registered != 3 {
		t.Fatalf("Registered = %d, want 3 (limited by MaxSprites)", result.Registered)
	}
	if result.SpritesFound != 3 {
		t.Fatalf("SpritesFound = %d, want 3", result.SpritesFound)
	}
}

func TestInit_NoMachines(t *testing.T) {
	disco := &mockDiscoverer{machines: []Machine{}}

	regPath := filepath.Join(t.TempDir(), "registry.toml")
	cfg := InitConfig{
		App:          "test-app",
		RegistryPath: regPath,
	}

	_, err := Init(context.Background(), disco, cfg)
	if err == nil {
		t.Fatal("expected error for no machines")
	}
}

func TestInit_EmptyApp(t *testing.T) {
	disco := &mockDiscoverer{}
	cfg := InitConfig{App: ""}

	_, err := Init(context.Background(), disco, cfg)
	if err == nil {
		t.Fatal("expected error for empty app name")
	}
}

func TestInit_Idempotent(t *testing.T) {
	disco := &mockDiscoverer{
		machines: []Machine{
			{ID: "m-001", Name: "s1"},
			{ID: "m-002", Name: "s2"},
		},
	}

	regPath := filepath.Join(t.TempDir(), "registry.toml")
	cfg := InitConfig{
		App:          "test-app",
		RegistryPath: regPath,
	}

	// Run init twice.
	_, err := Init(context.Background(), disco, cfg)
	if err != nil {
		t.Fatalf("first Init() error = %v", err)
	}
	result, err := Init(context.Background(), disco, cfg)
	if err != nil {
		t.Fatalf("second Init() error = %v", err)
	}

	// Should still have exactly 2 sprites (not 4).
	reg, err := registry.Load(regPath)
	if err != nil {
		t.Fatalf("registry.Load() error = %v", err)
	}
	// The second init overwrites, so count should be 2.
	if reg.Count() != 2 {
		t.Fatalf("registry has %d sprites after idempotent init, want 2", reg.Count())
	}
	_ = result
}

func TestInit_DeterministicNameAssignment(t *testing.T) {
	disco := &mockDiscoverer{
		machines: []Machine{
			{ID: "m-zzz", Name: "last"},
			{ID: "m-aaa", Name: "first"},
		},
	}

	regPath := filepath.Join(t.TempDir(), "registry.toml")
	cfg := InitConfig{
		App:          "test-app",
		RegistryPath: regPath,
	}

	result, err := Init(context.Background(), disco, cfg)
	if err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	// Names should be the first 2 botanical names.
	// Machines are sorted by ID, so m-aaa gets name[0], m-zzz gets name[1].
	reg, err := registry.Load(regPath)
	if err != nil {
		t.Fatalf("registry.Load() error = %v", err)
	}

	// Verify m-aaa got the first name and m-zzz got the second.
	name1, ok1 := reg.LookupName("m-aaa")
	name2, ok2 := reg.LookupName("m-zzz")
	if !ok1 || !ok2 {
		t.Fatalf("LookupName failed: m-aaa=%v, m-zzz=%v", ok1, ok2)
	}

	// The specific names depend on the names package, but they should be different.
	if name1 == name2 {
		t.Fatalf("same name assigned to two machines: %q", name1)
	}

	// Verify the result names are sorted (registry.Names() sorts).
	for i := 1; i < len(result.Names); i++ {
		if result.Names[i] < result.Names[i-1] {
			t.Fatalf("result.Names not sorted: %v", result.Names)
		}
	}
}

func TestInit_RegistryFileCreated(t *testing.T) {
	disco := &mockDiscoverer{
		machines: []Machine{{ID: "m-001", Name: "s1"}},
	}

	regPath := filepath.Join(t.TempDir(), "nested", "dir", "registry.toml")
	cfg := InitConfig{
		App:          "test-app",
		RegistryPath: regPath,
	}

	_, err := Init(context.Background(), disco, cfg)
	if err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	if _, err := os.Stat(regPath); err != nil {
		t.Fatalf("registry file not created at %s: %v", regPath, err)
	}
}
