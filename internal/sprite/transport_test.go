package sprite

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestNewFallbackTransport(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	mockCLI := &MockSpriteCLI{
		ListFn: func(ctx context.Context) ([]string, error) {
			return []string{"sprite1", "sprite2"}, nil
		},
	}

	transport, err := NewFallbackTransport(mockCLI, "misty-step")
	if err != nil {
		t.Fatalf("NewFallbackTransport() error = %v", err)
	}

	if transport == nil {
		t.Fatal("transport should not be nil")
	}

	// Test that it delegates to CLI
	names, err := transport.List(ctx)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(names) != 2 || names[0] != "sprite1" {
		t.Errorf("List() = %v, want [sprite1 sprite2]", names)
	}

	// Check method was recorded
	if transport.Method() != TransportCLI {
		t.Errorf("Method() = %v, want %v", transport.Method(), TransportCLI)
	}

	// Check metrics were recorded
	metrics := transport.Metrics()
	if metrics.CLICalls != 1 {
		t.Errorf("CLICalls = %d, want 1", metrics.CLICalls)
	}
}

func TestNewFallbackTransportNilCLI(t *testing.T) {
	t.Parallel()

	_, err := NewFallbackTransport(nil, "misty-step")
	if err == nil {
		t.Fatal("expected error for nil CLI")
	}
	if !strings.Contains(err.Error(), "CLI client is required") {
		t.Errorf("error = %v, want 'CLI client is required'", err)
	}
}

func TestFallbackTransportMetrics(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	mockCLI := &MockSpriteCLI{
		ListFn: func(ctx context.Context) ([]string, error) {
			return []string{"sprite1"}, nil
		},
		ExecFn: func(ctx context.Context, sprite, command string, stdin []byte) (string, error) {
			return "output", nil
		},
		CreateFn: func(ctx context.Context, name, org string) error {
			return nil
		},
	}

	transport, _ := NewFallbackTransport(mockCLI, "misty-step")

	// Execute several operations
	_, _ = transport.List(ctx)
	_, _ = transport.Exec(ctx, "sprite1", "echo hello", nil)
	_ = transport.Create(ctx, "newsprite", "misty-step")

	metrics := transport.Metrics()
	if metrics.CLICalls != 3 {
		t.Errorf("CLICalls = %d, want 3", metrics.CLICalls)
	}
	if metrics.CLIErrors != 0 {
		t.Errorf("CLIErrors = %d, want 0", metrics.CLIErrors)
	}
	if metrics.APICalls != 0 {
		t.Errorf("APICalls = %d, want 0", metrics.APICalls)
	}
	if metrics.Fallbacks != 0 {
		t.Errorf("Fallbacks = %d, want 0 (no API available)", metrics.Fallbacks)
	}
}

func TestFallbackTransportErrorRecording(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	testErr := errors.New("test error")
	mockCLI := &MockSpriteCLI{
		ListFn: func(ctx context.Context) ([]string, error) {
			return nil, testErr
		},
	}

	transport, _ := NewFallbackTransport(mockCLI, "misty-step")
	_, _ = transport.List(ctx)

	metrics := transport.Metrics()
	if metrics.CLICalls != 1 {
		t.Errorf("CLICalls = %d, want 1", metrics.CLICalls)
	}
	if metrics.CLIErrors != 1 {
		t.Errorf("CLIErrors = %d, want 1", metrics.CLIErrors)
	}
}

func TestFallbackTransportMethodUpdates(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	mockCLI := &MockSpriteCLI{
		ListFn: func(ctx context.Context) ([]string, error) {
			return nil, nil
		},
		UploadFn: func(ctx context.Context, name, remotePath string, content []byte) error {
			return nil
		},
	}

	transport, _ := NewFallbackTransport(mockCLI, "misty-step")

	// Initially no method set
	if transport.Method() != "" {
		t.Errorf("initial Method() = %v, want empty", transport.Method())
	}

	// List sets method to CLI
	_, _ = transport.List(ctx)
	if transport.Method() != TransportCLI {
		t.Errorf("after List, Method() = %v, want %v", transport.Method(), TransportCLI)
	}

	// Upload sets method to CLI
	_ = transport.Upload(ctx, "sprite1", "/tmp/test", []byte("content"))
	if transport.Method() != TransportCLI {
		t.Errorf("after Upload, Method() = %v, want %v", transport.Method(), TransportCLI)
	}
}

func TestFallbackTransportExecWithEnv(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	var capturedEnv map[string]string
	mockCLI := &MockSpriteCLI{
		ExecWithEnvFn: func(ctx context.Context, sprite, command string, stdin []byte, env map[string]string) (string, error) {
			capturedEnv = env
			return "output", nil
		},
	}

	transport, _ := NewFallbackTransport(mockCLI, "misty-step")

	env := map[string]string{"KEY": "value"}
	_, err := transport.ExecWithEnv(ctx, "sprite1", "echo hello", nil, env)
	if err != nil {
		t.Fatalf("ExecWithEnv() error = %v", err)
	}

	if capturedEnv == nil || capturedEnv["KEY"] != "value" {
		t.Errorf("env not passed correctly, got %v", capturedEnv)
	}
}

func TestFallbackTransportAllMethods(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	mockCLI := &MockSpriteCLI{
		DestroyFn: func(ctx context.Context, name, org string) error {
			return nil
		},
		CheckpointCreateFn: func(ctx context.Context, name, org string) error {
			return nil
		},
		CheckpointListFn: func(ctx context.Context, name, org string) (string, error) {
			return "checkpoints", nil
		},
		UploadFileFn: func(ctx context.Context, name, org, localPath, remotePath string) error {
			return nil
		},
		APIFn: func(ctx context.Context, org, endpoint string) (string, error) {
			return "api response", nil
		},
		APISpriteFn: func(ctx context.Context, org, sprite, endpoint string) (string, error) {
			return "sprite api response", nil
		},
	}

	transport, _ := NewFallbackTransport(mockCLI, "misty-step")

	// Test all methods
	if err := transport.Destroy(ctx, "sprite1", "misty-step"); err != nil {
		t.Errorf("Destroy() error = %v", err)
	}

	if err := transport.CheckpointCreate(ctx, "sprite1", "misty-step"); err != nil {
		t.Errorf("CheckpointCreate() error = %v", err)
	}

	if out, err := transport.CheckpointList(ctx, "sprite1", "misty-step"); err != nil || out != "checkpoints" {
		t.Errorf("CheckpointList() = %v, %v, want checkpoints, nil", out, err)
	}

	if err := transport.UploadFile(ctx, "sprite1", "misty-step", "local", "remote"); err != nil {
		t.Errorf("UploadFile() error = %v", err)
	}

	if out, err := transport.API(ctx, "misty-step", "/test"); err != nil || out != "api response" {
		t.Errorf("API() = %v, %v, want api response, nil", out, err)
	}

	if out, err := transport.APISprite(ctx, "misty-step", "sprite1", "/test"); err != nil || out != "sprite api response" {
		t.Errorf("APISprite() = %v, %v, want sprite api response, nil", out, err)
	}

	// Verify all counted as CLI calls
	metrics := transport.Metrics()
	if metrics.CLICalls != 6 {
		t.Errorf("CLICalls = %d, want 6", metrics.CLICalls)
	}
}
