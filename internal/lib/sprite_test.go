package lib

import (
	"context"
	"errors"
	"testing"
)

func TestSpriteCLIExists(t *testing.T) {
	runner := &mockRunner{results: []RunResult{{Stdout: "bramble\nthorn\n"}}}
	sprite := NewSpriteCLI(runner, "sprite", "misty-step")

	exists, err := sprite.Exists(context.Background(), "thorn")
	if err != nil {
		t.Fatalf("exists check failed: %v", err)
	}
	if !exists {
		t.Fatalf("expected thorn to exist")
	}
}

func TestSpriteCLICreateForwardsError(t *testing.T) {
	runner := &mockRunner{errors: []error{errors.New("boom")}}
	sprite := NewSpriteCLI(runner, "sprite", "misty-step")

	if err := sprite.Create(context.Background(), "fern"); err == nil {
		t.Fatalf("expected error")
	}
}

func TestSpriteCLIDestroyAndCheckpoint(t *testing.T) {
	runner := &mockRunner{}
	sprite := NewSpriteCLI(runner, "sprite", "misty-step")
	if err := sprite.Destroy(context.Background(), "fern", true); err != nil {
		t.Fatalf("destroy failed: %v", err)
	}
	if err := sprite.CheckpointCreate(context.Background(), "fern"); err != nil {
		t.Fatalf("checkpoint failed: %v", err)
	}
	reqs := runner.Requests()
	if len(reqs) != 2 {
		t.Fatalf("expected 2 requests, got %d", len(reqs))
	}
}
