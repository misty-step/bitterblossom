package watchdog

import "time"

// State describes fleet watchdog outcomes for one sprite.
type State string

const (
	StateActive   State = "active"
	StateIdle     State = "idle"
	StateComplete State = "complete"
	StateBlocked  State = "blocked"
	StateDead     State = "dead"
	StateStale    State = "stale"
	StateError    State = "error"
)

type stateInput struct {
	AgentRunning  bool
	HasComplete   bool
	HasBlocked    bool
	HasTask       bool
	Elapsed       time.Duration
	CommitsLast2h int
}

func evaluateState(input stateInput, staleAfter time.Duration) State {
	switch {
	case input.HasComplete:
		return StateComplete
	case input.HasBlocked:
		return StateBlocked
	case !input.AgentRunning:
		if input.HasTask {
			return StateDead
		}
		return StateIdle
	case staleAfter > 0 && input.Elapsed >= staleAfter && input.CommitsLast2h == 0:
		return StateStale
	default:
		return StateActive
	}
}

// ActionType is a watchdog action to take after classification.
type ActionType string

const (
	ActionNone         ActionType = ""
	ActionRedispatch   ActionType = "redispatch"
	ActionInvestigate  ActionType = "investigate"
	ActionManualAction ActionType = "manual_dispatch"
)

func decideAction(state State, hasPrompt bool) ActionType {
	switch state {
	case StateDead:
		if hasPrompt {
			return ActionRedispatch
		}
		return ActionManualAction
	case StateStale:
		return ActionInvestigate
	default:
		return ActionNone
	}
}
