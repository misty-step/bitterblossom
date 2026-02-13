package sprite

import (
	"context"
	"errors"
	"fmt"
	"net"
	"syscall"
	"testing"
	"time"
)

func TestClassifyError(t *testing.T) {
	tests := []struct {
		name          string
		err           error
		wantKind      TransportErrorKind
		wantRetryable bool
	}{
		{
			name:          "nil error",
			err:           nil,
			wantKind:      "",
			wantRetryable: false,
		},
		{
			name:          "generic error",
			err:           errors.New("something went wrong"),
			wantKind:      "",
			wantRetryable: false,
		},
		{
			name:          "i/o timeout",
			err:           errors.New("read tcp 10.0.0.1:443: i/o timeout"),
			wantKind:      TransportTimeout,
			wantRetryable: true,
		},
		{
			name:          "TLS handshake timeout",
			err:           errors.New("TLS handshake timeout"),
			wantKind:      TransportTimeout,
			wantRetryable: true,
		},
		{
			name:          "connection refused",
			err:           errors.New("dial tcp: connection refused"),
			wantKind:      TransportConnection,
			wantRetryable: true,
		},
		{
			name:          "connection reset",
			err:           errors.New("read: connection reset by peer"),
			wantKind:      TransportIO,
			wantRetryable: true,
		},
		{
			name:          "broken pipe",
			err:           errors.New("write: broken pipe"),
			wantKind:      TransportIO,
			wantRetryable: true,
		},
		{
			name:          "net.Error timeout",
			err:           &testNetError{timeout: true},
			wantKind:      TransportTimeout,
			wantRetryable: true,
		},
		{
			name:          "net.Error temporary (deprecated, no longer retryable)",
			err:           &testNetError{temporary: true},
			wantKind:      "",
			wantRetryable: false,
		},
		{
			name:          "syscall ETIMEDOUT",
			err:           syscall.ETIMEDOUT,
			wantKind:      TransportTimeout,
			wantRetryable: true,
		},
		{
			name:          "syscall ECONNRESET",
			err:           syscall.ECONNRESET,
			wantKind:      TransportIO,
			wantRetryable: true,
		},
		{
			name:          "syscall ECONNREFUSED",
			err:           syscall.ECONNREFUSED,
			wantKind:      TransportConnection,
			wantRetryable: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotKind, gotRetryable := ClassifyError(tt.err)
			if gotKind != tt.wantKind {
				t.Errorf("ClassifyError() gotKind = %v, want %v", gotKind, tt.wantKind)
			}
			if gotRetryable != tt.wantRetryable {
				t.Errorf("ClassifyError() gotRetryable = %v, want %v", gotRetryable, tt.wantRetryable)
			}
		})
	}
}

func TestWithRetry_Success(t *testing.T) {
	cfg := RetryConfig{
		MaxRetries:  3,
		BaseDelay:   10 * time.Millisecond,
		MaxDelay:    100 * time.Millisecond,
		JitterRatio: 0,
	}

	callCount := 0
	fn := func() error {
		callCount++
		return nil
	}

	metrics, err := WithRetry(context.Background(), "test", cfg, fn)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if callCount != 1 {
		t.Errorf("expected 1 call, got %d", callCount)
	}
	if metrics.Attempts != 1 {
		t.Errorf("expected 1 attempt in metrics, got %d", metrics.Attempts)
	}
	if metrics.Retries != 0 {
		t.Errorf("expected 0 retries in metrics, got %d", metrics.Retries)
	}
}

func TestWithRetry_NonRetryableError(t *testing.T) {
	cfg := RetryConfig{
		MaxRetries:  3,
		BaseDelay:   10 * time.Millisecond,
		MaxDelay:    100 * time.Millisecond,
		JitterRatio: 0,
	}

	expectedErr := errors.New("command not found")
	callCount := 0
	fn := func() error {
		callCount++
		return expectedErr
	}

	metrics, err := WithRetry(context.Background(), "test", cfg, fn)
	if err != expectedErr {
		t.Fatalf("expected %v, got %v", expectedErr, err)
	}
	if callCount != 1 {
		t.Errorf("expected 1 call for non-retryable error, got %d", callCount)
	}
	if metrics.Attempts != 1 {
		t.Errorf("expected 1 attempt in metrics, got %d", metrics.Attempts)
	}
}

func TestWithRetry_TransportErrorThenSuccess(t *testing.T) {
	cfg := RetryConfig{
		MaxRetries:  3,
		BaseDelay:   10 * time.Millisecond,
		MaxDelay:    100 * time.Millisecond,
		JitterRatio: 0,
	}

	callCount := 0
	fn := func() error {
		callCount++
		if callCount < 3 {
			return errors.New("i/o timeout")
		}
		return nil
	}

	metrics, err := WithRetry(context.Background(), "test", cfg, fn)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if callCount != 3 {
		t.Errorf("expected 3 calls, got %d", callCount)
	}
	if metrics.Attempts != 3 {
		t.Errorf("expected 3 attempts in metrics, got %d", metrics.Attempts)
	}
	if metrics.Retries != 2 {
		t.Errorf("expected 2 retries in metrics, got %d", metrics.Retries)
	}
	if metrics.TransportErrs[TransportTimeout] != 2 {
		t.Errorf("expected 2 timeout errors in metrics, got %d", metrics.TransportErrs[TransportTimeout])
	}
}

func TestWithRetry_ExhaustedRetries(t *testing.T) {
	cfg := RetryConfig{
		MaxRetries:  2,
		BaseDelay:   10 * time.Millisecond,
		MaxDelay:    100 * time.Millisecond,
		JitterRatio: 0,
	}

	callCount := 0
	fn := func() error {
		callCount++
		return errors.New("i/o timeout")
	}

	metrics, err := WithRetry(context.Background(), "test", cfg, fn)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if callCount != 3 { // initial + 2 retries
		t.Errorf("expected 3 calls, got %d", callCount)
	}

	var transportErr *TransportError
	if !errors.As(err, &transportErr) {
		t.Errorf("expected TransportError, got %T", err)
	}
	if transportErr.Kind != TransportTimeout {
		t.Errorf("expected TransportTimeout, got %v", transportErr.Kind)
	}
	if metrics.Attempts != 3 {
		t.Errorf("expected 3 attempts in metrics, got %d", metrics.Attempts)
	}
}

func TestWithRetry_ContextCancellation(t *testing.T) {
	cfg := RetryConfig{
		MaxRetries:  10,
		BaseDelay:   100 * time.Millisecond,
		MaxDelay:    1 * time.Second,
		JitterRatio: 0,
	}

	ctx, cancel := context.WithCancel(context.Background())
	callCount := 0
	fn := func() error {
		callCount++
		if callCount == 2 {
			cancel()
		}
		return errors.New("i/o timeout")
	}

	_, err := WithRetry(ctx, "test", cfg, fn)
	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got %v", err)
	}
}

func TestCalculateDelay(t *testing.T) {
	tests := []struct {
		name   string
		attempt int
		cfg    RetryConfig
		minDelay time.Duration
		maxDelay time.Duration
	}{
		{
			name:   "first attempt no backoff",
			attempt: 0,
			cfg: RetryConfig{
				BaseDelay:   100 * time.Millisecond,
				MaxDelay:    1 * time.Second,
				JitterRatio: 0,
			},
			minDelay: 100 * time.Millisecond,
			maxDelay: 100 * time.Millisecond,
		},
		{
			name:   "exponential backoff",
			attempt: 2,
			cfg: RetryConfig{
				BaseDelay:   100 * time.Millisecond,
				MaxDelay:    1 * time.Second,
				JitterRatio: 0,
			},
			minDelay: 400 * time.Millisecond,
			maxDelay: 400 * time.Millisecond,
		},
		{
			name:   "max delay cap",
			attempt: 10,
			cfg: RetryConfig{
				BaseDelay:   100 * time.Millisecond,
				MaxDelay:    500 * time.Millisecond,
				JitterRatio: 0,
			},
			minDelay: 500 * time.Millisecond,
			maxDelay: 500 * time.Millisecond,
		},
		{
			name:   "with jitter",
			attempt: 0,
			cfg: RetryConfig{
				BaseDelay:   100 * time.Millisecond,
				MaxDelay:    1 * time.Second,
				JitterRatio: 0.5,
			},
			minDelay: 50 * time.Millisecond,
			maxDelay: 150 * time.Millisecond,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Run multiple times to account for jitter randomness
			for i := 0; i < 100; i++ {
				delay := calculateDelay(tt.attempt, tt.cfg)
				if delay < tt.minDelay || delay > tt.maxDelay {
					t.Errorf("calculateDelay() = %v, want between %v and %v", delay, tt.minDelay, tt.maxDelay)
				}
			}
		})
	}
}

func TestIsTransportError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "nil error",
			err:  nil,
			want: false,
		},
		{
			name: "generic error",
			err:  errors.New("something went wrong"),
			want: false,
		},
		{
			name: "transport error",
			err:  &TransportError{Kind: TransportTimeout},
			want: true,
		},
		{
			name: "wrapped transport error",
			err:  fmt.Errorf("wrapped: %w", &TransportError{Kind: TransportTimeout}),
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsTransportError(tt.err); got != tt.want {
				t.Errorf("IsTransportError() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestTransportErrorKindFrom(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want TransportErrorKind
	}{
		{
			name: "nil error",
			err:  nil,
			want: "",
		},
		{
			name: "generic error",
			err:  errors.New("something went wrong"),
			want: "",
		},
		{
			name: "timeout error",
			err:  &TransportError{Kind: TransportTimeout},
			want: TransportTimeout,
		},
		{
			name: "io error",
			err:  &TransportError{Kind: TransportIO},
			want: TransportIO,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := TransportErrorKindFrom(tt.err); got != tt.want {
				t.Errorf("TransportErrorKindFrom() = %v, want %v", got, tt.want)
			}
		})
	}
}

// testNetError implements net.Error for testing.
type testNetError struct {
	timeout   bool
	temporary bool
	msg       string
}

func (e *testNetError) Error() string {
	if e.msg != "" {
		return e.msg
	}
	return "test net error"
}

func (e *testNetError) Timeout() bool {
	return e.timeout
}

func (e *testNetError) Temporary() bool {
	return e.temporary
}

var _ net.Error = (*testNetError)(nil)