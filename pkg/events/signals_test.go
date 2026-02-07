package events

import (
	"testing"
	"time"
)

func TestStallDetectorDetectsAndResets(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 2, 7, 18, 0, 0, 0, time.UTC)
	current := now
	detector := NewStallDetector(StallSignalConfig{
		Enabled:   true,
		Threshold: 2 * time.Minute,
		Severity:  SeverityWarning,
	}, func() time.Time { return current })

	detector.Observe(ProgressEvent{
		Meta:         Meta{TS: now, SpriteName: "bramble", EventKind: KindProgress},
		Commits:      1,
		FilesChanged: 1,
	})

	current = now.Add(3 * time.Minute)
	signals := detector.Tick(current)
	if len(signals) != 1 {
		t.Fatalf("Tick() signals = %d, want 1", len(signals))
	}
	if signals[0].Name != SignalSpriteStalled {
		t.Fatalf("signal name = %q", signals[0].Name)
	}

	// Duplicate ticks should not re-fire until activity resumes.
	if out := detector.Tick(current.Add(time.Minute)); len(out) != 0 {
		t.Fatalf("duplicate tick signals = %d, want 0", len(out))
	}

	detector.Observe(ProgressEvent{
		Meta:         Meta{TS: current.Add(90 * time.Second), SpriteName: "bramble", EventKind: KindProgress},
		Commits:      2,
		FilesChanged: 2,
	})
	current = current.Add(5 * time.Minute)
	if out := detector.Tick(current); len(out) != 1 {
		t.Fatalf("expected re-fire after new activity, got %d signals", len(out))
	}
}

func TestBuildFailureDetectorMatchesCodeAndMessage(t *testing.T) {
	t.Parallel()

	detector := NewBuildFailureDetector(BuildFailureSignalConfig{
		Enabled:         true,
		Severity:        SeverityCritical,
		SuggestedAction: "fix build",
		ErrorCodes:      []string{"build_failed"},
		MessageContains: []string{"compile failed"},
	})

	byCode := detector.Observe(ErrorEvent{
		Meta:    Meta{TS: time.Now().UTC(), SpriteName: "bramble", EventKind: KindError},
		Code:    "build_failed",
		Message: "pipeline failed",
	})
	if len(byCode) != 1 {
		t.Fatalf("code match signals = %d, want 1", len(byCode))
	}

	byMessage := detector.Observe(ErrorEvent{
		Meta:    Meta{TS: time.Now().UTC(), SpriteName: "bramble", EventKind: KindError},
		Code:    "runtime",
		Message: "Compile failed at package events",
	})
	if len(byMessage) != 1 {
		t.Fatalf("message match signals = %d, want 1", len(byMessage))
	}

	ignored := detector.Observe(ProgressEvent{
		Meta:         Meta{TS: time.Now().UTC(), SpriteName: "bramble", EventKind: KindProgress},
		Commits:      1,
		FilesChanged: 1,
	})
	if len(ignored) != 0 {
		t.Fatalf("non-error event should not emit signals")
	}
}

func TestRepeatedErrorDetectorThresholdAndWindow(t *testing.T) {
	t.Parallel()

	detector := NewRepeatedErrorDetector(RepeatedErrorSignalConfig{
		Enabled:   true,
		Window:    2 * time.Minute,
		Threshold: 3,
		Severity:  SeverityWarning,
	})

	base := time.Date(2026, 2, 7, 18, 0, 0, 0, time.UTC)
	emit := func(at time.Time) []Signal {
		return detector.Observe(ErrorEvent{
			Meta:    Meta{TS: at, SpriteName: "bramble", EventKind: KindError},
			Message: "boom",
		})
	}

	if out := emit(base); len(out) != 0 {
		t.Fatalf("signal too early on first error: %d", len(out))
	}
	if out := emit(base.Add(30 * time.Second)); len(out) != 0 {
		t.Fatalf("signal too early on second error: %d", len(out))
	}
	if out := emit(base.Add(time.Minute)); len(out) != 1 {
		t.Fatalf("third error should trigger, got %d", len(out))
	}
	// Within cooldown window, do not emit again.
	if out := emit(base.Add(90 * time.Second)); len(out) != 0 {
		t.Fatalf("should suppress duplicate repeated-error signal, got %d", len(out))
	}
	// Past window, sequence should trigger again.
	if out := emit(base.Add(4 * time.Minute)); len(out) != 0 {
		t.Fatalf("new window first error should not trigger, got %d", len(out))
	}
	if out := emit(base.Add(4*time.Minute + 10*time.Second)); len(out) != 0 {
		t.Fatalf("new window second error should not trigger, got %d", len(out))
	}
	if out := emit(base.Add(4*time.Minute + 20*time.Second)); len(out) != 1 {
		t.Fatalf("new window third error should trigger, got %d", len(out))
	}
}

func TestSignalEngineConfig(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 2, 7, 18, 0, 0, 0, time.UTC)
	current := now
	cfg := DefaultSignalConfig()
	cfg.Now = func() time.Time { return current }
	engine := NewConfiguredSignalEngine(cfg)

	out := engine.Observe(ErrorEvent{
		Meta:    Meta{TS: now, SpriteName: "bramble", EventKind: KindError},
		Code:    "build_failed",
		Message: "build failed",
	})
	if len(out) == 0 {
		t.Fatal("expected build failure signal")
	}

	engine.Observe(DispatchEvent{
		Meta: Meta{TS: now, SpriteName: "bramble", EventKind: KindDispatch},
		Task: "Fix auth",
	})
	current = now.Add(15 * time.Minute)
	tickSignals := engine.Tick()
	if len(tickSignals) == 0 {
		t.Fatal("expected stall signal from Tick()")
	}
}
