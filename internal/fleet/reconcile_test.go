package fleet

import (
	"reflect"
	"testing"

	"github.com/misty-step/bitterblossom/internal/sprite"
)

func TestReconcileAlreadyConvergedReturnsNoActions(t *testing.T) {
	desired := Composition{
		Version: 1,
		Sprites: []SpriteSpec{{Name: "bramble", Persona: sprite.Persona{Name: "bramble"}}},
	}
	actual := []SpriteStatus{{Name: "bramble", Persona: "bramble", ConfigVersion: "1", State: sprite.StateIdle}}

	actions := Reconcile(desired, actual)
	if len(actions) != 0 {
		t.Fatalf("len(actions) = %d, want 0", len(actions))
	}
}

func TestReconcileMissingSpriteCreatesProvisionAction(t *testing.T) {
	desired := Composition{
		Version: 2,
		Sprites: []SpriteSpec{{Name: "bramble", Persona: sprite.Persona{Name: "bramble"}}},
	}

	actions := Reconcile(desired, nil)
	if len(actions) != 1 {
		t.Fatalf("len(actions) = %d, want 1", len(actions))
	}

	provision, ok := actions[0].(*ProvisionAction)
	if !ok {
		t.Fatalf("actions[0] type = %T, want *ProvisionAction", actions[0])
	}
	if provision.Sprite.Name != "bramble" {
		t.Fatalf("provision.Sprite.Name = %q, want bramble", provision.Sprite.Name)
	}
	if provision.ConfigVersion != "2" {
		t.Fatalf("provision.ConfigVersion = %q, want 2", provision.ConfigVersion)
	}
}

func TestReconcileDeadSpriteCreatesProvisionAction(t *testing.T) {
	desired := Composition{
		Version: 2,
		Sprites: []SpriteSpec{{Name: "bramble", Persona: sprite.Persona{Name: "bramble"}}},
	}
	actual := []SpriteStatus{{Name: "bramble", Persona: "bramble", ConfigVersion: "2", State: sprite.StateDead}}

	actions := Reconcile(desired, actual)
	if len(actions) != 1 {
		t.Fatalf("len(actions) = %d, want 1", len(actions))
	}
	if _, ok := actions[0].(*ProvisionAction); !ok {
		t.Fatalf("actions[0] type = %T, want *ProvisionAction", actions[0])
	}
}

func TestReconcileExtraSpriteCreatesTeardownAction(t *testing.T) {
	desired := Composition{
		Version: 1,
		Sprites: []SpriteSpec{{Name: "bramble", Persona: sprite.Persona{Name: "bramble"}}},
	}
	actual := []SpriteStatus{
		{Name: "bramble", Persona: "bramble", ConfigVersion: "1", State: sprite.StateIdle},
		{Name: "thorn", Persona: "thorn", ConfigVersion: "1", State: sprite.StateIdle},
	}

	actions := Reconcile(desired, actual)
	if len(actions) != 1 {
		t.Fatalf("len(actions) = %d, want 1", len(actions))
	}
	teardown, ok := actions[0].(*TeardownAction)
	if !ok {
		t.Fatalf("actions[0] type = %T, want *TeardownAction", actions[0])
	}
	if teardown.Name != "thorn" {
		t.Fatalf("teardown.Name = %q, want thorn", teardown.Name)
	}
}

func TestReconcilePersonaMismatchCreatesUpdateAction(t *testing.T) {
	desired := Composition{
		Version: 1,
		Sprites: []SpriteSpec{{Name: "bramble", Persona: sprite.Persona{Name: "bramble"}}},
	}
	actual := []SpriteStatus{{Name: "bramble", Persona: "thorn", ConfigVersion: "1", State: sprite.StateIdle}}

	actions := Reconcile(desired, actual)
	if len(actions) != 1 {
		t.Fatalf("len(actions) = %d, want 1", len(actions))
	}
	update, ok := actions[0].(*UpdateAction)
	if !ok {
		t.Fatalf("actions[0] type = %T, want *UpdateAction", actions[0])
	}
	if !containsChange(update.Changes, `persona "thorn" -> "bramble"`) {
		t.Fatalf("update.Changes = %v, expected persona drift", update.Changes)
	}
}

func TestReconcileConfigMismatchCreatesUpdateAction(t *testing.T) {
	desired := Composition{
		Version: 2,
		Sprites: []SpriteSpec{{Name: "bramble", Persona: sprite.Persona{Name: "bramble"}}},
	}
	actual := []SpriteStatus{{Name: "bramble", Persona: "bramble", ConfigVersion: "1", State: sprite.StateIdle}}

	actions := Reconcile(desired, actual)
	if len(actions) != 1 {
		t.Fatalf("len(actions) = %d, want 1", len(actions))
	}
	update, ok := actions[0].(*UpdateAction)
	if !ok {
		t.Fatalf("actions[0] type = %T, want *UpdateAction", actions[0])
	}
	if !containsChange(update.Changes, `config "1" -> "2"`) {
		t.Fatalf("update.Changes = %v, expected config drift", update.Changes)
	}
}

func TestReconcileActiveDriftCreatesUpdateAndRedispatch(t *testing.T) {
	desired := Composition{
		Version: 2,
		Sprites: []SpriteSpec{{Name: "bramble", Persona: sprite.Persona{Name: "bramble"}}},
	}
	actual := []SpriteStatus{{Name: "bramble", Persona: "thorn", ConfigVersion: "1", State: sprite.StateWorking}}

	actions := Reconcile(desired, actual)
	if len(actions) != 2 {
		t.Fatalf("len(actions) = %d, want 2", len(actions))
	}

	kinds := []ActionKind{actions[0].Kind(), actions[1].Kind()}
	want := []ActionKind{ActionUpdate, ActionRedispatch}
	if !reflect.DeepEqual(kinds, want) {
		t.Fatalf("action kinds = %v, want %v", kinds, want)
	}
}

func TestBuildPlanIncludesMissingExtraAndDrift(t *testing.T) {
	desired := Composition{
		Version: 1,
		Sprites: []SpriteSpec{
			{Name: "bramble", Persona: sprite.Persona{Name: "bramble"}},
			{Name: "fern", Persona: sprite.Persona{Name: "fern"}},
		},
	}
	actual := []SpriteStatus{
		{Name: "bramble", Persona: "thorn", ConfigVersion: "1", State: sprite.StateBlocked},
		{Name: "moss", Persona: "moss", ConfigVersion: "1", State: sprite.StateIdle},
	}

	plan := BuildPlan(desired, actual)
	if len(plan.Missing) != 1 || plan.Missing[0].Name != "fern" {
		t.Fatalf("plan.Missing = %v", plan.Missing)
	}
	if len(plan.Extra) != 1 || plan.Extra[0].Name != "moss" {
		t.Fatalf("plan.Extra = %v", plan.Extra)
	}
	if len(plan.Drift) != 1 || plan.Drift[0].Name != "bramble" {
		t.Fatalf("plan.Drift = %v", plan.Drift)
	}
}

func TestBuildPlanTreatsDuplicateActualAsTeardown(t *testing.T) {
	desired := Composition{
		Version: 1,
		Sprites: []SpriteSpec{{Name: "bramble", Persona: sprite.Persona{Name: "bramble"}}},
	}
	actual := []SpriteStatus{
		{Name: "bramble", Persona: "bramble", ConfigVersion: "1", MachineID: "m1"},
		{Name: "bramble", Persona: "bramble", ConfigVersion: "1", MachineID: "m2"},
	}

	plan := BuildPlan(desired, actual)
	if len(plan.Actions) != 1 {
		t.Fatalf("len(plan.Actions) = %d, want 1", len(plan.Actions))
	}
	teardown, ok := plan.Actions[0].(*TeardownAction)
	if !ok {
		t.Fatalf("plan.Actions[0] type = %T, want *TeardownAction", plan.Actions[0])
	}
	if teardown.MachineID != "m2" {
		t.Fatalf("teardown.MachineID = %q, want m2", teardown.MachineID)
	}
}

func containsChange(changes []string, expected string) bool {
	for _, change := range changes {
		if change == expected {
			return true
		}
	}
	return false
}
