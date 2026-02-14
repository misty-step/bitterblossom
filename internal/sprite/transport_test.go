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

	names, err := transport.List(ctx)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(names) != 2 || names[0] != "sprite1" {
		t.Errorf("List() = %v, want [sprite1 sprite2]", names)
	}

	if transport.Method() != TransportCLI {
		t.Errorf("Method() = %v, want %v", transport.Method(), TransportCLI)
	}

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

	transport, err := NewFallbackTransport(mockCLI, "misty-step")
	if err != nil {
		t.Fatalf("NewFallbackTransport() error = %v", err)
	}

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

	transport, err := NewFallbackTransport(mockCLI, "misty-step")
	if err != nil {
		t.Fatalf("NewFallbackTransport() error = %v", err)
	}
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

	transport, err := NewFallbackTransport(mockCLI, "misty-step")
	if err != nil {
		t.Fatalf("NewFallbackTransport() error = %v", err)
	}

	if transport.Method() != "" {
		t.Errorf("initial Method() = %v, want empty", transport.Method())
	}

	_, _ = transport.List(ctx)
	if transport.Method() != TransportCLI {
		t.Errorf("after List, Method() = %v, want %v", transport.Method(), TransportCLI)
	}

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

	transport, err := NewFallbackTransport(mockCLI, "misty-step")
	if err != nil {
		t.Fatalf("NewFallbackTransport() error = %v", err)
	}

	env := map[string]string{"KEY": "value"}
	_, err = transport.ExecWithEnv(ctx, "sprite1", "echo hello", nil, env)
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

	transport, err := NewFallbackTransport(mockCLI, "misty-step")
	if err != nil {
		t.Fatalf("NewFallbackTransport() error = %v", err)
	}

	tests := []struct {
		name    string
		fn      func() error
		wantOut string
	}{
		{"Destroy", func() error {
			return transport.Destroy(ctx, "sprite1", "misty-step")
		}, ""},
		{"CheckpointCreate", func() error {
			return transport.CheckpointCreate(ctx, "sprite1", "misty-step")
		}, ""},
		{"CheckpointList", func() error {
			out, err := transport.CheckpointList(ctx, "sprite1", "misty-step")
			if out != "checkpoints" {
				t.Errorf("CheckpointList() output = %q, want %q", out, "checkpoints")
			}
			return err
		}, "checkpoints"},
		{"UploadFile", func() error {
			return transport.UploadFile(ctx, "sprite1", "misty-step", "local", "remote")
		}, ""},
		{"API", func() error {
			out, err := transport.API(ctx, "misty-step", "/test")
			if out != "api response" {
				t.Errorf("API() output = %q, want %q", out, "api response")
			}
			return err
		}, "api response"},
		{"APISprite", func() error {
			out, err := transport.APISprite(ctx, "misty-step", "sprite1", "/test")
			if out != "sprite api response" {
				t.Errorf("APISprite() output = %q, want %q", out, "sprite api response")
			}
			return err
		}, "sprite api response"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.fn(); err != nil {
				t.Errorf("%s() error = %v", tt.name, err)
			}
		})
	}

	metrics := transport.Metrics()
	if metrics.CLICalls != 6 {
		t.Errorf("CLICalls = %d, want 6", metrics.CLICalls)
	}
}
