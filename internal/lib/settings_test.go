package lib

import (
	"encoding/json"
	"os"
	"testing"
)

func TestPrepareSettings(t *testing.T) {
	template := t.TempDir() + "/settings.json"
	if err := os.WriteFile(template, []byte(`{"env":{"ANTHROPIC_AUTH_TOKEN":"placeholder","OTHER":"1"}}`), 0o644); err != nil {
		t.Fatalf("write template: %v", err)
	}

	rendered, err := PrepareSettings(template, "secret-token")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer func() { _ = rendered.Cleanup() }()

	payload, err := os.ReadFile(rendered.Path())
	if err != nil {
		t.Fatalf("read rendered: %v", err)
	}

	var doc map[string]any
	if err := json.Unmarshal(payload, &doc); err != nil {
		t.Fatalf("unmarshal rendered: %v", err)
	}
	env, _ := doc["env"].(map[string]any)
	if got := env["ANTHROPIC_AUTH_TOKEN"]; got != "secret-token" {
		t.Fatalf("expected token to be replaced, got %v", got)
	}
}

func TestPrepareSettingsRequiresToken(t *testing.T) {
	template := t.TempDir() + "/settings.json"
	if err := os.WriteFile(template, []byte(`{"env":{}}`), 0o644); err != nil {
		t.Fatalf("write template: %v", err)
	}

	_, err := PrepareSettings(template, "")
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
}
