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

	// Parse the envelope format
	var resp contracts.Response
	if err := json.Unmarshal(stdout.Bytes(), &resp); err != nil {
		t.Fatalf("json.Unmarshal() error = %v, output=%q", err, stdout.String())
	}
	if resp.Version != "v1" {
		t.Fatalf("resp.Version = %v, want v1", resp.Version)
	}
	if resp.Command != "compose.diff" {
		t.Fatalf("resp.Command = %v, want compose.diff", resp.Command)
	}
	data, ok := resp.Data.([]any)
	if !ok {
		t.Fatalf("resp.Data type = %T, want []any", resp.Data)
	}
	if len(data) != 1 {
		t.Fatalf("len(data) = %d, want 1", len(data))
	}
	// Check the first item has expected fields
	item, ok := data[0].(map[string]any)
	if !ok {
		t.Fatalf("data[0] type = %T, want map[string]any", data[0])
	}
	if item["kind"] != "provision" {
		t.Fatalf("data[0].kind = %v, want provision", item["kind"])
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
