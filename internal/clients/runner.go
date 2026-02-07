package clients

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
)

// Runner executes external commands.
type Runner interface {
	Run(ctx context.Context, name string, args ...string) (string, int, error)
}

// ExecRunner executes commands with os/exec.
type ExecRunner struct{}

// Run executes a command and returns combined output and exit code.
func (ExecRunner) Run(ctx context.Context, name string, args ...string) (string, int, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	err := cmd.Run()
	output := out.String()
	if err == nil {
		return output, 0, nil
	}

	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return output, exitErr.ExitCode(), fmt.Errorf("%s %v failed: %w", name, args, err)
	}
	return output, -1, fmt.Errorf("%s %v failed: %w", name, args, err)
}
