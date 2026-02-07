package events

import (
	"testing"
	"time"
)

func TestBySprite(t *testing.T) {
	t.Parallel()

	event := DispatchEvent{
		Meta: Meta{TS: time.Now().UTC(), SpriteName: "Bramble", EventKind: KindDispatch},
		Task: "Fix auth",
	}

	if !BySprite("bramble")(event) {
		t.Fatal("expected case-insensitive sprite match")
	}
	if BySprite("thorn")(event) {
		t.Fatal("expected sprite mismatch")
	}
}

func TestByKind(t *testing.T) {
	t.Parallel()

	event := DoneEvent{
		Meta:   Meta{TS: time.Now().UTC(), SpriteName: "bramble", EventKind: KindDone},
		Branch: "fix/auth",
	}

	if !ByKind(KindDone)(event) {
		t.Fatal("expected kind match")
	}
	if ByKind(KindDispatch)(event) {
		t.Fatal("expected kind mismatch")
	}
}

func TestByTimeRangeAndParseKinds(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 2, 7, 18, 0, 0, 0, time.UTC)
	event := ProgressEvent{
		Meta:         Meta{TS: now, SpriteName: "bramble", EventKind: KindProgress},
		Commits:      1,
		FilesChanged: 2,
	}

	if !ByTimeRange(now.Add(-time.Second), now.Add(time.Second))(event) {
		t.Fatal("expected event in range")
	}
	if ByTimeRange(now.Add(time.Second), time.Time{})(event) {
		t.Fatal("expected event before start to be filtered")
	}

	kinds, err := ParseKinds("done, blocked")
	if err != nil {
		t.Fatalf("ParseKinds() error = %v", err)
	}
	if len(kinds) != 2 || kinds[0] != KindDone || kinds[1] != KindBlocked {
		t.Fatalf("ParseKinds() unexpected result: %#v", kinds)
	}
}

func TestApply(t *testing.T) {
	t.Parallel()

	base := time.Date(2026, 2, 7, 18, 0, 0, 0, time.UTC)
	input := []Event{
		DispatchEvent{Meta: Meta{TS: base, SpriteName: "bramble", EventKind: KindDispatch}, Task: "task"},
		DoneEvent{Meta: Meta{TS: base.Add(time.Minute), SpriteName: "thorn", EventKind: KindDone}, Branch: "x"},
		DoneEvent{Meta: Meta{TS: base.Add(2 * time.Minute), SpriteName: "bramble", EventKind: KindDone}, Branch: "y"},
	}

	out := Apply(input,
		BySprite("bramble"),
		ByKind(KindDone),
		ByTimeRange(base, base.Add(3*time.Minute)),
	)

	if len(out) != 1 {
		t.Fatalf("Apply() len = %d, want 1", len(out))
	}
	if out[0].Kind() != KindDone || out[0].Sprite() != "bramble" {
		t.Fatalf("Apply() unexpected event: %#v", out[0])
	}
}
