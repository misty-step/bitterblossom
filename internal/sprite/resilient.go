package sprite

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
)

// ResilientCLI wraps a SpriteCLI with retry logic for transport errors.
type ResilientCLI struct {
	inner  SpriteCLI
	config RetryConfig
}

// NewResilientCLI creates a CLI wrapper with retry logic.
func NewResilientCLI(inner SpriteCLI) *ResilientCLI {
	return &ResilientCLI{
		inner:  inner,
		config: DefaultRetryConfig(),
	}
}

// NewResilientCLIWithConfig creates a CLI wrapper with custom retry config.
func NewResilientCLIWithConfig(inner SpriteCLI, config RetryConfig) *ResilientCLI {
	return &ResilientCLI{
		inner:  inner,
		config: config,
	}
}

// List delegates to inner CLI (typically doesn't need retry).
func (c *ResilientCLI) List(ctx context.Context) ([]string, error) {
	return c.inner.List(ctx)
}

// Exec runs a command with retry logic for transport errors.
func (c *ResilientCLI) Exec(ctx context.Context, sprite, command string, stdin []byte) (string, error) {
	var result string
	metrics, err := WithRetry(ctx, fmt.Sprintf("exec on %s", sprite), c.config, func() error {
		var innerErr error
		result, innerErr = c.inner.Exec(ctx, sprite, command, stdin)
		return innerErr
	})

	if err != nil {
		return "", c.enhanceError("Exec", sprite, err, metrics)
	}

	c.logSuccess("Exec", sprite, metrics)
	return result, nil
}

// ExecWithEnv runs a command with env and retry logic for transport errors.
func (c *ResilientCLI) ExecWithEnv(ctx context.Context, sprite, command string, stdin []byte, env map[string]string) (string, error) {
	var result string
	metrics, err := WithRetry(ctx, fmt.Sprintf("exec-with-env on %s", sprite), c.config, func() error {
		var innerErr error
		result, innerErr = c.inner.ExecWithEnv(ctx, sprite, command, stdin, env)
		return innerErr
	})

	if err != nil {
		return "", c.enhanceError("ExecWithEnv", sprite, err, metrics)
	}

	c.logSuccess("ExecWithEnv", sprite, metrics)
	return result, nil
}

// Create delegates to inner CLI (typically doesn't need retry).
func (c *ResilientCLI) Create(ctx context.Context, name, org string) error {
	return c.inner.Create(ctx, name, org)
}

// Destroy delegates to inner CLI (typically doesn't need retry).
func (c *ResilientCLI) Destroy(ctx context.Context, name, org string) error {
	return c.inner.Destroy(ctx, name, org)
}

// CheckpointCreate delegates to inner CLI.
func (c *ResilientCLI) CheckpointCreate(ctx context.Context, name, org string) error {
	return c.inner.CheckpointCreate(ctx, name, org)
}

// CheckpointList delegates to inner CLI.
func (c *ResilientCLI) CheckpointList(ctx context.Context, name, org string) (string, error) {
	return c.inner.CheckpointList(ctx, name, org)
}

// UploadFile delegates to inner CLI (already has some retry logic).
func (c *ResilientCLI) UploadFile(ctx context.Context, name, org, localPath, remotePath string) error {
	return c.inner.UploadFile(ctx, name, org, localPath, remotePath)
}

// Upload writes content with retry logic for transport errors.
func (c *ResilientCLI) Upload(ctx context.Context, name, remotePath string, content []byte) error {
	metrics, err := WithRetry(ctx, fmt.Sprintf("upload to %s", name), c.config, func() error {
		return c.inner.Upload(ctx, name, remotePath, content)
	})

	if err != nil {
		return c.enhanceError("Upload", name, err, metrics)
	}

	c.logSuccess("Upload", name, metrics)
	return nil
}

// API delegates to inner CLI.
func (c *ResilientCLI) API(ctx context.Context, org, endpoint string) (string, error) {
	return c.inner.API(ctx, org, endpoint)
}

// APISprite delegates to inner CLI.
func (c *ResilientCLI) APISprite(ctx context.Context, org, spriteName, endpoint string) (string, error) {
	return c.inner.APISprite(ctx, org, spriteName, endpoint)
}

// logSuccess logs successful operations that required retries.
func (c *ResilientCLI) logSuccess(op, sprite string, metrics *RetryMetrics) {
	if metrics.Retries > 0 {
		slog.Debug("sprite operation succeeded after retries",
			slog.String("op", op),
			slog.String("sprite", sprite),
			slog.Int("attempts", metrics.Attempts),
			slog.Int("retries", metrics.Retries),
		)
	}
}

// enhanceError adds retry context to errors.
func (c *ResilientCLI) enhanceError(op, sprite string, err error, metrics *RetryMetrics) error {
	if metrics == nil || metrics.Attempts <= 1 {
		return err
	}

	// Already a transport error - add context
	var transportErr *TransportError
	if errors.As(err, &transportErr) {
		return fmt.Errorf("%s on %s failed after %d attempts (%d retries): %w", op, sprite, metrics.Attempts, metrics.Retries, transportErr)
	}

	// Non-transport error after retries (e.g., context cancelled)
	return fmt.Errorf("%s on %s failed after %d attempts: %w", op, sprite, metrics.Attempts, err)
}