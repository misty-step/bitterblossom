package main

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/misty-step/bitterblossom/internal/sprite"
)

// spriteCLIRemote wraps the internal sprite CLI with resilient retry logic.
type spriteCLIRemote struct {
	inner sprite.SpriteCLI
}

func newSpriteCLIRemote(binary, org string) *spriteCLIRemote {
	binary = strings.TrimSpace(binary)
	if binary == "" {
		binary = "sprite"
	}
	// Create base CLI and wrap with resilient retry logic for transport errors
	base := sprite.NewCLIWithOrg(binary, org)
	resilient := sprite.NewResilientCLI(base)
	return &spriteCLIRemote{inner: resilient}
}

func (r *spriteCLIRemote) List(ctx context.Context) ([]string, error) {
	return r.inner.List(ctx)
}

func (r *spriteCLIRemote) Exec(ctx context.Context, spriteName, remoteCommand string, stdin []byte) (string, error) {
	return r.inner.Exec(ctx, spriteName, remoteCommand, stdin)
}

func (r *spriteCLIRemote) ExecWithEnv(ctx context.Context, spriteName, remoteCommand string, stdin []byte, env map[string]string) (string, error) {
	return r.inner.ExecWithEnv(ctx, spriteName, remoteCommand, stdin, env)
}

func (r *spriteCLIRemote) Upload(ctx context.Context, spriteName, remotePath string, content []byte) error {
	return r.inner.Upload(ctx, spriteName, remotePath, content)
}

// ProbeConnectivity checks if a sprite is reachable with a short timeout.
// Uses a 5-second timeout to fail fast on unreachable sprites.
func (r *spriteCLIRemote) ProbeConnectivity(ctx context.Context, spriteName string) error {
	// Create a 5-second timeout context for the probe
	probeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	// Simple echo command to test connectivity
	_, err := r.inner.Exec(probeCtx, spriteName, "echo ok", nil)
	return err
}

// buildEnvArgs returns the CLI args for passing environment variables to the
// sprite CLI. The sprite CLI expects a single -env flag with comma-separated
// KEY=VALUE pairs. Returns an error if any value contains a comma, since the
// sprite CLI uses commas as delimiters with no escape mechanism.
func buildEnvArgs(env map[string]string) ([]string, error) {
	if len(env) == 0 {
		return nil, nil
	}
	pairs := make([]string, 0, len(env))
	for k, v := range env {
		if strings.Contains(v, ",") {
			return nil, fmt.Errorf("env var %q value contains a comma, which is not supported by the sprite CLI -env flag delimiter", k)
		}
		pairs = append(pairs, k+"="+v)
	}
	sort.Strings(pairs)
	return []string{"-env", strings.Join(pairs, ",")}, nil
}

