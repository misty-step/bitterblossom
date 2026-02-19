package main

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestCheckActiveDispatchLoop_IdleSprite(t *testing.T) {
	t.Parallel()

	r := &fakeSpriteScriptRunner{out: nil, exitCode: 0, err: nil}
	isBusy, err := checkActiveDispatchLoop(context.Background(), r.run)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if isBusy {
		t.Fatal("expected sprite to be idle")
	}
	if !r.called {
		t.Fatal("runner should be called")
	}
}

func TestCheckActiveDispatchLoop_BusySprite(t *testing.T) {
	t.Parallel()

	output := "1234 bash /home/sprite/workspace/.ralph.sh\n"
	r := &fakeSpriteScriptRunner{out: []byte(output), exitCode: 1, err: nil}
	isBusy, err := checkActiveDispatchLoop(context.Background(), r.run)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if !isBusy {
		t.Fatal("expected sprite to be busy")
	}
}

func TestCheckActiveDispatchLoop_HandlesRunnerError(t *testing.T) {
	t.Parallel()

	r := &fakeSpriteScriptRunner{out: nil, exitCode: 0, err: errors.New("network")}
	_, err := checkActiveDispatchLoop(context.Background(), r.run)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "check dispatch loop") {
		t.Fatalf("err = %q, want to contain %q", err.Error(), "check dispatch loop")
	}
}

func TestCheckActiveDispatchLoop_UnexpectedExitCode(t *testing.T) {
	t.Parallel()

	r := &fakeSpriteScriptRunner{out: nil, exitCode: 2, err: nil}
	isBusy, err := checkActiveDispatchLoop(context.Background(), r.run)
	if err != nil {
		t.Fatalf("expected nil error for unexpected exit code, got %v", err)
	}
	if isBusy {
		t.Fatal("expected isBusy=false for unexpected exit code")
	}
}