package fleet

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/misty-step/bitterblossom/internal/registry"
)

type fakeStatusChecker struct {
	statusByMachine map[string]LiveStatus
	errByMachine    map[string]error
}

func (f fakeStatusChecker) Check(_ context.Context, machineID string) (LiveStatus, error) {
	if f.statusByMachine != nil {
		if s, ok := f.statusByMachine[machineID]; ok {
			return s, f.errByMachine[machineID]
		}
	}
	return LiveStatus{State: "unknown"}, f.errByMachine[machineID]
}

func writeRegistry(t *testing.T, sprites map[string]string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "registry.toml")
	reg := &registry.Registry{Sprites: make(map[string]registry.SpriteEntry, len(sprites))}
	now := time.Date(2026, time.February, 10, 12, 0, 0, 0, time.UTC)
	for name, id := range sprites {
		reg.Sprites[name] = registry.SpriteEntry{
			MachineID: id,
			CreatedAt: now,
		}
	}
	if err := reg.Save(path); err != nil {
		t.Fatalf("reg.Save() error = %v", err)
	}
	return path
}

func TestDispatch_AutoAssignSkipsBusy(t *testing.T) {
	t.Parallel()

	regPath := writeRegistry(t, map[string]string{
		"bramble": "m-1",
		"fern":    "m-2",
	})
	checker := fakeStatusChecker{
		statusByMachine: map[string]LiveStatus{
			"m-1": {State: "running", Task: "issue #98", Runtime: "12m"},
			"m-2": {State: "idle"},
		},
	}

	f, err := NewDispatchFleet(DispatchConfig{
		RegistryPath: regPath,
		Status:       checker,
		Now:          func() time.Time { return time.Date(2026, time.February, 10, 12, 1, 0, 0, time.UTC) },
	})
	if err != nil {
		t.Fatalf("NewDispatchFleet() error = %v", err)
	}

	assignment, err := f.Dispatch(context.Background(), DispatchRequest{Issue: 186, Repo: "misty-step/bitterblossom"})
	if err != nil {
		t.Fatalf("Dispatch() error = %v", err)
	}
	if assignment.Sprite != "fern" {
		t.Fatalf("assignment.Sprite = %q, want fern", assignment.Sprite)
	}
	if assignment.MachineID != "m-2" {
		t.Fatalf("assignment.MachineID = %q, want m-2", assignment.MachineID)
	}

	reg, loadErr := registry.Load(regPath)
	if loadErr != nil {
		t.Fatalf("registry.Load() error = %v", loadErr)
	}
	entry := reg.Sprites["fern"]
	if entry.AssignedIssue != 186 {
		t.Fatalf("registry assigned_issue = %d, want 186", entry.AssignedIssue)
	}
	if entry.AssignedRepo != "misty-step/bitterblossom" {
		t.Fatalf("registry assigned_repo = %q, want misty-step/bitterblossom", entry.AssignedRepo)
	}
	if entry.AssignedAt.IsZero() {
		t.Fatalf("registry assigned_at is zero, want set")
	}
}

func TestDispatch_AllBusyReturnsFleetBusyError(t *testing.T) {
	t.Parallel()

	regPath := writeRegistry(t, map[string]string{
		"bramble": "m-1",
		"fern":    "m-2",
	})
	checker := fakeStatusChecker{
		statusByMachine: map[string]LiveStatus{
			"m-1": {State: "running", Task: "A"},
			"m-2": {State: "blocked", Task: "B"},
		},
	}

	f, err := NewDispatchFleet(DispatchConfig{
		RegistryPath: regPath,
		Status:       checker,
	})
	if err != nil {
		t.Fatalf("NewDispatchFleet() error = %v", err)
	}

	_, dispatchErr := f.Dispatch(context.Background(), DispatchRequest{Issue: 186, Repo: "misty-step/bitterblossom"})
	if dispatchErr == nil {
		t.Fatalf("expected error, got nil")
	}
	var busyErr *FleetBusyError
	if !errors.As(dispatchErr, &busyErr) {
		t.Fatalf("expected FleetBusyError, got %T: %v", dispatchErr, dispatchErr)
	}
	if len(busyErr.Sprites) != 2 {
		t.Fatalf("busy sprites = %d, want 2", len(busyErr.Sprites))
	}
}

func TestDispatch_ConcurrentDoesNotDoubleAssign(t *testing.T) {
	t.Parallel()

	regPath := writeRegistry(t, map[string]string{
		"bramble": "m-1",
		"fern":    "m-2",
	})
	checker := fakeStatusChecker{
		statusByMachine: map[string]LiveStatus{
			"m-1": {State: "idle"},
			"m-2": {State: "idle"},
		},
	}

	f, err := NewDispatchFleet(DispatchConfig{
		RegistryPath:   regPath,
		Status:         checker,
		ReservationTTL: 10 * time.Minute,
		Now:            func() time.Time { return time.Date(2026, time.February, 10, 12, 2, 0, 0, time.UTC) },
	})
	if err != nil {
		t.Fatalf("NewDispatchFleet() error = %v", err)
	}

	var wg sync.WaitGroup
	wg.Add(2)

	assignments := make(chan *Assignment, 2)
	errs := make(chan error, 2)

	go func() {
		defer wg.Done()
		a, e := f.Dispatch(context.Background(), DispatchRequest{Issue: 1, Repo: "x/y"})
		assignments <- a
		errs <- e
	}()
	go func() {
		defer wg.Done()
		a, e := f.Dispatch(context.Background(), DispatchRequest{Issue: 2, Repo: "x/y"})
		assignments <- a
		errs <- e
	}()

	wg.Wait()
	close(assignments)
	close(errs)

	var got []*Assignment
	for e := range errs {
		if e != nil {
			t.Fatalf("Dispatch() error = %v", e)
		}
	}
	for a := range assignments {
		got = append(got, a)
	}
	if len(got) != 2 {
		t.Fatalf("assignments = %d, want 2", len(got))
	}
	if got[0].Sprite == got[1].Sprite {
		t.Fatalf("expected distinct sprite assignments, got %q and %q", got[0].Sprite, got[1].Sprite)
	}
}

// racingStatusChecker returns different results on successive calls for the
// same machine, simulating a sprite becoming busy between check and reserve.
type racingStatusChecker struct {
	mu       sync.Mutex
	calls    map[string]int
	sequence map[string][]LiveStatus
}

func (r *racingStatusChecker) Check(_ context.Context, machineID string) (LiveStatus, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.calls == nil {
		r.calls = make(map[string]int)
	}
	idx := r.calls[machineID]
	r.calls[machineID]++
	seq := r.sequence[machineID]
	if idx < len(seq) {
		return seq[idx], nil
	}
	return seq[len(seq)-1], nil
}

func TestDispatch_ReserveThenRecheckRace(t *testing.T) {
	t.Parallel()

	regPath := writeRegistry(t, map[string]string{
		"bramble": "m-1",
		"fern":    "m-2",
	})

	// bramble: first check idle, recheck after reserve returns running (race)
	// fern: always idle
	checker := &racingStatusChecker{
		sequence: map[string][]LiveStatus{
			"m-1": {{State: "idle"}, {State: "running", Task: "issue #99"}},
			"m-2": {{State: "idle"}, {State: "idle"}},
		},
	}

	f, err := NewDispatchFleet(DispatchConfig{
		RegistryPath: regPath,
		Status:       checker,
		Now:          func() time.Time { return time.Date(2026, time.February, 10, 12, 5, 0, 0, time.UTC) },
	})
	if err != nil {
		t.Fatalf("NewDispatchFleet() error = %v", err)
	}

	assignment, err := f.Dispatch(context.Background(), DispatchRequest{Issue: 300, Repo: "x/y"})
	if err != nil {
		t.Fatalf("Dispatch() error = %v", err)
	}

	// Should skip bramble (became busy after reserve) and assign fern instead.
	if assignment.Sprite != "fern" {
		t.Fatalf("assignment.Sprite = %q, want fern (bramble should have been unreserved)", assignment.Sprite)
	}

	// Verify bramble's reservation was cleared.
	reg, loadErr := registry.Load(regPath)
	if loadErr != nil {
		t.Fatalf("registry.Load() error = %v", loadErr)
	}
	bramble := reg.Sprites["bramble"]
	if bramble.AssignedIssue != 0 || !bramble.AssignedAt.IsZero() {
		t.Fatalf("bramble should be unreserved; got issue=%d at=%v", bramble.AssignedIssue, bramble.AssignedAt)
	}

	// Verify fern is properly reserved.
	fern := reg.Sprites["fern"]
	if fern.AssignedIssue != 300 {
		t.Fatalf("fern.AssignedIssue = %d, want 300", fern.AssignedIssue)
	}
}

func TestDispatch_ExplicitSpriteRecheckFails(t *testing.T) {
	t.Parallel()

	regPath := writeRegistry(t, map[string]string{
		"bramble": "m-1",
	})

	// bramble: idle on first check, running on recheck
	checker := &racingStatusChecker{
		sequence: map[string][]LiveStatus{
			"m-1": {{State: "idle"}, {State: "running", Task: "issue #42"}},
		},
	}

	f, err := NewDispatchFleet(DispatchConfig{
		RegistryPath: regPath,
		Status:       checker,
		Now:          func() time.Time { return time.Date(2026, time.February, 10, 12, 6, 0, 0, time.UTC) },
	})
	if err != nil {
		t.Fatalf("NewDispatchFleet() error = %v", err)
	}

	_, dispatchErr := f.Dispatch(context.Background(), DispatchRequest{
		Sprite: "bramble",
		Issue:  500,
		Repo:   "x/y",
	})
	if dispatchErr == nil {
		t.Fatal("expected FleetBusyError when explicit sprite fails recheck")
	}
	var busyErr *FleetBusyError
	if !errors.As(dispatchErr, &busyErr) {
		t.Fatalf("expected FleetBusyError, got %T: %v", dispatchErr, dispatchErr)
	}

	// Verify reservation was cleared.
	reg, loadErr := registry.Load(regPath)
	if loadErr != nil {
		t.Fatalf("registry.Load() error = %v", loadErr)
	}
	bramble := reg.Sprites["bramble"]
	if bramble.AssignedIssue != 0 {
		t.Fatalf("bramble should be unreserved after recheck failure; got issue=%d", bramble.AssignedIssue)
	}
}

func TestPlanDispatch_DoesNotMutateRegistry(t *testing.T) {
	t.Parallel()

	regPath := writeRegistry(t, map[string]string{
		"bramble": "m-1",
	})
	checker := fakeStatusChecker{
		statusByMachine: map[string]LiveStatus{
			"m-1": {State: "idle"},
		},
	}

	f, err := NewDispatchFleet(DispatchConfig{
		RegistryPath: regPath,
		Status:       checker,
		Now:          func() time.Time { return time.Date(2026, time.February, 10, 12, 3, 0, 0, time.UTC) },
	})
	if err != nil {
		t.Fatalf("NewDispatchFleet() error = %v", err)
	}

	a, err := f.PlanDispatch(context.Background(), DispatchRequest{Issue: 186, Repo: "misty-step/bitterblossom"})
	if err != nil {
		t.Fatalf("PlanDispatch() error = %v", err)
	}
	if a.Sprite != "bramble" {
		t.Fatalf("assignment.Sprite = %q, want bramble", a.Sprite)
	}

	reg, loadErr := registry.Load(regPath)
	if loadErr != nil {
		t.Fatalf("registry.Load() error = %v", loadErr)
	}
	entry := reg.Sprites["bramble"]
	if entry.AssignedIssue != 0 || entry.AssignedRepo != "" || !entry.AssignedAt.IsZero() {
		t.Fatalf("expected plan to leave registry unmodified; got issue=%d repo=%q at=%v", entry.AssignedIssue, entry.AssignedRepo, entry.AssignedAt)
	}
}
