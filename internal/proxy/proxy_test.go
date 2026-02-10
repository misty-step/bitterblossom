package proxy

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"
)

func TestStartCommand(t *testing.T) {
	tests := []struct {
		name  string
		model string
		port  string
		want  []string
	}{
		{
			name:  "default values",
			model: "",
			port:  "",
			want:  []string{"nohup", "node", ProxyScriptPath},
		},
		{
			name:  "custom model",
			model: "custom-model",
			port:  "",
			want:  []string{"nohup", "node", ProxyScriptPath},
		},
		{
			name:  "custom port",
			model: "",
			port:  "5000",
			want:  []string{"nohup", "node", ProxyScriptPath},
		},
		{
			name:  "custom model and port",
			model: "custom-model",
			port:  "5000",
			want:  []string{"nohup", "node", ProxyScriptPath},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := StartCommand(tc.model, tc.port)
			if len(got) != len(tc.want) {
				t.Errorf("StartCommand() = %v, want %v", got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Errorf("StartCommand()[%d] = %v, want %v", i, got[i], tc.want[i])
				}
			}
		})
	}
}

func TestStartEnv(t *testing.T) {
	tests := []struct {
		name             string
		model            string
		port             string
		openRouterAPIKey string
		wantModel        string
		wantPort         string
	}{
		{
			name:             "default values",
			model:            "",
			port:             "",
			openRouterAPIKey: "test-key",
			wantModel:        DefaultModel,
			wantPort:         strconv.Itoa(ProxyPort),
		},
		{
			name:             "custom model and port",
			model:            "custom-model",
			port:             "5000",
			openRouterAPIKey: "secret-key",
			wantModel:        "custom-model",
			wantPort:         "5000",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := StartEnv(tc.model, tc.port, tc.openRouterAPIKey)

			requiredKeys := []string{"PROXY_PORT", "TARGET_MODEL", "UPSTREAM_BASE", "UPSTREAM_PATH", "OPENROUTER_API_KEY"}
			for _, key := range requiredKeys {
				if _, ok := got[key]; !ok {
					t.Errorf("StartEnv() missing key %q", key)
				}
			}

			if got["PROXY_PORT"] != tc.wantPort {
				t.Errorf("StartEnv()[PROXY_PORT] = %v, want %v", got["PROXY_PORT"], tc.wantPort)
			}
			if got["TARGET_MODEL"] != tc.wantModel {
				t.Errorf("StartEnv()[TARGET_MODEL] = %v, want %v", got["TARGET_MODEL"], tc.wantModel)
			}
			if got["OPENROUTER_API_KEY"] != tc.openRouterAPIKey {
				t.Errorf("StartEnv()[OPENROUTER_API_KEY] = %v, want %v", got["OPENROUTER_API_KEY"], tc.openRouterAPIKey)
			}
			if got["UPSTREAM_BASE"] != DefaultUpstreamBase {
				t.Errorf("StartEnv()[UPSTREAM_BASE] = %v, want %v", got["UPSTREAM_BASE"], DefaultUpstreamBase)
			}
			if got["UPSTREAM_PATH"] != DefaultUpstreamPath {
				t.Errorf("StartEnv()[UPSTREAM_PATH] = %v, want %v", got["UPSTREAM_PATH"], DefaultUpstreamPath)
			}
		})
	}
}

func TestHealthURL(t *testing.T) {
	tests := []struct {
		name string
		port string
		want string
	}{
		{
			name: "default port",
			port: "",
			want: fmt.Sprintf("http://127.0.0.1:%d/health", ProxyPort),
		},
		{
			name: "custom port",
			port: "5000",
			want: "http://127.0.0.1:5000/health",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := HealthURL(tc.port)
			if got != tc.want {
				t.Errorf("HealthURL() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestIsRunning(t *testing.T) {
	t.Run("server returns OK", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"status":"ok"}`))
		}))
		defer server.Close()

		// Extract port from server URL (format: http://127.0.0.1:PORT)
		port := extractPort(server.URL)

		// IsRunning checks 127.0.0.1, so we need to check if the server is accessible
		// The test server binds to 127.0.0.1 by default
		if !IsRunning(port) {
			t.Error("IsRunning() = false, want true")
		}
	})

	t.Run("server returns error status", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer server.Close()

		port := extractPort(server.URL)
		if IsRunning(port) {
			t.Error("IsRunning() = true, want false")
		}
	})

	t.Run("no server running", func(t *testing.T) {
		// Use port 1 which should never be accessible
		if IsRunning("1") {
			t.Error("IsRunning() = true, want false for unreachable port")
		}
	})
}

func TestWaitForHealthy(t *testing.T) {
	t.Run("immediately healthy", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"status":"ok"}`))
		}))
		defer server.Close()

		port := extractPort(server.URL)
		if !WaitForHealthy(port, 2*time.Second) {
			t.Error("WaitForHealthy() = false, want true")
		}
	})

	t.Run("timeout on unreachable port", func(t *testing.T) {
		start := time.Now()
		if WaitForHealthy("1", 200*time.Millisecond) {
			t.Error("WaitForHealthy() = true, want false")
		}
		elapsed := time.Since(start)
		if elapsed < 200*time.Millisecond {
			t.Errorf("WaitForHealthy() returned too early: %v", elapsed)
		}
	})
}

// extractPort extracts the port number from a URL like http://127.0.0.1:12345
func extractPort(url string) string {
	for i := len(url) - 1; i >= 0; i-- {
		if url[i] == ':' {
			return url[i+1:]
		}
	}
	return ""
}
