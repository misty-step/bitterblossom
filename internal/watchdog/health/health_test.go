package health

import (
	"testing"
	"time"
)

func TestEvaluate_ProcessDead(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		input Input
	}{
		{
			name: "no PID and no processes",
			input: Input{
				PIDExists:    false,
				ProcessCount: 0,
				HasTask:      true,
			},
		},
		{
			name: "no PID and zero process count",
			input: Input{
				PIDExists:    false,
				ProcessCount: 0,
				HasTask:      false,
			},
		},
	}

	cfg := DefaultConfig()

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			result := Evaluate(tc.input, cfg)

			if result.Status != StatusDead {
				t.Errorf("Status = %q, want %q", result.Status, StatusDead)
			}
			if result.ProcessAlive {
				t.Error("ProcessAlive = true, want false")
			}
			if result.Reason == "" {
				t.Error("Reason should not be empty")
			}
		})
	}
}

func TestEvaluate_ProcessAlive(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		input Input
	}{
		{
			name: "PID exists",
			input: Input{
				PIDExists:    true,
				ProcessCount: 0,
			},
		},
		{
			name: "process count > 0",
			input: Input{
				PIDExists:    false,
				ProcessCount: 1,
			},
		},
		{
			name: "both PID and process count",
			input: Input{
				PIDExists:    true,
				ProcessCount: 1,
			},
		},
	}

	cfg := DefaultConfig()

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			result := Evaluate(tc.input, cfg)

			if !result.ProcessAlive {
				t.Error("ProcessAlive = false, want true")
			}
			if result.Status == StatusDead {
				t.Errorf("Status = %q, should not be dead when process is alive", result.Status)
			}
		})
	}
}

func TestEvaluate_HealthyStates(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name         string
		input        Input
		wantStatus   Status
		wantReason   string
		wantResponsive bool
	}{
		{
			name: "task completed",
			input: Input{
				PIDExists:   true,
				HasComplete: true,
				HasTask:     true,
			},
			wantStatus:     StatusHealthy,
			wantReason:     "task completed",
			wantResponsive: true,
		},
		{
			name: "task blocked",
			input: Input{
				PIDExists:  true,
				HasBlocked: true,
				HasTask:    true,
			},
			wantStatus:     StatusHealthy,
			wantReason:     "task blocked (waiting for input)",
			wantResponsive: true,
		},
		{
			name: "no active task",
			input: Input{
				PIDExists: true,
				HasTask:   false,
			},
			wantStatus:     StatusHealthy,
			wantReason:     "no active task",
			wantResponsive: true,
		},
		{
			name: "active with recent commits",
			input: Input{
				PIDExists:     true,
				HasTask:       true,
				ElapsedTime:   30 * time.Minute,
				CommitsLast2h: 3,
			},
			wantStatus:     StatusHealthy,
			wantReason:     "3 commits in last 2h",
			wantResponsive: true,
		},
		{
			name: "active with dirty repos",
			input: Input{
				PIDExists:     true,
				HasTask:       true,
				ElapsedTime:   45 * time.Minute,
				CommitsLast2h: 0,
				DirtyRepos:    2,
			},
			wantStatus:     StatusHealthy,
			wantReason:     "active work in progress",
			wantResponsive: true,
		},
		{
			name: "active with ahead commits",
			input: Input{
				PIDExists:     true,
				HasTask:       true,
				ElapsedTime:   45 * time.Minute,
				CommitsLast2h: 0,
				AheadCommits:  5,
			},
			wantStatus:     StatusHealthy,
			wantReason:     "active work in progress",
			wantResponsive: true,
		},
		{
			name: "running normally under threshold",
			input: Input{
				PIDExists:     true,
				HasTask:       true,
				ElapsedTime:   20 * time.Minute,
				CommitsLast2h: 0,
			},
			wantStatus:     StatusHealthy,
			wantReason:     "process running normally",
			wantResponsive: true,
		},
	}

	cfg := DefaultConfig()

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			result := Evaluate(tc.input, cfg)

			if result.Status != tc.wantStatus {
				t.Errorf("Status = %q, want %q", result.Status, tc.wantStatus)
			}
			if result.Responsive != tc.wantResponsive {
				t.Errorf("Responsive = %v, want %v", result.Responsive, tc.wantResponsive)
			}
			if result.Reason != tc.wantReason {
				t.Errorf("Reason = %q, want %q", result.Reason, tc.wantReason)
			}
		})
	}
}

func TestEvaluate_ZombieDetection(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name       string
		input      Input
		cfg        Config
		wantStatus Status
		wantZombie bool
	}{
		{
			name: "stale - no commits beyond threshold",
			input: Input{
				PIDExists:     true,
				HasTask:       true,
				ElapsedTime:   3 * time.Hour,
				CommitsLast2h: 0,
			},
			cfg:        DefaultConfig(),
			wantStatus: StatusZombie,
			wantZombie: true,
		},
		{
			name: "not stale - has commits",
			input: Input{
				PIDExists:     true,
				HasTask:       true,
				ElapsedTime:   3 * time.Hour,
				CommitsLast2h: 2,
			},
			cfg:        DefaultConfig(),
			wantStatus: StatusHealthy,
			wantZombie: false,
		},
		{
			name: "no activity beyond min interval",
			input: Input{
				PIDExists:     true,
				HasTask:       true,
				ElapsedTime:   45 * time.Minute,
				CommitsLast2h: 0,
				DirtyRepos:    0,
				AheadCommits:  0,
			},
			cfg:        DefaultConfig(),
			wantStatus: StatusZombie,
			wantZombie: true,
		},
		{
			name: "within min activity interval",
			input: Input{
				PIDExists:     true,
				HasTask:       true,
				ElapsedTime:   15 * time.Minute,
				CommitsLast2h: 0,
				DirtyRepos:    0,
				AheadCommits:  0,
			},
			cfg:        DefaultConfig(),
			wantStatus: StatusHealthy,
			wantZombie: false,
		},
		{
			name: "custom short threshold",
			input: Input{
				PIDExists:     true,
				HasTask:       true,
				ElapsedTime:   35 * time.Minute,
				CommitsLast2h: 0,
			},
			cfg: Config{
				StaleThreshold:      30 * time.Minute,
				MinActivityInterval: 15 * time.Minute,
			},
			wantStatus: StatusZombie,
			wantZombie: true,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			result := Evaluate(tc.input, tc.cfg)

			if result.Status != tc.wantStatus {
				t.Errorf("Status = %q, want %q", result.Status, tc.wantStatus)
			}
			if result.IsZombie() != tc.wantZombie {
				t.Errorf("IsZombie() = %v, want %v", result.IsZombie(), tc.wantZombie)
			}
			if tc.wantZombie && result.Responsive {
				t.Error("Responsive = true for zombie process, want false")
			}
			if result.Reason == "" {
				t.Error("Reason should not be empty")
			}
		})
	}
}

func TestEvaluate_ProcessCount(t *testing.T) {
	t.Parallel()

	input := Input{
		ProcessCount: 3,
		PIDExists:    true,
		HasTask:      false,
	}

	result := Evaluate(input, DefaultConfig())

	if result.ProcessCount != 3 {
		t.Errorf("ProcessCount = %d, want 3", result.ProcessCount)
	}
}

func TestEvaluate_CommitsRecent(t *testing.T) {
	t.Parallel()

	input := Input{
		PIDExists:     true,
		HasTask:       true,
		CommitsLast2h: 5,
		ElapsedTime:   30 * time.Minute,
	}

	result := Evaluate(input, DefaultConfig())

	if result.CommitsRecent != 5 {
		t.Errorf("CommitsRecent = %d, want 5", result.CommitsRecent)
	}
}

func TestCheck_IsAlive(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name         string
		processAlive bool
		want         bool
	}{
		{"alive", true, true},
		{"dead", false, false},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			check := Check{ProcessAlive: tc.processAlive}
			if got := check.IsAlive(); got != tc.want {
				t.Errorf("IsAlive() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestCheck_NeedsIntervention(t *testing.T) {
	t.Parallel()

	cases := []struct {
		status Status
		want   bool
	}{
		{StatusHealthy, false},
		{StatusZombie, true},
		{StatusDead, true},
		{StatusUnknown, false},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(string(tc.status), func(t *testing.T) {
			t.Parallel()

			check := Check{Status: tc.status}
			if got := check.NeedsIntervention(); got != tc.want {
				t.Errorf("NeedsIntervention() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestDefaultConfig(t *testing.T) {
	t.Parallel()

	cfg := DefaultConfig()

	if cfg.StaleThreshold != 2*time.Hour {
		t.Errorf("StaleThreshold = %v, want %v", cfg.StaleThreshold, 2*time.Hour)
	}
	if cfg.MinActivityInterval != 30*time.Minute {
		t.Errorf("MinActivityInterval = %v, want %v", cfg.MinActivityInterval, 30*time.Minute)
	}
}

func TestEvaluate_EdgeCases(t *testing.T) {
	t.Parallel()

	t.Run("zero elapsed time", func(t *testing.T) {
		t.Parallel()

		input := Input{
			PIDExists:   true,
			HasTask:     true,
			ElapsedTime: 0,
		}

		result := Evaluate(input, DefaultConfig())

		if result.Status != StatusHealthy {
			t.Errorf("Status = %q, want %q for zero elapsed time", result.Status, StatusHealthy)
		}
	})

	t.Run("zero thresholds in config", func(t *testing.T) {
		t.Parallel()

		input := Input{
			PIDExists:     true,
			HasTask:       true,
			ElapsedTime:   5 * time.Hour,
			CommitsLast2h: 0,
		}

		cfg := Config{
			StaleThreshold:      0,
			MinActivityInterval: 0,
		}

		result := Evaluate(input, cfg)

		// With zero thresholds, should fall through to healthy
		if result.Status != StatusHealthy {
			t.Errorf("Status = %q, want %q with zero thresholds", result.Status, StatusHealthy)
		}
	})

	t.Run("multiple processes detected", func(t *testing.T) {
		t.Parallel()

		input := Input{
			PIDExists:    false,
			ProcessCount: 3,
			HasTask:      true,
			ElapsedTime:  10 * time.Minute,
		}

		result := Evaluate(input, DefaultConfig())

		if !result.ProcessAlive {
			t.Error("ProcessAlive = false, want true when ProcessCount > 0")
		}
		if result.ProcessCount != 3 {
			t.Errorf("ProcessCount = %d, want 3", result.ProcessCount)
		}
	})
}
