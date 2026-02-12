package proxy

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

// mockRemoteExecutor is a mock implementation of RemoteExecutor for testing.
type mockRemoteExecutor struct {
	execFunc        func(ctx context.Context, sprite, remoteCommand string, stdin []byte) (string, error)
	execWithEnvFunc func(ctx context.Context, sprite, remoteCommand string, stdin []byte, env map[string]string) (string, error)
	uploadFunc      func(ctx context.Context, sprite, remotePath string, content []byte) error
	execCalls       []execCall
	uploadCalls     []uploadCall
}

type execCall struct {
	sprite  string
	command string
	stdin   []byte
}

type uploadCall struct {
	sprite string
	path   string
	content []byte
}

func (m *mockRemoteExecutor) Exec(ctx context.Context, sprite, remoteCommand string, stdin []byte) (string, error) {
	m.execCalls = append(m.execCalls, execCall{sprite: sprite, command: remoteCommand, stdin: stdin})
	if m.execFunc != nil {
		return m.execFunc(ctx, sprite, remoteCommand, stdin)
	}
	return "", nil
}

func (m *mockRemoteExecutor) ExecWithEnv(ctx context.Context, sprite, remoteCommand string, stdin []byte, env map[string]string) (string, error) {
	m.execCalls = append(m.execCalls, execCall{sprite: sprite, command: remoteCommand, stdin: stdin})
	if m.execWithEnvFunc != nil {
		return m.execWithEnvFunc(ctx, sprite, remoteCommand, stdin, env)
	}
	return "", nil
}

func (m *mockRemoteExecutor) Upload(ctx context.Context, sprite, remotePath string, content []byte) error {
	m.uploadCalls = append(m.uploadCalls, uploadCall{sprite: sprite, path: remotePath, content: content})
	if m.uploadFunc != nil {
		return m.uploadFunc(ctx, sprite, remotePath, content)
	}
	return nil
}

func TestNewLifecycle(t *testing.T) {
	mock := &mockRemoteExecutor{}
	lifecycle := NewLifecycle(mock)

	if lifecycle.executor != mock {
		t.Error("expected executor to be set")
	}
	if lifecycle.port != SpriteProxyPort {
		t.Errorf("expected port %d, got %d", SpriteProxyPort, lifecycle.port)
	}
	if lifecycle.timeout != DefaultProxyTimeout {
		t.Errorf("expected timeout %v, got %v", DefaultProxyTimeout, lifecycle.timeout)
	}
}

func TestNewLifecycleWithPort(t *testing.T) {
	mock := &mockRemoteExecutor{}
	lifecycle := NewLifecycleWithPort(mock, 5000)

	if lifecycle.port != 5000 {
		t.Errorf("expected port 5000, got %d", lifecycle.port)
	}
}

func TestLifecycle_ProxyURL(t *testing.T) {
	tests := []struct {
		name string
		port int
		want string
	}{
		{
			name: "default port",
			port: 4000,
			want: "http://localhost:4000",
		},
		{
			name: "custom port",
			port: 5000,
			want: "http://localhost:5000",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			lifecycle := &Lifecycle{port: tc.port}
			got := lifecycle.ProxyURL()
			if got != tc.want {
				t.Errorf("ProxyURL() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestLifecycle_IsRunning(t *testing.T) {
	t.Run("proxy is healthy", func(t *testing.T) {
		mock := &mockRemoteExecutor{
			execFunc: func(ctx context.Context, sprite, remoteCommand string, stdin []byte) (string, error) {
				return "200", nil
			},
		}
		lifecycle := NewLifecycle(mock)

		running, err := lifecycle.IsRunning(context.Background(), "test-sprite")
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if !running {
			t.Error("expected IsRunning to return true")
		}
	})

	t.Run("proxy is not healthy", func(t *testing.T) {
		mock := &mockRemoteExecutor{
			execFunc: func(ctx context.Context, sprite, remoteCommand string, stdin []byte) (string, error) {
				return "000", nil
			},
		}
		lifecycle := NewLifecycle(mock)

		running, err := lifecycle.IsRunning(context.Background(), "test-sprite")
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if running {
			t.Error("expected IsRunning to return false")
		}
	})

	t.Run("exec error", func(t *testing.T) {
		mock := &mockRemoteExecutor{
			execFunc: func(ctx context.Context, sprite, remoteCommand string, stdin []byte) (string, error) {
				return "", errors.New("connection refused")
			},
		}
		lifecycle := NewLifecycle(mock)

		_, err := lifecycle.IsRunning(context.Background(), "test-sprite")
		if err == nil {
			t.Error("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "proxy health check failed") {
			t.Errorf("unexpected error message: %v", err)
		}
	})

	t.Run("uses correct health URL", func(t *testing.T) {
		var capturedCommand string
		mock := &mockRemoteExecutor{
			execFunc: func(ctx context.Context, sprite, remoteCommand string, stdin []byte) (string, error) {
				capturedCommand = remoteCommand
				return "200", nil
			},
		}
		lifecycle := NewLifecycle(mock)

		_, _ = lifecycle.IsRunning(context.Background(), "test-sprite")

		if !strings.Contains(capturedCommand, "http://localhost:4000/health") {
			t.Errorf("expected command to contain health URL, got: %s", capturedCommand)
		}
	})
}

func TestLifecycle_Start(t *testing.T) {
	t.Run("successful start", func(t *testing.T) {
		mock := &mockRemoteExecutor{}
		lifecycle := NewLifecycle(mock)

		err := lifecycle.Start(context.Background(), "test-sprite", "test-api-key")
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}

		// Check that mkdir was called
		if len(mock.execCalls) < 1 {
			t.Fatal("expected at least 1 exec call")
		}
		if !strings.Contains(mock.execCalls[0].command, "mkdir -p") {
			t.Errorf("expected mkdir command, got: %s", mock.execCalls[0].command)
		}

		// Check that upload was called
		if len(mock.uploadCalls) != 1 {
			t.Fatalf("expected 1 upload call, got %d", len(mock.uploadCalls))
		}
		if mock.uploadCalls[0].path != SpriteProxyPath {
			t.Errorf("expected upload to %s, got %s", SpriteProxyPath, mock.uploadCalls[0].path)
		}

		// Check that start script was called
		if len(mock.execCalls) < 2 {
			t.Fatal("expected at least 2 exec calls")
		}
		if !strings.Contains(mock.execCalls[1].command, "node") {
			t.Errorf("expected node command, got: %s", mock.execCalls[1].command)
		}
	})

	t.Run("mkdir fails", func(t *testing.T) {
		mock := &mockRemoteExecutor{
			execFunc: func(ctx context.Context, sprite, remoteCommand string, stdin []byte) (string, error) {
				return "", errors.New("permission denied")
			},
		}
		lifecycle := NewLifecycle(mock)

		err := lifecycle.Start(context.Background(), "test-sprite", "test-api-key")
		if err == nil {
			t.Error("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to create .bb directory") {
			t.Errorf("unexpected error message: %v", err)
		}
	})

	t.Run("upload fails", func(t *testing.T) {
		mock := &mockRemoteExecutor{
			uploadFunc: func(ctx context.Context, sprite, remotePath string, content []byte) error {
				return errors.New("disk full")
			},
		}
		lifecycle := NewLifecycle(mock)

		err := lifecycle.Start(context.Background(), "test-sprite", "test-api-key")
		if err == nil {
			t.Error("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to upload proxy script") {
			t.Errorf("unexpected error message: %v", err)
		}
	})

	t.Run("start command fails", func(t *testing.T) {
		callCount := 0
		mock := &mockRemoteExecutor{
			execFunc: func(ctx context.Context, sprite, remoteCommand string, stdin []byte) (string, error) {
				callCount++
				if callCount > 1 { // Second exec call (mkdir is first)
					return "", errors.New("command not found")
				}
				return "", nil
			},
		}
		lifecycle := NewLifecycle(mock)

		err := lifecycle.Start(context.Background(), "test-sprite", "test-api-key")
		if err == nil {
			t.Error("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to start proxy") {
			t.Errorf("unexpected error message: %v", err)
		}
	})
}

func TestLifecycle_Stop(t *testing.T) {
	t.Run("successful stop", func(t *testing.T) {
		mock := &mockRemoteExecutor{}
		lifecycle := NewLifecycle(mock)

		err := lifecycle.Stop(context.Background(), "test-sprite")
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}

		if len(mock.execCalls) != 1 {
			t.Fatalf("expected 1 exec call, got %d", len(mock.execCalls))
		}
		if !strings.Contains(mock.execCalls[0].command, "pkill") && !strings.Contains(mock.execCalls[0].command, "pgrep") {
			t.Errorf("expected kill command, got: %s", mock.execCalls[0].command)
		}
	})

	t.Run("exec error", func(t *testing.T) {
		mock := &mockRemoteExecutor{
			execFunc: func(ctx context.Context, sprite, remoteCommand string, stdin []byte) (string, error) {
				return "", errors.New("connection refused")
			},
		}
		lifecycle := NewLifecycle(mock)

		err := lifecycle.Stop(context.Background(), "test-sprite")
		if err == nil {
			t.Error("expected error, got nil")
		}
	})
}

func TestLifecycle_WaitForHealthy(t *testing.T) {
	t.Run("becomes healthy quickly", func(t *testing.T) {
		callCount := 0
		mock := &mockRemoteExecutor{
			execFunc: func(ctx context.Context, sprite, remoteCommand string, stdin []byte) (string, error) {
				callCount++
				if callCount >= 2 {
					return "200", nil
				}
				return "000", nil
			},
		}
		lifecycle := NewLifecycle(mock)
		lifecycle.SetTimeout(2 * time.Second)

		err := lifecycle.WaitForHealthy(context.Background(), "test-sprite")
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("timeout", func(t *testing.T) {
		mock := &mockRemoteExecutor{
			execFunc: func(ctx context.Context, sprite, remoteCommand string, stdin []byte) (string, error) {
				return "000", nil
			},
		}
		lifecycle := NewLifecycle(mock)
		lifecycle.SetTimeout(200 * time.Millisecond)

		start := time.Now()
		err := lifecycle.WaitForHealthy(context.Background(), "test-sprite")
		elapsed := time.Since(start)

		if err == nil {
			t.Error("expected timeout error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to become healthy") {
			t.Errorf("unexpected error message: %v", err)
		}
		if elapsed < 200*time.Millisecond {
			t.Errorf("returned too early: %v", elapsed)
		}
	})

	t.Run("context cancellation", func(t *testing.T) {
		mock := &mockRemoteExecutor{
			execFunc: func(ctx context.Context, sprite, remoteCommand string, stdin []byte) (string, error) {
				return "000", nil
			},
		}
		lifecycle := NewLifecycle(mock)
		lifecycle.SetTimeout(5 * time.Second)

		ctx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately

		err := lifecycle.WaitForHealthy(ctx, "test-sprite")
		if err == nil {
			t.Error("expected error from cancelled context")
		}
	})
}

func TestLifecycle_EnsureProxy(t *testing.T) {
	t.Run("proxy already running", func(t *testing.T) {
		mock := &mockRemoteExecutor{
			execFunc: func(ctx context.Context, sprite, remoteCommand string, stdin []byte) (string, error) {
				return "200", nil
			},
		}
		lifecycle := NewLifecycle(mock)

		url, err := lifecycle.EnsureProxy(context.Background(), "test-sprite", "test-key")
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if url != "http://localhost:4000" {
			t.Errorf("unexpected URL: %v", url)
		}

		// Should not have uploaded or started anything
		if len(mock.uploadCalls) != 0 {
			t.Error("expected no upload calls when proxy already running")
		}
	})

	t.Run("starts proxy when not running", func(t *testing.T) {
		callCount := 0
		mock := &mockRemoteExecutor{
			execFunc: func(ctx context.Context, sprite, remoteCommand string, stdin []byte) (string, error) {
				callCount++
				// First call (IsRunning) returns not running
				// Second call (mkdir) succeeds
				// Third call (start) succeeds
				// Fourth+ call (WaitForHealthy) returns healthy
				if callCount == 1 {
					return "000", nil
				}
				return "200", nil
			},
		}
		lifecycle := NewLifecycle(mock)
		lifecycle.SetTimeout(2 * time.Second)

		url, err := lifecycle.EnsureProxy(context.Background(), "test-sprite", "test-key")
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if url != "http://localhost:4000" {
			t.Errorf("unexpected URL: %v", url)
		}

		// Should have uploaded the script
		if len(mock.uploadCalls) != 1 {
			t.Errorf("expected 1 upload call, got %d", len(mock.uploadCalls))
		}
	})

	t.Run("start fails", func(t *testing.T) {
		mock := &mockRemoteExecutor{
			execFunc: func(ctx context.Context, sprite, remoteCommand string, stdin []byte) (string, error) {
				// IsRunning fails, but we try to start anyway
				if strings.Contains(remoteCommand, "curl") {
					return "", errors.New("connection refused")
				}
				// mkdir fails
				return "", errors.New("permission denied")
			},
		}
		lifecycle := NewLifecycle(mock)

		_, err := lifecycle.EnsureProxy(context.Background(), "test-sprite", "test-key")
		if err == nil {
			t.Error("expected error when start fails")
		}
	})
}

func TestLifecycle_SetTimeout(t *testing.T) {
	mock := &mockRemoteExecutor{}
	lifecycle := NewLifecycle(mock)

	newTimeout := 30 * time.Second
	lifecycle.SetTimeout(newTimeout)

	if lifecycle.timeout != newTimeout {
		t.Errorf("expected timeout %v, got %v", newTimeout, lifecycle.timeout)
	}
}

func TestBuildStartProxyScript(t *testing.T) {
	t.Run("with environment variables", func(t *testing.T) {
		env := map[string]string{
			"PROXY_PORT":         "4000",
			"TARGET_MODEL":       "test-model",
			"OPENROUTER_API_KEY": "test-key",
		}
		script := buildStartProxyScript("/path/to/proxy.mjs", env)

		if !strings.Contains(script, "export PROXY_PORT='4000'") {
			t.Error("expected PROXY_PORT export")
		}
		if !strings.Contains(script, "export TARGET_MODEL='test-model'") {
			t.Error("expected TARGET_MODEL export")
		}
		if !strings.Contains(script, "export OPENROUTER_API_KEY='test-key'") {
			t.Error("expected OPENROUTER_API_KEY export")
		}
		if !strings.Contains(script, "nohup node '/path/to/proxy.mjs'") {
			t.Error("expected nohup node command")
		}
	})

	t.Run("escapes single quotes", func(t *testing.T) {
		env := map[string]string{
			"KEY": "value'with'quotes",
		}
		script := buildStartProxyScript("/path/to/proxy.mjs", env)

		// Should properly escape single quotes
		if !strings.Contains(script, `export KEY='value'"'"'with'"'"'quotes'`) {
			t.Errorf("expected proper quote escaping, got: %s", script)
		}
	})
}


