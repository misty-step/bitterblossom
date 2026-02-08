package dispatch

import "fmt"

// DispatchState captures the dispatch lifecycle.
type DispatchState string

const (
	StatePending        DispatchState = "pending"
	StateProvisioning   DispatchState = "provisioning"
	StateReady          DispatchState = "ready"
	StatePromptUploaded DispatchState = "prompt_uploaded"
	StateRunning        DispatchState = "running"
	StateCompleted      DispatchState = "completed"
	StateFailed         DispatchState = "failed"
)

// DispatchEvent is consumed by the state machine.
type DispatchEvent string

const (
	EventProvisionRequired  DispatchEvent = "provision_required"
	EventProvisionSucceeded DispatchEvent = "provision_succeeded"
	EventMachineReady       DispatchEvent = "machine_ready"
	EventPromptUploaded     DispatchEvent = "prompt_uploaded"
	EventAgentStarted       DispatchEvent = "agent_started"
	EventOneShotComplete    DispatchEvent = "oneshot_complete"
	EventFailure            DispatchEvent = "failure"
)

var stateTransitions = map[DispatchState]map[DispatchEvent]DispatchState{
	StatePending: {
		EventProvisionRequired: StateProvisioning,
		EventMachineReady:      StateReady,
		EventFailure:           StateFailed,
	},
	StateProvisioning: {
		EventProvisionSucceeded: StateReady,
		EventFailure:            StateFailed,
	},
	StateReady: {
		EventPromptUploaded: StatePromptUploaded,
		EventFailure:        StateFailed,
	},
	StatePromptUploaded: {
		EventAgentStarted: StateRunning,
		EventFailure:      StateFailed,
	},
	StateRunning: {
		EventOneShotComplete: StateCompleted,
		EventFailure:         StateFailed,
	},
	StateCompleted: {
		EventFailure: StateFailed,
	},
	StateFailed: {
		EventFailure: StateFailed,
	},
}

func advanceState(current DispatchState, event DispatchEvent) (DispatchState, error) {
	nextByEvent, ok := stateTransitions[current]
	if !ok {
		return current, fmt.Errorf("unknown state %q", current)
	}
	next, ok := nextByEvent[event]
	if !ok {
		return current, fmt.Errorf("state %q does not allow event %q", current, event)
	}
	return next, nil
}
