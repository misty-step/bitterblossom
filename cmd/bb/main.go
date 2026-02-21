package main

import (
	"context"
	"errors"
	"fmt"
	"os"
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
// Uses SPRITE_TOKEN directly if set, otherwise exchanges FLY_API_TOKEN.
func spriteToken() (string, error) {
	if t := os.Getenv("SPRITE_TOKEN"); t != "" {
		return t, nil
	}

	flyToken := os.Getenv("FLY_API_TOKEN")
	if flyToken == "" {
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
