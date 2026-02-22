package main

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
)

func TestTokenExchangeErrUnauthorizedHint(t *testing.T) {
	t.Parallel()

	err := tokenExchangeErr(fmt.Errorf("unauthorized"))
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	msg := err.Error()
	if !strings.Contains(msg, "FLY_API_TOKEN may be expired") {
		t.Errorf("error = %q, want hint about expired FLY_API_TOKEN", msg)
	}
	if !strings.Contains(msg, "fly tokens create") {
		t.Errorf("error = %q, want 'fly tokens create' hint", msg)
	}
}

func TestTokenExchangeErrUnauthorizedCaseInsensitive(t *testing.T) {
	t.Parallel()

	for _, text := range []string{"Unauthorized", "UNAUTHORIZED", "request unauthorized by server"} {
		err := tokenExchangeErr(fmt.Errorf("%s", text))
		msg := err.Error()
		if !strings.Contains(msg, "FLY_API_TOKEN may be expired") {
			t.Errorf("tokenExchangeErr(%q) = %q, want expired-token hint", text, msg)
		}
	}
}

func TestTokenExchangeErrPreservesWrappedError(t *testing.T) {
	t.Parallel()

	orig := fmt.Errorf("unauthorized")
	wrapped := tokenExchangeErr(orig)

	// %w wrapping must be unwrappable
	if !errors.Is(wrapped, orig) {
		t.Errorf("errors.Is(wrapped, orig) = false; want true")
	}
}

func TestTokenExchangeErrOtherErrorNoOrgHint(t *testing.T) {
	t.Parallel()

	err := tokenExchangeErr(fmt.Errorf("connection refused"))
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	msg := err.Error()
	if strings.Contains(msg, "FLY_API_TOKEN may be expired") {
		t.Errorf("error = %q, should not contain expired hint for non-auth error", msg)
	}
	if strings.Contains(msg, "SPRITES_ORG") {
		t.Errorf("error = %q, should not contain misleading SPRITES_ORG hint for connection error", msg)
	}
	if !strings.Contains(msg, "connection refused") {
		t.Errorf("error = %q, want original error preserved", msg)
	}
}

// TestSpriteTokenMissingEnv verifies the error when neither token env var is set
// and sprite CLI fallback also fails. The new fallback path surfaces a clear error
// that mentions the three auth paths tried.
func TestSpriteTokenMissingEnv(t *testing.T) {
	// Not parallel: mutates process environment.
	t.Setenv("FLY_API_TOKEN", "")
	t.Setenv("SPRITE_TOKEN", "")

	_, err := spriteToken()
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	// The new error message covers all three auth paths (SPRITE_TOKEN, FLY_API_TOKEN, sprite CLI).
	if !strings.Contains(err.Error(), "SPRITE_TOKEN, FLY_API_TOKEN, or sprite CLI auth required") {
		t.Errorf("err = %q, want to contain %q", err.Error(), "SPRITE_TOKEN, FLY_API_TOKEN, or sprite CLI auth required")
	}
}

// TestGetSpriteCLIFlyToken_MissingHome verifies the error when HOME is unset.
func TestGetSpriteCLIFlyToken_MissingHome(t *testing.T) {
	t.Setenv("HOME", "")

	_, _, err := getSpriteCLIFlyToken()
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "HOME unset") {
		t.Errorf("err = %q, want to contain %q", err.Error(), "HOME unset")
	}
}

// TestGetSpriteCLIFlyToken_MissingFile verifies the error when sprites.json doesn't exist.
func TestGetSpriteCLIFlyToken_MissingFile(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	_, _, err := getSpriteCLIFlyToken()
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "read sprites.json") {
		t.Errorf("err = %q, want to contain %q", err.Error(), "read sprites.json")
	}
}

// TestGetSpriteCLIFlyToken_MalformedJSON verifies the error on bad JSON in sprites.json.
func TestGetSpriteCLIFlyToken_MalformedJSON(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	dir := tmp + "/.sprites"
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dir+"/sprites.json", []byte("{bad json"), 0o600); err != nil {
		t.Fatal(err)
	}

	_, _, err := getSpriteCLIFlyToken()
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "parse sprites.json") {
		t.Errorf("err = %q, want to contain %q", err.Error(), "parse sprites.json")
	}
}

// TestGetSpriteCLIFlyToken_MissingCurrentSelection verifies error when current_selection is empty.
func TestGetSpriteCLIFlyToken_MissingCurrentSelection(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	dir := tmp + "/.sprites"
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	content := `{"current_selection":{"url":"","org":""},"urls":{}}`
	if err := os.WriteFile(dir+"/sprites.json", []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	_, _, err := getSpriteCLIFlyToken()
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "missing current_selection") {
		t.Errorf("err = %q, want to contain %q", err.Error(), "missing current_selection")
	}
}
