package main

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
)

func TestResolveSpritesOrgPriority(t *testing.T) {
	t.Setenv("SPRITES_ORG", "sprites-org")
	t.Setenv("FLY_ORG", "fly-org")
	if got := resolveSpritesOrg(); got != "sprites-org" {
		t.Fatalf("resolveSpritesOrg() = %q, want %q", got, "sprites-org")
	}

	t.Setenv("SPRITES_ORG", "")
	if got := resolveSpritesOrg(); got != "fly-org" {
		t.Fatalf("resolveSpritesOrg() = %q, want %q", got, "fly-org")
	}

	t.Setenv("FLY_ORG", "")
	if got := resolveSpritesOrg(); got != "personal" {
		t.Fatalf("resolveSpritesOrg() = %q, want %q", got, "personal")
	}
}

func TestFlyAuthTokenFromCLIUsesFirstAvailableBinary(t *testing.T) {
	origLookPath := lookPath
	origRunCommandOutput := runCommandOutput
	t.Cleanup(func() {
		lookPath = origLookPath
		runCommandOutput = origRunCommandOutput
	})

	lookPath = func(bin string) (string, error) {
		if bin == "flyctl" {
			return "/usr/local/bin/flyctl", nil
		}
		return "", errors.New("not found")
	}
	runCommandOutput = func(ctx context.Context, name string, args ...string) ([]byte, error) {
		if name != "flyctl" {
			t.Fatalf("unexpected binary %q", name)
		}
		return []byte("The 'fly auth token' command is deprecated. Use 'fly tokens create' instead.\nfrom-cli-token\n"), nil
	}

	got, err := flyAuthTokenFromCLI(context.Background())
	if err != nil {
		t.Fatalf("flyAuthTokenFromCLI() error = %v", err)
	}
	if got != "from-cli-token" {
		t.Fatalf("flyAuthTokenFromCLI() = %q, want %q", got, "from-cli-token")
	}
}

func TestParseFlyAuthTokenOutput(t *testing.T) {
	token, err := parseFlyAuthTokenOutput([]byte("warning text with spaces\n\nfm2_token_value\n"))
	if err != nil {
		t.Fatalf("parseFlyAuthTokenOutput() error = %v", err)
	}
	if token != "fm2_token_value" {
		t.Fatalf("parseFlyAuthTokenOutput() = %q, want %q", token, "fm2_token_value")
	}
}

func TestSpriteTokenFallsBackToFlyCLI(t *testing.T) {
	origCreate := createSpritesToken
	origLookPath := lookPath
	origRunCommandOutput := runCommandOutput
	t.Cleanup(func() {
		createSpritesToken = origCreate
		lookPath = origLookPath
		runCommandOutput = origRunCommandOutput
	})

	t.Setenv("SPRITE_TOKEN", "")
	t.Setenv("SPRITES_ORG", "misty-step")
	t.Setenv("FLY_API_TOKEN", "FlyV1 bad-env-token")

	exchanges := 0
	createSpritesToken = func(ctx context.Context, flyMacaroon, orgSlug, inviteCode string, apiURL ...string) (string, error) {
		exchanges++
		if flyMacaroon == "bad-env-token" {
			return "", errors.New("unauthorized")
		}
		if flyMacaroon == "good-cli-token" {
			return "sprite-token-from-cli", nil
		}
		return "", errors.New("unexpected token")
	}
	lookPath = func(bin string) (string, error) {
		if bin == "flyctl" {
			return "/usr/local/bin/flyctl", nil
		}
		return "", errors.New("not found")
	}
	runCommandOutput = func(ctx context.Context, name string, args ...string) ([]byte, error) {
		if name != "flyctl" {
			return nil, errors.New("unexpected binary")
		}
		return []byte("good-cli-token\n"), nil
	}

	got, err := spriteToken()
	if err != nil {
		t.Fatalf("spriteToken() error = %v", err)
	}
	if got != "sprite-token-from-cli" {
		t.Fatalf("spriteToken() = %q, want %q", got, "sprite-token-from-cli")
	}
	if exchanges != 2 {
		t.Fatalf("exchange calls = %d, want 2", exchanges)
	}
}

func TestSpriteTokenReturnsErrorWhenNoCredentialsResolve(t *testing.T) {
	origCreate := createSpritesToken
	origLookPath := lookPath
	origRunCommandOutput := runCommandOutput
	t.Cleanup(func() {
		createSpritesToken = origCreate
		lookPath = origLookPath
		runCommandOutput = origRunCommandOutput
	})

	t.Setenv("SPRITE_TOKEN", "")
	t.Setenv("FLY_API_TOKEN", "")
	t.Setenv("SPRITES_ORG", "")
	t.Setenv("FLY_ORG", "")

	lookPath = func(bin string) (string, error) {
		return "", os.ErrNotExist
	}
	runCommandOutput = func(ctx context.Context, name string, args ...string) ([]byte, error) {
		return nil, errors.New("should not be called")
	}
	createSpritesToken = func(ctx context.Context, flyMacaroon, orgSlug, inviteCode string, apiURL ...string) (string, error) {
		return "", errors.New("should not be called")
	}

	_, err := spriteToken()
	if err == nil {
		t.Fatal("expected error when no token sources are available")
	}
	if !strings.Contains(err.Error(), "unable to resolve sprites token") {
		t.Fatalf("err = %q, want to contain %q", err.Error(), "unable to resolve sprites token")
	}
}
