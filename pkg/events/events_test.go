package events

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"
	"time"
)

func TestMarshalUnmarshalRoundTrip(t *testing.T) {
	t.Parallel()

	ts := time.Date(2026, 2, 7, 18, 0, 0, 0, time.UTC)
	cases := []struct {
		name string
		in   Event
	}{
		{
			name: "provision",
			in: &ProvisionEvent{
				Meta:    Meta{TS: ts, SpriteName: "bramble", EventKind: KindProvision},
				Persona: "bramble",
			},
		},
		{
			name: "dispatch",
			in: &DispatchEvent{
				Meta: Meta{TS: ts, SpriteName: "bramble", EventKind: KindDispatch},
				Task: "Fix auth",
				Repo: "cerberus",
			},
		},
		{
			name: "heartbeat",
			in: &HeartbeatEvent{
				Meta:          Meta{TS: ts, SpriteName: "bramble", EventKind: KindHeartbeat},
				UptimeSeconds: 120,
				AgentPID:      4321,
				CPUPercent:    15.2,
				MemoryBytes:   512 * 1024 * 1024,
				Branch:        "fix/auth",
				LastCommit:    "abc123",
			},
		},
		{
			name: "progress",
			in: &ProgressEvent{
				Meta:         Meta{TS: ts, SpriteName: "bramble", EventKind: KindProgress},
				Branch:       "fix/auth",
				Commits:      3,
				FilesChanged: 7,
				Activity:     "git_commit",
			},
		},
		{
			name: "done",
			in: &DoneEvent{
				Meta:   Meta{TS: ts, SpriteName: "bramble", EventKind: KindDone},
				Branch: "fix/auth",
				PR:     51,
			},
		},
		{
			name: "blocked",
			in: &BlockedEvent{
				Meta:   Meta{TS: ts, SpriteName: "bramble", EventKind: KindBlocked},
				Reason: "waiting for credentials",
			},
		},
		{
			name: "error",
			in: &ErrorEvent{
				Meta:    Meta{TS: ts, SpriteName: "bramble", EventKind: KindError},
				Code:    "E_RUNTIME",
				Message: "dispatch failed",
			},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			payload, err := MarshalEvent(tc.in)
			if err != nil {
				t.Fatalf("MarshalEvent: %v", err)
			}

			decoded, err := UnmarshalEvent(payload)
			if err != nil {
				t.Fatalf("UnmarshalEvent: %v", err)
			}

			if decoded.Kind() != tc.in.Kind() {
				t.Fatalf("kind mismatch: want %s got %s", tc.in.Kind(), decoded.Kind())
			}
			if decoded.Sprite() != tc.in.Sprite() {
				t.Fatalf("sprite mismatch: want %s got %s", tc.in.Sprite(), decoded.Sprite())
			}
			if !decoded.Timestamp().Equal(tc.in.Timestamp()) {
				t.Fatalf("timestamp mismatch: want %v got %v", tc.in.Timestamp(), decoded.Timestamp())
			}
		})
	}
}

func TestUnmarshalEventUnknownKind(t *testing.T) {
	t.Parallel()

	_, err := UnmarshalEvent([]byte(`{"ts":"2026-02-07T18:00:00Z","sprite":"bramble","event":"unknown"}`))
	if !errors.Is(err, ErrUnknownKind) {
		t.Fatalf("expected ErrUnknownKind, got %v", err)
	}
}

func TestMarshalEventValidation(t *testing.T) {
	t.Parallel()

	_, err := MarshalEvent(nil)
	if !errors.Is(err, ErrInvalidEvent) {
		t.Fatalf("expected ErrInvalidEvent, got %v", err)
	}

	_, err = MarshalEvent(&DispatchEvent{
		Meta: Meta{TS: time.Now().UTC(), SpriteName: "bramble", EventKind: KindDispatch},
	})
	if !errors.Is(err, ErrInvalidEvent) {
		t.Fatalf("expected ErrInvalidEvent, got %v", err)
	}

	_, err = MarshalEvent(DispatchEvent{
		Meta: Meta{TS: time.Now().UTC(), SpriteName: "bramble", EventKind: KindDispatch},
	})
	if !errors.Is(err, ErrInvalidEvent) {
		t.Fatalf("expected ErrInvalidEvent for value dispatch event, got %v", err)
	}

	_, err = MarshalEvent(BlockedEvent{
		Meta: Meta{TS: time.Now().UTC(), SpriteName: "bramble", EventKind: KindBlocked},
	})
	if !errors.Is(err, ErrInvalidEvent) {
		t.Fatalf("expected ErrInvalidEvent for value blocked event, got %v", err)
	}

	_, err = MarshalEvent(ErrorEvent{
		Meta: Meta{TS: time.Now().UTC(), SpriteName: "bramble", EventKind: KindError},
	})
	if !errors.Is(err, ErrInvalidEvent) {
		t.Fatalf("expected ErrInvalidEvent for value error event, got %v", err)
	}
}

func TestEmitterAndReader(t *testing.T) {
	t.Parallel()

	buffer := &bytes.Buffer{}
	emitter, err := NewEmitter(buffer)
	if err != nil {
		t.Fatalf("NewEmitter: %v", err)
	}

	ts := time.Date(2026, 2, 7, 18, 0, 0, 0, time.UTC)
	events := []Event{
		&ProvisionEvent{Meta: Meta{TS: ts, SpriteName: "bramble", EventKind: KindProvision}, Persona: "bramble"},
		&DispatchEvent{Meta: Meta{TS: ts, SpriteName: "bramble", EventKind: KindDispatch}, Task: "Fix auth"},
		&DoneEvent{Meta: Meta{TS: ts, SpriteName: "bramble", EventKind: KindDone}, Branch: "fix/auth", PR: 51},
	}

	for _, event := range events {
		if err := emitter.Emit(event); err != nil {
			t.Fatalf("Emit(%s): %v", event.Kind(), err)
		}
	}

	reader, err := NewReader(strings.NewReader(buffer.String()))
	if err != nil {
		t.Fatalf("NewReader: %v", err)
	}

	for i, expected := range events {
		got, err := reader.Next()
		if err != nil {
			t.Fatalf("reader.Next #%d: %v", i+1, err)
		}
		if got.Kind() != expected.Kind() {
			t.Fatalf("kind mismatch #%d: want %s got %s", i+1, expected.Kind(), got.Kind())
		}
	}

	_, err = reader.Next()
	if !errors.Is(err, io.EOF) {
		t.Fatalf("expected EOF, got %v", err)
	}
}

func TestReaderSkipsEmptyLines(t *testing.T) {
	t.Parallel()

	input := "\n\n" + `{"ts":"2026-02-07T18:00:00Z","sprite":"bramble","event":"progress","commits":1,"files_changed":2}` + "\n\n"
	reader, err := NewReader(strings.NewReader(input))
	if err != nil {
		t.Fatalf("NewReader: %v", err)
	}

	event, err := reader.Next()
	if err != nil {
		t.Fatalf("Next: %v", err)
	}
	if event.Kind() != KindProgress {
		t.Fatalf("expected progress, got %s", event.Kind())
	}
}

func TestEmitterAndReaderConstructorValidation(t *testing.T) {
	t.Parallel()

	if _, err := NewEmitter(nil); err == nil {
		t.Fatal("expected nil writer error")
	}
	if _, err := NewReader(nil); err == nil {
		t.Fatal("expected nil reader error")
	}
}

func TestEmitterWriteError(t *testing.T) {
	t.Parallel()

	emitter, err := NewEmitter(failingWriter{})
	if err != nil {
		t.Fatalf("NewEmitter: %v", err)
	}

	err = emitter.Emit(&ProvisionEvent{
		Meta:    Meta{TS: time.Now().UTC(), SpriteName: "bramble", EventKind: KindProvision},
		Persona: "bramble",
	})
	if err == nil {
		t.Fatal("expected write error")
	}
}

func TestReaderInvalidJSON(t *testing.T) {
	t.Parallel()

	reader, err := NewReader(strings.NewReader("not-json\n"))
	if err != nil {
		t.Fatalf("NewReader: %v", err)
	}
	if _, err := reader.Next(); err == nil {
		t.Fatal("expected parse error")
	}
}

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) {
	return 0, errors.New("write failure")
}
