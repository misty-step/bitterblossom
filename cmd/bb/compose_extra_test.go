package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/misty-step/bitterblossom/internal/fleet"
	"github.com/misty-step/bitterblossom/internal/sprite"
	"github.com/spf13/cobra"
)

func TestPrintHelpers(t *testing.T) {
	t.Parallel()

	cmd := &cobra.Command{}
	var out bytes.Buffer
	cmd.SetOut(&out)
	if err := printJSON(cmd, map[string]string{"ok": "yes"}); err != nil {
		t.Fatalf("printJSON() error = %v", err)
	}
	if !strings.Contains(out.String(), `"ok": "yes"`) {
		t.Fatalf("printJSON output = %q", out.String())
	}

	out.Reset()
	if err := printActionsHuman(cmd, nil); err != nil {
		t.Fatalf("printActionsHuman(nil) error = %v", err)
	}
	if !strings.Contains(out.String(), "Fleet already converged.") {
		t.Fatalf("printActionsHuman(nil) output = %q", out.String())
	}

	out.Reset()
	actions := []fleet.Action{
		&fleet.TeardownAction{Name: "thorn"},
		&fleet.ProvisionAction{Sprite: fleet.SpriteSpec{Name: "bramble", Persona: sprite.Persona{Name: "bramble"}}},
	}
	if err := printActionsHuman(cmd, actions); err != nil {
		t.Fatalf("printActionsHuman(actions) error = %v", err)
	}
	if !strings.Contains(out.String(), "ACTION") || !strings.Contains(out.String(), "teardown") {
		t.Fatalf("printActionsHuman output = %q", out.String())
	}

	cmd.SetOut(errorWriter{})
	if err := printJSON(cmd, map[string]string{"x": "y"}); err == nil {
		t.Fatal("printJSON() expected write error")
	}
}

func TestLoadFleetStateErrors(t *testing.T) {
	t.Parallel()

	parseErr := errors.New("parse failed")
	listErr := errors.New("list failed")

	baseOpts := composeOptions{
		CompositionPath: "unused.yaml",
	}

	_, _, _, err := loadFleetState(context.Background(), baseOpts, composeDeps{
		parseComposition: func(string) (fleet.Composition, error) { return fleet.Composition{}, parseErr },
	})
	if !errors.Is(err, parseErr) {
		t.Fatalf("parse error = %v, want %v", err, parseErr)
	}

	_, _, _, err = loadFleetState(context.Background(), baseOpts, composeDeps{
		parseComposition: func(string) (fleet.Composition, error) { return testComposition(), nil },
		newCLI: func(string, string) sprite.SpriteCLI {
			return &sprite.MockSpriteCLI{
				ListFn: func(context.Context) ([]string, error) { return nil, listErr },
			}
		},
	})
	if !errors.Is(err, listErr) {
		t.Fatalf("list error = %v, want %v", err, listErr)
	}
}

func TestComposeRuntimeProvisionTeardownUpdateRedispatch(t *testing.T) {
	t.Parallel()

	createCalls := 0
	destroyCalls := 0
	execCalls := 0

	mock := &sprite.MockSpriteCLI{
		CreateFn: func(_ context.Context, name, org string) error {
			createCalls++
			return nil
		},
		DestroyFn: func(_ context.Context, name, org string) error {
			destroyCalls++
			if name == "broken" {
				return errors.New("destroy failed")
			}
			if name == "gone" {
				return errors.New("sprite not found")
			}
			return nil
		},
		ExecFn: func(_ context.Context, name, command string, stdin []byte) (string, error) {
			execCalls++
			if name == "fail" {
				return "", errors.New("exec failed")
			}
			if name == "gone" {
				return "", errors.New("sprite not found")
			}
			return "", nil
		},
	}

	runtime := newComposeRuntime(mock, "test-org")

	// Provision
	if err := runtime.Provision(context.Background(), fleet.ProvisionAction{
		Sprite: fleet.SpriteSpec{Name: "new", Persona: sprite.Persona{Name: "new"}},
	}); err != nil {
		t.Fatalf("Provision() error = %v", err)
	}
	if createCalls != 1 {
		t.Fatalf("createCalls = %d, want 1", createCalls)
	}

	// Teardown
	if err := runtime.Teardown(context.Background(), fleet.TeardownAction{Name: "existing"}); err != nil {
		t.Fatalf("Teardown() error = %v", err)
	}
	if destroyCalls != 1 {
		t.Fatalf("destroyCalls = %d, want 1", destroyCalls)
	}

	// Teardown error (real failure)
	if err := runtime.Teardown(context.Background(), fleet.TeardownAction{Name: "broken"}); err == nil {
		t.Fatal("Teardown(broken) expected error")
	}

	// Teardown not-found (should succeed gracefully)
	if err := runtime.Teardown(context.Background(), fleet.TeardownAction{Name: "gone"}); err != nil {
		t.Fatalf("Teardown(gone) unexpected error = %v", err)
	}

	// Update (destroy + create)
	destroyCalls = 0
	createCalls = 0
	if err := runtime.Update(context.Background(), fleet.UpdateAction{
		Desired: fleet.SpriteSpec{Name: "update", Persona: sprite.Persona{Name: "update"}},
	}); err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if destroyCalls != 1 || createCalls != 1 {
		t.Fatalf("Update: destroyCalls=%d createCalls=%d, want 1,1", destroyCalls, createCalls)
	}

	// Redispatch
	if err := runtime.Redispatch(context.Background(), fleet.RedispatchAction{Name: "running"}); err != nil {
		t.Fatalf("Redispatch() error = %v", err)
	}
	if execCalls != 1 {
		t.Fatalf("execCalls = %d, want 1", execCalls)
	}

	// Redispatch error
	if err := runtime.Redispatch(context.Background(), fleet.RedispatchAction{Name: "fail"}); err == nil {
		t.Fatal("Redispatch(fail) expected error")
	}

	// Redispatch not-found (should succeed gracefully)
	if err := runtime.Redispatch(context.Background(), fleet.RedispatchAction{Name: "gone"}); err != nil {
		t.Fatalf("Redispatch(gone) unexpected error = %v", err)
	}
}

func TestComposeStatusHumanOutput(t *testing.T) {
	t.Parallel()

	deps := composeDeps{
		parseComposition: func(string) (fleet.Composition, error) {
			return testComposition(), nil
		},
		newCLI: func(string, string) sprite.SpriteCLI {
			return &sprite.MockSpriteCLI{
				ListFn: func(context.Context) ([]string, error) {
					return []string{"bramble", "thorn"}, nil
				},
			}
		},
	}

	opts := composeOptions{
		CompositionPath: "unused.yaml",
	}

	cmd := &cobra.Command{}
	var out bytes.Buffer
	cmd.SetOut(&out)

	if err := runComposeStatus(context.Background(), cmd, opts, deps); err != nil {
		t.Fatalf("runComposeStatus() error = %v", err)
	}
	if !strings.Contains(out.String(), "Composition:") || !strings.Contains(out.String(), "SPRITE") {
		t.Fatalf("status output = %q", out.String())
	}
}

func TestRunComposeStatusError(t *testing.T) {
	t.Parallel()

	cmd := &cobra.Command{}
	cmd.SetOut(io.Discard)

	err := runComposeStatus(context.Background(), cmd, composeOptions{
		CompositionPath: "missing.yaml",
	}, composeDeps{
		parseComposition: func(string) (fleet.Composition, error) {
			return fleet.Composition{}, errors.New("parse failed")
		},
	})
	if err == nil {
		t.Fatal("runComposeStatus() expected error")
	}
}

type errorWriter struct{}

func (errorWriter) Write([]byte) (int, error) {
	return 0, errors.New("write failed")
}

func TestRunComposeApplyAndDiffVariants(t *testing.T) {
	t.Parallel()

	baseOpts := composeOptions{
		CompositionPath: "unused.yaml",
	}

	t.Run("dry run json", func(t *testing.T) {
		t.Parallel()

		deps := composeDeps{
			parseComposition: func(string) (fleet.Composition, error) { return testComposition(), nil },
			newCLI: func(string, string) sprite.SpriteCLI {
				return &sprite.MockSpriteCLI{
					ListFn: func(context.Context) ([]string, error) { return nil, nil },
				}
			},
		}

		cmd := &cobra.Command{}
		var out bytes.Buffer
		cmd.SetOut(&out)

		opts := baseOpts
		opts.JSON = true
		if err := runComposeApply(context.Background(), cmd, opts, deps); err != nil {
			t.Fatalf("runComposeApply() error = %v", err)
		}
		if !strings.Contains(out.String(), `"execute": false`) {
			t.Fatalf("dry-run json output = %q", out.String())
		}
	})

	t.Run("dry run converged", func(t *testing.T) {
		t.Parallel()

		deps := composeDeps{
			parseComposition: func(string) (fleet.Composition, error) { return testComposition(), nil },
			newCLI: func(string, string) sprite.SpriteCLI {
				return &sprite.MockSpriteCLI{
					ListFn: func(context.Context) ([]string, error) {
						return []string{"bramble"}, nil
					},
				}
			},
		}

		cmd := &cobra.Command{}
		var out bytes.Buffer
		cmd.SetOut(&out)
		if err := runComposeApply(context.Background(), cmd, baseOpts, deps); err != nil {
			t.Fatalf("runComposeApply() error = %v", err)
		}
		// With metadata populated from composition, the reconciler sees convergence.
		output := out.String()
		if !strings.Contains(output, "Fleet already converged") {
			t.Fatalf("dry-run output = %q, want converged", output)
		}
	})

	t.Run("execute json", func(t *testing.T) {
		t.Parallel()

		createCalls := 0
		deps := composeDeps{
			parseComposition: func(string) (fleet.Composition, error) { return testComposition(), nil },
			newCLI: func(string, string) sprite.SpriteCLI {
				return &sprite.MockSpriteCLI{
					ListFn: func(context.Context) ([]string, error) { return nil, nil },
					CreateFn: func(context.Context, string, string) error {
						createCalls++
						return nil
					},
				}
			},
		}

		cmd := &cobra.Command{}
		var out bytes.Buffer
		cmd.SetOut(&out)

		opts := baseOpts
		opts.JSON = true
		opts.Execute = true
		if err := runComposeApply(context.Background(), cmd, opts, deps); err != nil {
			t.Fatalf("runComposeApply() error = %v", err)
		}
		if createCalls != 1 {
			t.Fatalf("createCalls = %d, want 1", createCalls)
		}
		if !strings.Contains(out.String(), `"execute": true`) {
			t.Fatalf("execute json output = %q", out.String())
		}
	})

	t.Run("execute runtime error", func(t *testing.T) {
		t.Parallel()

		deps := composeDeps{
			parseComposition: func(string) (fleet.Composition, error) { return testComposition(), nil },
			newCLI: func(string, string) sprite.SpriteCLI {
				return &sprite.MockSpriteCLI{
					ListFn: func(context.Context) ([]string, error) { return nil, nil },
					CreateFn: func(context.Context, string, string) error {
						return errors.New("create failed")
					},
				}
			},
		}

		cmd := &cobra.Command{}
		cmd.SetOut(io.Discard)

		opts := baseOpts
		opts.Execute = true
		if err := runComposeApply(context.Background(), cmd, opts, deps); err == nil {
			t.Fatal("runComposeApply() expected runtime error")
		}
	})

	t.Run("diff human and error", func(t *testing.T) {
		t.Parallel()

		okDeps := composeDeps{
			parseComposition: func(string) (fleet.Composition, error) { return testComposition(), nil },
			newCLI: func(string, string) sprite.SpriteCLI {
				return &sprite.MockSpriteCLI{
					ListFn: func(context.Context) ([]string, error) { return nil, nil },
				}
			},
		}

		cmd := &cobra.Command{}
		var out bytes.Buffer
		cmd.SetOut(&out)
		if err := runComposeDiff(context.Background(), cmd, baseOpts, okDeps); err != nil {
			t.Fatalf("runComposeDiff() error = %v", err)
		}
		if !strings.Contains(out.String(), "provision") {
			t.Fatalf("diff output = %q", out.String())
		}

		badDeps := composeDeps{
			parseComposition: func(string) (fleet.Composition, error) { return fleet.Composition{}, errors.New("parse fail") },
		}
		if err := runComposeDiff(context.Background(), cmd, baseOpts, badDeps); err == nil {
			t.Fatal("runComposeDiff() expected error")
		}
	})
}

func TestDefaultComposeDeps(t *testing.T) {
	t.Parallel()

	deps := defaultComposeDeps()

	cli := deps.newCLI("sprite", "test-org")
	if cli == nil {
		t.Fatal("expected non-nil sprite CLI")
	}
	typed, ok := cli.(sprite.CLI)
	if !ok {
		t.Fatalf("newCLI returned %T, want sprite.CLI", cli)
	}
	if typed.Org != "test-org" {
		t.Fatalf("CLI.Org = %q, want test-org", typed.Org)
	}
}

func TestComposeRuntimeUpdateDestroyError(t *testing.T) {
	t.Parallel()

	// When destroy fails with a real error (not "not found"), Update propagates it.
	createCalls := 0
	mock := &sprite.MockSpriteCLI{
		DestroyFn: func(context.Context, string, string) error { return errors.New("destroy failed") },
		CreateFn: func(context.Context, string, string) error {
			createCalls++
			return nil
		},
	}
	runtime := newComposeRuntime(mock, "test-org")

	err := runtime.Update(context.Background(), fleet.UpdateAction{
		Desired: fleet.SpriteSpec{Name: "x", Persona: sprite.Persona{Name: "x"}},
	})
	if err == nil {
		t.Fatal("Update() expected destroy error to propagate")
	}
	if !strings.Contains(err.Error(), "destroying sprite") {
		t.Fatalf("Update() error = %v, want wrapped destroy error", err)
	}
	if createCalls != 0 {
		t.Fatalf("createCalls = %d, want 0 (should not attempt create after destroy failure)", createCalls)
	}
}

func TestComposeRuntimeUpdateDestroyNotFound(t *testing.T) {
	t.Parallel()

	// When destroy fails with "not found", Update ignores it and proceeds to create.
	createCalls := 0
	mock := &sprite.MockSpriteCLI{
		DestroyFn: func(context.Context, string, string) error { return errors.New("sprite not found") },
		CreateFn: func(context.Context, string, string) error {
			createCalls++
			return nil
		},
	}
	runtime := newComposeRuntime(mock, "test-org")

	err := runtime.Update(context.Background(), fleet.UpdateAction{
		Desired: fleet.SpriteSpec{Name: "x", Persona: sprite.Persona{Name: "x"}},
	})
	if err != nil {
		t.Fatalf("Update() error = %v (not-found should be ignored)", err)
	}
	if createCalls != 1 {
		t.Fatalf("createCalls = %d, want 1", createCalls)
	}
}

func TestIsNotFoundError(t *testing.T) {
	t.Parallel()

	if isNotFoundError(nil) {
		t.Fatal("nil should not be not-found")
	}
	if !isNotFoundError(errors.New("sprite not found")) {
		t.Fatal("'not found' should be detected")
	}
	if !isNotFoundError(errors.New("resource does not exist")) {
		t.Fatal("'does not exist' should be detected")
	}
	if isNotFoundError(errors.New("auth failure")) {
		t.Fatal("unrelated error should not match")
	}
}
