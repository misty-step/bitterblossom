package fleet

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/misty-step/bitterblossom/internal/sprite"
)

func TestActionsViewAndDescriptions(t *testing.T) {
	t.Parallel()

	actions := []Action{
		&RedispatchAction{Name: "fern", Reason: "busy"},
		&TeardownAction{Name: "moss"},
		&UpdateAction{Desired: SpriteSpec{Name: "bramble"}},
		&ProvisionAction{
			Sprite: SpriteSpec{
				Name:    "clover",
				Persona: sprite.Persona{Name: "clover"},
			},
			ConfigVersion: "7",
			Reason:        "missing",
		},
	}

	view := ActionsView(actions)
	if len(view) != 4 {
		t.Fatalf("len(view) = %d, want 4", len(view))
	}

	gotKinds := []ActionKind{view[0].Kind, view[1].Kind, view[2].Kind, view[3].Kind}
	wantKinds := []ActionKind{ActionTeardown, ActionUpdate, ActionProvision, ActionRedispatch}
	if !reflect.DeepEqual(gotKinds, wantKinds) {
		t.Fatalf("kinds = %v, want %v", gotKinds, wantKinds)
	}

	if got := (&ProvisionAction{Sprite: SpriteSpec{Name: "fern"}}).Description(); got != `provision sprite "fern"` {
		t.Fatalf("provision description = %q", got)
	}
	if got := (&TeardownAction{Name: "fern", Reason: "extra"}).Description(); got != `teardown sprite "fern" (extra)` {
		t.Fatalf("teardown description = %q", got)
	}
	if got := (&UpdateAction{Desired: SpriteSpec{Name: "fern"}, Changes: []string{"persona changed"}, Reason: "drift"}).Description(); got != `update sprite "fern" (persona changed; drift)` {
		t.Fatalf("update description = %q", got)
	}
	if got := (&RedispatchAction{Name: "fern"}).Description(); got != `redispatch workload for sprite "fern"` {
		t.Fatalf("redispatch description = %q", got)
	}
}

func TestActionExecuteWithoutRuntimeAcrossKinds(t *testing.T) {
	t.Parallel()

	cases := []Action{
		&ProvisionAction{Sprite: SpriteSpec{Name: "a"}},
		&UpdateAction{Desired: SpriteSpec{Name: "b"}},
		&RedispatchAction{Name: "c"},
	}

	for _, action := range cases {
		if err := action.Execute(context.Background()); !errors.Is(err, ErrActionRuntimeNotBound) {
			t.Fatalf("%T Execute() error = %v, want ErrActionRuntimeNotBound", action, err)
		}
	}
}
