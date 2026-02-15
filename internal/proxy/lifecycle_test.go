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
	execFunc          func(ctx context.Context, sprite, remoteCommand string, stdin []byte) (string, error)
	execWithEnvFunc   func(ctx context.Context, sprite, remoteCommand string, stdin []byte, env map[string]string) (string, error)
	uploadFunc        func(ctx context.Context, sprite, remotePath string, content []byte) error
	execCalls         []execCall
	execWithEnvCalls  []execWithEnvCall
	uploadCalls       []uploadCall
}

type execCall struct {
	sprite  string
	command string
	stdin   []byte
}

type execWithEnvCall struct {
	sprite  string
	command string
	stdin   []byte
	env     map[string]string
}

type uploadCall struct {
	sprite  string
	path    string
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
	m.execWithEnvCalls = append(m.execWithEnvCalls, execWithEnvCall{sprite: sprite, command: remoteCommand, stdin: stdin, env: env})
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

		// Exec sequence: cleanup (pgrep kill) -> mkdir -> write key file -> start node
		if len(mock.execCalls) < 4 {
			t.Fatalf("expected at least 4 exec calls, got %d", len(mock.execCalls))
		}
		if !strings.Contains(mock.execCalls[0].command, "pgrep") {
			t.Errorf("expected first exec to kill existing proxy, got: %s", mock.execCalls[0].command)
		}
		if !strings.Contains(mock.execCalls[1].command, "mkdir -p") {
			t.Errorf("expected second exec to be mkdir, got: %s", mock.execCalls[1].command)
		}

		// Third call should write the API key to a secure file
		if !strings.Contains(mock.execCalls[2].command, "openrouter.key") {
			t.Errorf("expected third exec to write API key file, got: %s", mock.execCalls[2].command)
		}
		if !strings.Contains(mock.execCalls[2].command, "chmod 600") {
			t.Errorf("expected third exec to set file permissions, got: %s", mock.execCalls[2].command)
		}

		// Check that upload was called
		if len(mock.uploadCalls) != 1 {
			t.Fatalf("expected 1 upload call, got %d", len(mock.uploadCalls))
		}
		if mock.uploadCalls[0].path != SpriteProxyPath {
			t.Errorf("expected upload to %s, got %s", SpriteProxyPath, mock.uploadCalls[0].path)
		}

		// Fourth call should start the node proxy
		if !strings.Contains(mock.execCalls[3].command, "node") {
			t.Errorf("expected fourth exec to be node command, got: %s", mock.execCalls[3].command)
		}
		// The API key should not appear in the node command itself
		if strings.Contains(mock.execCalls[3].command, "test-api-key") {
			t.Errorf("expected API key to not appear in remote command, got: %s", mock.execCalls[3].command)
		}

		// Check that ExecWithEnv was called with the key file path
		if len(mock.execWithEnvCalls) != 1 {
			t.Fatalf("expected 1 execWithEnv call, got %d", len(mock.execWithEnvCalls))
		}
		env := mock.execWithEnvCalls[0].env
		if env["OPENROUTER_API_KEY_FILE"] != APIKeyFilePath {
			t.Errorf("expected OPENROUTER_API_KEY_FILE env var, got: %s", env["OPENROUTER_API_KEY_FILE"])
		}
		if env["OPENROUTER_API_KEY"] != "" {
			t.Errorf("expected OPENROUTER_API_KEY env var to be empty, got: %s", env["OPENROUTER_API_KEY"])
		}
	})

	t.Run("mkdir fails", func(t *testing.T) {
		callCount := 0
		mock := &mockRemoteExecutor{
			execFunc: func(ctx context.Context, sprite, remoteCommand string, stdin []byte) (string, error) {
				callCount++
				if callCount == 1 {
					return "", nil // cleanup succeeds
				}
				return "", errors.New("permission denied") // mkdir fails
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
		mock := &mockRemoteExecutor{
			execWithEnvFunc: func(ctx context.Context, sprite, remoteCommand string, stdin []byte, env map[string]string) (string, error) {
				return "", errors.New("command not found")
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
		// Should include diagnostics in error message
		if !strings.Contains(err.Error(), "Diagnostics") {
			t.Errorf("expected diagnostics in error message, got: %v", err)
		}
		if !strings.Contains(err.Error(), "Next steps") {
			t.Errorf("expected next steps hint in error message, got: %v", err)
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
				if callCount == 1 { // IsRunning: not running
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
	t.Run("starts node in background", func(t *testing.T) {
		script := buildStartProxyScript("/path/to/proxy.mjs")
		if !strings.Contains(script, "nohup node '/path/to/proxy.mjs'") {
			t.Error("expected nohup node command")
		}
	})

	t.Run("escapes single quotes in path", func(t *testing.T) {
		script := buildStartProxyScript("/path/to/proxy'with'quotes.mjs")
		if !strings.Contains(script, `nohup node '/path/to/proxy'"'"'with'"'"'quotes.mjs'`) {
			t.Errorf("expected proper quote escaping, got: %s", script)
		}
	})

	t.Run("captures stderr to log file", func(t *testing.T) {
		script := buildStartProxyScript("/path/to/proxy.mjs")
		if !strings.Contains(script, "2>>") {
			t.Error("expected stderr redirection to log file")
		}
		if !strings.Contains(script, ProxyLogPath) {
			t.Errorf("expected log path %s in script, got: %s", ProxyLogPath, script)
		}
	})

	t.Run("creates log directory", func(t *testing.T) {
		script := buildStartProxyScript("/path/to/proxy.mjs")
		if !strings.Contains(script, "mkdir -p") {
			t.Error("expected mkdir command for log directory")
		}
	})
}

func TestLifecycle_CollectDiagnostics(t *testing.T) {
	t.Run("collects all diagnostics", func(t *testing.T) {
		execCount := 0
		mock := &mockRemoteExecutor{
			execFunc: func(ctx context.Context, sprite, remoteCommand string, stdin []byte) (string, error) {
				execCount++
				switch execCount {
				case 1:
					return "Mem: 1Gi available", nil
				case 2:
					return "PID USER COMMAND\n123 sprite node proxy.mjs", nil
				case 3:
					return "Error: Cannot find module 'express'", nil
				default:
					return "", nil
				}
			},
		}
		lifecycle := NewLifecycle(mock)

		diagnostics, err := lifecycle.CollectDiagnostics(context.Background(), "test-sprite")
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if diagnostics.MemoryAvailable != "Mem: 1Gi available" {
			t.Errorf("unexpected memory: %s", diagnostics.MemoryAvailable)
		}
		if diagnostics.ProcessList != "PID USER COMMAND\n123 sprite node proxy.mjs" {
			t.Errorf("unexpected processes: %s", diagnostics.ProcessList)
		}
		if diagnostics.ProxyLogTail != "Error: Cannot find module 'express'" {
			t.Errorf("unexpected log tail: %s", diagnostics.ProxyLogTail)
		}
	})

	t.Run("handles exec errors gracefully", func(t *testing.T) {
		mock := &mockRemoteExecutor{
			execFunc: func(ctx context.Context, sprite, remoteCommand string, stdin []byte) (string, error) {
				return "", errors.New("command failed")
			},
		}
		lifecycle := NewLifecycle(mock)

		diagnostics, err := lifecycle.CollectDiagnostics(context.Background(), "test-sprite")
		if err != nil {
			t.Errorf("expected nil error but got: %v", err)
		}
		if diagnostics == nil {
			t.Error("expected diagnostics even when commands fail")
		}
	})

	t.Run("surfaces errors for unreachable sprite (issue #358)", func(t *testing.T) {
		mock := &mockRemoteExecutor{
			execFunc: func(ctx context.Context, sprite, remoteCommand string, stdin []byte) (string, error) {
				return "", errors.New("i/o timeout: sprite unreachable")
			},
		}
		lifecycle := NewLifecycle(mock)

		diagnostics, err := lifecycle.CollectDiagnostics(context.Background(), "unreachable-sprite")
		if err != nil {
			t.Errorf("expected nil error but got: %v", err)
		}
		if diagnostics == nil {
			t.Fatal("expected diagnostics even when commands fail")
		}

		// Check that errors are surfaced in the diagnostic fields
		if diagnostics.MemoryAvailable == "" {
			t.Error("expected MemoryAvailable to contain error, got empty string")
		}
		if !strings.Contains(diagnostics.MemoryAvailable, "failed") {
			t.Errorf("expected MemoryAvailable to contain 'failed', got: %s", diagnostics.MemoryAvailable)
		}
		if !strings.Contains(diagnostics.MemoryAvailable, "i/o timeout") {
			t.Errorf("expected MemoryAvailable to contain original error, got: %s", diagnostics.MemoryAvailable)
		}

		if diagnostics.ProcessList == "" {
			t.Error("expected ProcessList to contain error, got empty string")
		}
		if !strings.Contains(diagnostics.ProcessList, "failed") {
			t.Errorf("expected ProcessList to contain 'failed', got: %s", diagnostics.ProcessList)
		}

		if diagnostics.ProxyLogTail == "" {
			t.Error("expected ProxyLogTail to contain error, got empty string")
		}
		if !strings.Contains(diagnostics.ProxyLogTail, "failed") {
			t.Errorf("expected ProxyLogTail to contain 'failed', got: %s", diagnostics.ProxyLogTail)
		}

		// Test that FormatError shows these error messages instead of blank output
		baseErr := errors.New("proxy health check failed after 30s")
		formatted := diagnostics.FormatError(baseErr, "unreachable-sprite")

		if strings.Contains(formatted, "Memory:\n\n") {
			t.Error("FormatError should not show empty Memory section - issue #358 regression")
		}
		if strings.Contains(formatted, "Processes:\n\n") {
			t.Error("FormatError should not show empty Processes section - issue #358 regression")
		}
		if strings.Contains(formatted, "Proxy log (last 50 lines):\n\n") {
			t.Error("FormatError should not show empty Proxy log section - issue #358 regression")
		}

		// Each field should show error info - check for "failed" in the formatted output
		if !strings.Contains(formatted, "failed") {
			t.Errorf("FormatError should show failure indicators, got:\n%s", formatted)
		}
	})
}

func TestDiagnostics_FormatError(t *testing.T) {
	d := &Diagnostics{
		MemoryAvailable: "Mem: 512M available",
		ProcessList:     "123 sprite node",
		ProxyLogTail:    "Error: port already in use",
	}

	err := errors.New("connection refused")
	formatted := d.FormatError(err, "bramble")

	if !strings.Contains(formatted, "proxy health check failed") {
		t.Error("expected 'proxy health check failed' in error")
	}
	if !strings.Contains(formatted, "Diagnostics") {
		t.Error("expected 'Diagnostics' section")
	}
	if !strings.Contains(formatted, "Next steps") {
		t.Error("expected 'Next steps' section")
	}
	if !strings.Contains(formatted, "bramble") {
		t.Error("expected sprite name in error")
	}
	if !strings.Contains(formatted, "bb status bramble") {
		t.Error("expected bb status hint")
	}
	if !strings.Contains(formatted, "512M available") {
		t.Error("expected memory info")
	}
	if !strings.Contains(formatted, "port already in use") {
		t.Error("expected log tail")
	}
}
