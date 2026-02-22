package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
	sprites "github.com/superfly/sprites-go"
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
	if flyToken != "" {
		macaroon := strings.TrimPrefix(flyToken, "FlyV1 ")
		org := os.Getenv("SPRITES_ORG")
		if org == "" {
			org = os.Getenv("FLY_ORG")
		}
		if org == "" {
			org = "personal"
		}
		fmt.Fprintf(os.Stderr, "exchanging FLY_API_TOKEN for sprites token (org=%s)...\n", org)
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if token, err := sprites.CreateToken(ctx, macaroon, org, ""); err == nil {
			return token, nil
		} else {
			fmt.Fprintf(os.Stderr, "FLY_API_TOKEN exchange failed (%v); trying sprite CLI...\n", err)
		}
	} else {
		fmt.Fprint(os.Stderr, "no SPRITE_TOKEN or FLY_API_TOKEN; trying sprite CLI...\n")
	}
	flyTokenCLI, orgCLI, err := getSpriteCLIFlyToken()
	if err != nil {
		return "", fmt.Errorf("SPRITE_TOKEN, FLY_API_TOKEN, or sprite CLI auth required: %w", err)
	}
	macaroon := strings.TrimPrefix(flyTokenCLI, "FlyV1 ")
	fmt.Fprintf(os.Stderr, "exchanging sprite CLI token for sprites token (org=%s)...\n", orgCLI)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	token, err := sprites.CreateToken(ctx, macaroon, orgCLI, "")
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

func getSpriteCLIFlyToken() (string, string, error) {
	home := os.Getenv("HOME")
	if home == "" {
		return "", "", errors.New("HOME unset")
	}
	configPath := filepath.Join(home, ".sprites", "sprites.json")
	data, err := os.ReadFile(configPath)
	if err != nil {
		return "", "", fmt.Errorf("read sprites.json: %w", err)
	}
	var c struct {
		Current struct {
			URL string `json:"url"`
			Org string `json:"org"`
		} `json:"current_selection"`
		URLs map[string]struct {
			Orgs map[string]struct {
				KeyringKey string `json:"keyring_key"`
			} `json:"orgs"`
		} `json:"urls"`
	}
	if err := json.Unmarshal(data, &c); err != nil {
		return "", "", fmt.Errorf("parse sprites.json: %w", err)
	}
	url := c.Current.URL
	org := c.Current.Org
	if url == "" || org == "" {
		return "", "", errors.New("missing current_selection in sprites.json")
	}
	urlEntry, ok := c.URLs[url]
	if !ok {
		return "", "", fmt.Errorf("no %s in urls", url)
	}
	for _, o := range urlEntry.Orgs {
		if o.KeyringKey == "" {
			continue
		}
		cmd := exec.Command("security", "find-generic-password", "-s", o.KeyringKey, "-w")
		cmd.Env = []string{"LC_ALL=C"}
		out, err := cmd.Output()
		if err != nil {
			continue
		}
		flyToken := strings.TrimSpace(string(out))
		if flyToken != "" && strings.HasPrefix(flyToken, "FlyV1 ") {
			return flyToken, org, nil
		}
	}
	return "", "", errors.New("no valid Fly token in sprite CLI keyring")
}
