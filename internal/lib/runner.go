package lib

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strings"
)

// RunRequest describes one external command execution.
type RunRequest struct {
	Cmd      string
	Args     []string
	Dir      string
	Env      []string
	Stdin    string
	Mutating bool
}

// RunResult captures process output.
type RunResult struct {
	Stdout   string
	Stderr   string
	ExitCode int
}

// Runner executes external commands.
type Runner interface {
	Run(ctx context.Context, req RunRequest) (RunResult, error)
}

// ExecRunner uses os/exec with optional dry-run behavior.
type ExecRunner struct {
	Logger *slog.Logger
	DryRun bool
}

func (r *ExecRunner) Run(ctx context.Context, req RunRequest) (RunResult, error) {
	if req.Cmd == "" {
		return RunResult{}, &ValidationError{Field: "command", Message: "must not be empty"}
	}

	if r.DryRun && req.Mutating {
		if r.Logger != nil {
			r.Logger.InfoContext(ctx, "dry-run: skipped mutating command", "cmd", req.Cmd, "args", req.Args)
		}
		return RunResult{}, nil
	}

	cmd := exec.CommandContext(ctx, req.Cmd, req.Args...)
	if req.Dir != "" {
		cmd.Dir = req.Dir
	}
	if len(req.Env) > 0 {
		cmd.Env = append(os.Environ(), req.Env...)
	}
	if req.Stdin != "" {
		cmd.Stdin = strings.NewReader(req.Stdin)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	result := RunResult{Stdout: stdout.String(), Stderr: stderr.String()}
	if cmd.ProcessState != nil {
		result.ExitCode = cmd.ProcessState.ExitCode()
	}
	if err != nil {
		return result, &CommandError{
			Command:  req.Cmd,
			Args:     append([]string(nil), req.Args...),
			ExitCode: result.ExitCode,
			Stdout:   result.Stdout,
			Stderr:   result.Stderr,
			Err:      err,
		}
	}

	if r.Logger != nil {
		r.Logger.DebugContext(ctx, "command succeeded", "cmd", req.Cmd, "args", req.Args)
	}
	return result, nil
}

func FormatCommand(cmd string, args []string) string {
	joined := strings.Join(args, " ")
	if joined == "" {
		return cmd
	}
	return fmt.Sprintf("%s %s", cmd, joined)
}
