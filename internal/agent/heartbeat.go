package agent

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/misty-step/bitterblossom/pkg/events"
)

const DefaultHeartbeatInterval = time.Minute

// HeartbeatSnapshot is attached to heartbeat events.
type HeartbeatSnapshot struct {
	UptimeSeconds      int64
	AgentPID           int
	CPUPercent         float64
	MemoryBytes        uint64
	Branch             string
	LastCommit         string
	UncommittedChanges bool
}

// HeartbeatSource provides runtime values for heartbeat emission.
type HeartbeatSource interface {
	HeartbeatSnapshot(ctx context.Context) (HeartbeatSnapshot, error)
}

// Heartbeat emits periodic liveness and resource events.
type Heartbeat struct {
	interval time.Duration
	sprite   string
	source   HeartbeatSource
	emitter  EventEmitter
	onEmit   func(time.Time)
	now      func() time.Time
}

// NewHeartbeat constructs a heartbeat emitter.
func NewHeartbeat(interval time.Duration, sprite string, source HeartbeatSource, emitter EventEmitter, onEmit func(time.Time)) *Heartbeat {
	if interval <= 0 {
		interval = DefaultHeartbeatInterval
	}
	return &Heartbeat{
		interval: interval,
		sprite:   sprite,
		source:   source,
		emitter:  emitter,
		onEmit:   onEmit,
		now:      time.Now,
	}
}

// Run emits heartbeat events until context cancellation.
func (h *Heartbeat) Run(ctx context.Context, wg *sync.WaitGroup) {
	if wg != nil {
		defer wg.Done()
	}

	ticker := time.NewTicker(h.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			now := h.now().UTC()
			snapshot, err := h.source.HeartbeatSnapshot(ctx)
			if err != nil {
				_ = h.emitter.Emit(&events.ErrorEvent{
					Meta:    events.Meta{TS: now, SpriteName: h.sprite, EventKind: events.KindError},
					Code:    "heartbeat_snapshot",
					Message: err.Error(),
				})
				continue
			}

			uncommitted := snapshot.UncommittedChanges
			_ = h.emitter.Emit(&events.HeartbeatEvent{
				Meta:               events.Meta{TS: now, SpriteName: h.sprite, EventKind: events.KindHeartbeat},
				UptimeSeconds:      snapshot.UptimeSeconds,
				AgentPID:           snapshot.AgentPID,
				CPUPercent:         snapshot.CPUPercent,
				MemoryBytes:        snapshot.MemoryBytes,
				Branch:             snapshot.Branch,
				LastCommit:         snapshot.LastCommit,
				UncommittedChanges: &uncommitted,
			})
			if h.onEmit != nil {
				h.onEmit(now)
			}
		}
	}
}

// ProcessUsage captures a process resource sample.
type ProcessUsage struct {
	CPUPercent float64
	MemoryBytes uint64
}

// ProcessSampler samples process CPU and memory.
type ProcessSampler interface {
	Sample(ctx context.Context, pid int) (ProcessUsage, error)
}

type psSampler struct {
	runner commandRunner
}

func newPSSampler() *psSampler {
	return &psSampler{runner: execRunner{}}
}

func (s *psSampler) Sample(ctx context.Context, pid int) (ProcessUsage, error) {
	if pid <= 0 {
		return ProcessUsage{}, nil
	}
	if s.runner == nil {
		s.runner = execRunner{}
	}

	output, err := s.runner.Run(ctx, "ps", "-o", "%cpu=", "-o", "rss=", "-p", strconv.Itoa(pid))
	if err != nil {
		return ProcessUsage{}, fmt.Errorf("sample process %d: %w", pid, err)
	}

	fields := strings.Fields(output)
	if len(fields) < 2 {
		return ProcessUsage{}, fmt.Errorf("unexpected ps output: %q", output)
	}
	cpuPercent, err := strconv.ParseFloat(fields[0], 64)
	if err != nil {
		return ProcessUsage{}, fmt.Errorf("parse cpu from %q: %w", fields[0], err)
	}
	rssKB, err := strconv.ParseUint(fields[1], 10, 64)
	if err != nil {
		return ProcessUsage{}, fmt.Errorf("parse rss from %q: %w", fields[1], err)
	}

	return ProcessUsage{CPUPercent: cpuPercent, MemoryBytes: rssKB * 1024}, nil
}
