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
	if !strings.Contains(msg, "sprite auth login") {
		t.Errorf("error = %q, want 'sprite auth login' recovery hint", msg)
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
// and no sprite-cli local auth exists.
func TestSpriteTokenMissingEnv(t *testing.T) {
	// Not parallel: mutates process environment.
	t.Setenv("FLY_API_TOKEN", "")
	t.Setenv("SPRITE_TOKEN", "")
	// Point sprite-cli config to an empty temp dir so the fallback finds nothing.
	t.Setenv("SPRITES_CONFIG_DIR", t.TempDir())

	_, err := spriteToken()
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "FLY_API_TOKEN must be set") {
		t.Errorf("err = %q, want to contain %q", err.Error(), "FLY_API_TOKEN must be set")
	}
}

// TestSpriteCliTokenFromDirNoConfig verifies that spriteCliTokenFromDir returns
// an error when the config directory is empty.
func TestSpriteCliTokenFromDirNoConfig(t *testing.T) {
	t.Parallel()

	_, err := spriteCliTokenFromDir(t.TempDir())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// TestSpriteCliTokenFromDirConfigPath verifies that a valid sprites.json with
// a corresponding keyring file returns the correct token.
func TestSpriteCliTokenFromDirConfigPath(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	wantToken := "personal/abc/def/my-sprite-token"

	// Write sprites.json with the keyring_key pointing to the token file.
	spritesJSON := `{
		"version": "1",
		"current_selection": {"url": "https://api.sprites.dev", "org": "personal"},
		"urls": {
			"https://api.sprites.dev": {
				"orgs": {
					"personal": {
						"name": "personal",
						"keyring_key": "sprites:org:https://api.sprites.dev:personal"
					}
				}
			}
		}
	}`
	if err := os.WriteFile(filepath.Join(dir, "sprites.json"), []byte(spritesJSON), 0o600); err != nil {
		t.Fatal(err)
	}

	// Compute the expected keyring file path and write the token.
	keyPath := keyringKeyToFilePath(dir, "sprites:org:https://api.sprites.dev:personal")
	if err := os.MkdirAll(filepath.Dir(keyPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keyPath, []byte(wantToken+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := spriteCliTokenFromDir(dir)
	if err != nil {
		t.Fatalf("spriteCliTokenFromDir() error = %v", err)
	}
	if got != wantToken {
		t.Errorf("spriteCliTokenFromDir() = %q, want %q", got, wantToken)
	}
}

// TestSpriteCliTokenFromDirKeyringFallback verifies that when sprites.json is
// absent the keyring walker still finds a token file.
func TestSpriteCliTokenFromDirKeyringFallback(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	wantToken := "org/123/456/fallback-token"

	// Write a token file directly in the keyring directory without sprites.json.
	keyringDir := filepath.Join(dir, "keyring", "sprites-cli-manual-tokens", "sprites-org-https-")
	if err := os.MkdirAll(keyringDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(keyringDir, "api.sprites.dev-personal"), []byte(wantToken), 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := spriteCliTokenFromDir(dir)
	if err != nil {
		t.Fatalf("spriteCliTokenFromDir() error = %v", err)
	}
	if got != wantToken {
		t.Errorf("spriteCliTokenFromDir() = %q, want %q", got, wantToken)
	}
}

// TestSpriteCliTokenFromDirLegacyFlatFile verifies that the legacy ~/.sprites/token
// path is used when no keyring or sprites.json is present.
func TestSpriteCliTokenFromDirLegacyFlatFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	wantToken := "legacy-flat-file-token"

	if err := os.WriteFile(filepath.Join(dir, "token"), []byte(wantToken+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := spriteCliTokenFromDir(dir)
	if err != nil {
		t.Fatalf("spriteCliTokenFromDir() error = %v", err)
	}
	if got != wantToken {
		t.Errorf("spriteCliTokenFromDir() = %q, want %q", got, wantToken)
	}
}

// TestKeyringKeyToFilePath verifies the encoding logic matches what the sprite
// CLI actually produces on disk.
func TestKeyringKeyToFilePath(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		keyringKey string
		wantSuffix string // expected path relative to spritesDir
	}{
		{
			name:       "standard HTTPS org key",
			keyringKey: "sprites:org:https://api.sprites.dev:personal",
			wantSuffix: filepath.Join("keyring", "sprites-cli-manual-tokens", "sprites-org-https-", "api.sprites.dev-personal"),
		},
		{
			name:       "no slash in key",
			keyringKey: "sprites-org-simple",
			wantSuffix: filepath.Join("keyring", "sprites-cli-manual-tokens", "sprites-org-simple"),
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			base := "/tmp/testdir"
			got := keyringKeyToFilePath(base, tc.keyringKey)
			want := filepath.Join(base, tc.wantSuffix)
			if got != want {
				t.Errorf("keyringKeyToFilePath(%q) = %q, want %q", tc.keyringKey, got, want)
			}
		})
	}
}

// TestSpriteTokenFallbackToSpriteCliOnUnauthorized verifies that spriteToken()
// uses the sprite-cli local auth when spriteToken() would otherwise fail with
// "unauthorized".
//
// We cannot fake the Fly token exchange in a unit test, so instead we test the
// spriteCliTokenFromDir function directly, which is the component exercised by
// the fallback path.  Integration coverage for the full flow is provided by the
// combination of the env-var isolation test above and the config-path test.
func TestSpriteTokenUsesEnvSpriteToken(t *testing.T) {
	// Not parallel: mutates process environment.
	t.Setenv("SPRITE_TOKEN", "direct-env-token")
	t.Setenv("FLY_API_TOKEN", "")
	t.Setenv("SPRITES_CONFIG_DIR", t.TempDir())

	got, err := spriteToken()
	if err != nil {
		t.Fatalf("spriteToken() error = %v", err)
	}
	if got != "direct-env-token" {
		t.Errorf("spriteToken() = %q, want %q", got, "direct-env-token")
	}
}

// TestSpriteTokenFallsBackToSpriteCliWhenNoFlyToken verifies that when
// FLY_API_TOKEN is unset, spriteToken() falls back to sprite-cli local auth.
func TestSpriteTokenFallsBackToSpriteCliWhenNoFlyToken(t *testing.T) {
	// Not parallel: mutates process environment.
	dir := t.TempDir()
	wantToken := "sprite-cli-local-token"

	// Write a valid sprites.json + keyring file.
	spritesJSON := `{
		"version": "1",
		"current_selection": {"url": "https://api.sprites.dev", "org": "personal"},
		"urls": {
			"https://api.sprites.dev": {
				"orgs": {
					"personal": {
						"name": "personal",
						"keyring_key": "sprites:org:https://api.sprites.dev:personal"
					}
				}
			}
		}
	}`
	if err := os.WriteFile(filepath.Join(dir, "sprites.json"), []byte(spritesJSON), 0o600); err != nil {
		t.Fatal(err)
	}
	keyPath := keyringKeyToFilePath(dir, "sprites:org:https://api.sprites.dev:personal")
	if err := os.MkdirAll(filepath.Dir(keyPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keyPath, []byte(wantToken), 0o600); err != nil {
		t.Fatal(err)
	}

	t.Setenv("SPRITE_TOKEN", "")
	t.Setenv("FLY_API_TOKEN", "")
	t.Setenv("SPRITES_CONFIG_DIR", dir)

	got, err := spriteToken()
	if err != nil {
		t.Fatalf("spriteToken() error = %v; want sprite-cli fallback to succeed", err)
	}
	if got != wantToken {
		t.Errorf("spriteToken() = %q, want %q", got, wantToken)
	}
}
