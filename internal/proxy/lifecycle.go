package proxy

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/misty-step/bitterblossom/internal/shellutil"
)

const (
	// DefaultProxyTimeout is the default timeout for proxy health check operations.
	DefaultProxyTimeout = 10 * time.Second

	// SpriteProxyPath is where the proxy script is located on the sprite.
	SpriteProxyPath = "/home/sprite/.bb/proxy.mjs"

	// SpriteProxyPort is the port the proxy listens on (on the sprite).
	SpriteProxyPort = 4000
)

// RemoteExecutor executes commands on a remote sprite.
type RemoteExecutor interface {
	Exec(ctx context.Context, sprite, remoteCommand string, stdin []byte) (string, error)
	ExecWithEnv(ctx context.Context, sprite, remoteCommand string, stdin []byte, env map[string]string) (string, error)
	Upload(ctx context.Context, sprite, remotePath string, content []byte) error
}

// Lifecycle manages the proxy lifecycle on a sprite.
type Lifecycle struct {
	executor RemoteExecutor
	port     int
	timeout  time.Duration
}

// NewLifecycle creates a new proxy lifecycle manager.
func NewLifecycle(executor RemoteExecutor) *Lifecycle {
	return &Lifecycle{
		executor: executor,
		port:     SpriteProxyPort,
		timeout:  DefaultProxyTimeout,
	}
}

// NewLifecycleWithPort creates a new proxy lifecycle manager with a custom port.
func NewLifecycleWithPort(executor RemoteExecutor, port int) *Lifecycle {
	return &Lifecycle{
		executor: executor,
		port:     port,
		timeout:  DefaultProxyTimeout,
	}
}

// healthURL returns the health check URL for the proxy on the sprite.
func (l *Lifecycle) healthURL() string {
	return fmt.Sprintf("http://localhost:%d/health", l.port)
}

// IsRunning checks if the proxy is healthy on the target sprite.
// It performs a health check via the sprite's localhost.
func (l *Lifecycle) IsRunning(ctx context.Context, sprite string) (bool, error) {
	script := fmt.Sprintf(`
set -e
curl -s --max-time 2 -o /dev/null -w "%%{http_code}" %s
`, shellutil.Quote(l.healthURL()))

	output, err := l.executor.Exec(ctx, sprite, script, nil)
	if err != nil {
		return false, fmt.Errorf("proxy health check failed: %w", err)
	}

	// Check if the output is "200"
	return output == "200", nil
}

// Start starts the proxy on the target sprite in the background.
// It kills any existing process on the proxy port first, then uploads and starts.
func (l *Lifecycle) Start(ctx context.Context, sprite string, openRouterAPIKey string) error {
	// Kill any existing proxy to prevent EADDRINUSE from zombie processes.
	// Stop() is idempotent — handles "no process" gracefully via || true.
	if err := l.Stop(ctx, sprite); err != nil {
		return fmt.Errorf("failed to clean up existing proxy: %w", err)
	}

	// Ensure the .bb directory exists
	mkdirScript := "mkdir -p /home/sprite/.bb"
	if _, err := l.executor.Exec(ctx, sprite, mkdirScript, nil); err != nil {
		return fmt.Errorf("failed to create .bb directory: %w", err)
	}

	// Upload the proxy script
	if err := l.executor.Upload(ctx, sprite, SpriteProxyPath, ProxyScript); err != nil {
		return fmt.Errorf("failed to upload proxy script: %w", err)
	}

	// Start the proxy in the background
	port := strconv.Itoa(l.port)
	env := StartEnv("", port, openRouterAPIKey)

	startScript := buildStartProxyScript(SpriteProxyPath, env)
	if _, err := l.executor.Exec(ctx, sprite, startScript, nil); err != nil {
		return fmt.Errorf("failed to start proxy: %w", err)
	}

	return nil
}

// Stop stops the proxy on the target sprite. Idempotent — safe to call
// even if no proxy is running (uses || true for graceful no-process handling).
func (l *Lifecycle) Stop(ctx context.Context, sprite string) error {
	stopScript := fmt.Sprintf(`
set -e
# Find and kill the proxy process
PID=$(pgrep -f "node.*%s" || true)
if [ -n "$PID" ]; then
  kill "$PID" 2>/dev/null || true
  sleep 1
  # Force kill if still running
  if kill -0 "$PID" 2>/dev/null; then
    kill -9 "$PID" 2>/dev/null || true
  fi
fi
# Clean up PID file if it exists
rm -f %s
`, SpriteProxyPath, ProxyPIDFile)

	if _, err := l.executor.Exec(ctx, sprite, stopScript, nil); err != nil {
		return fmt.Errorf("failed to stop proxy: %w", err)
	}

	return nil
}

// WaitForHealthy waits for the proxy to become healthy within the timeout.
// It polls the health endpoint until it responds with 200 or the timeout is reached.
func (l *Lifecycle) WaitForHealthy(ctx context.Context, sprite string) error {
	ctx, cancel := context.WithTimeout(ctx, l.timeout)
	defer cancel()

	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("proxy failed to become healthy within %v", l.timeout)
		case <-ticker.C:
			running, err := l.IsRunning(ctx, sprite)
			if err != nil {
				// Health check errors during startup (connection refused, etc.) are expected
				// while the proxy is initializing. Continue polling until timeout.
				continue
			}
			if running {
				return nil
			}
		}
	}
}

// EnsureProxy ensures the proxy is running on the target sprite.
// If the proxy is not running, it starts it and waits for it to become healthy.
// Returns the proxy URL that should be used for ANTHROPIC_BASE_URL.
func (l *Lifecycle) EnsureProxy(ctx context.Context, sprite string, openRouterAPIKey string) (string, error) {
	// Health check errors (connection refused, timeout) mean proxy isn't running.
	// Fall through to start it; real errors surface during Start.
	running, _ := l.IsRunning(ctx, sprite)
	if running {
		return l.ProxyURL(), nil
	}

	// Start the proxy
	if err := l.Start(ctx, sprite, openRouterAPIKey); err != nil {
		return "", fmt.Errorf("failed to start proxy: %w", err)
	}

	// Wait for it to become healthy
	if err := l.WaitForHealthy(ctx, sprite); err != nil {
		return "", err
	}

	return l.ProxyURL(), nil
}

// ProxyURL returns the URL for the proxy (for use as ANTHROPIC_BASE_URL).
func (l *Lifecycle) ProxyURL() string {
	return fmt.Sprintf("http://localhost:%d", l.port)
}

// SetTimeout sets a custom timeout for proxy operations.
func (l *Lifecycle) SetTimeout(timeout time.Duration) {
	l.timeout = timeout
}

// buildStartProxyScript creates a script to start the proxy in the background.
func buildStartProxyScript(proxyPath string, env map[string]string) string {
	// Build environment variable exports
	envExports := ""
	for k, v := range env {
		envExports += fmt.Sprintf("export %s=%s\n", k, shellutil.Quote(v))
	}

	return fmt.Sprintf(`
set -e
%s
# Start proxy in background
nohup node %s >/dev/null 2>&1 &
echo $! > %s
`, envExports, shellutil.Quote(proxyPath), ProxyPIDFile)
}

// HTTPClient is used for making HTTP requests (can be mocked in tests).
var HTTPClient = &http.Client{
	Timeout: 2 * time.Second,
}
