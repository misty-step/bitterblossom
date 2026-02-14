package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"github.com/misty-step/bitterblossom/internal/fleet"
	"github.com/misty-step/bitterblossom/internal/sprite"
	"github.com/misty-step/bitterblossom/pkg/fly"
	"github.com/spf13/cobra"
)

func TestMapMachineStateTable(t *testing.T) {
	t.Parallel()

	cases := []struct {
		input string
		want  sprite.State
	}{
		{input: "running", want: sprite.StateWorking},
		{input: "started", want: sprite.StateWorking},
		{input: "stopped", want: sprite.StateIdle},
		{input: "failed", want: sprite.StateDead},
		{input: "blocked", want: sprite.StateBlocked},
		{input: "done", want: sprite.StateDone},
		{input: "provisioned", want: sprite.StateProvisioned},
		{input: "something-else", want: sprite.StateIdle},
	}

	for _, tc := range cases {
		if got := mapMachineState(tc.input); got != tc.want {
			t.Fatalf("mapMachineState(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestDefaultFlyTokenPrecedence(t *testing.T) {
	t.Setenv("FLY_TOKEN", "fallback")
	t.Setenv("FLY_API_TOKEN", "preferred")
	if got := defaultFlyToken(); got != "preferred" {
		t.Fatalf("defaultFlyToken() = %q, want preferred", got)
	}

	t.Setenv("FLY_API_TOKEN", "")
	if got := defaultFlyToken(); got != "fallback" {
		t.Fatalf("defaultFlyToken() fallback = %q, want fallback", got)
	}
}

func TestPrintHelpers(t *testing.T) {
	t.Parallel()

	cmd := &cobra.Command{}
	var out bytes.Buffer
	cmd.SetOut(&out)
	if err := printJSON(cmd, "compose.test", map[string]string{"ok": "yes"}); err != nil {
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
	if err := printJSON(cmd, "compose.test", map[string]string{"x": "y"}); err == nil {
		t.Fatal("printJSON() expected write error")
	}
}

func TestLoadFleetStateErrors(t *testing.T) {
	t.Parallel()

	parseErr := errors.New("parse failed")
	newClientErr := errors.New("new client failed")
	listErr := errors.New("list failed")

	baseOpts := composeOptions{
		CompositionPath: "unused.yaml",
		App:             "app",
		Token:           "token",
		APIURL:          fly.DefaultBaseURL,
	}

	_, _, _, err := loadFleetState(context.Background(), baseOpts, composeDeps{
		parseComposition: func(string) (fleet.Composition, error) { return fleet.Composition{}, parseErr },
	})
	if !errors.Is(err, parseErr) {
		t.Fatalf("parse error = %v, want %v", err, parseErr)
	}

	_, _, _, err = loadFleetState(context.Background(), composeOptions{Token: "token"}, composeDeps{
		parseComposition: func(string) (fleet.Composition, error) { return testComposition(), nil },
	})
	if err == nil || !strings.Contains(err.Error(), "FLY_APP and FLY_API_TOKEN are required") {
		t.Fatalf("missing app error = %v", err)
	}

	_, _, _, err = loadFleetState(context.Background(), composeOptions{App: "app"}, composeDeps{
		parseComposition: func(string) (fleet.Composition, error) { return testComposition(), nil },
	})
	if err == nil || !strings.Contains(err.Error(), "FLY_APP and FLY_API_TOKEN are required") {
		t.Fatalf("missing token error = %v", err)
	}

	_, _, _, err = loadFleetState(context.Background(), baseOpts, composeDeps{
		parseComposition: func(string) (fleet.Composition, error) { return testComposition(), nil },
		newClient:        func(string, string) (fly.MachineClient, error) { return nil, newClientErr },
	})
	if !errors.Is(err, newClientErr) {
		t.Fatalf("newClient error = %v, want %v", err, newClientErr)
	}

	_, _, _, err = loadFleetState(context.Background(), baseOpts, composeDeps{
		parseComposition: func(string) (fleet.Composition, error) { return testComposition(), nil },
		newClient: func(string, string) (fly.MachineClient, error) {
			return &fly.MockMachineClient{
				ListFn: func(context.Context, string) ([]fly.Machine, error) { return nil, listErr },
			}, nil
		},
	})
	if !errors.Is(err, listErr) {
		t.Fatalf("list error = %v, want %v", err, listErr)
	}
}

func TestComposeRuntimeProvisionTeardownUpdateRedispatch(t *testing.T) {
	t.Parallel()

	mock := &fly.MockMachineClient{}
	runtime := newComposeRuntime("app", mock, []fleet.SpriteStatus{
		{Name: "existing", MachineID: "m-existing"},
		{Name: "blank", MachineID: ""},
	})

	if runtime.machineIDs["existing"] != "m-existing" {
		t.Fatalf("machineIDs did not include existing mapping")
	}
	if _, ok := runtime.machineIDs["blank"]; ok {
		t.Fatalf("machineIDs should ignore empty machine id")
	}

	createCalls := 0
	mock.CreateFn = func(_ context.Context, req fly.CreateRequest) (fly.Machine, error) {
		createCalls++
		if req.Name == "new" {
			return fly.Machine{ID: "m-new"}, nil
		}
		return fly.Machine{ID: "m-update"}, nil
	}

	if err := runtime.Provision(context.Background(), fleet.ProvisionAction{
		Sprite: fleet.SpriteSpec{Name: "existing", Persona: sprite.Persona{Name: "existing"}},
	}); err != nil {
		t.Fatalf("Provision(existing) error = %v", err)
	}
	if createCalls != 0 {
		t.Fatalf("createCalls for existing = %d, want 0", createCalls)
	}

	if err := runtime.Provision(context.Background(), fleet.ProvisionAction{
		Sprite:        fleet.SpriteSpec{Name: "new", Persona: sprite.Persona{Name: "new"}},
		ConfigVersion: "3",
	}); err != nil {
		t.Fatalf("Provision(new) error = %v", err)
	}
	if createCalls != 1 {
		t.Fatalf("createCalls after new provision = %d, want 1", createCalls)
	}
	if runtime.machineIDs["new"] != "m-new" {
		t.Fatalf("new mapping = %q, want m-new", runtime.machineIDs["new"])
	}

	if err := runtime.Teardown(context.Background(), fleet.TeardownAction{Name: "missing"}); err != nil {
		t.Fatalf("Teardown(missing) error = %v", err)
	}

	destroyCalls := 0
	mock.DestroyFn = func(_ context.Context, _ string, machineID string) error {
		destroyCalls++
		if machineID == "m-existing" {
			return fly.APIError{StatusCode: 404}
		}
		if machineID == "m-hard-fail" {
			return errors.New("destroy failed")
		}
		return nil
	}

	if err := runtime.Teardown(context.Background(), fleet.TeardownAction{Name: "existing"}); err != nil {
		t.Fatalf("Teardown(existing) error = %v", err)
	}
	if _, ok := runtime.machineIDs["existing"]; ok {
		t.Fatalf("existing should be removed after not-found destroy")
	}

	runtime.machineIDs["broken"] = "m-hard-fail"
	if err := runtime.Teardown(context.Background(), fleet.TeardownAction{Name: "broken"}); err == nil {
		t.Fatal("Teardown(broken) expected error")
	}

	runtime.machineIDs["update"] = "m-update-old"
	mock.DestroyFn = func(_ context.Context, _ string, machineID string) error {
		if machineID == "m-update-old" {
			return fly.APIError{StatusCode: 404}
		}
		return nil
	}
	if err := runtime.Update(context.Background(), fleet.UpdateAction{
		Desired:       fleet.SpriteSpec{Name: "update", Persona: sprite.Persona{Name: "update"}},
		DesiredConfig: "9",
	}); err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if runtime.machineIDs["update"] != "m-update" {
		t.Fatalf("update mapping = %q, want m-update", runtime.machineIDs["update"])
	}

	execCalls := 0
	mock.ExecFn = func(_ context.Context, _ string, machineID string, _ fly.ExecRequest) (fly.ExecResult, error) {
		execCalls++
		if machineID == "m-update" {
			return fly.ExecResult{}, fly.APIError{StatusCode: 404}
		}
		return fly.ExecResult{}, errors.New("exec failed")
	}

	if err := runtime.Redispatch(context.Background(), fleet.RedispatchAction{Name: "missing"}); err != nil {
		t.Fatalf("Redispatch(missing) error = %v", err)
	}

	if err := runtime.Redispatch(context.Background(), fleet.RedispatchAction{Name: "update"}); err != nil {
		t.Fatalf("Redispatch(not found) error = %v", err)
	}
	if _, ok := runtime.machineIDs["update"]; ok {
		t.Fatalf("update should be removed after redispatch not-found")
	}

	runtime.machineIDs["fail"] = "m-fail"
	if err := runtime.Redispatch(context.Background(), fleet.RedispatchAction{Name: "fail"}); err == nil {
		t.Fatal("Redispatch(fail) expected error")
	}
	if execCalls < 2 {
		t.Fatalf("expected at least 2 exec calls, got %d", execCalls)
	}
}

func TestComposeStatusAndHelpers(t *testing.T) {
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
						{
							ID:    "m2",
							Name:  "thorn",
							State: "stopped",
							Metadata: map[string]string{
								"persona": "thorn",
							},
						},
					}, nil
				},
			}, nil
		},
	}

	opts := composeOptions{
		CompositionPath: "unused.yaml",
		App:             "app",
		Token:           "token",
		APIURL:          fly.DefaultBaseURL,
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

	statuses := machinesToSpriteStatuses([]fly.Machine{
		{ID: "2", Name: "b", State: "running"},
		{ID: "1", Name: "a", State: "stopped"},
	})
	if len(statuses) != 2 || statuses[0].Name != "a" {
		t.Fatalf("machinesToSpriteStatuses() = %+v", statuses)
	}

	if !isNotFound(fly.APIError{StatusCode: 404}) {
		t.Fatal("isNotFound should match APIError 404")
	}
	if isNotFound(errors.New("other")) {
		t.Fatal("isNotFound should be false for non-APIError")
	}
}

func TestRunComposeStatusError(t *testing.T) {
	t.Parallel()

	cmd := &cobra.Command{}
	cmd.SetOut(io.Discard)

	err := runComposeStatus(context.Background(), cmd, composeOptions{
		CompositionPath: "missing.yaml",
		App:             "app",
		Token:           "token",
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
		App:             "app",
		Token:           "token",
		APIURL:          fly.DefaultBaseURL,
	}

	t.Run("dry run json", func(t *testing.T) {
		t.Parallel()

		deps := composeDeps{
			parseComposition: func(string) (fleet.Composition, error) { return testComposition(), nil },
			newClient: func(string, string) (fly.MachineClient, error) {
				return &fly.MockMachineClient{
					ListFn: func(context.Context, string) ([]fly.Machine, error) { return nil, nil },
				}, nil
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
			newClient: func(string, string) (fly.MachineClient, error) {
				return &fly.MockMachineClient{
					ListFn: func(context.Context, string) ([]fly.Machine, error) {
						return []fly.Machine{{
							ID:    "m1",
							Name:  "bramble",
							State: "running",
							Metadata: map[string]string{
								"persona":        "bramble",
								"config_version": "1",
							},
						}}, nil
					},
				}, nil
			},
		}

		cmd := &cobra.Command{}
		var out bytes.Buffer
		cmd.SetOut(&out)
		if err := runComposeApply(context.Background(), cmd, baseOpts, deps); err != nil {
			t.Fatalf("runComposeApply() error = %v", err)
		}
		if !strings.Contains(out.String(), "Fleet already converged.") {
			t.Fatalf("dry-run converged output = %q", out.String())
		}
	})

	t.Run("execute json", func(t *testing.T) {
		t.Parallel()

		createCalls := 0
		deps := composeDeps{
			parseComposition: func(string) (fleet.Composition, error) { return testComposition(), nil },
			newClient: func(string, string) (fly.MachineClient, error) {
				return &fly.MockMachineClient{
					ListFn: func(context.Context, string) ([]fly.Machine, error) { return nil, nil },
					CreateFn: func(context.Context, fly.CreateRequest) (fly.Machine, error) {
						createCalls++
						return fly.Machine{ID: "m1"}, nil
					},
				}, nil
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
			newClient: func(string, string) (fly.MachineClient, error) {
				return &fly.MockMachineClient{
					ListFn: func(context.Context, string) ([]fly.Machine, error) { return nil, nil },
					CreateFn: func(context.Context, fly.CreateRequest) (fly.Machine, error) {
						return fly.Machine{}, errors.New("create failed")
					},
				}, nil
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

	t.Run("execute converged", func(t *testing.T) {
		t.Parallel()

		deps := composeDeps{
			parseComposition: func(string) (fleet.Composition, error) { return testComposition(), nil },
			newClient: func(string, string) (fly.MachineClient, error) {
				return &fly.MockMachineClient{
					ListFn: func(context.Context, string) ([]fly.Machine, error) {
						return []fly.Machine{{
							ID:    "m1",
							Name:  "bramble",
							State: "running",
							Metadata: map[string]string{
								"persona":        "bramble",
								"config_version": "1",
							},
						}}, nil
					},
				}, nil
			},
		}

		cmd := &cobra.Command{}
		var out bytes.Buffer
		cmd.SetOut(&out)

		opts := baseOpts
		opts.Execute = true
		if err := runComposeApply(context.Background(), cmd, opts, deps); err != nil {
			t.Fatalf("runComposeApply() error = %v", err)
		}
		if !strings.Contains(out.String(), "Fleet already converged.") {
			t.Fatalf("execute converged output = %q", out.String())
		}
	})

	t.Run("diff human and error", func(t *testing.T) {
		t.Parallel()

		okDeps := composeDeps{
			parseComposition: func(string) (fleet.Composition, error) { return testComposition(), nil },
			newClient: func(string, string) (fly.MachineClient, error) {
				return &fly.MockMachineClient{
					ListFn: func(context.Context, string) ([]fly.Machine, error) { return nil, nil },
				}, nil
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

	path := filepath.Clean(filepath.Join("..", "..", "compositions", "v1.yaml"))
	composition, err := deps.parseComposition(path)
	if err != nil {
		t.Fatalf("default parseComposition() error = %v", err)
	}
	if composition.Name == "" {
		t.Fatalf("expected non-empty composition name")
	}

	client, err := deps.newClient("token", "https://api.sprites.dev/v1")
	if err != nil {
		t.Fatalf("default newClient() error = %v", err)
	}
	typed, ok := client.(*fly.Client)
	if !ok {
		t.Fatalf("newClient returned %T, want *fly.Client", client)
	}
	if typed == nil {
		t.Fatal("expected non-nil fly client")
	}
}

func TestComposeRuntimeUpdateDestroyError(t *testing.T) {
	t.Parallel()

	mock := &fly.MockMachineClient{
		DestroyFn: func(context.Context, string, string) error { return errors.New("destroy failed") },
	}
	runtime := newComposeRuntime("app", mock, []fleet.SpriteStatus{{Name: "x", MachineID: "m-x"}})

	err := runtime.Update(context.Background(), fleet.UpdateAction{
		Desired: fleet.SpriteSpec{Name: "x", Persona: sprite.Persona{Name: "x"}},
	})
	if err == nil || !strings.Contains(err.Error(), "destroy failed") {
		t.Fatalf("Update() error = %v", err)
	}
}

func TestComposeRuntimeTeardownDirectMachineID(t *testing.T) {
	t.Parallel()

	called := false
	mock := &fly.MockMachineClient{
		DestroyFn: func(_ context.Context, app, machineID string) error {
			called = true
			if app != "app" || machineID != "m-direct" {
				t.Fatalf("Destroy() args app=%q machineID=%q", app, machineID)
			}
			return nil
		},
	}
	runtime := newComposeRuntime("app", mock, nil)
	if err := runtime.Teardown(context.Background(), fleet.TeardownAction{Name: "sprite", MachineID: "m-direct"}); err != nil {
		t.Fatalf("Teardown() error = %v", err)
	}
	if !called {
		t.Fatal("expected Destroy() call")
	}
}

func TestIsNotFoundWithWrappedAPIError(t *testing.T) {
	t.Parallel()

	err := errors.New("wrapped: " + (fly.APIError{StatusCode: http.StatusNotFound}).Error())
	if isNotFound(err) {
		t.Fatal("isNotFound() should be false for non-APIError wrappers without errors.As support")
	}
}
