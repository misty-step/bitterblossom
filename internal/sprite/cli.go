package sprite

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

const defaultBinary = "sprite"

var (
	// ErrMockNotImplemented indicates no behavior is configured for a mock method.
	ErrMockNotImplemented = errors.New("sprite: mock method not implemented")
)

// SpriteCLI abstracts sprite CLI operations for testability.
type SpriteCLI interface {
	List(ctx context.Context) ([]string, error)
	Exec(ctx context.Context, sprite, command string, stdin []byte) (string, error)
	Create(ctx context.Context, name, org string) error
	Destroy(ctx context.Context, name, org string) error
	CheckpointCreate(ctx context.Context, name, org string) error
	CheckpointList(ctx context.Context, name, org string) (string, error)
	UploadFile(ctx context.Context, name, org, localPath, remotePath string) error
	Upload(ctx context.Context, name, remotePath string, content []byte) error
	API(ctx context.Context, org, endpoint string) (string, error)
	APISprite(ctx context.Context, org, sprite, endpoint string) (string, error)
}

// CLI executes sprite CLI commands.
type CLI struct {
	Binary string
	Org    string
}

// NewCLI creates a CLI adapter. Empty binary falls back to "sprite".
func NewCLI(binary string) CLI {
	return CLI{Binary: strings.TrimSpace(binary)}
}

// NewCLIWithOrg creates a CLI adapter with default org passed via -o.
func NewCLIWithOrg(binary, org string) CLI {
	return CLI{
		Binary: strings.TrimSpace(binary),
		Org:    strings.TrimSpace(org),
	}
}

func (c CLI) command() string {
	if c.Binary == "" {
		return defaultBinary
	}
	return c.Binary
}

func (c CLI) resolvedOrg(org string) string {
	if strings.TrimSpace(org) != "" {
		return strings.TrimSpace(org)
	}
	return strings.TrimSpace(c.Org)
}

func withOrgArgs(base []string, org string) []string {
	if org == "" {
		return base
	}
	// Insert -o before "--" so the flag reaches sprite CLI, not the remote shell.
	out := make([]string, 0, len(base)+2)
	for i, arg := range base {
		if arg == "--" {
			out = append(out, base[:i]...)
			out = append(out, "-o", org)
			out = append(out, base[i:]...)
			return out
		}
	}
	out = append(out, base...)
	out = append(out, "-o", org)
	return out
}

func createArgs(name, org string) []string {
	// sprite CLI uses single-dash long flags (e.g. -skip-console), not GNU-style --flags.
	args := []string{"create", "-skip-console"}
	if org != "" {
		args = append(args, "-o", org)
	}
	args = append(args, name)
	return args
}

func destroyArgs(name, org string) []string {
	// sprite CLI uses single-dash long flags (e.g. -force), not GNU-style --flags.
	args := []string{"destroy", "-force"}
	if org != "" {
		args = append(args, "-o", org)
	}
	args = append(args, name)
	return args
}

func (c CLI) run(ctx context.Context, args []string, stdin []byte) (string, error) {
	cmd := exec.CommandContext(ctx, c.command(), args...)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if len(stdin) > 0 {
		cmd.Stdin = bytes.NewReader(stdin)
	}
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("running sprite %s: %w (%s)", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return stdout.String(), nil
}

// List returns available sprite names.
func (c CLI) List(ctx context.Context) ([]string, error) {
	out, err := c.run(ctx, withOrgArgs([]string{"list"}, c.resolvedOrg("")), nil)
	if err != nil {
		return nil, fmt.Errorf("listing sprites: %w", err)
	}
	lines := strings.Split(out, "\n")
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
	args := withOrgArgs([]string{"exec", "-s", sprite, "--", "bash", "-ceu", remoteCommand}, c.resolvedOrg(""))
	out, err := c.run(ctx, args, stdin)
	if err != nil {
		return "", fmt.Errorf("executing on sprite %q: %w", sprite, err)
	}
	return out, nil
}

// Create creates a sprite in the target org.
func (c CLI) Create(ctx context.Context, name, org string) error {
	_, err := c.run(ctx, createArgs(name, c.resolvedOrg(org)), nil)
	if err != nil {
		return fmt.Errorf("create sprite %q: %w", name, err)
	}
	return nil
}

// Destroy destroys a sprite in the target org.
func (c CLI) Destroy(ctx context.Context, name, org string) error {
	_, err := c.run(ctx, destroyArgs(name, c.resolvedOrg(org)), nil)
	if err != nil {
		return fmt.Errorf("destroy sprite %q: %w", name, err)
	}
	return nil
}

// CheckpointCreate creates a checkpoint for one sprite.
func (c CLI) CheckpointCreate(ctx context.Context, name, org string) error {
	_, err := c.run(ctx, withOrgArgs([]string{"checkpoint", "create", "-s", name}, c.resolvedOrg(org)), nil)
	if err != nil {
		return fmt.Errorf("create checkpoint for sprite %q: %w", name, err)
	}
	return nil
}

// CheckpointList lists checkpoints for one sprite.
func (c CLI) CheckpointList(ctx context.Context, name, org string) (string, error) {
	out, err := c.run(ctx, withOrgArgs([]string{"checkpoint", "list", "-s", name}, c.resolvedOrg(org)), nil)
	if err != nil {
		return "", fmt.Errorf("list checkpoints for sprite %q: %w", name, err)
	}
	return out, nil
}

// UploadFile uploads one local file to a sprite path.
func (c CLI) UploadFile(ctx context.Context, name, org, localPath, remotePath string) error {
	args := withOrgArgs(
		[]string{"exec", "-s", name, "-file", localPath + ":" + remotePath, "--", "echo", "uploaded"},
		c.resolvedOrg(org),
	)
	if _, err := c.run(ctx, args, nil); err != nil {
		return fmt.Errorf("upload %q to sprite %q:%q: %w", localPath, name, remotePath, err)
	}
	return nil
}

// Upload writes content directly to a sprite path.
func (c CLI) Upload(ctx context.Context, name, remotePath string, content []byte) error {
	args := withOrgArgs(
		[]string{"exec", "-s", name, "--", "cat", ">", remotePath},
		c.resolvedOrg(""),
	)
	if _, err := c.run(ctx, args, content); err != nil {
		return fmt.Errorf("upload content to sprite %q:%q: %w", name, remotePath, err)
	}
	return nil
}

// API calls sprite API endpoint in one org.
func (c CLI) API(ctx context.Context, org, endpoint string) (string, error) {
	out, err := c.run(ctx, withOrgArgs([]string{"api", endpoint}, c.resolvedOrg(org)), nil)
	if err != nil {
		return "", fmt.Errorf("sprite api %q: %w", endpoint, err)
	}
	return out, nil
}

// APISprite calls sprite API endpoint scoped to one sprite.
func (c CLI) APISprite(ctx context.Context, org, spriteName, endpoint string) (string, error) {
	out, err := c.run(ctx, withOrgArgs([]string{"api", "-s", spriteName, endpoint}, c.resolvedOrg(org)), nil)
	if err != nil {
		return "", fmt.Errorf("sprite api %q for %q: %w", endpoint, spriteName, err)
	}
	return out, nil
}

// MockSpriteCLI is an injectable fake for tests.
type MockSpriteCLI struct {
	ListFn             func(ctx context.Context) ([]string, error)
	ExecFn             func(ctx context.Context, sprite, command string, stdin []byte) (string, error)
	CreateFn           func(ctx context.Context, name, org string) error
	DestroyFn          func(ctx context.Context, name, org string) error
	CheckpointCreateFn func(ctx context.Context, name, org string) error
	CheckpointListFn   func(ctx context.Context, name, org string) (string, error)
	UploadFileFn       func(ctx context.Context, name, org, localPath, remotePath string) error
	UploadFn           func(ctx context.Context, name, remotePath string, content []byte) error
	APIFn              func(ctx context.Context, org, endpoint string) (string, error)
	APISpriteFn        func(ctx context.Context, org, sprite, endpoint string) (string, error)
}

func (m *MockSpriteCLI) List(ctx context.Context) ([]string, error) {
	if m.ListFn == nil {
		return nil, ErrMockNotImplemented
	}
	return m.ListFn(ctx)
}

func (m *MockSpriteCLI) Exec(ctx context.Context, spriteName, command string, stdin []byte) (string, error) {
	if m.ExecFn == nil {
		return "", ErrMockNotImplemented
	}
	return m.ExecFn(ctx, spriteName, command, stdin)
}

func (m *MockSpriteCLI) Create(ctx context.Context, name, org string) error {
	if m.CreateFn == nil {
		return ErrMockNotImplemented
	}
	return m.CreateFn(ctx, name, org)
}

func (m *MockSpriteCLI) Destroy(ctx context.Context, name, org string) error {
	if m.DestroyFn == nil {
		return ErrMockNotImplemented
	}
	return m.DestroyFn(ctx, name, org)
}

func (m *MockSpriteCLI) CheckpointCreate(ctx context.Context, name, org string) error {
	if m.CheckpointCreateFn == nil {
		return ErrMockNotImplemented
	}
	return m.CheckpointCreateFn(ctx, name, org)
}

func (m *MockSpriteCLI) CheckpointList(ctx context.Context, name, org string) (string, error) {
	if m.CheckpointListFn == nil {
		return "", ErrMockNotImplemented
	}
	return m.CheckpointListFn(ctx, name, org)
}

func (m *MockSpriteCLI) UploadFile(ctx context.Context, name, org, localPath, remotePath string) error {
	if m.UploadFileFn == nil {
		return ErrMockNotImplemented
	}
	return m.UploadFileFn(ctx, name, org, localPath, remotePath)
}

func (m *MockSpriteCLI) Upload(ctx context.Context, name, remotePath string, content []byte) error {
	if m.UploadFn == nil {
		return ErrMockNotImplemented
	}
	return m.UploadFn(ctx, name, remotePath, content)
}

func (m *MockSpriteCLI) API(ctx context.Context, org, endpoint string) (string, error) {
	if m.APIFn == nil {
		return "", ErrMockNotImplemented
	}
	return m.APIFn(ctx, org, endpoint)
}

func (m *MockSpriteCLI) APISprite(ctx context.Context, org, spriteName, endpoint string) (string, error) {
	if m.APISpriteFn == nil {
		return "", ErrMockNotImplemented
	}
	return m.APISpriteFn(ctx, org, spriteName, endpoint)
}
