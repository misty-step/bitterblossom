package fleet

import (
	"path/filepath"
	"testing"

	"github.com/misty-step/bitterblossom/internal/sprite"
)

func TestBuildPlanUsesUnknownForBlankDriftValues(t *testing.T) {
	t.Parallel()

	desired := Composition{
		Version: 0,
		Sprites: []SpriteSpec{{Name: "bramble", Persona: sprite.Persona{Name: "bramble"}}},
	}
	actual := []SpriteStatus{{Name: "bramble", Persona: "bramble", ConfigVersion: "9", State: sprite.StateIdle}}

	plan := BuildPlan(desired, actual)
	if len(plan.Actions) != 1 {
		t.Fatalf("len(plan.Actions) = %d, want 1", len(plan.Actions))
	}
	update, ok := plan.Actions[0].(*UpdateAction)
	if !ok {
		t.Fatalf("plan.Actions[0] type = %T, want *UpdateAction", plan.Actions[0])
	}
	if !containsChange(update.Changes, `config "9" -> "<unknown>"`) {
		t.Fatalf("update.Changes = %v, want unknown desired config marker", update.Changes)
	}
}

func TestParseCompositionAlias(t *testing.T) {
	t.Parallel()

	path := filepath.Clean(filepath.Join("..", "..", "compositions", "v1.yaml"))
	composition, err := ParseComposition(path)
	if err != nil {
		t.Fatalf("ParseComposition() error = %v", err)
	}
	if composition.Name == "" {
		t.Fatal("expected non-empty composition name")
	}
}
