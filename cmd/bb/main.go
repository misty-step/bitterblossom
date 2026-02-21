package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
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
// Priority:
//  1. SPRITE_TOKEN env var (direct, no exchange)
//  2. FLY_API_TOKEN → Fly token exchange
//  3. sprite-cli local auth (~/.sprites/) when exchange returns unauthorized
func spriteToken() (string, error) {
	if t := os.Getenv("SPRITE_TOKEN"); t != "" {
		return t, nil
	}

	flyToken := os.Getenv("FLY_API_TOKEN")
	if flyToken == "" {
		// No Fly token — try sprite-cli local auth before erroring.
		if tok, err := spriteCliToken(); err == nil {
			return tok, nil
		}
		return "", fmt.Errorf("SPRITE_TOKEN or FLY_API_TOKEN must be set")
	}

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
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unauthorized") {
			// Exchange rejected — try sprite-cli local auth before surfacing an error.
			if tok, ferr := spriteCliToken(); ferr == nil {
				_, _ = fmt.Fprintf(os.Stderr, "fly token exchange unauthorized; using sprite-cli local auth\n")
				return tok, nil
			}
		}
		return "", tokenExchangeErr(err)
	}

	return token, nil
}

// tokenExchangeErr wraps a CreateToken error with an actionable hint.
// Matches "unauthorized" case-insensitively — sprites-go doesn't expose typed
// errors, so string matching is the only detection path. If the SDK changes
// its error text, this degrades to the generic message (not silently wrong).
func tokenExchangeErr(err error) error {
	if strings.Contains(strings.ToLower(err.Error()), "unauthorized") {
		return fmt.Errorf(
			"token exchange failed: %w\n"+
				"FLY_API_TOKEN may be expired. Recovery options:\n"+
				"  1. Renew FLY_API_TOKEN:  export FLY_API_TOKEN=$(fly tokens create)\n"+
				"  2. Use sprite-cli auth:  sprite auth login",
			err,
		)
	}
	return fmt.Errorf("token exchange failed: %w", err)
}

// spriteCliConfig is the minimal subset of ~/.sprites/sprites.json we need.
type spriteCliConfig struct {
	Version          string `json:"version"`
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

// spriteCliToken reads the token stored by `sprite auth setup` / `sprite login`
// from the sprite CLI's local config (~/.sprites/).
//
// The sprite CLI persists tokens under ~/.sprites/sprites.json, with the actual
// token value written to a flat file keyring at
// ~/.sprites/keyring/<service>/<encoded-key>. This function resolves the active
// org's keyring key to a token, falling back to a full keyring walk and legacy
// flat-file paths.
//
// If no token is found, it returns a non-nil error; callers should treat that
// as "not logged in via sprite-cli".
func spriteCliToken() (string, error) {
	return spriteCliTokenFromDir(spritesCLIDir())
}

// spritesCLIDir returns the sprite CLI config directory.
// Respects SPRITES_CONFIG_DIR override for testing.
func spritesCLIDir() string {
	if d := os.Getenv("SPRITES_CONFIG_DIR"); d != "" {
		return d
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".sprites")
}

// spriteCliTokenFromDir resolves a sprite CLI token from the given config dir.
// Extracted for testing: callers can pass a temp dir with a known structure.
func spriteCliTokenFromDir(spritesDir string) (string, error) {
	if spritesDir == "" {
		return "", fmt.Errorf("sprite CLI config dir is empty")
	}

	// Primary: read sprites.json, resolve the active org's keyring key.
	if tok, err := spriteCliTokenFromConfig(spritesDir); err == nil && tok != "" {
		return tok, nil
	}

	// Secondary: walk the entire keyring directory for the first readable token.
	if tok, err := spriteCliTokenFromKeyring(spritesDir); err == nil && tok != "" {
		return tok, nil
	}

	// Legacy flat-file paths written by older sprite CLI versions.
	home, _ := os.UserHomeDir()
	for _, candidate := range []string{
		filepath.Join(spritesDir, "token"),
		filepath.Join(home, ".config", "sprites", "token"),
	} {
		if data, err := os.ReadFile(candidate); err == nil {
			if tok := strings.TrimSpace(string(data)); tok != "" {
				return tok, nil
			}
		}
	}

	return "", fmt.Errorf("no sprite-cli token found; run `sprite auth login` or `sprite auth setup --token <token>`")
}

// spriteCliTokenFromConfig reads sprites.json and resolves the active org's
// keyring key to a token file path.
func spriteCliTokenFromConfig(spritesDir string) (string, error) {
	data, err := os.ReadFile(filepath.Join(spritesDir, "sprites.json"))
	if err != nil {
		return "", err
	}

	var cfg spriteCliConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return "", fmt.Errorf("spriteCliTokenFromConfig: malformed sprites.json: %w", err)
	}

	apiURL := cfg.CurrentSelection.URL
	org := cfg.CurrentSelection.Org
	if apiURL == "" || org == "" {
		return "", fmt.Errorf("spriteCliTokenFromConfig: no active selection in sprites.json")
	}

	urlEntry, ok := cfg.URLs[apiURL]
	if !ok {
		return "", fmt.Errorf("spriteCliTokenFromConfig: URL %q not in sprites.json", apiURL)
	}
	orgEntry, ok := urlEntry.Orgs[org]
	if !ok {
		return "", fmt.Errorf("spriteCliTokenFromConfig: org %q not in sprites.json", org)
	}

	keyringKey := orgEntry.KeyringKey
	if keyringKey == "" {
		return "", fmt.Errorf("spriteCliTokenFromConfig: empty keyring_key for org %q", org)
	}

	tokPath := keyringKeyToFilePath(spritesDir, keyringKey)
	data, err = os.ReadFile(tokPath)
	if err != nil {
		return "", fmt.Errorf("spriteCliTokenFromConfig: could not read keyring file %q: %w", tokPath, err)
	}

	tok := strings.TrimSpace(string(data))
	if tok == "" {
		return "", fmt.Errorf("spriteCliTokenFromConfig: keyring file is empty")
	}
	return tok, nil
}

// keyringKeyToFilePath converts a sprite CLI keyring key into its on-disk path.
//
// The sprite CLI file-based keyring (used when no system keyring is available)
// stores tokens at:
//
//	<spritesDir>/keyring/sprites-cli-manual-tokens/<encoded-key>
//
// The encoding replaces ':' with '-' and uses the first '/' in the key as a
// directory separator. For example:
//
//	sprites:org:https://api.sprites.dev:personal
//	→ <spritesDir>/keyring/sprites-cli-manual-tokens/sprites-org-https-/api.sprites.dev-personal
//
// The service name "sprites-cli-manual-tokens" is hard-coded by the sprite CLI.
func keyringKeyToFilePath(spritesDir, keyringKey string) string {
	// Replace colons with hyphens.
	encoded := strings.ReplaceAll(keyringKey, ":", "-")
	// The first '/' in the encoded key acts as a directory boundary.
	dir, file, hasSlash := strings.Cut(encoded, "/")
	if !hasSlash || file == "" {
		return filepath.Join(spritesDir, "keyring", "sprites-cli-manual-tokens", encoded)
	}
	return filepath.Join(spritesDir, "keyring", "sprites-cli-manual-tokens", dir, file)
}

// spriteCliTokenFromKeyring walks <spritesDir>/keyring/sprites-cli-manual-tokens
// and returns the content of the first readable non-empty file.
func spriteCliTokenFromKeyring(spritesDir string) (string, error) {
	keyringDir := filepath.Join(spritesDir, "keyring", "sprites-cli-manual-tokens")
	var found string
	_ = filepath.WalkDir(keyringDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || found != "" {
			return nil
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil
		}
		if tok := strings.TrimSpace(string(data)); tok != "" {
			found = tok
		}
		return nil
	})
	if found == "" {
		return "", fmt.Errorf("spriteCliTokenFromKeyring: no token files found in %s", keyringDir)
	}
	return found, nil
}

// requireEnv returns the value of an environment variable or an error.
func requireEnv(name string) (string, error) {
	v := os.Getenv(name)
	if v == "" {
		return "", fmt.Errorf("%s must be set", name)
	}
	return v, nil
}
