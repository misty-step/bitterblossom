package sprite

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
)

const defaultBinary = "sprite"

// CLI executes sprite CLI commands.
type CLI struct {
	Binary string
}

// NewCLI creates a CLI adapter. Empty binary falls back to "sprite".
func NewCLI(binary string) CLI {
	return CLI{Binary: strings.TrimSpace(binary)}
}

func (c CLI) command() string {
	if c.Binary == "" {
		return defaultBinary
	}
	return c.Binary
}

// List returns available sprite names.
func (c CLI) List(ctx context.Context) ([]string, error) {
	cmd := exec.CommandContext(ctx, c.command(), "list")
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("listing sprites: %w (%s)", err, strings.TrimSpace(stderr.String()))
	}
	lines := strings.Split(stdout.String(), "\n")
	names := make([]string, 0, len(lines))
	for _, line := range lines {
		name := strings.TrimSpace(line)
		if name == "" {
			continue
		}
		names = append(names, name)
	}
	return names, nil
}

// Exec runs a remote command on one sprite using bash -ceu.
func (c CLI) Exec(ctx context.Context, sprite, remoteCommand string, stdin []byte) (string, error) {
	args := []string{"exec", "-s", sprite, "--", "bash", "-ceu", remoteCommand}
	cmd := exec.CommandContext(ctx, c.command(), args...)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if len(stdin) > 0 {
		cmd.Stdin = bytes.NewReader(stdin)
	}
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("executing on sprite %q: %w (%s)", sprite, err, strings.TrimSpace(stderr.String()))
	}
	return stdout.String(), nil
}
