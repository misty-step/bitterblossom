package main

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestNewTimeoutResult(t *testing.T) {
	t.Parallel()

	start := time.Now().Add(-5 * time.Second)
	result, err := newTimeoutResult(start)

	if err != nil {
		t.Fatalf("expected nil error, got: %v", err)
	}
	if result.State != "timeout" {
		t.Errorf("expected State='timeout', got: %s", result.State)
	}
	if !strings.Contains(result.Error, "timed out after") {
		t.Errorf("expected Error to contain 'timed out after', got: %s", result.Error)
	}
	if result.Runtime == "" {
		t.Error("expected non-empty Runtime")
	}
}

func TestPollSpriteStatus_ProgressMessages(t *testing.T) {
	t.Parallel()

	var progressMessages []string
	progress := func(msg string) {
		progressMessages = append(progressMessages, msg)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	// Create a fake remote that never completes
	remote := newSpriteCLIRemote("fake", "")

	// This should timeout and return
	_, _ = pollSpriteStatus(ctx, remote, "test-sprite", 1*time.Hour, progress)

	foundWaiting := false
	for _, msg := range progressMessages {
		if strings.Contains(msg, "Waiting for") {
			foundWaiting = true
			break
		}
	}

	if !foundWaiting {
		t.Errorf("expected 'Waiting for' message in progress callbacks, got: %v", progressMessages)
	}
}

func TestPollSpriteStatus_TimeoutResult(t *testing.T) {
	t.Parallel()

	var progressMessages []string
	progress := func(msg string) {
		progressMessages = append(progressMessages, msg)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	remote := newSpriteCLIRemote("fake", "")
	result, err := pollSpriteStatus(ctx, remote, "test-sprite", 1*time.Hour, progress)

	if result == nil {
		t.Fatal("expected non-nil result on timeout")
	}
	if result.State != "timeout" {
		t.Errorf("expected State='timeout', got: %s", result.State)
	}
	if !strings.Contains(result.Error, "timed out") {
		t.Errorf("expected Error to contain 'timed out', got: %s", result.Error)
	}
	if result.Runtime == "" {
		t.Error("expected non-empty Runtime on timeout")
	}
	if err != nil {
		t.Errorf("expected nil error (handled internally), got: %v", err)
	}
}
