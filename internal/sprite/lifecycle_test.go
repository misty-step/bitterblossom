package sprite

import (
	"errors"
	"testing"
)

func TestValidateTransition(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		from    State
		to      State
		wantErr error
	}{
		{name: "provisioned to idle", from: StateProvisioned, to: StateIdle},
		{name: "idle to working", from: StateIdle, to: StateWorking},
		{name: "working to done", from: StateWorking, to: StateDone},
		{name: "working to blocked", from: StateWorking, to: StateBlocked},
		{name: "any to dead", from: StateDone, to: StateDead},
		{name: "dead to provisioned", from: StateDead, to: StateProvisioned},
		{name: "invalid", from: StateIdle, to: StateProvisioned, wantErr: ErrInvalidTransition},
		{name: "invalid state", from: State("x"), to: StateIdle, wantErr: ErrInvalidState},
		{name: "invalid target", from: StateIdle, to: State("x"), wantErr: ErrInvalidState},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := validateTransition(tc.from, tc.to)
			if tc.wantErr == nil && err != nil {
				t.Fatalf("expected no error, got %v", err)
			}
			if tc.wantErr != nil && !errors.Is(err, tc.wantErr) {
				t.Fatalf("expected %v, got %v", tc.wantErr, err)
			}
		})
	}
}

func TestLifecycleDispatchDecision(t *testing.T) {
	t.Parallel()

	queueLifecycle, err := newLifecycle(BusyQueue)
	if err != nil {
		t.Fatalf("newLifecycle(queue): %v", err)
	}
	errorLifecycle, err := newLifecycle(BusyError)
	if err != nil {
		t.Fatalf("newLifecycle(error): %v", err)
	}

	decision, err := queueLifecycle.dispatchDecision(StateWorking)
	if err != nil {
		t.Fatalf("queue strategy should not error: %v", err)
	}
	if decision != dispatchQueue {
		t.Fatalf("expected queue decision, got %v", decision)
	}

	_, err = errorLifecycle.dispatchDecision(StateWorking)
	if !errors.Is(err, ErrBusy) {
		t.Fatalf("expected ErrBusy, got %v", err)
	}

	_, err = queueLifecycle.dispatchDecision(StateProvisioned)
	if !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("expected ErrInvalidTransition, got %v", err)
	}

	_, err = queueLifecycle.dispatchDecision(StateDead)
	if !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("expected ErrInvalidTransition, got %v", err)
	}

	_, err = queueLifecycle.dispatchDecision(State("bogus"))
	if !errors.Is(err, ErrInvalidState) {
		t.Fatalf("expected ErrInvalidState, got %v", err)
	}
}

func TestNewLifecycleRejectsInvalidStrategy(t *testing.T) {
	t.Parallel()

	_, err := newLifecycle(BusyStrategy("bogus"))
	if err == nil {
		t.Fatal("expected invalid strategy error")
	}
}

func TestLifecycleSignalTransition(t *testing.T) {
	t.Parallel()

	lc, err := newLifecycle(BusyQueue)
	if err != nil {
		t.Fatalf("newLifecycle: %v", err)
	}

	next, err := lc.signalTransition(StateWorking, SignalDone)
	if err != nil {
		t.Fatalf("signalTransition: %v", err)
	}
	if next != StateDone {
		t.Fatalf("expected done, got %s", next)
	}

	_, err = lc.signalTransition(StateIdle, SignalDone)
	if !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("expected ErrInvalidTransition, got %v", err)
	}

	_, err = lc.signalTransition(StateIdle, Signal("unknown"))
	if !errors.Is(err, ErrInvalidSignal) {
		t.Fatalf("expected ErrInvalidSignal, got %v", err)
	}
}
