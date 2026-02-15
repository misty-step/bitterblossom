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
	t.Run("collects all diagnostics via atomic collection", func(t *testing.T) {
		mock := &mockRemoteExecutor{
			execFunc: func(ctx context.Context, sprite, remoteCommand string, stdin []byte) (string, error) {
				// Return atomic output format with all sections
				return `---MEMORY---
Mem: 1Gi available
---END_MEMORY---
---PROCESSES---
PID USER COMMAND
123 sprite node proxy.mjs
---END_PROCESSES---
---PROXY_LOG---
Error: Cannot find module 'express'
---END_PROXY_LOG---`, nil
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

	t.Run("falls back to fresh collection on transport error", func(t *testing.T) {
		callCount := 0
		mock := &mockRemoteExecutor{
			execFunc: func(ctx context.Context, sprite, remoteCommand string, stdin []byte) (string, error) {
				callCount++
				// First call (atomic) fails with transport error
				if strings.Contains(remoteCommand, "---MEMORY---") {
					return "", errors.New("signal: killed")
				}
				// Fallback calls succeed
				if strings.Contains(remoteCommand, "free") {
					return "Mem: 2Gi available", nil
				}
				if strings.Contains(remoteCommand, "ps aux") {
					return "PID 123 node proxy", nil
				}
				if strings.Contains(remoteCommand, "tail") {
					return "Proxy log line", nil
				}
				return "", nil
			},
		}
		lifecycle := NewLifecycle(mock)

		diagnostics, err := lifecycle.CollectDiagnostics(context.Background(), "test-sprite")
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if diagnostics.MemoryAvailable != "Mem: 2Gi available" {
			t.Errorf("unexpected memory: %s", diagnostics.MemoryAvailable)
		}
		if diagnostics.ProcessList != "PID 123 node proxy" {
			t.Errorf("unexpected processes: %s", diagnostics.ProcessList)
		}
		if diagnostics.ProxyLogTail != "Proxy log line" {
			t.Errorf("unexpected log tail: %s", diagnostics.ProxyLogTail)
		}
	})

	t.Run("reports transport failure when both collection methods fail (issue #350)", func(t *testing.T) {
		mock := &mockRemoteExecutor{
			execFunc: func(ctx context.Context, sprite, remoteCommand string, stdin []byte) (string, error) {
				return "", errors.New("signal: killed")
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

		// Should report transport failure instead of empty fields (issue #350 fix)
		if !strings.Contains(diagnostics.MemoryAvailable, "diagnostics unavailable") {
			t.Errorf("expected 'diagnostics unavailable' message, got: %s", diagnostics.MemoryAvailable)
		}
		if !strings.Contains(diagnostics.ProcessList, "diagnostics unavailable") {
			t.Errorf("expected 'diagnostics unavailable' message, got: %s", diagnostics.ProcessList)
		}
		if !strings.Contains(diagnostics.ProxyLogTail, "diagnostics unavailable") {
			t.Errorf("expected 'diagnostics unavailable' message, got: %s", diagnostics.ProxyLogTail)
		}

		// Test that FormatError shows informative message instead of blank output
		baseErr := errors.New("proxy health check failed after 30s")
		formatted := diagnostics.FormatError(baseErr, "unreachable-sprite")

		if strings.Contains(formatted, "Memory:\n\n") {
			t.Error("FormatError should not show empty Memory section - issue #350 regression")
		}
		if strings.Contains(formatted, "Processes:\n\n") {
			t.Error("FormatError should not show empty Processes section - issue #350 regression")
		}
		if strings.Contains(formatted, "Proxy log (last 50 lines):\n\n") {
			t.Error("FormatError should not show empty Proxy log section - issue #350 regression")
		}

		// Should show transport failure message
		if !strings.Contains(formatted, "diagnostics unavailable") {
			t.Errorf("FormatError should show 'diagnostics unavailable', got:\n%s", formatted)
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

// Tests for issue #350 fix: atomic diagnostics collection and transport failure handling

func TestExtractSection(t *testing.T) {
	tests := []struct {
		name     string
		output   string
		section  string
		want     string
		wantOk   bool
	}{
		{
			name: "extracts memory section",
			output: `---MEMORY---
Mem: 1Gi available
---END_MEMORY---`,
			section: "MEMORY",
			want:    "Mem: 1Gi available",
			wantOk:  true,
		},
		{
			name: "extracts processes section",
			output: `---PROCESSES---
PID USER COMMAND
123 sprite node
---END_PROCESSES---`,
			section: "PROCESSES",
			want:    "PID USER COMMAND\n123 sprite node",
			wantOk:  true,
		},
		{
			name: "extracts proxy log section",
			output: `---PROXY_LOG---
Error: Cannot find module
    at require
---END_PROXY_LOG---`,
			section: "PROXY_LOG",
			want:    "Error: Cannot find module\n    at require",
			wantOk:  true,
		},
		{
			name:    "missing section",
			output:  `---MEMORY---
Mem: 1Gi
---END_MEMORY---`,
			section: "PROXY_LOG",
			want:    "",
			wantOk:  false,
		},
		{
			name:    "missing end marker",
			output:  `---MEMORY---
Mem: 1Gi`,
			section: "MEMORY",
			want:    "",
			wantOk:  false,
		},
		{
			name: "multiple sections extracts correct one",
			output: `---MEMORY---
Mem: 1Gi
---END_MEMORY---
---PROCESSES---
node proxy
---END_PROCESSES---`,
			section: "PROCESSES",
			want:    "node proxy",
			wantOk:  true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := extractSection(tc.output, tc.section)
			if ok != tc.wantOk {
				t.Errorf("extractSection() ok = %v, want %v", ok, tc.wantOk)
			}
			if got != tc.want {
				t.Errorf("extractSection() = %q, want %q", got, tc.want)
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
			name: "signal killed",
			err:  errors.New("signal: killed"),
			want: true,
		},
		{
			name: "context deadline exceeded",
			err:  errors.New("context deadline exceeded"),
			want: true,
		},
		{
			name: "connection refused",
			err:  errors.New("connection refused"),
			want: true,
		},
		{
			name: "i/o timeout",
			err:  errors.New("read tcp: i/o timeout"),
			want: true,
		},
		{
			name: "network unreachable",
			err:  errors.New("network is unreachable"),
			want: true,
		},
		{
			name: "no such host",
			err:  errors.New("lookup: no such host"),
			want: true,
		},
		{
			name: "generic error not transport",
			err:  errors.New("command not found"),
			want: false,
		},
		{
			name: "wrapped transport error",
			err:  errors.New("exec failed: signal: killed during operation"),
			want: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := isTransportError(tc.err)
			if got != tc.want {
				t.Errorf("isTransportError(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}

func TestLifecycle_collectDiagnosticsAtomic(t *testing.T) {
	t.Run("successful atomic collection", func(t *testing.T) {
		mock := &mockRemoteExecutor{
			execFunc: func(ctx context.Context, sprite, remoteCommand string, stdin []byte) (string, error) {
				// Verify it's an atomic script (contains all sections)
				if !strings.Contains(remoteCommand, "---MEMORY---") {
					t.Error("expected atomic script to contain MEMORY section marker")
				}
				if !strings.Contains(remoteCommand, "---PROCESSES---") {
					t.Error("expected atomic script to contain PROCESSES section marker")
				}
				if !strings.Contains(remoteCommand, "---PROXY_LOG---") {
					t.Error("expected atomic script to contain PROXY_LOG section marker")
				}

				// Return valid output with all sections
				return `---MEMORY---
Mem: 2Gi available
---END_MEMORY---
---PROCESSES---
123 sprite node proxy.mjs
---END_PROCESSES---
---PROXY_LOG---
Error: port in use
---END_PROXY_LOG---`, nil
			},
		}
		lifecycle := NewLifecycle(mock)

		d, err := lifecycle.collectDiagnosticsAtomic(context.Background(), "test-sprite")
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}

		if d.MemoryAvailable != "Mem: 2Gi available" {
			t.Errorf("unexpected memory: %s", d.MemoryAvailable)
		}
		if d.ProcessList != "123 sprite node proxy.mjs" {
			t.Errorf("unexpected processes: %s", d.ProcessList)
		}
		if d.ProxyLogTail != "Error: port in use" {
			t.Errorf("unexpected log tail: %s", d.ProxyLogTail)
		}
	})

	t.Run("returns error on exec failure", func(t *testing.T) {
		mock := &mockRemoteExecutor{
			execFunc: func(ctx context.Context, sprite, remoteCommand string, stdin []byte) (string, error) {
				return "", errors.New("signal: killed")
			},
		}
		lifecycle := NewLifecycle(mock)

		_, err := lifecycle.collectDiagnosticsAtomic(context.Background(), "test-sprite")
		if err == nil {
			t.Error("expected error for exec failure")
		}
		if !strings.Contains(err.Error(), "atomic diagnostics collection failed") {
			t.Errorf("unexpected error message: %v", err)
		}
	})

	t.Run("returns parse error for malformed output", func(t *testing.T) {
		mock := &mockRemoteExecutor{
			execFunc: func(ctx context.Context, sprite, remoteCommand string, stdin []byte) (string, error) {
				return "malformed output without markers", nil
			},
		}
		lifecycle := NewLifecycle(mock)

		d, err := lifecycle.collectDiagnosticsAtomic(context.Background(), "test-sprite")
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}

		if !strings.Contains(d.MemoryAvailable, "parse error") {
			t.Errorf("expected parse error in MemoryAvailable, got: %s", d.MemoryAvailable)
		}
		if !strings.Contains(d.ProcessList, "parse error") {
			t.Errorf("expected parse error in ProcessList, got: %s", d.ProcessList)
		}
		if !strings.Contains(d.ProxyLogTail, "parse error") {
			t.Errorf("expected parse error in ProxyLogTail, got: %s", d.ProxyLogTail)
		}
	})
}

func TestLifecycle_collectDiagnosticsFresh(t *testing.T) {
	t.Run("successful fresh collection", func(t *testing.T) {
		callCount := 0
		mock := &mockRemoteExecutor{
			execFunc: func(ctx context.Context, sprite, remoteCommand string, stdin []byte) (string, error) {
				callCount++
				switch {
				case strings.Contains(remoteCommand, "free"):
					return "Mem: 4Gi total", nil
				case strings.Contains(remoteCommand, "ps aux"):
					return "456 sprite node", nil
				case strings.Contains(remoteCommand, "tail"):
					return "Proxy started on port 4000", nil
				}
				return "", nil
			},
		}
		lifecycle := NewLifecycle(mock)

		d, err := lifecycle.collectDiagnosticsFresh(context.Background(), "test-sprite")
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}

		if d.MemoryAvailable != "Mem: 4Gi total" {
			t.Errorf("unexpected memory: %s", d.MemoryAvailable)
		}
		if d.ProcessList != "456 sprite node" {
			t.Errorf("unexpected processes: %s", d.ProcessList)
		}
		if d.ProxyLogTail != "Proxy started on port 4000" {
			t.Errorf("unexpected log tail: %s", d.ProxyLogTail)
		}
	})

	t.Run("returns transport failure message when all calls fail", func(t *testing.T) {
		mock := &mockRemoteExecutor{
			execFunc: func(ctx context.Context, sprite, remoteCommand string, stdin []byte) (string, error) {
				return "", errors.New("connection refused")
			},
		}
		lifecycle := NewLifecycle(mock)

		d, err := lifecycle.collectDiagnosticsFresh(context.Background(), "test-sprite")
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}

		if d.MemoryAvailable != "diagnostics unavailable — transport failure" {
			t.Errorf("expected transport failure message, got: %s", d.MemoryAvailable)
		}
		if d.ProcessList != "diagnostics unavailable — transport failure" {
			t.Errorf("expected transport failure message, got: %s", d.ProcessList)
		}
		if d.ProxyLogTail != "diagnostics unavailable — transport failure" {
			t.Errorf("expected transport failure message, got: %s", d.ProxyLogTail)
		}
	})

	t.Run("partial success shows available data", func(t *testing.T) {
		callCount := 0
		mock := &mockRemoteExecutor{
			execFunc: func(ctx context.Context, sprite, remoteCommand string, stdin []byte) (string, error) {
				callCount++
				if callCount == 1 { // Memory succeeds
					return "Mem: 8Gi", nil
				}
				return "", errors.New("timeout") // Others fail
			},
		}
		lifecycle := NewLifecycle(mock)

		d, err := lifecycle.collectDiagnosticsFresh(context.Background(), "test-sprite")
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}

		if d.MemoryAvailable != "Mem: 8Gi" {
			t.Errorf("expected memory data, got: %s", d.MemoryAvailable)
		}
		if d.ProcessList != "diagnostics unavailable — transport failure" {
			t.Errorf("expected transport failure for processes, got: %s", d.ProcessList)
		}
	})
}

func TestLifecycle_CollectDiagnostics_Integration(t *testing.T) {
	t.Run("uses atomic collection when available", func(t *testing.T) {
		callCount := 0
		mock := &mockRemoteExecutor{
			execFunc: func(ctx context.Context, sprite, remoteCommand string, stdin []byte) (string, error) {
				callCount++
				// First call is atomic (has all section markers)
				if strings.Contains(remoteCommand, "---MEMORY---") {
					return `---MEMORY---
Mem: 1Gi
---END_MEMORY---
---PROCESSES---
node
---END_PROCESSES---
---PROXY_LOG---
log
---END_PROXY_LOG---`, nil
				}
				return "", nil
			},
		}
		lifecycle := NewLifecycle(mock)

		d, err := lifecycle.CollectDiagnostics(context.Background(), "test-sprite")
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if d.MemoryAvailable != "Mem: 1Gi" {
			t.Errorf("unexpected result: %s", d.MemoryAvailable)
		}
		// Should only make 1 exec call (atomic)
		if callCount != 1 {
			t.Errorf("expected 1 exec call for atomic collection, got %d", callCount)
		}
	})

	t.Run("falls back to fresh collection on transport error", func(t *testing.T) {
		callCount := 0
		mock := &mockRemoteExecutor{
			execFunc: func(ctx context.Context, sprite, remoteCommand string, stdin []byte) (string, error) {
				callCount++
				// First call (atomic) fails with transport error
				if strings.Contains(remoteCommand, "---MEMORY---") {
					return "", errors.New("signal: killed")
				}
				// Fallback calls succeed
				if strings.Contains(remoteCommand, "free") {
					return "Mem: 2Gi", nil
				}
				if strings.Contains(remoteCommand, "ps aux") {
					return "node proxy", nil
				}
				if strings.Contains(remoteCommand, "tail") {
					return "proxy log", nil
				}
				return "", nil
			},
		}
		lifecycle := NewLifecycle(mock)

		d, err := lifecycle.CollectDiagnostics(context.Background(), "test-sprite")
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if d.MemoryAvailable != "Mem: 2Gi" {
			t.Errorf("expected fallback memory data, got: %s", d.MemoryAvailable)
		}
		if d.ProcessList != "node proxy" {
			t.Errorf("expected fallback process data, got: %s", d.ProcessList)
		}
		if d.ProxyLogTail != "proxy log" {
			t.Errorf("expected fallback log data, got: %s", d.ProxyLogTail)
		}
		// Should make 4 calls: 1 atomic (fails) + 3 individual fresh calls
		if callCount != 4 {
			t.Errorf("expected 4 exec calls (1 failed atomic + 3 fresh), got %d", callCount)
		}
	})

	t.Run("reports transport failure when both methods fail", func(t *testing.T) {
		mock := &mockRemoteExecutor{
			execFunc: func(ctx context.Context, sprite, remoteCommand string, stdin []byte) (string, error) {
				return "", errors.New("signal: killed")
			},
		}
		lifecycle := NewLifecycle(mock)

		d, err := lifecycle.CollectDiagnostics(context.Background(), "test-sprite")
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		// Should report transport failure message
		if !strings.Contains(d.MemoryAvailable, "diagnostics unavailable") {
			t.Errorf("expected transport failure message, got: %s", d.MemoryAvailable)
		}
	})
}
