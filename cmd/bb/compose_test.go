package main

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/misty-step/bitterblossom/internal/fleet"
	"github.com/misty-step/bitterblossom/internal/sprite"
	"github.com/misty-step/bitterblossom/pkg/fly"
	"github.com/spf13/cobra"
)

func TestComposeDiffJSON(t *testing.T) {
	t.Parallel()

	deps := composeDeps{
		parseComposition: func(string) (fleet.Composition, error) {
			return testComposition(), nil
		},
		newClient: func(string, string) (fly.MachineClient, error) {
			return &fly.MockMachineClient{
				ListFn: func(context.Context, string) ([]fly.Machine, error) {
					return nil, nil
				},
			}, nil
		},
	}

	cmd := newComposeCmdWithDeps(deps)
	var stdout bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stdout)
	cmd.SetArgs([]string{"--app", "test-app", "--token", "test-token", "--json", "diff"})

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
		newClient: func(string, string) (fly.MachineClient, error) {
			return &fly.MockMachineClient{
				ListFn: func(context.Context, string) ([]fly.Machine, error) {
					return nil, nil
				},
				CreateFn: func(context.Context, fly.CreateRequest) (fly.Machine, error) {
					createCalls++
					return fly.Machine{ID: "m1"}, nil
				},
			}, nil
		},
	}

	cmd := newComposeCmdWithDeps(deps)
	var stdout bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stdout)
	cmd.SetArgs([]string{"--app", "test-app", "--token", "test-token", "apply"})

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
		newClient: func(string, string) (fly.MachineClient, error) {
			return &fly.MockMachineClient{
				ListFn: func(context.Context, string) ([]fly.Machine, error) {
					return nil, nil
				},
				CreateFn: func(context.Context, fly.CreateRequest) (fly.Machine, error) {
					createCalls++
					return fly.Machine{ID: "m1"}, nil
				},
			}, nil
		},
	}

	cmd := newComposeCmdWithDeps(deps)
	var stdout bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stdout)
	cmd.SetArgs([]string{"--app", "test-app", "--token", "test-token", "apply", "--execute"})

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
		newClient: func(string, string) (fly.MachineClient, error) {
			return &fly.MockMachineClient{
				ListFn: func(context.Context, string) ([]fly.Machine, error) {
					return []fly.Machine{
						{
							ID:    "m1",
							Name:  "bramble",
							State: "running",
							Metadata: map[string]string{
								"persona":        "bramble",
								"config_version": "1",
							},
						},
					}, nil
				},
			}, nil
		},
	}

	cmd := newComposeCmdWithDeps(deps)
	var stdout bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stdout)
	cmd.SetArgs([]string{"--app", "test-app", "--token", "test-token", "--json", "status"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("cmd.Execute() error = %v", err)
	}

	var payload map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("json.Unmarshal() error = %v, output=%q", err, stdout.String())
	}
	if payload["desired"].(float64) != 1 {
		t.Fatalf("desired = %v, want 1", payload["desired"])
	}
	if payload["actual"].(float64) != 1 {
		t.Fatalf("actual = %v, want 1", payload["actual"])
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
