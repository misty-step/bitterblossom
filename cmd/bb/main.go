package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	sprites "github.com/superfly/sprites-go"

	"github.com/spf13/cobra"
)

var (
	version = "dev"
	commit  = "unknown"
	date    = "unknown"
)

type exitError struct {
	Code int
	Err  error
}

func (e *exitError) Error() string {
	if e == nil || e.Err == nil {
		return "command failed"
	}
	return e.Err.Error()
}

func (e *exitError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func main() {
	root := &cobra.Command{
		Use:           "bb",
		Short:         "Bitterblossom — sprite dispatch CLI",
		SilenceUsage:  true,
		SilenceErrors: true,
		Version:       version,
	}

	root.AddCommand(
		newVersionCmd(),
		newDispatchCmd(),
		newSetupCmd(),
		newLogsCmd(),
		newStatusCmd(),
		newKillCmd(),
	)

	if err := root.Execute(); err != nil {
		var coded *exitError
		if errors.As(err, &coded) {
			if coded.Err != nil {
				_, _ = fmt.Fprintln(os.Stderr, coded.Err)
			}
			os.Exit(coded.Code)
		}
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print bb version",
		RunE: func(cmd *cobra.Command, _ []string) error {
			_, err := fmt.Fprintf(cmd.OutOrStdout(), "bb %s (%s, %s)\n", version, commit, date)
			return err
		},
	}
}

// spriteToken returns a bearer token for the Sprites API.
//
// Resolution order:
//  1. SPRITE_TOKEN env var (direct token, used as-is)
//  2. FLY_API_TOKEN env var: exchange with sprites.CreateToken
//     — if exchange returns "unauthorized", fall through to (3)
//  3. ~/.sprites/sprites.json + ~/.sprites/keyring (sprite-cli local auth)
//
// Fails with an actionable error if all sources miss.
func spriteToken() (string, error) {
	if t := os.Getenv("SPRITE_TOKEN"); t != "" {
		return t, nil
	}

	flyToken := os.Getenv("FLY_API_TOKEN")
	if flyToken != "" {
		// Strip "FlyV1 " prefix — CreateToken prepends it
		macaroon := strings.TrimPrefix(flyToken, "FlyV1 ")

		org := os.Getenv("SPRITES_ORG")
		if org == "" {
			org = os.Getenv("FLY_ORG") // fall back to FLY_ORG from .env.bb
		}
		if org == "" {
			org = "personal"
		}

		_, _ = fmt.Fprintf(os.Stderr, "exchanging fly token for sprites token (org=%s)...\n", org)
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		token, err := sprites.CreateToken(ctx, macaroon, org, "")
		if err == nil {
			return token, nil
		}

		// On unauthorized, fall through to sprite-cli local auth.
		if !strings.Contains(strings.ToLower(err.Error()), "unauthorized") {
			return "", tokenExchangeErr(err)
		}
		_, _ = fmt.Fprintf(os.Stderr, "fly token exchange unauthorized; trying sprite-cli local auth...\n")
	}

	// Fallback: sprite-cli local auth (~/.sprites/sprites.json + keyring)
	t, err := spritesJSONToken(spritesDir())
	if err == nil {
		return t, nil
	}

	return "", fmt.Errorf(
		"no valid sprite token found.\n"+
			"  Try one of:\n"+
			"    export SPRITE_TOKEN=<token>\n"+
			"    export FLY_API_TOKEN=$(fly tokens create org -o personal)\n"+
			"    sprite auth login   # then retry",
	)
}

// spritesDir returns the path to the sprite-cli config directory.
// Defaults to ~/.sprites; can be overridden by SPRITES_DIR env var (tests).
func spritesDir() string {
	if d := os.Getenv("SPRITES_DIR"); d != "" {
		return d
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".sprites")
}

// spritesConfig is the minimal shape of ~/.sprites/sprites.json we care about.
type spritesConfig struct {
	CurrentSelection struct {
		URL string `json:"url"`
		Org string `json:"org"`
	} `json:"current_selection"`
	URLs map[string]struct {
		Orgs map[string]struct {
			KeyringKey string `json:"keyring_key"`
		} `json:"orgs"`
	} `json:"urls"`
}

// spritesJSONToken reads the active token from the sprite-cli local auth store.
// It consults sprites.json for the active org/url, then reads the token from
// the file-backed keyring at <spritesDir>/keyring/sprites-cli-manual-tokens/<key-path>.
func spritesJSONToken(dir string) (string, error) {
	if dir == "" {
		return "", fmt.Errorf("sprites config dir unknown")
	}

	cfgPath := filepath.Join(dir, "sprites.json")
	data, err := os.ReadFile(cfgPath)
	if err != nil {
		return "", fmt.Errorf("read %s: %w", cfgPath, err)
	}

	var cfg spritesConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return "", fmt.Errorf("parse %s: %w", cfgPath, err)
	}

	apiURL := cfg.CurrentSelection.URL
	org := cfg.CurrentSelection.Org
	if apiURL == "" || org == "" {
		return "", fmt.Errorf("sprites.json: no active selection")
	}

	// Resolve the keyring key from the per-org config.
	urlEntry, ok := cfg.URLs[apiURL]
	if !ok {
		return "", fmt.Errorf("sprites.json: url %q not found", apiURL)
	}
	orgEntry, ok := urlEntry.Orgs[org]
	if !ok {
		return "", fmt.Errorf("sprites.json: org %q not found for %s", org, apiURL)
	}
	keyringKey := orgEntry.KeyringKey
	if keyringKey == "" {
		return "", fmt.Errorf("sprites.json: no keyring_key for org %q", org)
	}

	token, err := readKeyringFile(dir, keyringKey)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(token), nil
}

// readKeyringFile reads a token from the file-backed keyring.
// The sprite-cli stores tokens as files under:
//
//	<spritesDir>/keyring/sprites-cli-manual-tokens/<key-as-path>
//
// where the key path is derived from the keyring_key by replacing ":" with "-"
// and "//" with "/". Example:
//
//	sprites:org:https://api.sprites.dev:personal
//	→ sprites-org-https-/api.sprites.dev-personal
func readKeyringFile(dir, keyringKey string) (string, error) {
	// Convert keyring key to file path: ":" → "-", then "//" → "/"
	keyPath := strings.ReplaceAll(keyringKey, ":", "-")
	keyPath = strings.ReplaceAll(keyPath, "//", "/")

	path := filepath.Join(dir, "keyring", "sprites-cli-manual-tokens", keyPath)
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("keyring token not found at %s: %w", path, err)
	}
	return string(data), nil
}

// tokenExchangeErr wraps a CreateToken error with an actionable hint.
// Matches "unauthorized" case-insensitively — sprites-go doesn't expose typed
// errors, so string matching is the only detection path. If the SDK changes
// its error text, this degrades to the generic message (not silently wrong).
func tokenExchangeErr(err error) error {
	if strings.Contains(strings.ToLower(err.Error()), "unauthorized") {
		return fmt.Errorf("token exchange failed: %w\nHint: FLY_API_TOKEN may be expired. Try: export FLY_API_TOKEN=$(fly tokens create)", err)
	}
	return fmt.Errorf("token exchange failed: %w", err)
}

// requireEnv returns the value of an environment variable or an error.
func requireEnv(name string) (string, error) {
	v := os.Getenv(name)
	if v == "" {
		return "", fmt.Errorf("%s must be set", name)
	}
	return v, nil
}
