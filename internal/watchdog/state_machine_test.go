package watchdog

import (
	"testing"
	"time"
)

func TestEvaluateStateTable(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		input stateInput
		want  State
	}{
		{
			name: "complete wins",
			input: stateInput{
				HasComplete: true,
				HasTask:     true,
			},
			want: StateComplete,
		},
		{
			name: "blocked wins",
			input: stateInput{
				HasBlocked: true,
				HasTask:    true,
			},
			want: StateBlocked,
		},
		{
			name: "dead when not running with task",
			input: stateInput{
				AgentRunning: false,
				HasTask:      true,
			},
			want: StateDead,
		},
		{
			name: "idle when not running and no task",
			input: stateInput{
				AgentRunning: false,
				HasTask:      false,
			},
			want: StateIdle,
		},
		{
			name: "stale when running too long without commits",
			input: stateInput{
				AgentRunning:  true,
				HasTask:       true,
				Elapsed:       3 * time.Hour,
				CommitsLast2h: 0,
			},
			want: StateStale,
		},
		{
			name: "active when running with commits",
			input: stateInput{
				AgentRunning:  true,
				HasTask:       true,
				Elapsed:       3 * time.Hour,
				CommitsLast2h: 2,
			},
			want: StateActive,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := evaluateState(tc.input, 2*time.Hour)
			if got != tc.want {
				t.Fatalf("evaluateState(%+v) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestDecideAction(t *testing.T) {
	t.Parallel()

	cases := []struct {
		state     State
		hasPrompt bool
		want      ActionType
	}{
		{state: StateActive, hasPrompt: true, want: ActionNone},
		{state: StateStale, hasPrompt: true, want: ActionInvestigate},
		{state: StateDead, hasPrompt: true, want: ActionRedispatch},
		{state: StateDead, hasPrompt: false, want: ActionManualAction},
	}

	for _, tc := range cases {
		got := decideAction(tc.state, tc.hasPrompt)
		if got != tc.want {
			t.Fatalf("decideAction(%q, %v) = %q, want %q", tc.state, tc.hasPrompt, got, tc.want)
		}
	}
}
