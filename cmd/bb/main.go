package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	sprites "github.com/superfly/sprites-go"

	"github.com/spf13/cobra"
)

var (
	version = "dev"
	commit  = "unknown"
	date    = "unknown"

	createSpritesToken = sprites.CreateToken
	lookPath           = exec.LookPath
	runCommandOutput   = func(ctx context.Context, name string, args ...string) ([]byte, error) {
		return exec.CommandContext(ctx, name, args...).CombinedOutput()
	}
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
// Uses SPRITE_TOKEN directly if set, otherwise exchanges a Fly token.
// Fly token source order:
// 1) FLY_API_TOKEN env var
// 2) `flyctl auth token` / `fly auth token` fallback
func spriteToken() (string, error) {
	if t := strings.TrimSpace(os.Getenv("SPRITE_TOKEN")); t != "" {
		return t, nil
	}

	org := resolveSpritesOrg()
	var failures []string

	if flyToken := strings.TrimSpace(os.Getenv("FLY_API_TOKEN")); flyToken != "" {
		token, err := exchangeFlyTokenForSpritesToken(flyToken, org, "FLY_API_TOKEN")
		if err == nil {
			return token, nil
		}
		failures = append(failures, fmt.Sprintf("FLY_API_TOKEN exchange failed: %v", err))
	}

	cliCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cliFlyToken, err := flyAuthTokenFromCLI(cliCtx)
	if err != nil {
		failures = append(failures, fmt.Sprintf("fly auth token unavailable: %v", err))
	} else {
		token, exErr := exchangeFlyTokenForSpritesToken(cliFlyToken, org, "fly auth token")
		if exErr == nil {
			return token, nil
		}
		failures = append(failures, fmt.Sprintf("fly auth token exchange failed: %v", exErr))
	}

	if len(failures) == 0 {
		return "", fmt.Errorf("SPRITE_TOKEN or FLY_API_TOKEN must be set")
	}
	return "", fmt.Errorf("unable to resolve sprites token; %s", strings.Join(failures, "; "))
}

func resolveSpritesOrg() string {
	org := os.Getenv("SPRITES_ORG")
	if org == "" {
		org = os.Getenv("FLY_ORG") // fall back to FLY_ORG from .env.bb
	}
	if org == "" {
		org = "personal"
	}
	return org
}

func exchangeFlyTokenForSpritesToken(flyToken, org, source string) (string, error) {
	macaroon := strings.TrimSpace(strings.TrimPrefix(flyToken, "FlyV1 "))
	if macaroon == "" {
		return "", fmt.Errorf("empty fly token")
	}

	_, _ = fmt.Fprintf(os.Stderr, "exchanging fly token for sprites token (org=%s source=%s)...\n", org, source)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	token, err := createSpritesToken(ctx, macaroon, org, "")
	if err != nil {
		return "", fmt.Errorf("token exchange failed: %w (set SPRITES_ORG if not 'personal')", err)
	}
	return token, nil
}

func flyAuthTokenFromCLI(ctx context.Context) (string, error) {
	candidates := []string{"flyctl", "fly"}
	var failures []string
	for _, bin := range candidates {
		if _, err := lookPath(bin); err != nil {
			continue
		}
		out, err := runCommandOutput(ctx, bin, "auth", "token")
		if err != nil {
			failures = append(failures, fmt.Sprintf("%s auth token failed: %v", bin, err))
			continue
		}
		token, parseErr := parseFlyAuthTokenOutput(out)
		if parseErr != nil {
			failures = append(failures, fmt.Sprintf("%s auth token parse failed: %v", bin, parseErr))
			continue
		}
		return token, nil
	}

	if len(failures) == 0 {
		return "", fmt.Errorf("flyctl/fly not found")
	}
	return "", errors.New(strings.Join(failures, "; "))
}

func parseFlyAuthTokenOutput(raw []byte) (string, error) {
	lines := strings.Split(string(raw), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}
		if strings.ContainsAny(line, " \t") {
			continue
		}
		return line, nil
	}
	return "", fmt.Errorf("no token found in fly auth token output")
}

// requireEnv returns the value of an environment variable or an error.
func requireEnv(name string) (string, error) {
	v := os.Getenv(name)
	if v == "" {
		return "", fmt.Errorf("%s must be set", name)
	}
	return v, nil
}
