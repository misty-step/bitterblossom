package clients

import (
	"context"
	"testing"
)

func TestExecRunnerRunSuccess(t *testing.T) {
	r := ExecRunner{}
	out, code, err := r.Run(context.Background(), "sh", "-c", "printf ok")
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d", code)
	}
	if out != "ok" {
		t.Fatalf("output mismatch: %q", out)
	}
}

func TestExecRunnerRunFailure(t *testing.T) {
	r := ExecRunner{}
	_, code, err := r.Run(context.Background(), "sh", "-c", "exit 7")
	if err == nil {
		t.Fatal("expected non-nil error")
	}
	if code != 7 {
		t.Fatalf("expected exit code 7, got %d", code)
	}
}
