package events

import (
	"bytes"
	"io"
	"strings"
	"testing"
	"time"
)

func TestReaderNext(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 2, 7, 18, 0, 0, 0, time.UTC)
	events := []Event{
		DispatchEvent{Meta: Meta{TS: now, SpriteName: "bramble", EventKind: KindDispatch}, Task: "Fix auth"},
		DoneEvent{Meta: Meta{TS: now.Add(time.Minute), SpriteName: "bramble", EventKind: KindDone}, Branch: "fix/auth", PR: 1},
	}

	var stream bytes.Buffer
	for _, event := range events {
		payload, err := MarshalEvent(event)
		if err != nil {
			t.Fatalf("MarshalEvent() error = %v", err)
		}
		stream.Write(payload)
		stream.WriteString("\n\n")
	}

	reader, err := NewReader(&stream)
	if err != nil {
		t.Fatalf("NewReader() error = %v", err)
	}
	for i, want := range events {
		got, err := reader.Next()
		if err != nil {
			t.Fatalf("Next() index %d error = %v", i, err)
		}
		if got.Kind() != want.Kind() {
			t.Fatalf("Next() kind = %q, want %q", got.Kind(), want.Kind())
		}
	}

	if _, err := reader.Next(); err != io.EOF {
		t.Fatalf("Next() after last event = %v, want EOF", err)
	}
}

func TestReadAll(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 2, 7, 18, 0, 0, 0, time.UTC)
	var stream bytes.Buffer
	for _, event := range []Event{
		ProgressEvent{Meta: Meta{TS: now, SpriteName: "bramble", EventKind: KindProgress}, Commits: 1, FilesChanged: 1},
		ErrorEvent{Meta: Meta{TS: now.Add(time.Minute), SpriteName: "bramble", EventKind: KindError}, Message: "boom"},
	} {
		payload, _ := MarshalEvent(event)
		stream.Write(payload)
		stream.WriteByte('\n')
	}

	got, err := ReadAll(&stream)
	if err != nil {
		t.Fatalf("ReadAll() error = %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("ReadAll() len = %d, want 2", len(got))
	}
}

func TestReaderInvalidLineHasLineNumber(t *testing.T) {
	t.Parallel()

	reader, err := NewReader(strings.NewReader("\nnot-json\n"))
	if err != nil {
		t.Fatalf("NewReader() error = %v", err)
	}
	_, err = reader.Next()
	if err == nil {
		t.Fatal("Next() expected error")
	}
	if !strings.Contains(err.Error(), "line 2") {
		t.Fatalf("error should include line number, got %v", err)
	}
}
