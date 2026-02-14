package github

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestNewClient(t *testing.T) {
	t.Parallel()

	// Default client
	c := NewClient()
	if c.baseURL != DefaultBaseURL {
		t.Errorf("expected base URL %s, got %s", DefaultBaseURL, c.baseURL)
	}
	if c.httpClient.Timeout != DefaultTimeout {
		t.Errorf("expected timeout %v, got %v", DefaultTimeout, c.httpClient.Timeout)
	}

	// Client with token
	tokenClient := NewClientFromToken("test-token")
	if tokenClient.token != "test-token" {
		t.Errorf("expected token 'test-token', got %s", tokenClient.token)
	}
}

func TestNewClient_WithOptions(t *testing.T) {
	t.Parallel()

	customHTTP := &http.Client{Timeout: 30 * time.Second}
	c := NewClient(
		WithToken("my-token"),
		WithBaseURL("https://github.enterprise.com/api/v3"),
		WithHTTPClient(customHTTP),
	)

	if c.token != "my-token" {
		t.Errorf("expected token 'my-token', got %s", c.token)
	}
	if c.baseURL != "https://github.enterprise.com/api/v3" {
		t.Errorf("expected custom base URL, got %s", c.baseURL)
	}
	if c.httpClient != customHTTP {
		t.Error("expected custom HTTP client")
	}
}

func TestClient_GetIssue_HappyPath(t *testing.T) {
	t.Parallel()

	// Create mock server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check auth header
		auth := r.Header.Get("Authorization")
		if auth != "Bearer test-token" {
			t.Errorf("expected Authorization header, got: %s", auth)
		}

		// Check path
		if r.URL.Path != "/repos/owner/repo/issues/42" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}

		// Return mock issue
		issue := Issue{
			ID:      12345,
			Number:  42,
			Title:   "Test Issue",
			Body:    "This is a test issue body",
			State:   "open",
			URL:     "https://api.github.com/repos/owner/repo/issues/42",
			HTMLURL: "https://github.com/owner/repo/issues/42",
			Labels: []Label{
				{Name: "bug", Color: "d73a4a"},
				{Name: "good first issue", Color: "7057ff"},
			},
			User: User{
				ID:    1,
				Login: "testuser",
				Type:  "User",
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(issue)
	}))
	defer server.Close()

	client := NewClient(
		WithToken("test-token"),
		WithBaseURL(server.URL),
		WithHTTPClient(server.Client()),
	)

	ctx := context.Background()
	issue, err := client.GetIssue(ctx, "owner", "repo", 42)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if issue.Number != 42 {
		t.Errorf("expected issue 42, got %d", issue.Number)
	}
	if issue.Title != "Test Issue" {
		t.Errorf("expected title 'Test Issue', got %s", issue.Title)
	}
	if len(issue.Labels) != 2 {
		t.Errorf("expected 2 labels, got %d", len(issue.Labels))
	}
	if issue.Labels[0].Name != "bug" {
		t.Errorf("expected first label 'bug', got %s", issue.Labels[0].Name)
	}
}

func TestClient_GetIssue_Closed(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		issue := Issue{
			Number: 99,
			Title:  "Closed Issue",
			State:  "closed",
			Labels: []Label{},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(issue)
	}))
	defer server.Close()

	client := NewClient(
		WithToken("test-token"),
		WithBaseURL(server.URL),
		WithHTTPClient(server.Client()),
	)

	issue, err := client.GetIssue(context.Background(), "owner", "repo", 99)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !issue.Closed() {
		t.Error("expected issue to be closed")
	}
	if issue.State != "closed" {
		t.Errorf("expected state 'closed', got %s", issue.State)
	}
}

func TestClient_GetIssue_AuthError(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"message": "Bad credentials",
		})
	}))
	defer server.Close()

	client := NewClient(
		WithToken("bad-token"),
		WithBaseURL(server.URL),
		WithHTTPClient(server.Client()),
	)

	_, err := client.GetIssue(context.Background(), "owner", "repo", 1)
	if err == nil {
		t.Fatal("expected error for auth failure")
	}

	if !IsAuth(err) {
		t.Errorf("expected IsAuth error, got: %T: %v", err, err)
	}

	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Errorf("expected *APIError, got %T", err)
	} else {
		if apiErr.StatusCode != 401 {
			t.Errorf("expected status 401, got %d", apiErr.StatusCode)
		}
	}
}

func TestClient_GetIssue_NotFound(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"message":           "Not Found",
			"documentation_url": "https://docs.github.com/rest/issues/issues#get-an-issue",
		})
	}))
	defer server.Close()

	client := NewClient(
		WithToken("test-token"),
		WithBaseURL(server.URL),
		WithHTTPClient(server.Client()),
	)

	_, err := client.GetIssue(context.Background(), "owner", "repo", 999)
	if err == nil {
		t.Fatal("expected error for not found")
	}

	if !IsNotFound(err) {
		t.Errorf("expected IsNotFound error, got: %T: %v", err, err)
	}
}

func TestClient_GetIssue_RateLimited(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Headers must be set before WriteHeader
		w.Header().Set("X-RateLimit-Remaining", "0")
		w.WriteHeader(http.StatusForbidden)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"message": "API rate limit exceeded",
		})
	}))
	defer server.Close()

	client := NewClient(
		WithToken("test-token"),
		WithBaseURL(server.URL),
		WithHTTPClient(server.Client()),
	)

	_, err := client.GetIssue(context.Background(), "owner", "repo", 1)
	if err == nil {
		t.Fatal("expected error for rate limit")
	}

	if !IsRateLimited(err) {
		t.Errorf("expected IsRateLimited error, got: %T: %v", err, err)
	}

	var apiErr *APIError
	if errors.As(err, &apiErr) {
		if apiErr.StatusCode != 403 {
			t.Errorf("expected status 403, got %d", apiErr.StatusCode)
		}
		if apiErr.Type != "RATE_LIMITED" {
			t.Errorf("expected Type RATE_LIMITED, got %s", apiErr.Type)
		}
	}
}

func TestClient_GetIssue_RateLimited_ByMessage(t *testing.T) {
	t.Parallel()

	// Test rate limit detection via message content (no header)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"message": "API rate limit exceeded for user",
		})
	}))
	defer server.Close()

	client := NewClient(
		WithToken("test-token"),
		WithBaseURL(server.URL),
		WithHTTPClient(server.Client()),
	)

	_, err := client.GetIssue(context.Background(), "owner", "repo", 1)
	if err == nil {
		t.Fatal("expected error for rate limit")
	}

	if !IsRateLimited(err) {
		t.Errorf("expected IsRateLimited error, got: %T: %v", err, err)
	}
}

func TestClient_GetIssue_ServerError(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"message": "Service temporarily unavailable",
		})
	}))
	defer server.Close()

	client := NewClient(
		WithToken("test-token"),
		WithBaseURL(server.URL),
		WithHTTPClient(server.Client()),
	)

	_, err := client.GetIssue(context.Background(), "owner", "repo", 1)
	if err == nil {
		t.Fatal("expected error for server error")
	}

	var apiErr *APIError
	if errors.As(err, &apiErr) {
		if !apiErr.IsServerError() {
			t.Error("expected IsServerError to be true")
		}
		if apiErr.StatusCode != 503 {
			t.Errorf("expected status 503, got %d", apiErr.StatusCode)
		}
	}
}

func TestClient_GetIssue_InvalidRepo(t *testing.T) {
	t.Parallel()

	client := NewClient(WithToken("test-token"))

	// Missing owner
	_, err := client.GetIssue(context.Background(), "", "repo", 1)
	if !errors.Is(err, ErrInvalidRepo) {
		t.Errorf("expected ErrInvalidRepo, got: %v", err)
	}

	// Missing repo name
	_, err = client.GetIssue(context.Background(), "owner", "", 1)
	if !errors.Is(err, ErrInvalidRepo) {
		t.Errorf("expected ErrInvalidRepo, got: %v", err)
	}
}

func TestClient_GetIssue_MissingToken(t *testing.T) {
	t.Parallel()

	client := NewClient() // No token

	_, err := client.GetIssue(context.Background(), "owner", "repo", 1)
	if !errors.Is(err, ErrMissingToken) {
		t.Errorf("expected ErrMissingToken, got: %v", err)
	}
}

func TestAPIError_Error(t *testing.T) {
	t.Parallel()

	err := &APIError{
		StatusCode: 404,
		Message:    "Not Found",
		Type:       "NOT_FOUND",
	}

	msg := err.Error()
	if !strings.Contains(msg, "404") {
		t.Errorf("expected error to contain status code, got: %s", msg)
	}
	if !strings.Contains(msg, "Not Found") {
		t.Errorf("expected error to contain message, got: %s", msg)
	}
}

func TestAPIError_Unwrap(t *testing.T) {
	t.Parallel()

	tests := []struct {
		status   int
		errType  string
		expected error
	}{
		{401, "", ErrAuth},
		{403, "", ErrAuth},                    // non-rate-limited 403
		{403, "RATE_LIMITED", ErrRateLimited}, // rate-limited 403
		{404, "", ErrNotFound},
		{429, "", ErrRateLimited},
		{500, "", ErrServer},
		{503, "", ErrServer},
		{400, "", ErrInvalidRequest},
		{422, "", ErrInvalidRequest},
		{200, "", nil}, // Success codes don't map
	}

	for _, tc := range tests {
		t.Run(fmt.Sprintf("status_%d_%s", tc.status, tc.errType), func(t *testing.T) {
			err := &APIError{StatusCode: tc.status, Type: tc.errType}
			unwrapped := err.Unwrap()
			if tc.expected == nil {
				if unwrapped != nil {
					t.Errorf("expected nil unwrap, got %v", unwrapped)
				}
			} else if !errors.Is(err, tc.expected) {
				t.Errorf("expected unwrap to be %v, got %v", tc.expected, unwrapped)
			}
		})
	}
}

func TestSetToken(t *testing.T) {
	t.Parallel()

	client := NewClient()
	if client.token != "" {
		t.Error("expected empty token initially")
	}

	client.SetToken("new-token")
	if client.token != "new-token" {
		t.Errorf("expected token 'new-token', got %s", client.token)
	}
}
