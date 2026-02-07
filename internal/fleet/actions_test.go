package fleet

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/misty-step/bitterblossom/internal/sprite"
)

func TestSortActionsOrdersByKindThenName(t *testing.T) {
	actions := []Action{
		&ProvisionAction{Sprite: SpriteSpec{Name: "fern", Persona: sprite.Persona{Name: "fern"}}},
		&TeardownAction{Name: "zeta"},
		&RedispatchAction{Name: "alpha"},
		&UpdateAction{Desired: SpriteSpec{Name: "beta"}},
		&TeardownAction{Name: "alpha"},
	}

	sorted := SortActions(actions)
	got := make([]string, 0, len(sorted))
	for _, action := range sorted {
		got = append(got, string(action.Kind())+":"+action.SpriteName())
	}

	want := []string{
		"teardown:alpha",
		"teardown:zeta",
		"update:beta",
		"provision:fern",
		"redispatch:alpha",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("sorted actions = %v, want %v", got, want)
	}
}

func TestExecutorExecuteRunsRuntimeInOrder(t *testing.T) {
	runtime := &recordingRuntime{}
	executor := Executor{Runtime: runtime}
	actions := []Action{
		&ProvisionAction{Sprite: SpriteSpec{Name: "fern", Persona: sprite.Persona{Name: "fern"}}},
		&TeardownAction{Name: "thorn"},
		&UpdateAction{Desired: SpriteSpec{Name: "bramble"}},
		&RedispatchAction{Name: "bramble"},
	}

	if err := executor.Execute(context.Background(), actions); err != nil {
		t.Fatalf("Executor.Execute() error = %v", err)
	}

	want := []string{
		"teardown:thorn",
		"update:bramble",
		"provision:fern",
		"redispatch:bramble",
	}
	if !reflect.DeepEqual(runtime.calls, want) {
		t.Fatalf("runtime calls = %v, want %v", runtime.calls, want)
	}
}

func TestExecutorDryRun(t *testing.T) {
	actions := []Action{
		&ProvisionAction{Sprite: SpriteSpec{Name: "fern", Persona: sprite.Persona{Name: "fern"}}},
		&TeardownAction{Name: "thorn"},
	}

	lines := Executor{}.DryRun(actions)
	if len(lines) != 2 {
		t.Fatalf("len(lines) = %d, want 2", len(lines))
	}
	if lines[0] != "[dry-run] teardown sprite \"thorn\"" {
		t.Fatalf("lines[0] = %q", lines[0])
	}
	if lines[1] != "[dry-run] provision sprite \"fern\" persona=\"fern\"" {
		t.Fatalf("lines[1] = %q", lines[1])
	}
}

func TestActionExecuteWithoutRuntime(t *testing.T) {
	err := (&TeardownAction{Name: "thorn"}).Execute(context.Background())
	if !errors.Is(err, ErrActionRuntimeNotBound) {
		t.Fatalf("Execute() error = %v, want ErrActionRuntimeNotBound", err)
	}
}

func TestExecutorPropagatesRuntimeError(t *testing.T) {
	runtime := &recordingRuntime{teardownErr: errors.New("boom")}
	executor := Executor{Runtime: runtime}
	actions := []Action{&TeardownAction{Name: "thorn"}}

	err := executor.Execute(context.Background(), actions)
	if err == nil {
		t.Fatal("Executor.Execute() expected error, got nil")
	}
}

type recordingRuntime struct {
	calls       []string
	teardownErr error
}

func (r *recordingRuntime) Provision(_ context.Context, action ProvisionAction) error {
	r.calls = append(r.calls, "provision:"+action.Sprite.Name)
	return nil
}

func (r *recordingRuntime) Teardown(_ context.Context, action TeardownAction) error {
	r.calls = append(r.calls, "teardown:"+action.Name)
	return r.teardownErr
}

func (r *recordingRuntime) Update(_ context.Context, action UpdateAction) error {
	r.calls = append(r.calls, "update:"+action.Desired.Name)
	return nil
}

func (r *recordingRuntime) Redispatch(_ context.Context, action RedispatchAction) error {
	r.calls = append(r.calls, "redispatch:"+action.Name)
	return nil
}
