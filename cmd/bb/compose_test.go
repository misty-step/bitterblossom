package main

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/misty-step/bitterblossom/internal/contracts"
	"github.com/misty-step/bitterblossom/internal/fleet"
	"github.com/misty-step/bitterblossom/internal/sprite"
	"github.com/spf13/cobra"
)

func TestComposeDiffJSON(t *testing.T) {
	t.Parallel()

	deps := composeDeps{
		parseComposition: func(string) (fleet.Composition, error) {
			return testComposition(), nil
		},
		newCLI: func(string, string) sprite.SpriteCLI {
			return &sprite.MockSpriteCLI{
				ListFn: func(context.Context) ([]string, error) {
					return nil, nil
				},
			}
		},
	}

	cmd := newComposeCmdWithDeps(deps)
	var stdout bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stdout)
	cmd.SetArgs([]string{"--json", "diff"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("cmd.Execute() error = %v", err)
	}

	var payload []map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("json.Unmarshal() error = %v, output=%q", err, stdout.String())
	}
	if len(payload) != 1 {
		t.Fatalf("len(payload) = %d, want 1", len(payload))
	}
	if payload[0]["kind"] != "provision" {
		t.Fatalf("payload[0].kind = %v, want provision", payload[0]["kind"])
	}
}

func TestComposeApplyDryRunDefault(t *testing.T) {
	t.Parallel()

	createCalls := 0
	deps := composeDeps{
		parseComposition: func(string) (fleet.Composition, error) {
			return testComposition(), nil
		},
		newCLI: func(string, string) sprite.SpriteCLI {
			return &sprite.MockSpriteCLI{
				ListFn: func(context.Context) ([]string, error) {
					return nil, nil
				},
				CreateFn: func(context.Context, string, string) error {
					createCalls++
					return nil
				},
			}
		},
	}

	cmd := newComposeCmdWithDeps(deps)
	var stdout bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stdout)
	cmd.SetArgs([]string{"apply"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("cmd.Execute() error = %v", err)
	}
	if createCalls != 0 {
		t.Fatalf("createCalls = %d, want 0 for dry-run", createCalls)
	}
	if !strings.Contains(stdout.String(), "Dry run") {
		t.Fatalf("output = %q, want Dry run", stdout.String())
	}
}

func TestComposeApplyExecuteRunsActions(t *testing.T) {
	t.Parallel()

	createCalls := 0
	deps := composeDeps{
		parseComposition: func(string) (fleet.Composition, error) {
			return testComposition(), nil
		},
		newCLI: func(string, string) sprite.SpriteCLI {
			return &sprite.MockSpriteCLI{
				ListFn: func(context.Context) ([]string, error) {
					return nil, nil
				},
				CreateFn: func(context.Context, string, string) error {
					createCalls++
					return nil
				},
			}
		},
	}

	cmd := newComposeCmdWithDeps(deps)
	var stdout bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stdout)
	cmd.SetArgs([]string{"apply", "--execute"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("cmd.Execute() error = %v", err)
	}
	if createCalls != 1 {
		t.Fatalf("createCalls = %d, want 1", createCalls)
	}
	if !strings.Contains(stdout.String(), "Executed 1 action") {
		t.Fatalf("output = %q, want executed summary", stdout.String())
	}
}

func TestComposeStatusJSON(t *testing.T) {
	t.Parallel()

	deps := composeDeps{
		parseComposition: func(string) (fleet.Composition, error) {
			return testComposition(), nil
		},
		newCLI: func(string, string) sprite.SpriteCLI {
			return &sprite.MockSpriteCLI{
				ListFn: func(context.Context) ([]string, error) {
					return []string{"bramble"}, nil
				},
			}
		},
	}

	cmd := newComposeCmdWithDeps(deps)
	var stdout bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stdout)
	cmd.SetArgs([]string{"--json", "status"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("cmd.Execute() error = %v", err)
	}

	var payload struct {
		Version string `json:"version"`
		Command string `json:"command"`
		Data    struct {
			Desired int `json:"desired"`
			Actual  int `json:"actual"`
		} `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("json.Unmarshal() error = %v, output=%q", err, stdout.String())
	}
	if payload.Version != contracts.SchemaVersion {
		t.Fatalf("version = %q, want %q", payload.Version, contracts.SchemaVersion)
	}
	if payload.Command != "compose.status" {
		t.Fatalf("command = %q, want compose.status", payload.Command)
	}
	if payload.Data.Desired != 1 {
		t.Fatalf("desired = %d, want 1", payload.Data.Desired)
	}
	if payload.Data.Actual != 1 {
		t.Fatalf("actual = %d, want 1", payload.Data.Actual)
	}
}

func TestRootWiresComposeCommand(t *testing.T) {
	t.Parallel()

	root := newRootCmdWithComposeFactory(func() *cobra.Command {
		return &cobra.Command{Use: "compose"}
	})

	found := false
	for _, command := range root.Commands() {
		if command.Name() == "compose" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("compose command not wired to root")
	}
}

func TestNamesToSpriteStatuses(t *testing.T) {
	t.Parallel()

	comp := testComposition()
	statuses := namesToSpriteStatuses([]string{"charlie", "bramble", "alpha"}, comp)
	if len(statuses) != 3 {
		t.Fatalf("len = %d, want 3", len(statuses))
	}
	// Should be sorted by name
	if statuses[0].Name != "alpha" {
		t.Fatalf("statuses[0].Name = %q, want alpha", statuses[0].Name)
	}
	if statuses[1].Name != "bramble" {
		t.Fatalf("statuses[1].Name = %q, want bramble", statuses[1].Name)
	}
	if statuses[2].Name != "charlie" {
		t.Fatalf("statuses[2].Name = %q, want charlie", statuses[2].Name)
	}
	// All should have idle state
	for _, s := range statuses {
		if s.State != sprite.StateIdle {
			t.Fatalf("state for %q = %v, want idle", s.Name, s.State)
		}
	}
	// "bramble" matches composition: should have Persona and ConfigVersion
	if statuses[1].Persona != "bramble" {
		t.Fatalf("bramble persona = %q, want bramble", statuses[1].Persona)
	}
	if statuses[1].ConfigVersion != "1" {
		t.Fatalf("bramble config = %q, want 1", statuses[1].ConfigVersion)
	}
	// "alpha" and "charlie" are extras: no metadata
	if statuses[0].Persona != "" {
		t.Fatalf("alpha persona = %q, want empty", statuses[0].Persona)
	}
	if statuses[2].Persona != "" {
		t.Fatalf("charlie persona = %q, want empty", statuses[2].Persona)
	}
}

func TestNamesToSpriteStatusesEmpty(t *testing.T) {
	t.Parallel()

	statuses := namesToSpriteStatuses(nil, fleet.Composition{})
	if len(statuses) != 0 {
		t.Fatalf("len = %d, want 0", len(statuses))
	}
}

func testComposition() fleet.Composition {
	return fleet.Composition{
		Version: 1,
		Name:    "test",
		Sprites: []fleet.SpriteSpec{
			{
				Name:    "bramble",
				Persona: sprite.Persona{Name: "bramble"},
			},
		},
	}
}
