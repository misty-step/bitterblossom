package lifecycle

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestRenderSettingsInjectsToken(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	source := filepath.Join(dir, "settings.json")
	if err := os.WriteFile(source, []byte(`{"env":{"EXISTING":"1"}}`), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	rendered, err := RenderSettings(source, "test-token")
	if err != nil {
		t.Fatalf("RenderSettings() error = %v", err)
	}
	defer func() {
		_ = os.Remove(rendered)
	}()

	raw, err := os.ReadFile(rendered)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	env := payload["env"].(map[string]any)
	if env["ANTHROPIC_AUTH_TOKEN"] != "test-token" {
		t.Fatalf("token = %v, want test-token", env["ANTHROPIC_AUTH_TOKEN"])
	}
}

func TestRenderSettingsMissingToken(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	source := filepath.Join(dir, "settings.json")
	if err := os.WriteFile(source, []byte(`{"env":{}}`), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	if _, err := RenderSettings(source, ""); err == nil {
		t.Fatal("expected missing token error")
	}
}
