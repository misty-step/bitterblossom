package lib

import (
	"context"
	"fmt"
	"strings"
)

// SpriteCLI wraps sprite CLI invocations.
type SpriteCLI struct {
	Runner     Runner
	Binary     string
	Org        string
	RemoteHome string
}

func NewSpriteCLI(runner Runner, binary, org string) *SpriteCLI {
	if strings.TrimSpace(binary) == "" {
		binary = DefaultSpriteCLI
	}
	if strings.TrimSpace(org) == "" {
		org = DefaultOrg
	}
	return &SpriteCLI{
		Runner:     runner,
		Binary:     binary,
		Org:        org,
		RemoteHome: DefaultRemoteHome,
	}
}

func (s *SpriteCLI) ensureRunner() error {
	if s == nil || s.Runner == nil {
		return fmt.Errorf("sprite runner is not configured")
	}
	return nil
}

func (s *SpriteCLI) Exec(ctx context.Context, sprite string, mutating bool, remoteArgs ...string) (RunResult, error) {
	if err := s.ensureRunner(); err != nil {
		return RunResult{}, err
	}
	args := []string{"exec", "-o", s.Org, "-s", sprite, "--"}
	args = append(args, remoteArgs...)
	return s.Runner.Run(ctx, RunRequest{Cmd: s.Binary, Args: args, Mutating: mutating})
}

func (s *SpriteCLI) ExecWithFile(ctx context.Context, sprite, localPath, remotePath string, mutating bool, remoteArgs ...string) (RunResult, error) {
	if err := s.ensureRunner(); err != nil {
		return RunResult{}, err
	}
	fileArg := fmt.Sprintf("%s:%s", localPath, remotePath)
	args := []string{"exec", "-o", s.Org, "-s", sprite, "-file", fileArg, "--"}
	args = append(args, remoteArgs...)
	return s.Runner.Run(ctx, RunRequest{Cmd: s.Binary, Args: args, Mutating: mutating})
}

func (s *SpriteCLI) List(ctx context.Context) ([]string, error) {
	if err := s.ensureRunner(); err != nil {
		return nil, err
	}
	result, err := s.Runner.Run(ctx, RunRequest{Cmd: s.Binary, Args: []string{"list", "-o", s.Org}})
	if err != nil {
		return nil, err
	}
	var names []string
	for _, line := range strings.Split(result.Stdout, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		names = append(names, line)
	}
	return names, nil
}

func (s *SpriteCLI) Exists(ctx context.Context, sprite string) (bool, error) {
	names, err := s.List(ctx)
	if err != nil {
		return false, err
	}
	for _, name := range names {
		if name == sprite {
			return true, nil
		}
	}
	return false, nil
}

func (s *SpriteCLI) Create(ctx context.Context, sprite string) error {
	if err := s.ensureRunner(); err != nil {
		return err
	}
	_, err := s.Runner.Run(ctx, RunRequest{Cmd: s.Binary, Args: []string{"create", sprite, "-o", s.Org, "--skip-console"}, Mutating: true})
	return err
}

func (s *SpriteCLI) Destroy(ctx context.Context, sprite string, force bool) error {
	if err := s.ensureRunner(); err != nil {
		return err
	}
	args := []string{"destroy", sprite, "-o", s.Org}
	if force {
		args = append(args, "--force")
	}
	_, err := s.Runner.Run(ctx, RunRequest{Cmd: s.Binary, Args: args, Mutating: true})
	return err
}

func (s *SpriteCLI) CheckpointCreate(ctx context.Context, sprite string) error {
	if err := s.ensureRunner(); err != nil {
		return err
	}
	_, err := s.Runner.Run(ctx, RunRequest{Cmd: s.Binary, Args: []string{"checkpoint", "create", "-o", s.Org, "-s", sprite}, Mutating: true})
	return err
}
