package main

import (
	"strings"
	"testing"
	"time"
)

func TestWorkspaceMetadataPath(t *testing.T) {
	t.Parallel()

	got := workspaceMetadataPath("/home/sprite/workspace/bitterblossom")
	want := "/home/sprite/workspace/bitterblossom/.bb/workspace.json"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestNewWorkspaceMetadata(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.March, 6, 22, 0, 0, 0, time.FixedZone("CST", -6*60*60))
	got := newWorkspaceMetadata("bramble", "misty-step/bitterblossom", "/home/sprite/workspace/bitterblossom", "sprites/bramble.md", now)

	if got.SchemaVersion != 1 {
		t.Fatalf("schema_version = %d, want 1", got.SchemaVersion)
	}
	if got.ConfiguredAt != "2026-03-07T04:00:00Z" {
		t.Fatalf("configured_at = %q, want %q", got.ConfiguredAt, "2026-03-07T04:00:00Z")
	}
	if got.Repo != "misty-step/bitterblossom" {
		t.Fatalf("repo = %q", got.Repo)
	}
	if got.Sprite != "bramble" {
		t.Fatalf("sprite = %q", got.Sprite)
	}
}

func TestMarshalWorkspaceMetadata(t *testing.T) {
	t.Parallel()

	data, err := marshalWorkspaceMetadata(workspaceMetadata{
		SchemaVersion: 1,
		Repo:          "misty-step/bitterblossom",
		RepoDir:       "/home/sprite/workspace/bitterblossom",
		Sprite:        "bramble",
		Persona:       "sprites/bramble.md",
		ConfiguredAt:  "2026-03-07T04:00:00Z",
	})
	if err != nil {
		t.Fatalf("marshalWorkspaceMetadata() error = %v", err)
	}

	text := string(data)
	for _, want := range []string{
		`"schema_version": 1`,
		`"repo": "misty-step/bitterblossom"`,
		`"sprite": "bramble"`,
		`"persona": "sprites/bramble.md"`,
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("metadata JSON missing %q in %s", want, text)
		}
	}
	if !strings.HasSuffix(text, "\n") {
		t.Fatalf("metadata JSON should end with newline")
	}
}

func TestWorkspaceDiscoveryScriptPrefersMetadata(t *testing.T) {
	t.Parallel()

	script := workspaceDiscoveryScript()
	metaIdx := strings.Index(script, "/.bb/workspace.json")
	promptIdx := strings.Index(script, "/.dispatch-prompt.md")
	logIdx := strings.Index(script, "/ralph.log")

	if metaIdx == -1 || promptIdx == -1 || logIdx == -1 {
		t.Fatalf("script missing expected discovery checks")
	}
	if !(metaIdx < promptIdx && promptIdx < logIdx) {
		t.Fatalf("expected metadata -> prompt -> log order, got script:\n%s", script)
	}
}
