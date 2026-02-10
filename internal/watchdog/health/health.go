// Package health provides process health checks and zombie detection for agent processes.
package health

import (
	"fmt"
	"time"
)

// Status represents the health state of an agent process.
type Status string

const (
	// StatusHealthy indicates the process is running and producing work.
	StatusHealthy Status = "healthy"
	// StatusZombie indicates the process is running but not producing work.
	StatusZombie Status = "zombie"
	// StatusDead indicates the process is not running.
	StatusDead Status = "dead"
	// StatusUnknown indicates health status cannot be determined.
	StatusUnknown Status = "unknown"
)

// Config holds thresholds for zombie detection.
type Config struct {
	// StaleThreshold is how long without commits before marking as zombie.
	StaleThreshold time.Duration
	// MinActivityInterval is minimum expected time between outputs.
	MinActivityInterval time.Duration
}

// DefaultConfig returns sensible defaults for health checks.
func DefaultConfig() Config {
	return Config{
		StaleThreshold:      2 * time.Hour,
		MinActivityInterval: 30 * time.Minute,
	}
}

// Check represents the health state of an agent process.
type Check struct {
	Status          Status
	ProcessAlive    bool
	ProcessCount    int
	Responsive      bool
	LastActivity    time.Time
	TimeSinceOutput time.Duration
	CommitsRecent   int
	Reason          string
}

// Input contains all data needed to evaluate process health.
type Input struct {
	// PIDExists indicates if the agent.pid file exists and process is alive.
	PIDExists bool
	// ProcessCount is the number of matching agent processes.
	ProcessCount int
	// HasTask indicates if there's an active task assigned.
	HasTask bool
	// ElapsedTime is how long the current task has been running.
	ElapsedTime time.Duration
	// CommitsLast2h is the number of commits in the last 2 hours.
	CommitsLast2h int
	// HasComplete indicates if TASK_COMPLETE marker exists.
	HasComplete bool
	// HasBlocked indicates if BLOCKED.md marker exists.
	HasBlocked bool
	// DirtyRepos indicates uncommitted changes exist.
	DirtyRepos int
	// AheadCommits indicates unpushed commits exist.
	AheadCommits int
}

// Evaluate performs a comprehensive health check on an agent process.
func Evaluate(input Input, cfg Config) Check {
	check := Check{
		ProcessCount: input.ProcessCount,
		CommitsRecent: input.CommitsLast2h,
	}

	// Determine if process is alive
	check.ProcessAlive = input.PIDExists || input.ProcessCount > 0

	if !check.ProcessAlive {
		check.Status = StatusDead
		check.Reason = "no agent process detected"
		return check
	}

	// Process is alive - now check if it's healthy or zombie

	// If task is complete or blocked, not a zombie
	if input.HasComplete {
		check.Status = StatusHealthy
		check.Reason = "task completed"
		check.Responsive = true
		return check
	}

	if input.HasBlocked {
		check.Status = StatusHealthy
		check.Reason = "task blocked (waiting for input)"
		check.Responsive = true
		return check
	}

	// If there's no task, process is healthy but idle
	if !input.HasTask {
		check.Status = StatusHealthy
		check.Reason = "no active task"
		check.Responsive = true
		return check
	}

	// Process is running with a task - check for zombie indicators
	check.TimeSinceOutput = input.ElapsedTime

	// Check for stale process (running too long without commits)
	if cfg.StaleThreshold > 0 && input.ElapsedTime >= cfg.StaleThreshold {
		if input.CommitsLast2h == 0 {
			// No commits in monitoring window - likely zombie
			check.Status = StatusZombie
			check.Reason = fmt.Sprintf("no commits for %v (threshold: %v)",
				input.ElapsedTime.Round(time.Minute),
				cfg.StaleThreshold.Round(time.Minute))
			check.Responsive = false
			return check
		}
	}

	// Check if process has any signs of activity
	hasActivity := input.CommitsLast2h > 0 || input.DirtyRepos > 0 || input.AheadCommits > 0

	if cfg.MinActivityInterval > 0 && !hasActivity && input.ElapsedTime > cfg.MinActivityInterval {
		// Process running but no visible activity
		check.Status = StatusZombie
		check.Reason = fmt.Sprintf("no activity detected for %v",
			input.ElapsedTime.Round(time.Minute))
		check.Responsive = false
		return check
	}

	// Process appears healthy
	check.Status = StatusHealthy
	check.Responsive = true

	if input.CommitsLast2h > 0 {
		check.Reason = fmt.Sprintf("%d commits in last 2h", input.CommitsLast2h)
	} else if input.DirtyRepos > 0 || input.AheadCommits > 0 {
		check.Reason = "active work in progress"
	} else {
		check.Reason = "process running normally"
	}

	return check
}

// IsZombie returns true if the process is in a zombie state.
func (c Check) IsZombie() bool {
	return c.Status == StatusZombie
}

// IsAlive returns true if the process is running (healthy or zombie).
func (c Check) IsAlive() bool {
	return c.ProcessAlive
}

// NeedsIntervention returns true if manual or automatic action is needed.
func (c Check) NeedsIntervention() bool {
	return c.Status == StatusZombie || c.Status == StatusDead
}
