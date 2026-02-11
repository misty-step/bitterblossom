package main

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"sort"
	"strings"
)

type spriteCLIRemote struct {
	binary string
	org    string
}

func newSpriteCLIRemote(binary, org string) *spriteCLIRemote {
	binary = strings.TrimSpace(binary)
	if binary == "" {
		binary = "sprite"
	}
	return &spriteCLIRemote{
		binary: binary,
		org:    strings.TrimSpace(org),
	}
}

func (r *spriteCLIRemote) List(ctx context.Context) ([]string, error) {
	args := []string{"list"}
	if r.org != "" {
		args = append(args, "-o", r.org)
	}

	command := exec.CommandContext(ctx, r.binary, args...)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	if err := command.Run(); err != nil {
		return nil, fmt.Errorf("sprite list: %w: %s", err, strings.TrimSpace(stderr.String()))
	}

	lines := strings.Split(stdout.String(), "\n")
	result := make([]string, 0, len(lines))
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		result = append(result, trimmed)
	}
	return result, nil
}

func (r *spriteCLIRemote) Exec(ctx context.Context, sprite, remoteCommand string, stdin []byte) (string, error) {
	return r.ExecWithEnv(ctx, sprite, remoteCommand, stdin, nil)
}

func (r *spriteCLIRemote) ExecWithEnv(ctx context.Context, sprite, remoteCommand string, stdin []byte, env map[string]string) (string, error) {
	sprite = strings.TrimSpace(sprite)
	if sprite == "" {
		return "", fmt.Errorf("sprite exec: sprite is required")
	}

	args := []string{"exec"}
	if r.org != "" {
		args = append(args, "-o", r.org)
	}

	args = append(args, buildEnvArgs(env)...)

	args = append(args, "-s", sprite, "--", "bash", "-lc", remoteCommand)

	command := exec.CommandContext(ctx, r.binary, args...)
	if len(stdin) > 0 {
		command.Stdin = bytes.NewReader(stdin)
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	if err := command.Run(); err != nil {
		return strings.TrimSpace(stdout.String()), fmt.Errorf("sprite exec %s: %w: %s", sprite, err, strings.TrimSpace(stderr.String()))
	}
	return stdout.String(), nil
}

func (r *spriteCLIRemote) Upload(ctx context.Context, sprite, remotePath string, content []byte) error {
	command := "cat > " + shellQuote(remotePath)
	_, err := r.Exec(ctx, sprite, command, content)
	if err != nil {
		return fmt.Errorf("sprite upload %s:%s: %w", sprite, remotePath, err)
	}
	return nil
}

// buildEnvArgs returns the CLI args for passing environment variables to the
// sprite CLI. The sprite CLI expects a single -env flag with comma-separated
// KEY=VALUE pairs (not repeated -e flags).
func buildEnvArgs(env map[string]string) []string {
	if len(env) == 0 {
		return nil
	}
	keys := make([]string, 0, len(env))
	for k := range env {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	pairs := make([]string, 0, len(keys))
	for _, k := range keys {
		pairs = append(pairs, k+"="+env[k])
	}
	return []string{"-env", strings.Join(pairs, ",")}
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", `'"'"'`) + "'"
}
