package sprite

import (
	"errors"
	"testing"
)

type runtimeStub struct {
	ensureCalls   int
	dispatchCalls []Task
	teardownCalls int

	ensureErr   error
	dispatchErr error
	teardownErr error
}

func (r *runtimeStub) EnsureProvisioned(string, Persona) error {
	r.ensureCalls++
	return r.ensureErr
}

func (r *runtimeStub) Dispatch(_ string, task Task) error {
	if r.dispatchErr != nil {
		return r.dispatchErr
	}
	r.dispatchCalls = append(r.dispatchCalls, task)
	return nil
}

func (r *runtimeStub) Teardown(string) error {
	r.teardownCalls++
	return r.teardownErr
}

func TestNewSpriteValidation(t *testing.T) {
	t.Parallel()

	_, err := New("", Persona{})
	if err == nil {
		t.Fatal("expected error for empty name")
	}

	_, err = New("bramble", Persona{}, WithBusyStrategy(BusyStrategy("invalid")))
	if err == nil {
		t.Fatal("expected error for invalid strategy")
	}

	_, err = New("bramble", Persona{}, WithRuntime(nil))
	if err == nil {
		t.Fatal("expected error for nil runtime")
	}

	_, err = New("bramble", Persona{}, WithInitialState(State("unknown"), true))
	if err == nil {
		t.Fatal("expected error for invalid initial state")
	}
}

func TestProvisionIsIdempotent(t *testing.T) {
	t.Parallel()

	runtime := &runtimeStub{}
	s, err := New("bramble", Persona{Name: "bramble"}, WithRuntime(runtime))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	if err := s.Provision(); err != nil {
		t.Fatalf("Provision #1: %v", err)
	}
	if err := s.Provision(); err != nil {
		t.Fatalf("Provision #2: %v", err)
	}

	if runtime.ensureCalls != 2 {
		t.Fatalf("expected 2 ensure calls, got %d", runtime.ensureCalls)
	}
	if s.State() != StateProvisioned {
		t.Fatalf("expected state provisioned, got %s", s.State())
	}
}

func TestTeardownMissingSpriteIsSuccess(t *testing.T) {
	t.Parallel()

	runtime := &runtimeStub{}
	s, err := New("bramble", Persona{Name: "bramble"}, WithRuntime(runtime))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	if err := s.Teardown(); err != nil {
		t.Fatalf("Teardown: %v", err)
	}
	if runtime.teardownCalls != 0 {
		t.Fatalf("expected no teardown call, got %d", runtime.teardownCalls)
	}
}

func TestDefaultRuntimeLifecycle(t *testing.T) {
	t.Parallel()

	s, err := New("sage", Persona{Name: "sage"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if s.Name() != "sage" {
		t.Fatalf("expected name sage, got %s", s.Name())
	}
	if s.Persona().Name != "sage" {
		t.Fatalf("expected persona sage, got %s", s.Persona().Name)
	}

	if err := s.Provision(); err != nil {
		t.Fatalf("Provision: %v", err)
	}
	if err := s.Signal(SignalReady); err != nil {
		t.Fatalf("Signal ready: %v", err)
	}
	if _, err := s.Dispatch(Task{Description: "document architecture"}); err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if err := s.Signal(SignalDone); err != nil {
		t.Fatalf("Signal done: %v", err)
	}
	if err := s.Signal(SignalRetry); err != nil {
		t.Fatalf("Signal retry: %v", err)
	}
	if err := s.Teardown(); err != nil {
		t.Fatalf("Teardown: %v", err)
	}
}

func TestDispatchBusyQueueStrategy(t *testing.T) {
	t.Parallel()

	runtime := &runtimeStub{}
	s, err := New("bramble", Persona{Name: "bramble"}, WithRuntime(runtime), WithBusyStrategy(BusyQueue))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := s.Provision(); err != nil {
		t.Fatalf("Provision: %v", err)
	}
	if err := s.Signal(SignalReady); err != nil {
		t.Fatalf("Signal ready: %v", err)
	}

	task1 := Task{Description: "one"}
	task2 := Task{Description: "two"}

	result, err := s.Dispatch(task1)
	if err != nil {
		t.Fatalf("Dispatch #1: %v", err)
	}
	if result.Status != DispatchStarted {
		t.Fatalf("expected started, got %s", result.Status)
	}

	result, err = s.Dispatch(task2)
	if err != nil {
		t.Fatalf("Dispatch #2: %v", err)
	}
	if result.Status != DispatchQueued {
		t.Fatalf("expected queued, got %s", result.Status)
	}
	if result.QueueDepth != 1 {
		t.Fatalf("expected queue depth 1, got %d", result.QueueDepth)
	}
}

func TestDispatchDispatchErrorDoesNotChangeState(t *testing.T) {
	t.Parallel()

	runtime := &runtimeStub{dispatchErr: errors.New("boom")}
	s, err := New("thorn", Persona{Name: "thorn"}, WithRuntime(runtime))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := s.Provision(); err != nil {
		t.Fatalf("Provision: %v", err)
	}
	if err := s.Signal(SignalReady); err != nil {
		t.Fatalf("Signal ready: %v", err)
	}

	_, err = s.Dispatch(Task{Description: "task 1"})
	if err == nil {
		t.Fatalf("expected dispatch error")
	}
	if s.State() != StateIdle {
		t.Fatalf("expected state idle after dispatch error, got %s", s.State())
	}
}

func TestDispatchBusyErrorStrategy(t *testing.T) {
	t.Parallel()

	runtime := &runtimeStub{}
	s, err := New("thorn", Persona{Name: "thorn"}, WithRuntime(runtime), WithBusyStrategy(BusyError))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := s.Provision(); err != nil {
		t.Fatalf("Provision: %v", err)
	}
	if err := s.Signal(SignalReady); err != nil {
		t.Fatalf("Signal ready: %v", err)
	}

	if _, err := s.Dispatch(Task{Description: "task 1"}); err != nil {
		t.Fatalf("Dispatch #1: %v", err)
	}
	if _, err := s.Dispatch(Task{Description: "task 2"}); !errors.Is(err, ErrBusy) {
		t.Fatalf("expected ErrBusy, got %v", err)
	}
}

func TestSignalDoneDispatchesQueuedTask(t *testing.T) {
	t.Parallel()

	runtime := &runtimeStub{}
	s, err := New("fern", Persona{Name: "fern"}, WithRuntime(runtime))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := s.Provision(); err != nil {
		t.Fatalf("Provision: %v", err)
	}
	if err := s.Signal(SignalReady); err != nil {
		t.Fatalf("Signal ready: %v", err)
	}

	if _, err := s.Dispatch(Task{Description: "first"}); err != nil {
		t.Fatalf("Dispatch #1: %v", err)
	}
	if _, err := s.Dispatch(Task{Description: "second"}); err != nil {
		t.Fatalf("Dispatch #2: %v", err)
	}
	if err := s.Signal(SignalDone); err != nil {
		t.Fatalf("Signal done: %v", err)
	}

	snap := s.Snapshot()
	if snap.State != StateWorking {
		t.Fatalf("expected working after draining queue, got %s", snap.State)
	}
	if snap.Pending != 0 {
		t.Fatalf("expected empty queue, got %d", snap.Pending)
	}
}

func TestBlockedSpriteRequiresRetryBeforeDispatch(t *testing.T) {
	t.Parallel()

	runtime := &runtimeStub{}
	s, err := New("hemlock", Persona{Name: "hemlock"}, WithRuntime(runtime))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := s.Provision(); err != nil {
		t.Fatalf("Provision: %v", err)
	}
	if err := s.Signal(SignalReady); err != nil {
		t.Fatalf("Signal ready: %v", err)
	}
	if _, err := s.Dispatch(Task{Description: "task"}); err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if err := s.Signal(SignalBlocked); err != nil {
		t.Fatalf("Signal blocked: %v", err)
	}
	if _, err := s.Dispatch(Task{Description: "new task"}); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("expected ErrInvalidTransition, got %v", err)
	}
	if err := s.Signal(SignalRetry); err != nil {
		t.Fatalf("Signal retry: %v", err)
	}
	if _, err := s.Dispatch(Task{Description: "new task"}); err != nil {
		t.Fatalf("Dispatch after retry: %v", err)
	}
}

func TestDispatchRequiresProvision(t *testing.T) {
	t.Parallel()

	s, err := New("moss", Persona{Name: "moss"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := s.Dispatch(Task{Description: "x"}); !errors.Is(err, ErrNotProvisioned) {
		t.Fatalf("expected ErrNotProvisioned, got %v", err)
	}
}

func TestSignalBeforeProvisionIsNoop(t *testing.T) {
	t.Parallel()

	s, err := New("rowan", Persona{Name: "rowan"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := s.Signal(SignalReady); err != nil {
		t.Fatalf("Signal before provision should be noop: %v", err)
	}
	if s.State() != StateDead {
		t.Fatalf("expected dead initial state, got %s", s.State())
	}
}

func TestTeardownResetsState(t *testing.T) {
	t.Parallel()

	runtime := &runtimeStub{}
	s, err := New("willow", Persona{Name: "willow"}, WithRuntime(runtime))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := s.Provision(); err != nil {
		t.Fatalf("Provision: %v", err)
	}
	if err := s.Signal(SignalReady); err != nil {
		t.Fatalf("Signal ready: %v", err)
	}
	if _, err := s.Dispatch(Task{Description: "task"}); err != nil {
		t.Fatalf("Dispatch: %v", err)
	}

	if err := s.Teardown(); err != nil {
		t.Fatalf("Teardown: %v", err)
	}

	snap := s.Snapshot()
	if snap.Provisioned {
		t.Fatalf("expected unprovisioned after teardown")
	}
	if snap.State != StateDead {
		t.Fatalf("expected dead state, got %s", snap.State)
	}
	if snap.Pending != 0 {
		t.Fatalf("expected empty queue, got %d", snap.Pending)
	}
	if runtime.teardownCalls != 1 {
		t.Fatalf("expected one teardown call, got %d", runtime.teardownCalls)
	}
}
