package events

import (
	"context"
	"testing"
	"time"
)

func TestAggregateHistoricalBySpriteAndType(t *testing.T) {
	t.Parallel()

	start := time.Date(2026, 2, 7, 18, 0, 0, 0, time.UTC)
	end := start.Add(10 * time.Minute)
	input := []Event{
		ProvisionEvent{Meta: Meta{TS: start, SpriteName: "bramble", EventKind: KindProvision}},
		DispatchEvent{Meta: Meta{TS: start.Add(time.Minute), SpriteName: "bramble", EventKind: KindDispatch}, Task: "task"},
		ProgressEvent{Meta: Meta{TS: start.Add(2 * time.Minute), SpriteName: "bramble", EventKind: KindProgress}, Commits: 1, FilesChanged: 2},
		ErrorEvent{Meta: Meta{TS: start.Add(4 * time.Minute), SpriteName: "bramble", EventKind: KindError}, Message: "boom"},
		DoneEvent{Meta: Meta{TS: start.Add(9 * time.Minute), SpriteName: "thorn", EventKind: KindDone}, Branch: "x"},
	}

	snapshot := Aggregate(input, end.Sub(start), 90*time.Second, end)
	if snapshot.TotalEvents != 5 {
		t.Fatalf("TotalEvents = %d, want 5", snapshot.TotalEvents)
	}
	if snapshot.ByType[KindError] != 1 {
		t.Fatalf("ByType[error] = %d, want 1", snapshot.ByType[KindError])
	}
	if len(snapshot.BySprite) != 2 {
		t.Fatalf("BySprite len = %d, want 2", len(snapshot.BySprite))
	}
	if snapshot.BySprite["bramble"].TotalEvents != 4 {
		t.Fatalf("bramble total = %d, want 4", snapshot.BySprite["bramble"].TotalEvents)
	}
	if snapshot.ErrorRate <= 0 {
		t.Fatalf("ErrorRate = %f, want > 0", snapshot.ErrorRate)
	}
}

func TestAggregateComputesActivityGapsAndUptime(t *testing.T) {
	t.Parallel()

	start := time.Date(2026, 2, 7, 18, 0, 0, 0, time.UTC)
	end := start.Add(10 * time.Minute)
	input := []Event{
		ProgressEvent{Meta: Meta{TS: start.Add(30 * time.Second), SpriteName: "bramble", EventKind: KindProgress}, Commits: 1, FilesChanged: 1},
		ProgressEvent{Meta: Meta{TS: start.Add(9 * time.Minute), SpriteName: "bramble", EventKind: KindProgress}, Commits: 2, FilesChanged: 2},
	}

	snapshot := Aggregate(input, end.Sub(start), time.Minute, end)
	sprite := snapshot.BySprite["bramble"]
	if len(sprite.ActivityGaps) == 0 {
		t.Fatal("expected activity gaps")
	}
	if sprite.Uptime >= 1 {
		t.Fatalf("Uptime = %f, want < 1 due to large inactivity gaps", sprite.Uptime)
	}
	if snapshot.Uptime >= 1 {
		t.Fatalf("fleet Uptime = %f, want < 1", snapshot.Uptime)
	}
}

func TestAggregatorRealtimeConsume(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 2, 7, 18, 0, 0, 0, time.UTC)
	current := now
	agg := NewAggregator(AggregatorConfig{
		Window:       5 * time.Minute,
		GapThreshold: 2 * time.Minute,
		Now:          func() time.Time { return current },
	})

	in := make(chan Event, 4)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- agg.Consume(ctx, in) }()

	in <- DispatchEvent{Meta: Meta{TS: now, SpriteName: "bramble", EventKind: KindDispatch}, Task: "task"}
	in <- ErrorEvent{Meta: Meta{TS: now.Add(time.Minute), SpriteName: "bramble", EventKind: KindError}, Message: "boom"}
	close(in)

	if err := <-done; err != nil {
		t.Fatalf("Consume() error = %v", err)
	}

	current = now.Add(2 * time.Minute)
	snapshot := agg.Snapshot()
	if snapshot.TotalEvents != 2 {
		t.Fatalf("TotalEvents = %d, want 2", snapshot.TotalEvents)
	}
	if snapshot.BySprite["bramble"].ErrorRate <= 0 {
		t.Fatalf("ErrorRate = %f, want > 0", snapshot.BySprite["bramble"].ErrorRate)
	}
}

func TestAggregatorWindowPruning(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 2, 7, 18, 0, 0, 0, time.UTC)
	current := now
	agg := NewAggregator(AggregatorConfig{
		Window: 3 * time.Minute,
		Now:    func() time.Time { return current },
	})

	agg.Add(DispatchEvent{Meta: Meta{TS: now.Add(-5 * time.Minute), SpriteName: "bramble", EventKind: KindDispatch}, Task: "old"})
	agg.Add(DispatchEvent{Meta: Meta{TS: now.Add(-time.Minute), SpriteName: "bramble", EventKind: KindDispatch}, Task: "new"})

	snapshot := agg.Snapshot()
	if snapshot.TotalEvents != 1 {
		t.Fatalf("TotalEvents = %d, want 1 after pruning", snapshot.TotalEvents)
	}
}
