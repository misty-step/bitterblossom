package events

import (
	"bytes"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestEmitterEmitJSONL(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 2, 7, 18, 0, 0, 0, time.UTC)
	var out bytes.Buffer
	emitter, err := NewEmitter(&out)
	if err != nil {
		t.Fatalf("NewEmitter() error = %v", err)
	}
	event := DispatchEvent{
		Meta: Meta{TS: now, SpriteName: "bramble", EventKind: KindDispatch},
		Task: "Fix auth",
	}

	if err := emitter.Emit(event); err != nil {
		t.Fatalf("Emit() error = %v", err)
	}

	raw := out.String()
	if !strings.HasSuffix(raw, "\n") {
		t.Fatalf("output should end with newline: %q", raw)
	}
}

func TestEmitterErrors(t *testing.T) {
	t.Parallel()

	if _, err := NewEmitter(nil); err == nil {
		t.Fatal("NewEmitter(nil) expected error")
	}

	emitter, err := NewEmitter(failWriter{})
	if err != nil {
		t.Fatalf("NewEmitter() error = %v", err)
	}
	err = emitter.Emit(ProvisionEvent{
		Meta:    Meta{TS: time.Now().UTC(), SpriteName: "bramble", EventKind: KindProvision},
		Persona: "bramble",
	})
	if err == nil {
		t.Fatal("Emit() expected write error")
	}
}

type failWriter struct{}

func (failWriter) Write(_ []byte) (int, error) {
	return 0, errors.New("boom")
}
