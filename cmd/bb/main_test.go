package main

import (
	"errors"
	"os"
	"strings"
	"testing"
)

func TestWrapTokenExchangeErrUnauthorized(t *testing.T) {
	t.Parallel()

	err := wrapTokenExchangeErr(errors.New("unauthorized"))
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	msg := err.Error()
	if !strings.Contains(msg, "token exchange failed") {
		t.Errorf("err = %q, want to contain %q", msg, "token exchange failed")
	}
	if !strings.Contains(msg, "FLY_API_TOKEN may be expired") {
		t.Errorf("err = %q, want hint about expired FLY_API_TOKEN", msg)
	}
	if !strings.Contains(msg, "fly tokens create") {
		t.Errorf("err = %q, want 'fly tokens create' hint", msg)
	}
	// Original error must be wrapped (unwrappable).
	if !errors.Is(err, errors.New("unauthorized")) {
		// errors.Is won't match a plain errors.New by value; check string instead.
		if !strings.Contains(msg, "unauthorized") {
			t.Errorf("err = %q, want wrapped unauthorized", msg)
		}
	}
}

func TestWrapTokenExchangeErrOther(t *testing.T) {
	t.Parallel()

	err := wrapTokenExchangeErr(errors.New("connection refused"))
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	msg := err.Error()
	if !strings.Contains(msg, "token exchange failed") {
		t.Errorf("err = %q, want to contain %q", msg, "token exchange failed")
	}
	if !strings.Contains(msg, "SPRITES_ORG") {
		t.Errorf("err = %q, want SPRITES_ORG hint for non-unauthorized errors", msg)
	}
	if strings.Contains(msg, "FLY_API_TOKEN may be expired") {
		t.Errorf("err = %q, must NOT contain expired-token hint for non-unauthorized errors", msg)
	}
}

// TestSpriteTokenMissingEnv verifies the error when neither token env var is set.
func TestSpriteTokenMissingEnv(t *testing.T) {
	t.Parallel()

	orig := os.Getenv("FLY_API_TOKEN")
	origSprite := os.Getenv("SPRITE_TOKEN")
	t.Cleanup(func() {
		os.Setenv("FLY_API_TOKEN", orig)
		os.Setenv("SPRITE_TOKEN", origSprite)
	})
	os.Unsetenv("SPRITE_TOKEN")
	os.Unsetenv("FLY_API_TOKEN")

	_, err := spriteToken()
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "FLY_API_TOKEN must be set") {
		t.Errorf("err = %q, want to contain %q", err.Error(), "FLY_API_TOKEN must be set")
	}
}
