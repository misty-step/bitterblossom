package proxy

import (
	"fmt"
	"net/http"
	"strconv"
	"time"
)

const (
	// ProxyPort is the default port the proxy listens on.
	ProxyPort = 4000

	// ProxyScriptPath is where the proxy script is uploaded on the sprite.
	ProxyScriptPath = "/home/sprite/anthropic-proxy.mjs"

	// ProxyPIDFile is the path to the proxy PID file.
	// Using /run for better security than world-writable /tmp, fallback to /tmp if needed.
	ProxyPIDFile = "/run/sprite/anthropic-proxy.pid"

	// DefaultModel is the default model used when proxy provider is selected.
	DefaultModel = "moonshotai/kimi-k2.5"

	// DefaultUpstreamBase is the default upstream base URL.
	DefaultUpstreamBase = "https://openrouter.ai"

	// DefaultUpstreamPath is the default upstream path.
	DefaultUpstreamPath = "/api/v1/chat/completions"
)

// StartCommand returns the command to start the proxy on a sprite.
// The model parameter sets the TARGET_MODEL environment variable.
// The port parameter sets the PROXY_PORT environment variable.
func StartCommand(model, port string) []string {
	if model == "" {
		model = DefaultModel
	}

	return []string{
		"nohup",
		"node",
		ProxyScriptPath,
	}
}

// StartEnv returns the environment variables needed to run the proxy.
func StartEnv(model, port, openRouterAPIKey string) map[string]string {
	if model == "" {
		model = DefaultModel
	}
	if port == "" {
		port = strconv.Itoa(ProxyPort)
	}

	return map[string]string{
		"PROXY_PORT":         port,
		"TARGET_MODEL":       model,
		"UPSTREAM_BASE":      "https://openrouter.ai",
		"UPSTREAM_PATH":      "/api/v1/chat/completions",
		"OPENROUTER_API_KEY": openRouterAPIKey,
	}
}

// HealthURL returns the health check URL for the proxy.
func HealthURL(port string) string {
	if port == "" {
		port = strconv.Itoa(ProxyPort)
	}
	return fmt.Sprintf("http://127.0.0.1:%s/health", port)
}

// IsRunning checks if the proxy is healthy on the given port.
func IsRunning(port string) bool {
	if port == "" {
		port = strconv.Itoa(ProxyPort)
	}

	client := &http.Client{
		Timeout: 2 * time.Second,
	}

	resp, err := client.Get(HealthURL(port))
	if err != nil {
		return false
	}
	defer func() { _ = resp.Body.Close() }()

	return resp.StatusCode == http.StatusOK
}

// WaitForHealthy polls the proxy health endpoint until it responds
// or the timeout is reached. Returns true if healthy, false on timeout.
func WaitForHealthy(port string, timeout time.Duration) bool {
	if port == "" {
		port = strconv.Itoa(ProxyPort)
	}

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if IsRunning(port) {
			return true
		}
		time.Sleep(100 * time.Millisecond)
	}
	return false
}
