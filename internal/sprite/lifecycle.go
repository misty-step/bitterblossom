package sprite

import (
	"errors"
	"fmt"
)

var (
	// ErrInvalidState indicates an unknown sprite state.
	ErrInvalidState = errors.New("sprite: invalid state")
	// ErrInvalidSignal indicates an unsupported signal value.
	ErrInvalidSignal = errors.New("sprite: invalid signal")
	// ErrInvalidTransition indicates a disallowed state transition.
	ErrInvalidTransition = errors.New("sprite: invalid transition")
	// ErrNotProvisioned indicates an operation that requires a provisioned sprite.
	ErrNotProvisioned = errors.New("sprite: sprite is not provisioned")
	// ErrBusy indicates dispatch rejected because a sprite is already working.
	ErrBusy = errors.New("sprite: sprite is busy")
)

type dispatchDecision int

const (
	dispatchStart dispatchDecision = iota
	dispatchQueue
)

type lifecycle struct {
	busyStrategy BusyStrategy
}

func newLifecycle(strategy BusyStrategy) (lifecycle, error) {
	if !strategy.Valid() {
		return lifecycle{}, fmt.Errorf("sprite: unsupported busy strategy %q", strategy)
	}
	return lifecycle{busyStrategy: strategy}, nil
}

func (l lifecycle) dispatchDecision(state State) (dispatchDecision, error) {
	switch state {
	case StateIdle, StateDone:
		return dispatchStart, nil
	case StateWorking:
		if l.busyStrategy == BusyQueue {
			return dispatchQueue, nil
		}
		return 0, ErrBusy
	case StateBlocked:
		return 0, fmt.Errorf("%w: blocked sprites must receive %q first", ErrInvalidTransition, SignalRetry)
	case StateProvisioned:
		return 0, fmt.Errorf("%w: bootstrap signal %q required", ErrInvalidTransition, SignalReady)
	case StateDead:
		return 0, fmt.Errorf("%w: dead sprites must be provisioned", ErrInvalidTransition)
	default:
		return 0, fmt.Errorf("%w: %q", ErrInvalidState, state)
	}
}

func (l lifecycle) signalTransition(state State, signal Signal) (State, error) {
	transitions, ok := signalTransitions[signal]
	if !ok {
		return state, fmt.Errorf("%w: %q", ErrInvalidSignal, signal)
	}
	next, ok := transitions[state]
	if !ok {
		return state, fmt.Errorf("%w: cannot apply %q to %q", ErrInvalidTransition, signal, state)
	}
	return next, nil
}

var signalTransitions = map[Signal]map[State]State{
	SignalReady: {
		StateProvisioned: StateIdle,
		StateIdle:        StateIdle,
	},
	SignalDone: {
		StateWorking: StateDone,
		StateDone:    StateDone,
	},
	SignalBlocked: {
		StateWorking: StateBlocked,
		StateBlocked: StateBlocked,
	},
	SignalDead: {
		StateProvisioned: StateDead,
		StateIdle:        StateDead,
		StateWorking:     StateDead,
		StateDone:        StateDead,
		StateBlocked:     StateDead,
		StateDead:        StateDead,
	},
	SignalRetry: {
		StateDone:    StateIdle,
		StateBlocked: StateIdle,
		StateIdle:    StateIdle,
	},
}

var allowedTransitions = map[State]map[State]struct{}{
	StateProvisioned: {
		StateIdle: {},
		StateDead: {},
	},
	StateIdle: {
		StateWorking: {},
		StateDead:    {},
	},
	StateWorking: {
		StateDone:    {},
		StateBlocked: {},
		StateDead:    {},
	},
	StateDone: {
		StateIdle: {},
		StateDead: {},
	},
	StateBlocked: {
		StateIdle: {},
		StateDead: {},
	},
	StateDead: {
		StateProvisioned: {},
	},
}

func validateTransition(from, to State) error {
	if !from.Valid() {
		return fmt.Errorf("%w: %q", ErrInvalidState, from)
	}
	if !to.Valid() {
		return fmt.Errorf("%w: %q", ErrInvalidState, to)
	}
	if from == to {
		return nil
	}
	nexts, ok := allowedTransitions[from]
	if !ok {
		return fmt.Errorf("%w: %q", ErrInvalidState, from)
	}
	if _, ok := nexts[to]; !ok {
		return fmt.Errorf("%w: %q -> %q", ErrInvalidTransition, from, to)
	}
	return nil
}
