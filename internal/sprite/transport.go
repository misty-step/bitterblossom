// Package sprite provides sprite management primitives.
package sprite

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"
)

// TransportMethod indicates which transport mechanism was used.
type TransportMethod string

const (
	// TransportAPI indicates the Sprites API was used.
	TransportAPI TransportMethod = "api"
	// TransportCLI indicates the CLI fallback was used.
	TransportCLI TransportMethod = "cli"
)

// TransportMetrics holds telemetry for transport operations.
type TransportMetrics struct {
	mu sync.RWMutex

	APICalls   int64
	CLICalls   int64
	APIErrors  int64
	CLIErrors  int64
	APILatency time.Duration
	CLILatency time.Duration
	Fallbacks  int64 // Number of times CLI was used as API fallback
}

// Snapshot returns a thread-safe copy of metrics.
func (m *TransportMetrics) Snapshot() TransportMetricsSnapshot {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return TransportMetricsSnapshot{
		APICalls:   m.APICalls,
		CLICalls:   m.CLICalls,
		APIErrors:  m.APIErrors,
		CLIErrors:  m.CLIErrors,
		APILatency: m.APILatency,
		CLILatency: m.CLILatency,
		Fallbacks:  m.Fallbacks,
	}
}

// TransportMetricsSnapshot is an immutable view of transport metrics.
type TransportMetricsSnapshot struct {
	APICalls   int64
	CLICalls   int64
	APIErrors  int64
	CLIErrors  int64
	APILatency time.Duration
	CLILatency time.Duration
	Fallbacks  int64
}

// Transport is the sprite transport abstraction.
// It provides API-first operations with CLI fallback for unsupported features.
type Transport interface {
	SpriteCLI
	// Method returns the transport mechanism used for the last operation.
	Method() TransportMethod
	// Metrics returns transport telemetry.
	Metrics() TransportMetricsSnapshot
}

// FallbackTransport implements Transport with API-first strategy and CLI fallback.
type FallbackTransport struct {
	cli   SpriteCLI
	org   string
	logger *slog.Logger

	mu           sync.RWMutex
	lastMethod   TransportMethod
	metrics      TransportMetrics
}

// FallbackOption configures FallbackTransport.
type FallbackOption func(*FallbackTransport)

// WithLogger sets the logger for transport diagnostics.
func WithLogger(logger *slog.Logger) FallbackOption {
	return func(t *FallbackTransport) {
		if logger != nil {
			t.logger = logger
		}
	}
}

// NewFallbackTransport creates a transport with CLI fallback.
// The CLI is used as primary; future iterations will add API-first.
// This implements the transport interface defined in issue #262.
func NewFallbackTransport(cli SpriteCLI, org string, opts ...FallbackOption) (*FallbackTransport, error) {
	if cli == nil {
		return nil, errors.New("sprite: CLI client is required")
	}

	t := &FallbackTransport{
		cli:    cli,
		org:    strings.TrimSpace(org),
		logger: slog.Default(),
	}
	for _, opt := range opts {
		opt(t)
	}
	return t, nil
}

// Method returns the transport mechanism used for the last operation.
func (t *FallbackTransport) Method() TransportMethod {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.lastMethod
}

// Metrics returns a snapshot of transport telemetry.
func (t *FallbackTransport) Metrics() TransportMetricsSnapshot {
	return t.metrics.Snapshot()
}

func (t *FallbackTransport) setMethod(m TransportMethod) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.lastMethod = m
}

// List returns available sprite names.
// Uses CLI because the API returns machine names, not sprite registry names.
// Rationale: The registry is the source of truth for sprite names.
// Related: #262 - API-first transport with CLI fallback.
func (t *FallbackTransport) List(ctx context.Context) ([]string, error) {
	start := time.Now()
	t.setMethod(TransportCLI)

	names, err := t.cli.List(ctx)

	duration := time.Since(start)
	t.metrics.mu.Lock()
	t.metrics.CLICalls++
	t.metrics.CLILatency += duration
	if err != nil {
		t.metrics.CLIErrors++
	}
	t.metrics.mu.Unlock()

	if err != nil {
		return nil, fmt.Errorf("transport: list sprites: %w", err)
	}
	return names, nil
}

// Exec runs a remote command on a sprite.
func (t *FallbackTransport) Exec(ctx context.Context, sprite, command string, stdin []byte) (string, error) {
	return t.ExecWithEnv(ctx, sprite, command, stdin, nil)
}

// ExecWithEnv runs a remote command with environment variables.
// Uses CLI as the transport mechanism.
// Future: API exec will be used when environment variable support is added.
// Related: #262 - API-first transport with CLI fallback.
func (t *FallbackTransport) ExecWithEnv(ctx context.Context, sprite, command string, stdin []byte, env map[string]string) (string, error) {
	start := time.Now()
	t.setMethod(TransportCLI)

	output, err := t.cli.ExecWithEnv(ctx, sprite, command, stdin, env)

	duration := time.Since(start)
	t.metrics.mu.Lock()
	t.metrics.CLICalls++
	t.metrics.CLILatency += duration
	if err != nil {
		t.metrics.CLIErrors++
	}
	t.metrics.mu.Unlock()

	if err != nil {
		return output, fmt.Errorf("transport: exec: %w", err)
	}
	return output, nil
}

// Upload writes content to a sprite path.
// Uses CLI as the transport mechanism.
// Future: API upload will be used when direct file upload support is added.
// Related: #262 - API-first transport with CLI fallback.
func (t *FallbackTransport) Upload(ctx context.Context, sprite, remotePath string, content []byte) error {
	start := time.Now()
	t.setMethod(TransportCLI)

	err := t.cli.Upload(ctx, sprite, remotePath, content)

	duration := time.Since(start)
	t.metrics.mu.Lock()
	t.metrics.CLICalls++
	t.metrics.CLILatency += duration
	if err != nil {
		t.metrics.CLIErrors++
	}
	t.metrics.mu.Unlock()

	if err != nil {
		return fmt.Errorf("transport: upload: %w", err)
	}
	return nil
}

// Create creates a sprite.
func (t *FallbackTransport) Create(ctx context.Context, name, org string) error {
	start := time.Now()
	t.setMethod(TransportCLI)

	err := t.cli.Create(ctx, name, org)

	duration := time.Since(start)
	t.metrics.mu.Lock()
	t.metrics.CLICalls++
	t.metrics.CLILatency += duration
	if err != nil {
		t.metrics.CLIErrors++
	}
	t.metrics.mu.Unlock()

	if err != nil {
		return fmt.Errorf("transport: create: %w", err)
	}
	return nil
}

// Destroy destroys a sprite.
func (t *FallbackTransport) Destroy(ctx context.Context, name, org string) error {
	start := time.Now()
	t.setMethod(TransportCLI)

	err := t.cli.Destroy(ctx, name, org)

	duration := time.Since(start)
	t.metrics.mu.Lock()
	t.metrics.CLICalls++
	t.metrics.CLILatency += duration
	if err != nil {
		t.metrics.CLIErrors++
	}
	t.metrics.mu.Unlock()

	if err != nil {
		return fmt.Errorf("transport: destroy: %w", err)
	}
	return nil
}

// CheckpointCreate creates a checkpoint for one sprite.
func (t *FallbackTransport) CheckpointCreate(ctx context.Context, name, org string) error {
	start := time.Now()
	t.setMethod(TransportCLI)

	err := t.cli.CheckpointCreate(ctx, name, org)

	duration := time.Since(start)
	t.metrics.mu.Lock()
	t.metrics.CLICalls++
	t.metrics.CLILatency += duration
	if err != nil {
		t.metrics.CLIErrors++
	}
	t.metrics.mu.Unlock()

	if err != nil {
		return fmt.Errorf("transport: checkpoint create: %w", err)
	}
	return nil
}

// CheckpointList lists checkpoints for one sprite.
func (t *FallbackTransport) CheckpointList(ctx context.Context, name, org string) (string, error) {
	start := time.Now()
	t.setMethod(TransportCLI)

	out, err := t.cli.CheckpointList(ctx, name, org)

	duration := time.Since(start)
	t.metrics.mu.Lock()
	t.metrics.CLICalls++
	t.metrics.CLILatency += duration
	if err != nil {
		t.metrics.CLIErrors++
	}
	t.metrics.mu.Unlock()

	if err != nil {
		return out, fmt.Errorf("transport: checkpoint list: %w", err)
	}
	return out, nil
}

// UploadFile uploads one local file to a sprite path.
func (t *FallbackTransport) UploadFile(ctx context.Context, name, org, localPath, remotePath string) error {
	start := time.Now()
	t.setMethod(TransportCLI)

	err := t.cli.UploadFile(ctx, name, org, localPath, remotePath)

	duration := time.Since(start)
	t.metrics.mu.Lock()
	t.metrics.CLICalls++
	t.metrics.CLILatency += duration
	if err != nil {
		t.metrics.CLIErrors++
	}
	t.metrics.mu.Unlock()

	if err != nil {
		return fmt.Errorf("transport: upload file: %w", err)
	}
	return nil
}

// API calls sprite API endpoint in one org.
func (t *FallbackTransport) API(ctx context.Context, org, endpoint string) (string, error) {
	start := time.Now()
	t.setMethod(TransportCLI)

	out, err := t.cli.API(ctx, org, endpoint)

	duration := time.Since(start)
	t.metrics.mu.Lock()
	t.metrics.CLICalls++
	t.metrics.CLILatency += duration
	if err != nil {
		t.metrics.CLIErrors++
	}
	t.metrics.mu.Unlock()

	if err != nil {
		return out, fmt.Errorf("transport: api: %w", err)
	}
	return out, nil
}

// APISprite calls sprite API endpoint scoped to one sprite.
func (t *FallbackTransport) APISprite(ctx context.Context, org, spriteName, endpoint string) (string, error) {
	start := time.Now()
	t.setMethod(TransportCLI)

	out, err := t.cli.APISprite(ctx, org, spriteName, endpoint)

	duration := time.Since(start)
	t.metrics.mu.Lock()
	t.metrics.CLICalls++
	t.metrics.CLILatency += duration
	if err != nil {
		t.metrics.CLIErrors++
	}
	t.metrics.mu.Unlock()

	if err != nil {
		return out, fmt.Errorf("transport: api sprite: %w", err)
	}
	return out, nil
}
