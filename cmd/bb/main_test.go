package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
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

// TestSpriteTokenMissingEnv verifies the fallback error when no auth source works.
func TestSpriteTokenMissingEnv(t *testing.T) {
	// Not parallel: mutates process environment.
	t.Setenv("FLY_API_TOKEN", "")
	t.Setenv("SPRITE_TOKEN", "")
	t.Setenv("SPRITES_DIR", t.TempDir()) // empty dir → sprites.json missing → fallback fails

	_, err := spriteToken()
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	msg := err.Error()
	// Should mention actionable recovery steps.
	if !strings.Contains(msg, "SPRITE_TOKEN") {
		t.Errorf("err = %q, want actionable hint mentioning SPRITE_TOKEN", msg)
	}
	if !strings.Contains(msg, "FLY_API_TOKEN") {
		t.Errorf("err = %q, want actionable hint mentioning FLY_API_TOKEN", msg)
	}
}

// TestSpriteTokenDirectEnv verifies SPRITE_TOKEN is used as-is (happy path).
func TestSpriteTokenDirectEnv(t *testing.T) {
	// Not parallel: mutates process environment.
	t.Setenv("SPRITE_TOKEN", "direct-token-value")

	token, err := spriteToken()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if token != "direct-token-value" {
		t.Errorf("token = %q, want %q", token, "direct-token-value")
	}
}

// TestSpritesJSONTokenFallback verifies the sprites.json + keyring fallback path.
func TestSpritesJSONTokenFallback(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeSpritesFixture(t, dir, "https://api.sprites.dev", "personal", "test-token-from-keyring")

	token, err := spritesJSONToken(dir)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if token != "test-token-from-keyring" {
		t.Errorf("token = %q, want %q", token, "test-token-from-keyring")
	}
}

// TestSpritesJSONTokenFallbackMissingJSON verifies a clear error when sprites.json is absent.
func TestSpritesJSONTokenFallbackMissingJSON(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	_, err := spritesJSONToken(dir)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "sprites.json") {
		t.Errorf("err = %q, want mention of sprites.json", err.Error())
	}
}

// TestSpritesJSONTokenFallbackMissingKeyring verifies a clear error when the keyring file is absent.
func TestSpritesJSONTokenFallbackMissingKeyring(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	// Write sprites.json pointing to an org but don't create the keyring file.
	cfg := `{
  "version": "1",
  "current_selection": {"url": "https://api.sprites.dev", "org": "personal"},
  "urls": {
    "https://api.sprites.dev": {
      "orgs": {
        "personal": {"keyring_key": "sprites:org:https://api.sprites.dev:personal"}
      }
    }
  }
}`
	if err := os.WriteFile(filepath.Join(dir, "sprites.json"), []byte(cfg), 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := spritesJSONToken(dir)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "keyring token not found") {
		t.Errorf("err = %q, want mention of keyring token not found", err.Error())
	}
}

// TestSpriteTokenFallbackToSpritesJSON verifies the full spriteToken() chain falls
// through to sprites.json when no env vars are set.
func TestSpriteTokenFallbackToSpritesJSON(t *testing.T) {
	// Not parallel: mutates process environment.
	dir := t.TempDir()
	writeSpritesFixture(t, dir, "https://api.sprites.dev", "personal", "keyring-fallback-token")

	t.Setenv("SPRITE_TOKEN", "")
	t.Setenv("FLY_API_TOKEN", "")
	t.Setenv("SPRITES_DIR", dir)

	token, err := spriteToken()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if token != "keyring-fallback-token" {
		t.Errorf("token = %q, want %q", token, "keyring-fallback-token")
	}
}

// writeSpritesFixture creates a minimal sprites.json + keyring structure for tests.
func writeSpritesFixture(t *testing.T, dir, apiURL, org, token string) {
	t.Helper()

	cfg := fmt.Sprintf(`{
  "version": "1",
  "current_selection": {"url": %q, "org": %q},
  "urls": {
    %q: {
      "orgs": {
        %q: {"keyring_key": "sprites:org:%s:%s"}
      }
    }
  }
}`, apiURL, org, apiURL, org, apiURL, org)

	if err := os.WriteFile(filepath.Join(dir, "sprites.json"), []byte(cfg), 0o600); err != nil {
		t.Fatal(err)
	}

	// Derive keyring file path from key: ":" → "-", "//" → "/"
	keyringKey := fmt.Sprintf("sprites:org:%s:%s", apiURL, org)
	keyPath := strings.ReplaceAll(keyringKey, ":", "-")
	keyPath = strings.ReplaceAll(keyPath, "//", "/")

	keyringPath := filepath.Join(dir, "keyring", "sprites-cli-manual-tokens", keyPath)
	if err := os.MkdirAll(filepath.Dir(keyringPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keyringPath, []byte(token), 0o600); err != nil {
		t.Fatal(err)
	}
}
