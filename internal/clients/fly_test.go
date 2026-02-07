package clients

import (
	"context"
	"errors"
	"testing"
)

type recordRunner struct {
	called string
	out    string
	err    error
}

func (r *recordRunner) Run(_ context.Context, name string, args ...string) (string, int, error) {
	r.called = name
	for _, arg := range args {
		r.called += " " + arg
	}
	if r.err != nil {
		return r.out, 1, r.err
	}
	return r.out, 0, nil
}

func TestFlyCLISSHRun(t *testing.T) {
	r := &recordRunner{out: "ok"}
	fly := NewFlyCLI(r, "fly")
	out, err := fly.SSHRun(context.Background(), "misty-step", "thorn", "echo hi")
	if err != nil {
		t.Fatalf("SSHRun returned error: %v", err)
	}
	if out != "ok" {
		t.Fatalf("output mismatch: %q", out)
	}
	if r.called == "" {
		t.Fatal("expected runner call")
	}
}

func TestFlyCLISSHRunError(t *testing.T) {
	r := &recordRunner{err: errors.New("boom")}
	fly := NewFlyCLI(r, "fly")
	if _, err := fly.SSHRun(context.Background(), "misty-step", "thorn", "echo hi"); err == nil {
		t.Fatal("expected error")
	}
}
