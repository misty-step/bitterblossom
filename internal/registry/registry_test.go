package registry

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func mustParseTime(t *testing.T, s string) time.Time {
	t.Helper()
	tt, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		t.Fatalf("time.Parse(%q) error = %v", s, err)
	}
	return tt
}

func TestLoadSaveRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "registry.toml")

	r0 := &Registry{
		Meta: Meta{
			Account: "misty-step",
			App:     "bb-sprites",
			InitAt:  mustParseTime(t, "2026-02-09T20:58:00-08:00"),
		},
		Sprites: map[string]SpriteEntry{
			"bramble": {
				MachineID:      "d8901abc",
				CreatedAt:      mustParseTime(t, "2026-02-09T20:58:01-08:00"),
				AssignedIssue:  186,
				AssignedRepo:   "misty-step/bitterblossom",
				AssignedAt:     mustParseTime(t, "2026-02-09T21:00:00-08:00"),
			},
			"fern": {
				MachineID: "e7802def",
				CreatedAt: mustParseTime(t, "2026-02-09T20:58:02-08:00"),
			},
		},
	}

	if err := r0.Save(path); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	r1, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if r1.Meta.Account != r0.Meta.Account {
		t.Fatalf("Meta.Account mismatch: got %q want %q", r1.Meta.Account, r0.Meta.Account)
	}
	if r1.Meta.App != r0.Meta.App {
		t.Fatalf("Meta.App mismatch: got %q want %q", r1.Meta.App, r0.Meta.App)
	}
	if r1.Meta.InitAt.Format(time.RFC3339Nano) != r0.Meta.InitAt.Format(time.RFC3339Nano) {
		t.Fatalf("Meta.InitAt mismatch: got %q want %q", r1.Meta.InitAt.Format(time.RFC3339Nano), r0.Meta.InitAt.Format(time.RFC3339Nano))
	}

	if r1.Count() != r0.Count() {
		t.Fatalf("Count() mismatch: got %d want %d", r1.Count(), r0.Count())
	}
	for name, want := range r0.Sprites {
		got, ok := r1.Sprites[name]
		if !ok {
			t.Fatalf("missing sprite %q after Load()", name)
		}
		if got.MachineID != want.MachineID {
			t.Fatalf("sprite %q MachineID mismatch: got %q want %q", name, got.MachineID, want.MachineID)
		}
		if got.CreatedAt.Format(time.RFC3339Nano) != want.CreatedAt.Format(time.RFC3339Nano) {
			t.Fatalf("sprite %q CreatedAt mismatch: got %q want %q", name, got.CreatedAt.Format(time.RFC3339Nano), want.CreatedAt.Format(time.RFC3339Nano))
		}
		if got.AssignedIssue != want.AssignedIssue {
			t.Fatalf("sprite %q AssignedIssue mismatch: got %d want %d", name, got.AssignedIssue, want.AssignedIssue)
		}
		if got.AssignedRepo != want.AssignedRepo {
			t.Fatalf("sprite %q AssignedRepo mismatch: got %q want %q", name, got.AssignedRepo, want.AssignedRepo)
		}
		if got.AssignedAt.Format(time.RFC3339Nano) != want.AssignedAt.Format(time.RFC3339Nano) {
			t.Fatalf("sprite %q AssignedAt mismatch: got %q want %q", name, got.AssignedAt.Format(time.RFC3339Nano), want.AssignedAt.Format(time.RFC3339Nano))
		}
	}
}

func TestLoadMissingFileReturnsEmptyRegistry(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing-registry.toml")
	reg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if reg == nil {
		t.Fatal("expected non-nil registry")
	}
	if reg.Count() != 0 {
		t.Fatalf("expected empty registry, got Count() = %d", reg.Count())
	}
}

func TestLoadCorruptFileReturnsClearError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "registry.toml")
	if err := os.WriteFile(path, []byte("[meta]\ninit_at = not-a-time\n"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	msg := err.Error()
	if !strings.Contains(msg, "parsing registry") {
		t.Fatalf("expected error to mention parsing registry, got %q", msg)
	}
	if !strings.Contains(msg, "line") {
		t.Fatalf("expected error to include line information, got %q", msg)
	}
}

func TestLookupMachineAndLookupName(t *testing.T) {
	reg := &Registry{
		Sprites: map[string]SpriteEntry{
			"bramble": {MachineID: "d8901abc"},
			"fern":    {MachineID: "e7802def"},
		},
	}

	if got, ok := reg.LookupMachine("bramble"); !ok || got != "d8901abc" {
		t.Fatalf("LookupMachine(bramble) = (%q, %v), want (%q, true)", got, ok, "d8901abc")
	}
	if got, ok := reg.LookupMachine("missing"); ok || got != "" {
		t.Fatalf("LookupMachine(missing) = (%q, %v), want (%q, false)", got, ok, "")
	}

	if got, ok := reg.LookupName("e7802def"); !ok || got != "fern" {
		t.Fatalf("LookupName(e7802def) = (%q, %v), want (%q, true)", got, ok, "fern")
	}
	if got, ok := reg.LookupName("nope"); ok || got != "" {
		t.Fatalf("LookupName(nope) = (%q, %v), want (%q, false)", got, ok, "")
	}
}

func TestRegisterAddsAndUpdates(t *testing.T) {
	reg := &Registry{}

	reg.Register("bramble", "d8901abc")
	if reg.Count() != 1 {
		t.Fatalf("Count() after Register = %d, want 1", reg.Count())
	}
	if got, ok := reg.LookupMachine("bramble"); !ok || got != "d8901abc" {
		t.Fatalf("LookupMachine(bramble) = (%q, %v), want (%q, true)", got, ok, "d8901abc")
	}
	createdAt1 := reg.Sprites["bramble"].CreatedAt
	if createdAt1.IsZero() {
		t.Fatalf("expected CreatedAt to be set on new Register()")
	}

	reg.Register("bramble", "updated")
	if reg.Count() != 1 {
		t.Fatalf("Count() after update Register = %d, want 1", reg.Count())
	}
	if got, ok := reg.LookupMachine("bramble"); !ok || got != "updated" {
		t.Fatalf("LookupMachine(bramble) after update = (%q, %v), want (%q, true)", got, ok, "updated")
	}
	createdAt2 := reg.Sprites["bramble"].CreatedAt
	if !createdAt1.Equal(createdAt2) {
		t.Fatalf("expected CreatedAt to be preserved on update; got %q want %q", createdAt2.Format(time.RFC3339Nano), createdAt1.Format(time.RFC3339Nano))
	}
}

func TestUnregisterRemovesEntry(t *testing.T) {
	reg := &Registry{}
	reg.Register("bramble", "d8901abc")
	reg.Register("fern", "e7802def")

	reg.Unregister("bramble")
	if reg.Count() != 1 {
		t.Fatalf("Count() after Unregister = %d, want 1", reg.Count())
	}
	if _, ok := reg.LookupMachine("bramble"); ok {
		t.Fatalf("expected bramble to be unregistered")
	}
}

func TestNamesReturnsSortedList(t *testing.T) {
	reg := &Registry{
		Sprites: map[string]SpriteEntry{
			"fern":    {},
			"bramble": {},
			"aloe":    {},
		},
	}
	got := reg.Names()
	want := []string{"aloe", "bramble", "fern"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("Names() = %v, want %v", got, want)
	}
}

func TestDefaultPathContainsExpectedSuffix(t *testing.T) {
	path := filepath.ToSlash(DefaultPath())
	if !strings.Contains(path, ".config/bb/registry.toml") {
		t.Fatalf("DefaultPath() = %q, expected it to contain %q", path, ".config/bb/registry.toml")
	}
}

func TestValidateRegistryPath_BlocksSystemDirs(t *testing.T) {
	blocked := []string{
		"/etc/bb/registry.toml",
		"/usr/local/registry.toml",
		"/bin/registry.toml",
		"/sbin/registry.toml",
	}
	for _, path := range blocked {
		if _, err := validateRegistryPath(path); err == nil {
			t.Errorf("validateRegistryPath(%q) should be blocked", path)
		}
	}
}

func TestValidateRegistryPath_BlocksTraversal(t *testing.T) {
	// filepath.Abs resolves ".." so "/home/../etc/x.toml" → "/etc/x.toml" → blocked by system dir rule.
	if _, err := validateRegistryPath("/home/../etc/passwd.toml"); err == nil {
		t.Error("should block path traversal that resolves to /etc/")
	}
}

func TestValidateRegistryPath_RequiresToml(t *testing.T) {
	if _, err := validateRegistryPath("/home/user/registry.json"); err == nil {
		t.Error("should reject non-.toml extension")
	}
}

func TestValidateRegistryPath_AllowsValidPaths(t *testing.T) {
	// Temp dirs should be allowed for testing.
	dir := t.TempDir()
	path := filepath.Join(dir, "registry.toml")
	validated, err := validateRegistryPath(path)
	if err != nil {
		t.Fatalf("validateRegistryPath(%q) error = %v", path, err)
	}
	if validated == "" {
		t.Fatal("validated path is empty")
	}
}

func TestValidateRegistryPath_BlocksSymlinkedSystemDirs(t *testing.T) {
	// Create a symlink in a temp dir that points to /etc.
	// validateRegistryPath should resolve the symlink and block the path.
	tmpDir := t.TempDir()
	link := filepath.Join(tmpDir, "sneaky")
	if err := os.Symlink("/etc", link); err != nil {
		t.Skipf("cannot create symlink: %v", err)
	}

	path := filepath.Join(link, "registry.toml")
	_, err := validateRegistryPath(path)
	if err == nil {
		t.Fatalf("validateRegistryPath(%q) should be blocked (symlink to /etc)", path)
	}
	if !strings.Contains(err.Error(), "protected system directory") {
		t.Fatalf("expected protected system directory error, got: %v", err)
	}
}

func TestValidateRegistryPath_AllowsSymlinkedSafeDirs(t *testing.T) {
	// Symlink to a safe temp directory should be allowed.
	target := t.TempDir()
	tmpDir := t.TempDir()
	link := filepath.Join(tmpDir, "safe-link")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("cannot create symlink: %v", err)
	}

	path := filepath.Join(link, "registry.toml")
	validated, err := validateRegistryPath(path)
	if err != nil {
		t.Fatalf("validateRegistryPath(%q) error = %v (should allow safe symlink)", path, err)
	}
	if validated == "" {
		t.Fatal("validated path is empty")
	}
}
