package sprite

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"math/rand"
	"os/exec"
	"sort"
	"strings"
	"time"

	"github.com/misty-step/bitterblossom/internal/shellutil"
)

const defaultBinary = "sprite"

var (
	// ErrMockNotImplemented indicates no behavior is configured for a mock method.
	ErrMockNotImplemented = errors.New("sprite: mock method not implemented")

	// ErrTransportFailure indicates a transient network/transport error that may succeed on retry.
	ErrTransportFailure = errors.New("sprite: transport failure")

	// ErrCommandFailure indicates the command executed but returned a non-zero exit code.
	ErrCommandFailure = errors.New("sprite: command failure")

	// ErrTimeout indicates the operation timed out.
	ErrTimeout = errors.New("sprite: operation timed out")
)

// SpriteCLI abstracts sprite CLI operations for testability.
type SpriteCLI interface {
	List(ctx context.Context) ([]string, error)
	Exec(ctx context.Context, sprite, command string, stdin []byte) (string, error)
	ExecWithEnv(ctx context.Context, sprite, command string, stdin []byte, env map[string]string) (string, error)
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
		return "", fmt.Errorf("running sprite %s: %w (%s)", argsForLog(args), err, strings.TrimSpace(stderr.String()))
	}
	return stdout.String(), nil
}

func argsForLog(args []string) string {
	scrubbed := append([]string(nil), args...)
	for i := 0; i < len(scrubbed)-1; i++ {
		if scrubbed[i] == "-env" {
			scrubbed[i+1] = redactEnvPair(scrubbed[i+1])
			i++
		}
	}
	return strings.Join(scrubbed, " ")
}

func redactEnvPair(pair string) string {
	if idx := strings.Index(pair, "="); idx >= 0 {
		return pair[:idx+1] + "<redacted>"
	}
	return pair
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
	return c.ExecWithEnv(ctx, sprite, remoteCommand, stdin, nil)
}

// ExecWithEnv runs a remote command on one sprite using bash -ceu with environment variables.
func (c CLI) ExecWithEnv(ctx context.Context, sprite, remoteCommand string, stdin []byte, env map[string]string) (string, error) {
	args := []string{"exec", "-s", sprite}

	// Add environment variables using -env flag
	// sprite CLI supports: -env KEY=VALUE (can be specified multiple times)
	if len(env) > 0 {
		keys := make([]string, 0, len(env))
		for k := range env {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			args = append(args, "-env", k+"="+env[k])
		}
	}

	args = append(args, "--", "bash", "-ceu", remoteCommand)
	args = withOrgArgs(args, c.resolvedOrg(""))
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

func uploadFileArgs(name, localPath, remotePath string) ([]string, error) {
	if strings.Contains(localPath, ":") || strings.Contains(remotePath, ":") {
		return nil, fmt.Errorf("colon in path not supported by sprite CLI file transfer protocol: local=%q remote=%q", localPath, remotePath)
	}
	return []string{"exec", "-s", name, "-file", localPath + ":" + remotePath, "--", "true"}, nil
}

// UploadFile uploads one local file to a sprite path.
// Handles exit code 255 gracefully - the file may have uploaded successfully
// even if the post-upload command returns 255 (SSH connection issue).
func (c CLI) UploadFile(ctx context.Context, name, org, localPath, remotePath string) error {
	base, err := uploadFileArgs(name, localPath, remotePath)
	if err != nil {
		return err
	}
	args := withOrgArgs(base, c.resolvedOrg(org))
	if _, err := c.run(ctx, args, nil); err != nil {
		// Check if this is exit code 255 (SSH connection closed), which can
		// occur after successful file upload due to connection timing.
		// Verify the file exists before treating it as a failure.
		if exitErr, ok := err.(interface{ ExitCode() int }); ok && exitErr.ExitCode() == 255 {
			// Verify file was uploaded by checking its existence
			checkArgs := withOrgArgs(
				[]string{"exec", "-s", name, "--", "test", "-f", remotePath},
				c.resolvedOrg(org),
			)
			if _, checkErr := c.run(ctx, checkArgs, nil); checkErr == nil {
				// File exists, upload succeeded despite exit 255
				return nil
			}
		}
		return fmt.Errorf("upload %q to sprite %q:%q: %w", localPath, name, remotePath, err)
	}
	return nil
}

// Upload writes content directly to a sprite path via stdin.
func (c CLI) Upload(ctx context.Context, name, remotePath string, content []byte) error {
	// Use Exec (bash -ceu) so shell redirection works.
	// Direct args like ["cat", ">", path] fail because sprite exec
	// doesn't interpret shell metacharacters without a shell wrapper.
	cmd := "cat > " + shellutil.Quote(remotePath)
	_, err := c.Exec(ctx, name, cmd, content)
	if err != nil {
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

// ClassifyError categorizes a sprite CLI error for handling and retry decisions.
// Returns the classified error type and a boolean indicating if retry may help.
func ClassifyError(err error) (class error, retryable bool) {
	if err == nil {
		return nil, false
	}

	// Context errors are not retryable
	if errors.Is(err, context.DeadlineExceeded) {
		return fmt.Errorf("%w: %v", ErrTimeout, err), false
	}
	if errors.Is(err, context.Canceled) {
		return err, false
	}

	msg := strings.ToLower(err.Error())

	// Transport-level errors are retryable
	transportPatterns := []string{
		"i/o timeout",
		"read tcp",
		"write tcp",
		"connection refused",
		"connection reset",
		"no such host",
		"temporary failure",
		"broken pipe",
		"failed to connect",
	}
	for _, pattern := range transportPatterns {
		if strings.Contains(msg, pattern) {
			return fmt.Errorf("%w: %v", ErrTransportFailure, err), true
		}
	}

	// Exit code errors indicate command failure (not transport failure)
	if strings.Contains(msg, "exit status") || strings.Contains(msg, "exit code") {
		return fmt.Errorf("%w: %v", ErrCommandFailure, err), false
	}

	// Default: unknown error, conservatively non-retryable
	return err, false
}

// IsTransientError reports whether an error is a transient transport error
// that may succeed on retry.
func IsTransientError(err error) bool {
	_, retryable := ClassifyError(err)
	return retryable
}

// ResilientCLI wraps a SpriteCLI with retry logic for transient transport errors.
type ResilientCLI struct {
	inner      SpriteCLI
	maxRetries int
	baseDelay  time.Duration
	maxDelay   time.Duration
	sleep      func(time.Duration)
}

// ResilientOption configures a ResilientCLI.
type ResilientOption func(*ResilientCLI)

// WithMaxRetries sets the maximum number of retry attempts.
func WithMaxRetries(n int) ResilientOption {
	return func(r *ResilientCLI) {
		if n >= 0 {
			r.maxRetries = n
		}
	}
}

// WithBaseDelay sets the initial retry delay.
func WithBaseDelay(d time.Duration) ResilientOption {
	return func(r *ResilientCLI) {
		if d > 0 {
			r.baseDelay = d
		}
	}
}

// WithMaxDelay sets the maximum retry delay (cap for exponential backoff).
func WithMaxDelay(d time.Duration) ResilientOption {
	return func(r *ResilientCLI) {
		if d > 0 {
			r.maxDelay = d
		}
	}
}

// WithSleepFn overrides the sleep function (useful for tests).
func WithSleepFn(fn func(time.Duration)) ResilientOption {
	return func(r *ResilientCLI) {
		if fn != nil {
			r.sleep = fn
		}
	}
}

// NewResilientCLI wraps the given CLI with retry logic.
func NewResilientCLI(inner SpriteCLI, opts ...ResilientOption) *ResilientCLI {
	r := &ResilientCLI{
		inner:      inner,
		maxRetries: 3,
		baseDelay:  250 * time.Millisecond,
		maxDelay:   4 * time.Second,
		sleep:      time.Sleep,
	}
	for _, opt := range opts {
		opt(r)
	}
	return r
}

// jitter adds randomization to backoff to prevent thundering herd.
func jitter(d time.Duration) time.Duration {
	jitter := time.Duration(rand.Int63n(int64(d) / 2))
	return d + jitter
}

// backoffDelay calculates exponential backoff with cap.
func (r *ResilientCLI) backoffDelay(attempt int) time.Duration {
	delay := r.baseDelay
	for i := 0; i < attempt; i++ {
		delay *= 2
		if delay > r.maxDelay {
			delay = r.maxDelay
			break
		}
	}
	return jitter(delay)
}

// runWithRetry executes the given operation with retry logic for transient errors.
func (r *ResilientCLI) runWithRetry(ctx context.Context, op func() (string, error)) (string, error) {
	var lastErr error
	for attempt := 0; attempt <= r.maxRetries; attempt++ {
		if attempt > 0 {
			delay := r.backoffDelay(attempt - 1)
			timer := time.NewTimer(delay)
			select {
			case <-ctx.Done():
				timer.Stop()
				return "", ctx.Err()
			case <-timer.C:
			}
		}

		result, err := op()
		if err == nil {
			return result, nil
		}

		lastErr = err
		class, retryable := ClassifyError(err)
		if !retryable || attempt == r.maxRetries {
			return "", class
		}
		_ = class // classified error not used in retry path
	}
	return "", lastErr
}

// List returns available sprite names with retry.
func (r *ResilientCLI) List(ctx context.Context) ([]string, error) {
	// List doesn't benefit from simple string retry wrapper
	var lastErr error
	for attempt := 0; attempt <= r.maxRetries; attempt++ {
		if attempt > 0 {
			delay := r.backoffDelay(attempt - 1)
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(delay):
			}
		}

		result, err := r.inner.List(ctx)
		if err == nil {
			return result, nil
		}

		lastErr = err
		class, retryable := ClassifyError(err)
		if !retryable || attempt == r.maxRetries {
			return nil, class
		}
		_ = class
	}
	return nil, lastErr
}

// Exec runs a remote command on one sprite with retry.
func (r *ResilientCLI) Exec(ctx context.Context, sprite, command string, stdin []byte) (string, error) {
	return r.runWithRetry(ctx, func() (string, error) {
		return r.inner.Exec(ctx, sprite, command, stdin)
	})
}

// ExecWithEnv runs a remote command with environment variables and retry.
func (r *ResilientCLI) ExecWithEnv(ctx context.Context, sprite, command string, stdin []byte, env map[string]string) (string, error) {
	return r.runWithRetry(ctx, func() (string, error) {
		return r.inner.ExecWithEnv(ctx, sprite, command, stdin, env)
	})
}

// Create creates a sprite with retry.
func (r *ResilientCLI) Create(ctx context.Context, name, org string) error {
	_, err := r.runWithRetry(ctx, func() (string, error) {
		return "", r.inner.Create(ctx, name, org)
	})
	return err
}

// Destroy destroys a sprite with retry.
func (r *ResilientCLI) Destroy(ctx context.Context, name, org string) error {
	_, err := r.runWithRetry(ctx, func() (string, error) {
		return "", r.inner.Destroy(ctx, name, org)
	})
	return err
}

// CheckpointCreate creates a checkpoint with retry.
func (r *ResilientCLI) CheckpointCreate(ctx context.Context, name, org string) error {
	_, err := r.runWithRetry(ctx, func() (string, error) {
		return "", r.inner.CheckpointCreate(ctx, name, org)
	})
	return err
}

// CheckpointList lists checkpoints with retry.
func (r *ResilientCLI) CheckpointList(ctx context.Context, name, org string) (string, error) {
	return r.runWithRetry(ctx, func() (string, error) {
		return r.inner.CheckpointList(ctx, name, org)
	})
}

// UploadFile uploads a file with retry.
func (r *ResilientCLI) UploadFile(ctx context.Context, name, org, localPath, remotePath string) error {
	_, err := r.runWithRetry(ctx, func() (string, error) {
		return "", r.inner.UploadFile(ctx, name, org, localPath, remotePath)
	})
	return err
}

// Upload writes content directly with retry.
func (r *ResilientCLI) Upload(ctx context.Context, name, remotePath string, content []byte) error {
	_, err := r.runWithRetry(ctx, func() (string, error) {
		return "", r.inner.Upload(ctx, name, remotePath, content)
	})
	return err
}

// API calls sprite API endpoint with retry.
func (r *ResilientCLI) API(ctx context.Context, org, endpoint string) (string, error) {
	return r.runWithRetry(ctx, func() (string, error) {
		return r.inner.API(ctx, org, endpoint)
	})
}

// APISprite calls sprite API endpoint scoped to one sprite with retry.
func (r *ResilientCLI) APISprite(ctx context.Context, org, spriteName, endpoint string) (string, error) {
	return r.runWithRetry(ctx, func() (string, error) {
		return r.inner.APISprite(ctx, org, spriteName, endpoint)
	})
}

// MockSpriteCLI is an injectable fake for tests.
type MockSpriteCLI struct {
	ListFn             func(ctx context.Context) ([]string, error)
	ExecFn             func(ctx context.Context, sprite, command string, stdin []byte) (string, error)
	ExecWithEnvFn      func(ctx context.Context, sprite, command string, stdin []byte, env map[string]string) (string, error)
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

func (m *MockSpriteCLI) ExecWithEnv(ctx context.Context, spriteName, command string, stdin []byte, env map[string]string) (string, error) {
	if m.ExecWithEnvFn != nil {
		return m.ExecWithEnvFn(ctx, spriteName, command, stdin, env)
	}
	// Fall back to ExecFn if ExecWithEnvFn is not set
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
