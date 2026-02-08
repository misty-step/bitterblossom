package dispatch

import "testing"

func TestAdvanceStateTable(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		current DispatchState
		event   DispatchEvent
		want    DispatchState
		wantErr bool
	}{
		{
			name:    "pending to provisioning",
			current: StatePending,
			event:   EventProvisionRequired,
			want:    StateProvisioning,
		},
		{
			name:    "pending to ready",
			current: StatePending,
			event:   EventMachineReady,
			want:    StateReady,
		},
		{
			name:    "provisioning to ready",
			current: StateProvisioning,
			event:   EventProvisionSucceeded,
			want:    StateReady,
		},
		{
			name:    "ready to prompt uploaded",
			current: StateReady,
			event:   EventPromptUploaded,
			want:    StatePromptUploaded,
		},
		{
			name:    "prompt uploaded to running",
			current: StatePromptUploaded,
			event:   EventAgentStarted,
			want:    StateRunning,
		},
		{
			name:    "running to completed",
			current: StateRunning,
			event:   EventOneShotComplete,
			want:    StateCompleted,
		},
		{
			name:    "invalid transition",
			current: StateReady,
			event:   EventOneShotComplete,
			wantErr: true,
		},
		{
			name:    "failure from running",
			current: StateRunning,
			event:   EventFailure,
			want:    StateFailed,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := advanceState(tc.current, tc.event)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("advanceState(%q,%q) expected error", tc.current, tc.event)
				}
				return
			}
			if err != nil {
				t.Fatalf("advanceState(%q,%q) error = %v", tc.current, tc.event, err)
			}
			if got != tc.want {
				t.Fatalf("advanceState(%q,%q) = %q, want %q", tc.current, tc.event, got, tc.want)
			}
		})
	}
}
