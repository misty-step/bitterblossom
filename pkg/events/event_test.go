package events

import (
	"errors"
	"testing"
)

func TestKindValid(t *testing.T) {
	t.Parallel()

	valid := []Kind{
		KindProvision,
		KindDispatch,
		KindProgress,
		KindDone,
		KindBlocked,
		KindError,
	}
	for _, kind := range valid {
		if !kind.Valid() {
			t.Fatalf("kind %q should be valid", kind)
		}
	}

	if Kind("wat").Valid() {
		t.Fatal("unexpected valid custom kind")
	}
}

func TestParseKind(t *testing.T) {
	t.Parallel()

	kind, err := ParseKind(" Dispatch ")
	if err != nil {
		t.Fatalf("ParseKind() error = %v", err)
	}
	if kind != KindDispatch {
		t.Fatalf("ParseKind() = %q, want %q", kind, KindDispatch)
	}

	_, err = ParseKind("mystery")
	if !errors.Is(err, ErrUnknownKind) {
		t.Fatalf("ParseKind() error = %v, want ErrUnknownKind", err)
	}
}
