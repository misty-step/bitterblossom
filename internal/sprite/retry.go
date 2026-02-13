package sprite

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math/rand"
	"net"
	"syscall"
	"time"
)

// RetryConfig configures retry behavior for sprite operations.
type RetryConfig struct {
	MaxRetries  int
	BaseDelay   time.Duration
	MaxDelay    time.Duration
	JitterRatio float64 // 0.0-1.0, percentage of delay to add as jitter
}

// DefaultRetryConfig provides sensible defaults for sprite CLI operations.
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxRetries:  3,
		BaseDelay:   500 * time.Millisecond,
		MaxDelay:    5 * time.Second,
		JitterRatio: 0.3,
	}
}

// TransportError represents a retryable network/transport failure.
type TransportError struct {
	Op   string
	Err  error
	Kind TransportErrorKind
}

func (e *TransportError) Error() string {
	return fmt.Sprintf("transport error (%s) during %s: %v", e.Kind, e.Op, e.Err)
}

func (e *TransportError) Unwrap() error {
	return e.Err
}

// TransportErrorKind classifies the type of transport failure.
type TransportErrorKind string

const (
	TransportTimeout     TransportErrorKind = "timeout"
	TransportConnection  TransportErrorKind = "connection"
	TransportIO          TransportErrorKind = "io"
	TransportTemporary   TransportErrorKind = "temporary"
	TransportUnknown     TransportErrorKind = "unknown"
)

// RetryMetrics tracks retry behavior for observability.
type RetryMetrics struct {
	Attempts      int
	Retries       int
	TransportErrs map[TransportErrorKind]int
	LastErr       error
}

// ClassifyError determines if an error is a retryable transport error.
func ClassifyError(err error) (TransportErrorKind, bool) {
	if err == nil {
		return "", false
	}

	// Check for specific error types
	var netErr net.Error
	if errors.As(err, &netErr) {
		if netErr.Timeout() {
			return TransportTimeout, true
		}
		// Note: Temporary() is deprecated but we keep it as a fallback
		// for older Go versions and specific error implementations
	}

	// Check for syscall errors
	var errno syscall.Errno
	if errors.As(err, &errno) {
		switch errno {
		case syscall.ETIMEDOUT, syscall.ECONNRESET, syscall.EPIPE:
			return TransportIO, true
		case syscall.ECONNREFUSED, syscall.ENETUNREACH, syscall.EHOSTUNREACH:
			return TransportConnection, true
		}
	}

	// Check error message patterns
	errStr := err.Error()
	retryablePatterns := []struct {
		pattern string
		kind    TransportErrorKind
	}{
		{"i/o timeout", TransportTimeout},
		{"TLS handshake timeout", TransportTimeout},
		{"connection refused", TransportConnection},
		{"no such host", TransportConnection},
		{"connection reset", TransportIO},
		{"broken pipe", TransportIO},
		{"temporary failure", TransportTemporary},
	}

	for _, rp := range retryablePatterns {
		if containsSubstring(errStr, rp.pattern) {
			return rp.kind, true
		}
	}

	return "", false
}

func containsSubstring(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || contains(s, substr))
}

func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// WithRetry executes the given function with retry logic.
func WithRetry(ctx context.Context, op string, cfg RetryConfig, fn func() error) (*RetryMetrics, error) {
	metrics := &RetryMetrics{
		TransportErrs: make(map[TransportErrorKind]int),
	}

	for attempt := 0; attempt <= cfg.MaxRetries; attempt++ {
		metrics.Attempts = attempt + 1

		err := fn()
		if err == nil {
			return metrics, nil
		}

		metrics.LastErr = err

		// Classify the error
		kind, isTransport := ClassifyError(err)
		if !isTransport {
			// Non-transport errors are not retryable
			return metrics, err
		}

		metrics.TransportErrs[kind]++

		// Last attempt - don't retry
		if attempt >= cfg.MaxRetries {
			slog.Debug("retry exhausted for transport error",
				slog.String("op", op),
				slog.String("kind", string(kind)),
				slog.Int("attempts", metrics.Attempts),
				slog.Any("error", err),
			)
			return metrics, &TransportError{
				Op:   op,
				Err:  err,
				Kind: kind,
			}
		}

		// Calculate delay with jitter
		delay := calculateDelay(attempt, cfg)
		metrics.Retries++

		slog.Debug("retrying after transport error",
			slog.String("op", op),
			slog.String("kind", string(kind)),
			slog.Int("attempt", attempt+1),
			slog.Duration("delay", delay),
		)

		// Wait before retry, respecting context cancellation
		select {
		case <-ctx.Done():
			return metrics, ctx.Err()
		case <-time.After(delay):
			// Continue to next attempt
		}
	}

	return metrics, nil
}

// calculateDelay computes the retry delay with exponential backoff and jitter.
func calculateDelay(attempt int, cfg RetryConfig) time.Duration {
	// Exponential backoff: base * 2^attempt
	delay := cfg.BaseDelay * (1 << attempt)
	if delay > cfg.MaxDelay {
		delay = cfg.MaxDelay
	}

	// Add jitter: +/- jitterRatio * delay
	if cfg.JitterRatio > 0 {
		jitter := time.Duration(float64(delay) * cfg.JitterRatio)
		if jitter > 0 {
			offset := time.Duration(rand.Int63n(int64(2*jitter))) - jitter
			delay += offset
		}
	}

	if delay < 0 {
		return 0
	}
	return delay
}

// IsTransportError checks if an error is a transport error.
func IsTransportError(err error) bool {
	if err == nil {
		return false
	}
	var transportErr *TransportError
	return errors.As(err, &transportErr)
}

// TransportErrorKindFrom returns the kind of transport error, or empty string if not a transport error.
func TransportErrorKindFrom(err error) TransportErrorKind {
	if err == nil {
		return ""
	}
	var transportErr *TransportError
	if errors.As(err, &transportErr) {
		return transportErr.Kind
	}
	return ""
}