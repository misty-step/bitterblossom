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
	testErr := errors.New("test error")

	// Helper to create a mock CLI with all methods succeeding
	newSuccessMock := func() *MockSpriteCLI {
		return &MockSpriteCLI{
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
	}

	// Helper to create a mock CLI with all methods failing
	newErrorMock := func() *MockSpriteCLI {
		return &MockSpriteCLI{
			DestroyFn: func(ctx context.Context, name, org string) error {
				return testErr
			},
			CheckpointCreateFn: func(ctx context.Context, name, org string) error {
				return testErr
			},
			CheckpointListFn: func(ctx context.Context, name, org string) (string, error) {
				return "", testErr
			},
			UploadFileFn: func(ctx context.Context, name, org, localPath, remotePath string) error {
				return testErr
			},
			APIFn: func(ctx context.Context, org, endpoint string) (string, error) {
				return "", testErr
			},
			APISpriteFn: func(ctx context.Context, org, sprite, endpoint string) (string, error) {
				return "", testErr
			},
		}
	}

	tests := []struct {
		name           string
		mockCLI        *MockSpriteCLI
		wantCalls      int64
		wantErrors     int64
		methodTest     func(*FallbackTransport) (string, error)
		wantOutput     string
		wantErr        bool
		wantErrContain string
	}{
		{
			name:       "Destroy_success",
			mockCLI:    newSuccessMock(),
			wantCalls:  1,
			wantErrors: 0,
			methodTest: func(tr *FallbackTransport) (string, error) {
				return "", tr.Destroy(ctx, "sprite1", "misty-step")
			},
			wantOutput: "",
			wantErr:    false,
		},
		{
			name:           "Destroy_error",
			mockCLI:        newErrorMock(),
			wantCalls:      1,
			wantErrors:     1,
			methodTest:     func(tr *FallbackTransport) (string, error) { return "", tr.Destroy(ctx, "sprite1", "misty-step") },
			wantErr:        true,
			wantErrContain: "transport: destroy",
		},
		{
			name:       "CheckpointCreate_success",
			mockCLI:    newSuccessMock(),
			wantCalls:  1,
			wantErrors: 0,
			methodTest: func(tr *FallbackTransport) (string, error) {
				return "", tr.CheckpointCreate(ctx, "sprite1", "misty-step")
			},
			wantOutput: "",
			wantErr:    false,
		},
		{
			name:           "CheckpointCreate_error",
			mockCLI:        newErrorMock(),
			wantCalls:      1,
			wantErrors:     1,
			methodTest:     func(tr *FallbackTransport) (string, error) { return "", tr.CheckpointCreate(ctx, "sprite1", "misty-step") },
			wantErr:        true,
			wantErrContain: "transport: checkpoint create",
		},
		{
			name:       "CheckpointList_success",
			mockCLI:    newSuccessMock(),
			wantCalls:  1,
			wantErrors: 0,
			methodTest: func(tr *FallbackTransport) (string, error) {
				return tr.CheckpointList(ctx, "sprite1", "misty-step")
			},
			wantOutput: "checkpoints",
			wantErr:    false,
		},
		{
			name:           "CheckpointList_error",
			mockCLI:        newErrorMock(),
			wantCalls:      1,
			wantErrors:     1,
			methodTest:     func(tr *FallbackTransport) (string, error) { return tr.CheckpointList(ctx, "sprite1", "misty-step") },
			wantErr:        true,
			wantErrContain: "transport: checkpoint list",
		},
		{
			name:       "UploadFile_success",
			mockCLI:    newSuccessMock(),
			wantCalls:  1,
			wantErrors: 0,
			methodTest: func(tr *FallbackTransport) (string, error) {
				return "", tr.UploadFile(ctx, "sprite1", "misty-step", "local", "remote")
			},
			wantOutput: "",
			wantErr:    false,
		},
		{
			name:           "UploadFile_error",
			mockCLI:        newErrorMock(),
			wantCalls:      1,
			wantErrors:     1,
			methodTest:     func(tr *FallbackTransport) (string, error) { return "", tr.UploadFile(ctx, "sprite1", "misty-step", "local", "remote") },
			wantErr:        true,
			wantErrContain: "transport: upload file",
		},
		{
			name:       "API_success",
			mockCLI:    newSuccessMock(),
			wantCalls:  1,
			wantErrors: 0,
			methodTest: func(tr *FallbackTransport) (string, error) {
				return tr.API(ctx, "misty-step", "/test")
			},
			wantOutput: "api response",
			wantErr:    false,
		},
		{
			name:           "API_error",
			mockCLI:        newErrorMock(),
			wantCalls:      1,
			wantErrors:     1,
			methodTest:     func(tr *FallbackTransport) (string, error) { return tr.API(ctx, "misty-step", "/test") },
			wantErr:        true,
			wantErrContain: "transport: api",
		},
		{
			name:       "APISprite_success",
			mockCLI:    newSuccessMock(),
			wantCalls:  1,
			wantErrors: 0,
			methodTest: func(tr *FallbackTransport) (string, error) {
				return tr.APISprite(ctx, "misty-step", "sprite1", "/test")
			},
			wantOutput: "sprite api response",
			wantErr:    false,
		},
		{
			name:           "APISprite_error",
			mockCLI:        newErrorMock(),
			wantCalls:      1,
			wantErrors:     1,
			methodTest:     func(tr *FallbackTransport) (string, error) { return tr.APISprite(ctx, "misty-step", "sprite1", "/test") },
			wantErr:        true,
			wantErrContain: "transport: api sprite",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transport, err := NewFallbackTransport(tt.mockCLI, "misty-step")
			if err != nil {
				t.Fatalf("NewFallbackTransport() error = %v", err)
			}

			got, err := tt.methodTest(transport)

			if (err != nil) != tt.wantErr {
				t.Errorf("error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErrContain != "" && (err == nil || !strings.Contains(err.Error(), tt.wantErrContain)) {
				t.Errorf("error = %v, want error containing %q", err, tt.wantErrContain)
			}
			if got != tt.wantOutput {
				t.Errorf("output = %q, want %q", got, tt.wantOutput)
			}

			metrics := transport.Metrics()
			if metrics.CLICalls != tt.wantCalls {
				t.Errorf("CLICalls = %d, want %d", metrics.CLICalls, tt.wantCalls)
			}
			if metrics.CLIErrors != tt.wantErrors {
				t.Errorf("CLIErrors = %d, want %d", metrics.CLIErrors, tt.wantErrors)
			}
		})
	}
}
