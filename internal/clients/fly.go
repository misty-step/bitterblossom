package clients

import (
	"context"
	"fmt"
)

// FlyClient wraps fly CLI operations used as a fallback path.
type FlyClient interface {
	SSHRun(ctx context.Context, org, app, command string) (string, error)
}

// FlyCLI implements FlyClient.
type FlyCLI struct {
	Bin    string
	Runner Runner
}

// NewFlyCLI builds a FlyCLI.
func NewFlyCLI(r Runner, binary string) *FlyCLI {
	if binary == "" {
		binary = "fly"
	}
	return &FlyCLI{Bin: binary, Runner: r}
}

// SSHRun runs a command via fly ssh console.
func (f *FlyCLI) SSHRun(ctx context.Context, org, app, command string) (string, error) {
	if app == "" {
		return "", fmt.Errorf("app name required")
	}
	args := []string{"ssh", "console", "--app", app, "-C", command}
	if org != "" {
		args = append(args, "--org", org)
	}
	out, _, err := f.Runner.Run(ctx, f.Bin, args...)
	if err != nil {
		return out, err
	}
	return out, nil
}
