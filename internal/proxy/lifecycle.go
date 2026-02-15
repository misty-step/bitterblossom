package proxy

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/misty-step/bitterblossom/internal/shellutil"
)

const (
	// DefaultProxyTimeout is the default timeout for proxy health check operations.
	// 30s accommodates cold/warm sprite startup variance (10s was too tight).
	DefaultProxyTimeout = 30 * time.Second

	// SpriteProxyPath is where the proxy script is located on the sprite.
	SpriteProxyPath = "/home/sprite/.bb/proxy.mjs"

	// SpriteProxyPort is the port the proxy listens on (on the sprite).
	SpriteProxyPort = 4000

	// ProxyLogPath is where proxy stderr is captured for diagnostics.
	ProxyLogPath = "/home/sprite/.bb/proxy.log"
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

// APIKeyFilePath is where the OpenRouter API key is stored on the sprite.
// Stored in /home/sprite/.bb (not world-writable /tmp) with 600 permissions.
const APIKeyFilePath = "/home/sprite/.bb/openrouter.key"

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

	// Write API key to a secure file with 600 permissions (owner read/write only).
	// This prevents exposure via /proc/<pid>/environ.
	writeKeyScript := fmt.Sprintf(`printf '%%s' %s > %s && chmod 600 %s`,
		shellutil.Quote(openRouterAPIKey),
		APIKeyFilePath,
		APIKeyFilePath)
	if _, err := l.executor.Exec(ctx, sprite, writeKeyScript, nil); err != nil {
		return fmt.Errorf("failed to write API key file: %w", err)
	}

	// Upload the proxy script
	if err := l.executor.Upload(ctx, sprite, SpriteProxyPath, ProxyScript); err != nil {
		return fmt.Errorf("failed to upload proxy script: %w", err)
	}

	// Start the proxy in the background
	port := strconv.Itoa(l.port)
	env := StartEnvWithKeyFile("", port, APIKeyFilePath)

	startScript := buildStartProxyScript(SpriteProxyPath)
	if _, err := l.executor.ExecWithEnv(ctx, sprite, startScript, nil, env); err != nil {
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
// On timeout, collects diagnostics from the sprite to aid troubleshooting.
func (l *Lifecycle) WaitForHealthy(ctx context.Context, sprite string) error {
	ctx, cancel := context.WithTimeout(ctx, l.timeout)
	defer cancel()

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	var lastErr error
	for {
		select {
		case <-ctx.Done():
			// Collect diagnostics to help troubleshoot the failure
			diagnostics, diagErr := l.CollectDiagnostics(context.Background(), sprite)
			if diagErr == nil {
				return fmt.Errorf("%s", diagnostics.FormatError(lastErr, sprite))
			}
			// Fallback to simple error if diagnostics collection fails
			msg := fmt.Sprintf("proxy failed to become healthy within %v on port %d", l.timeout, l.port)
			if lastErr != nil {
				msg += fmt.Sprintf(" (last error: %v)", lastErr)
			}
			return fmt.Errorf("%s: %w", msg, ctx.Err())
		case <-ticker.C:
			running, err := l.IsRunning(ctx, sprite)
			if err != nil {
				lastErr = err
				continue
			}
			lastErr = nil
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
// Captures stderr to ProxyLogPath for diagnostic visibility.
func buildStartProxyScript(proxyPath string) string {
	return fmt.Sprintf(`
set -e
# Ensure log directory exists
mkdir -p $(dirname %s)
# Start proxy in background, capturing stderr for diagnostics
nohup node %s >/dev/null 2>>%s &
echo $! > %s
`, shellutil.Quote(ProxyLogPath), shellutil.Quote(proxyPath), shellutil.Quote(ProxyLogPath), ProxyPIDFile)
}

// HTTPClient is used for making HTTP requests (can be mocked in tests).
var HTTPClient = &http.Client{
	Timeout: 2 * time.Second,
}

// Diagnostics collects diagnostic information from a sprite when proxy health checks fail.
type Diagnostics struct {
	MemoryAvailable string
	ProcessList     string
	ProxyLogTail    string
}

// CollectDiagnostics gathers resource and log information from the sprite.
// When collection fails (e.g., sprite unreachable), errors are surfaced in the
// diagnostic fields rather than leaving them empty (fixes issue #358).
func (l *Lifecycle) CollectDiagnostics(ctx context.Context, sprite string) (*Diagnostics, error) {
	d := &Diagnostics{}

	// Get available memory
	memOutput, err := l.executor.Exec(ctx, sprite, "free -h 2>/dev/null || echo 'free not available'", nil)
	if err == nil {
		d.MemoryAvailable = strings.TrimSpace(memOutput)
	} else {
		d.MemoryAvailable = fmt.Sprintf("failed (%v)", err)
	}

	// Get process list filtered to node processes
	procOutput, err := l.executor.Exec(ctx, sprite, "ps aux | grep -E 'node|PID' | grep -v grep || echo 'no node processes'", nil)
	if err == nil {
		d.ProcessList = strings.TrimSpace(procOutput)
	} else {
		d.ProcessList = fmt.Sprintf("failed (%v)", err)
	}

	// Get recent proxy log entries (last 50 lines)
	logOutput, err := l.executor.Exec(ctx, sprite, fmt.Sprintf("tail -n 50 %s 2>/dev/null || echo 'no proxy log available'", ProxyLogPath), nil)
	if err == nil {
		d.ProxyLogTail = strings.TrimSpace(logOutput)
	} else {
		d.ProxyLogTail = fmt.Sprintf("failed (%v)", err)
	}

	return d, nil
}

// FormatError formats an error message with diagnostics and actionable hints.
func (d *Diagnostics) FormatError(baseErr error, sprite string) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("proxy health check failed: %v\n\n", baseErr))

	b.WriteString("=== Diagnostics ===\n")
	b.WriteString(fmt.Sprintf("Memory:\n%s\n\n", d.MemoryAvailable))
	b.WriteString(fmt.Sprintf("Processes:\n%s\n\n", d.ProcessList))
	b.WriteString(fmt.Sprintf("Proxy log (last 50 lines):\n%s\n\n", d.ProxyLogTail))

	b.WriteString("=== Next steps ===\n")
	b.WriteString(fmt.Sprintf("• Check sprite status: bb status %s\n", sprite))
	b.WriteString(fmt.Sprintf("• View full proxy log: sprite exec %s -- tail -f %s\n", sprite, ProxyLogPath))
	b.WriteString(fmt.Sprintf("• Check system logs: sprite exec %s -- journalctl -u proxy 2>/dev/null || dmesg | tail\n", sprite))
	b.WriteString(fmt.Sprintf("• Restart sprite if OOM suspected: bb stop %s && bb start %s\n", sprite, sprite))

	return b.String()
}
