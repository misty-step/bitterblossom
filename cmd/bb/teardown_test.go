package main

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/misty-step/bitterblossom/internal/lifecycle"
	"github.com/misty-step/bitterblossom/internal/sprite"
)

func TestTeardownCmdForceWiring(t *testing.T) {
	t.Parallel()

	var captured lifecycle.TeardownOpts
	deps := teardownDeps{
		getwd:  func() (string, error) { return t.TempDir(), nil },
		newCLI: func(string, string) sprite.SpriteCLI { return &sprite.MockSpriteCLI{} },
		teardown: func(_ context.Context, _ sprite.SpriteCLI, _ lifecycle.Config, opts lifecycle.TeardownOpts) (lifecycle.TeardownResult, error) {
			captured = opts
			return lifecycle.TeardownResult{Name: opts.Name, ArchivePath: opts.ArchiveDir + "/bramble"}, nil
		},
	}

	cmd := newTeardownCmdWithDeps(deps)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"--force", "bramble"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("cmd.Execute() error = %v", err)
	}
	if captured.Name != "bramble" || !captured.Force {
		t.Fatalf("unexpected teardown opts: %+v", captured)
	}

	var payload struct {
		Command string `json:"command"`
	}
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if payload.Command != "teardown" {
		t.Fatalf("command = %q, want teardown", payload.Command)
	}
}

func TestTeardownCmdConfirmationAbort(t *testing.T) {
	t.Parallel()

	called := false
	deps := teardownDeps{
		getwd:  func() (string, error) { return t.TempDir(), nil },
		newCLI: func(string, string) sprite.SpriteCLI { return &sprite.MockSpriteCLI{} },
		teardown: func(context.Context, sprite.SpriteCLI, lifecycle.Config, lifecycle.TeardownOpts) (lifecycle.TeardownResult, error) {
			called = true
			return lifecycle.TeardownResult{}, nil
		},
	}

	cmd := newTeardownCmdWithDeps(deps)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetIn(bytes.NewBufferString("n\n"))
	cmd.SetArgs([]string{"bramble"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("cmd.Execute() error = %v", err)
	}
	if called {
		t.Fatal("teardown should not run when user aborts confirmation")
	}

	output := out.String()
	start := strings.Index(output, "{")
	if start < 0 {
		t.Fatalf("expected JSON output, got %q", output)
	}
	var payload struct {
		Command string `json:"command"`
		Data    struct {
			Aborted bool `json:"aborted"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(output[start:]), &payload); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if payload.Command != "teardown" || !payload.Data.Aborted {
		t.Fatalf("unexpected payload: %+v", payload)
	}
}
