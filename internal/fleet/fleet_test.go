package fleet

import (
	"testing"

	"github.com/misty-step/bitterblossom/internal/sprite"
)

func TestFleetReconcile(t *testing.T) {
	t.Parallel()

	desired := Composition{
		Name: "test",
		Sprites: []SpriteSpec{
			{
				Name:       "bramble",
				Definition: "sprites/bramble.md",
				Persona: sprite.Persona{
					Name:       "bramble",
					Definition: "sprites/bramble.md",
				},
			},
			{
				Name:       "willow",
				Definition: "sprites/willow.md",
				Persona: sprite.Persona{
					Name:       "willow",
					Definition: "sprites/willow.md",
				},
			},
		},
	}

	bramble := mustSprite(t, "bramble", sprite.Persona{Name: "bramble", Definition: "sprites/bramble.md"}, sprite.StateIdle, true)
	willow := mustSprite(t, "willow", sprite.Persona{Name: "willow", Definition: "sprites/willow.md"}, sprite.StateDead, true)
	moss := mustSprite(t, "moss", sprite.Persona{Name: "moss", Definition: "sprites/moss.md"}, sprite.StateIdle, true)

	f := New(desired, []*sprite.Sprite{bramble, willow, moss})
	actions := f.Reconcile()

	if len(actions) != 2 {
		t.Fatalf("expected 2 actions, got %d", len(actions))
	}

	expected := map[ActionKind]string{
		ActionProvision: "willow",
		ActionTeardown:  "moss",
	}
	for _, action := range actions {
		wantSprite, ok := expected[action.Kind]
		if !ok {
			t.Fatalf("unexpected action kind: %s", action.Kind)
		}
		if action.Sprite != wantSprite {
			t.Fatalf("expected %s target %q, got %q", action.Kind, wantSprite, action.Sprite)
		}
	}
}

func TestFleetReconcileDetectsPersonaDrift(t *testing.T) {
	t.Parallel()

	desired := Composition{
		Name: "test",
		Sprites: []SpriteSpec{
			{
				Name:       "bramble",
				Definition: "sprites/bramble.md",
				Persona: sprite.Persona{
					Name:       "bramble",
					Definition: "sprites/bramble.md",
				},
			},
		},
	}

	actual := mustSprite(t, "bramble", sprite.Persona{Name: "thorn", Definition: "sprites/thorn.md"}, sprite.StateIdle, true)
	f := New(desired, []*sprite.Sprite{actual})

	actions := f.Reconcile()
	if len(actions) != 1 {
		t.Fatalf("expected one action, got %d", len(actions))
	}
	if actions[0].Kind != ActionReconfigure {
		t.Fatalf("expected reconfigure action, got %s", actions[0].Kind)
	}
	if actions[0].Sprite != "bramble" {
		t.Fatalf("expected bramble action, got %s", actions[0].Sprite)
	}
}

func TestFleetStatus(t *testing.T) {
	t.Parallel()

	desired := Composition{
		Name: "test",
		Sprites: []SpriteSpec{
			{
				Name:       "bramble",
				Definition: "sprites/bramble.md",
				Persona: sprite.Persona{
					Name:       "bramble",
					Definition: "sprites/bramble.md",
				},
			},
			{
				Name:       "willow",
				Definition: "sprites/willow.md",
				Persona: sprite.Persona{
					Name:       "willow",
					Definition: "sprites/willow.md",
				},
			},
		},
	}

	bramble := mustSprite(t, "bramble", sprite.Persona{Name: "bramble", Definition: "sprites/bramble.md"}, sprite.StateWorking, true)
	moss := mustSprite(t, "moss", sprite.Persona{Name: "moss", Definition: "sprites/moss.md"}, sprite.StateIdle, true)
	willow := mustSprite(t, "willow", sprite.Persona{Name: "willow", Definition: "sprites/willow.md"}, sprite.StateDead, false)

	report := New(desired, []*sprite.Sprite{bramble, moss, willow}).Status()

	if report.Desired != 2 {
		t.Fatalf("expected desired=2, got %d", report.Desired)
	}
	if report.Actual != 3 {
		t.Fatalf("expected actual=3, got %d", report.Actual)
	}
	if got := report.States[sprite.StateWorking]; got != 1 {
		t.Fatalf("expected working count 1, got %d", got)
	}
	if got := report.States[sprite.StateIdle]; got != 1 {
		t.Fatalf("expected idle count 1, got %d", got)
	}
	if got := report.States[sprite.StateDead]; got != 1 {
		t.Fatalf("expected dead count 1, got %d", got)
	}
	if len(report.Extra) != 1 || report.Extra[0] != "moss" {
		t.Fatalf("expected extra=[moss], got %v", report.Extra)
	}
	if len(report.UnprovisionedNames) != 1 || report.UnprovisionedNames[0] != "willow" {
		t.Fatalf("expected unprovisioned=[willow], got %v", report.UnprovisionedNames)
	}
}

func mustSprite(t *testing.T, name string, persona sprite.Persona, state sprite.State, provisioned bool) *sprite.Sprite {
	t.Helper()
	s, err := sprite.New(name, persona, sprite.WithInitialState(state, provisioned))
	if err != nil {
		t.Fatalf("sprite.New(%s): %v", name, err)
	}
	return s
}
