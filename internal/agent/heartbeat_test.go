package agent

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/misty-step/bitterblossom/pkg/events"
)

type staticHeartbeatSource struct {
	snapshot HeartbeatSnapshot
	err      error
}

func (s staticHeartbeatSource) HeartbeatSnapshot(context.Context) (HeartbeatSnapshot, error) {
	if s.err != nil {
		return HeartbeatSnapshot{}, s.err
	}
	return s.snapshot, nil
}

type fakeRunner struct {
	output string
	err    error
}

func (f fakeRunner) Run(context.Context, string, ...string) (string, error) {
	return f.output, f.err
}

func TestNewHeartbeatDefaultInterval(t *testing.T) {
	t.Parallel()

	hb := NewHeartbeat(0, "bramble", staticHeartbeatSource{}, &recordingEventEmitter{}, nil)
	if hb.interval != DefaultHeartbeatInterval {
		t.Fatalf("unexpected heartbeat interval: %s", hb.interval)
	}
}

func TestHeartbeatRunEmitsHeartbeatEvents(t *testing.T) {
	t.Parallel()

	emitter := &recordingEventEmitter{}
	hb := NewHeartbeat(10*time.Millisecond, "bramble", staticHeartbeatSource{snapshot: HeartbeatSnapshot{
		UptimeSeconds:      120,
		AgentPID:           999,
		CPUPercent:         20.5,
		MemoryBytes:        1024,
		Branch:             "feature/auth",
		LastCommit:         "abc123",
		UncommittedChanges: true,
	}}, emitter, nil)

	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup
	wg.Add(1)
	go hb.Run(ctx, &wg)
	time.Sleep(35 * time.Millisecond)
	cancel()
	wg.Wait()

	heartbeatFound := false
	for _, event := range emitter.Events() {
		if _, ok := event.(*events.HeartbeatEvent); ok {
			heartbeatFound = true
			break
		}
	}
	if !heartbeatFound {
		t.Fatalf("expected heartbeat event")
	}
}

func TestHeartbeatRunEmitsErrorOnSnapshotFailure(t *testing.T) {
	t.Parallel()

	emitter := &recordingEventEmitter{}
	hb := NewHeartbeat(10*time.Millisecond, "bramble", staticHeartbeatSource{err: fmt.Errorf("snapshot failed")}, emitter, nil)

	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup
	wg.Add(1)
	go hb.Run(ctx, &wg)
	time.Sleep(25 * time.Millisecond)
	cancel()
	wg.Wait()

	errorFound := false
	for _, event := range emitter.Events() {
		if _, ok := event.(*events.ErrorEvent); ok {
			errorFound = true
			break
		}
	}
	if !errorFound {
		t.Fatalf("expected error event")
	}
}

func TestPSSamplerSample(t *testing.T) {
	t.Parallel()

	sampler := &psSampler{runner: fakeRunner{output: "12.5 4096"}}
	usage, err := sampler.Sample(context.Background(), 123)
	if err != nil {
		t.Fatalf("sample: %v", err)
	}
	if usage.CPUPercent != 12.5 {
		t.Fatalf("unexpected cpu: %f", usage.CPUPercent)
	}
	if usage.MemoryBytes != 4096*1024 {
		t.Fatalf("unexpected memory bytes: %d", usage.MemoryBytes)
	}
}

func TestPSSamplerSampleForZeroPID(t *testing.T) {
	t.Parallel()

	sampler := &psSampler{runner: fakeRunner{output: "100.0 100"}}
	usage, err := sampler.Sample(context.Background(), 0)
	if err != nil {
		t.Fatalf("sample zero pid: %v", err)
	}
	if usage.CPUPercent != 0 || usage.MemoryBytes != 0 {
		t.Fatalf("expected zero usage for zero pid")
	}
}
