package sprite

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestResilientCLI_Exec_Success(t *testing.T) {
	mock := &MockSpriteCLI{
		ExecFn: func(ctx context.Context, sprite, command string, stdin []byte) (string, error) {
			return "output", nil
		},
	}

	cli := NewResilientCLI(mock)
	result, err := cli.Exec(context.Background(), "test-sprite", "echo hello", nil)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if result != "output" {
		t.Errorf("expected 'output', got %q", result)
	}
}

func TestResilientCLI_Exec_RetryThenSuccess(t *testing.T) {
	callCount := 0
	mock := &MockSpriteCLI{
		ExecFn: func(ctx context.Context, sprite, command string, stdin []byte) (string, error) {
			callCount++
			if callCount < 2 {
				return "", errors.New("read tcp: i/o timeout")
			}
			return "output", nil
		},
	}

	cli := NewResilientCLIWithConfig(mock, RetryConfig{
		MaxRetries:  3,
		BaseDelay:   10 * time.Millisecond,
		MaxDelay:    100 * time.Millisecond,
		JitterRatio: 0,
	})

	result, err := cli.Exec(context.Background(), "test-sprite", "echo hello", nil)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if result != "output" {
		t.Errorf("expected 'output', got %q", result)
	}
	if callCount != 2 {
		t.Errorf("expected 2 calls, got %d", callCount)
	}
}

func TestResilientCLI_Exec_ExhaustedRetries(t *testing.T) {
	callCount := 0
	mock := &MockSpriteCLI{
		ExecFn: func(ctx context.Context, sprite, command string, stdin []byte) (string, error) {
			callCount++
			return "", errors.New("read tcp: i/o timeout")
		},
	}

	cli := NewResilientCLIWithConfig(mock, RetryConfig{
		MaxRetries:  2,
		BaseDelay:   10 * time.Millisecond,
		MaxDelay:    100 * time.Millisecond,
		JitterRatio: 0,
	})

	_, err := cli.Exec(context.Background(), "test-sprite", "echo hello", nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if callCount != 3 { // initial + 2 retries
		t.Errorf("expected 3 calls, got %d", callCount)
	}

	var transportErr *TransportError
	if !errors.As(err, &transportErr) {
		t.Errorf("expected TransportError in error chain, got %T", err)
	}
}

func TestResilientCLI_Exec_NonRetryableError(t *testing.T) {
	callCount := 0
	expectedErr := errors.New("command not found")
	mock := &MockSpriteCLI{
		ExecFn: func(ctx context.Context, sprite, command string, stdin []byte) (string, error) {
			callCount++
			return "", expectedErr
		},
	}

	cli := NewResilientCLIWithConfig(mock, RetryConfig{
		MaxRetries:  3,
		BaseDelay:   10 * time.Millisecond,
		MaxDelay:    100 * time.Millisecond,
		JitterRatio: 0,
	})

	_, err := cli.Exec(context.Background(), "test-sprite", "echo hello", nil)
	if err != expectedErr {
		t.Fatalf("expected %v, got %v", expectedErr, err)
	}
	if callCount != 1 {
		t.Errorf("expected 1 call for non-retryable error, got %d", callCount)
	}
}

func TestResilientCLI_ExecWithEnv(t *testing.T) {
	callCount := 0
	mock := &MockSpriteCLI{
		ExecWithEnvFn: func(ctx context.Context, sprite, command string, stdin []byte, env map[string]string) (string, error) {
			callCount++
			if callCount < 2 {
				return "", errors.New("i/o timeout")
			}
			return "output", nil
		},
	}

	cli := NewResilientCLIWithConfig(mock, RetryConfig{
		MaxRetries:  3,
		BaseDelay:   10 * time.Millisecond,
		MaxDelay:    100 * time.Millisecond,
		JitterRatio: 0,
	})

	result, err := cli.ExecWithEnv(context.Background(), "test-sprite", "echo hello", nil, map[string]string{"KEY": "value"})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if result != "output" {
		t.Errorf("expected 'output', got %q", result)
	}
}

func TestResilientCLI_Upload(t *testing.T) {
	callCount := 0
	mock := &MockSpriteCLI{
		UploadFn: func(ctx context.Context, name, remotePath string, content []byte) error {
			callCount++
			if callCount < 2 {
				return errors.New("connection reset by peer")
			}
			return nil
		},
	}

	cli := NewResilientCLIWithConfig(mock, RetryConfig{
		MaxRetries:  3,
		BaseDelay:   10 * time.Millisecond,
		MaxDelay:    100 * time.Millisecond,
		JitterRatio: 0,
	})

	err := cli.Upload(context.Background(), "test-sprite", "/path/to/file", []byte("content"))
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if callCount != 2 {
		t.Errorf("expected 2 calls, got %d", callCount)
	}
}

func TestResilientCLI_Delegates(t *testing.T) {
	mock := &MockSpriteCLI{
		ListFn: func(ctx context.Context) ([]string, error) {
			return []string{"sprite1", "sprite2"}, nil
		},
		CreateFn: func(ctx context.Context, name, org string) error {
			return nil
		},
		DestroyFn: func(ctx context.Context, name, org string) error {
			return nil
		},
		CheckpointCreateFn: func(ctx context.Context, name, org string) error {
			return nil
		},
		CheckpointListFn: func(ctx context.Context, name, org string) (string, error) {
			return "checkpoint1\ncheckpoint2", nil
		},
		UploadFileFn: func(ctx context.Context, name, org, localPath, remotePath string) error {
			return nil
		},
		APIFn: func(ctx context.Context, org, endpoint string) (string, error) {
			return `{"sprites":[]}`, nil
		},
		APISpriteFn: func(ctx context.Context, org, sprite, endpoint string) (string, error) {
			return `{"name":"test"}`, nil
		},
	}

	cli := NewResilientCLI(mock)

	// Test List
	list, err := cli.List(context.Background())
	if err != nil {
		t.Fatalf("List: expected no error, got %v", err)
	}
	if len(list) != 2 {
		t.Errorf("List: expected 2 items, got %d", len(list))
	}

	// Test Create
	if err := cli.Create(context.Background(), "test", "org"); err != nil {
		t.Errorf("Create: expected no error, got %v", err)
	}

	// Test Destroy
	if err := cli.Destroy(context.Background(), "test", "org"); err != nil {
		t.Errorf("Destroy: expected no error, got %v", err)
	}

	// Test CheckpointCreate
	if err := cli.CheckpointCreate(context.Background(), "test", "org"); err != nil {
		t.Errorf("CheckpointCreate: expected no error, got %v", err)
	}

	// Test CheckpointList
	checkpoints, err := cli.CheckpointList(context.Background(), "test", "org")
	if err != nil {
		t.Errorf("CheckpointList: expected no error, got %v", err)
	}
	if checkpoints == "" {
		t.Error("CheckpointList: expected non-empty result")
	}

	// Test UploadFile
	if err := cli.UploadFile(context.Background(), "test", "org", "/local", "/remote"); err != nil {
		t.Errorf("UploadFile: expected no error, got %v", err)
	}

	// Test API
	apiResult, err := cli.API(context.Background(), "org", "/sprites")
	if err != nil {
		t.Errorf("API: expected no error, got %v", err)
	}
	if apiResult == "" {
		t.Error("API: expected non-empty result")
	}

	// Test APISprite
	apiSpriteResult, err := cli.APISprite(context.Background(), "org", "test", "/")
	if err != nil {
		t.Errorf("APISprite: expected no error, got %v", err)
	}
	if apiSpriteResult == "" {
		t.Error("APISprite: expected non-empty result")
	}
}