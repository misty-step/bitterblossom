package github

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"
)

const (
	// DefaultBaseURL is the GitHub API base URL.
	DefaultBaseURL = "https://api.github.com"

	// DefaultTimeout is the default request timeout.
	DefaultTimeout = 15 * time.Second
)

// Client provides typed access to the GitHub REST API.
//
// A Client must not be used concurrently for token mutation (SetToken)
// and requests. Construct the client with the final token or create
// separate clients per goroutine.
type Client struct {
	token      string
	baseURL    string
	httpClient *http.Client
}

// Option configures a Client.
type Option func(*Client)

// WithToken sets the authentication token.
func WithToken(token string) Option {
	return func(c *Client) {
		c.token = token
	}
}

// WithBaseURL sets a custom base URL (for Enterprise or testing).
func WithBaseURL(url string) Option {
	return func(c *Client) {
		c.baseURL = strings.TrimSuffix(url, "/")
	}
}

// WithHTTPClient sets a custom HTTP client.
func WithHTTPClient(httpClient *http.Client) Option {
	return func(c *Client) {
		c.httpClient = httpClient
	}
}

// NewClient creates a new GitHub API client.
func NewClient(opts ...Option) *Client {
	c := &Client{
		baseURL:    DefaultBaseURL,
		httpClient: &http.Client{Timeout: DefaultTimeout},
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// NewClientFromToken creates a client with just a token (convenience).
func NewClientFromToken(token string) *Client {
	return NewClient(WithToken(token))
}

// SetToken updates the client's authentication token.
func (c *Client) SetToken(token string) {
	c.token = token
}

// GetIssue fetches a single issue by number.
func (c *Client) GetIssue(ctx context.Context, owner, repo string, number int) (*Issue, error) {
	if err := c.validateRepo(owner, repo); err != nil {
		return nil, err
	}

	endpoint := fmt.Sprintf("/repos/%s/%s/issues/%d", owner, repo, number)
	req, err := c.newRequest(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}

	issue := &Issue{}
	if err := c.do(req, issue); err != nil {
		return nil, err
	}

	return issue, nil
}

// validateRepo checks that owner and repo are provided.
func (c *Client) validateRepo(owner, repo string) error {
	if owner == "" || repo == "" {
		return ErrInvalidRepo
	}
	return nil
}

// newRequest creates an HTTP request with proper headers.
func (c *Client) newRequest(ctx context.Context, method, endpoint string, body io.Reader) (*http.Request, error) {
	if c.token == "" {
		return nil, ErrMissingToken
	}

	url, err := c.fullURL(endpoint)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return nil, fmt.Errorf("github: create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	return req, nil
}

// fullURL resolves an endpoint path to a full URL.
// The endpoint may include query parameters (e.g., "/repos/o/r/issues?state=open").
func (c *Client) fullURL(endpoint string) (string, error) {
	base, err := url.Parse(c.baseURL)
	if err != nil {
		return "", fmt.Errorf("github: invalid base URL: %w", err)
	}

	// Parse the endpoint to separate path from query
	endpointURL, err := url.Parse(endpoint)
	if err != nil {
		return "", fmt.Errorf("github: invalid endpoint: %w", err)
	}

	// Join the base path with the endpoint path
	base.Path = path.Join(base.Path, endpointURL.Path)
	// Preserve query parameters
	base.RawQuery = endpointURL.RawQuery

	return base.String(), nil
}

// do executes an HTTP request and decodes the response into v.
func (c *Client) do(req *http.Request, v any) error {
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("github: request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= http.StatusBadRequest {
		return c.handleError(resp)
	}

	if v != nil {
		if err := json.NewDecoder(resp.Body).Decode(v); err != nil {
			return fmt.Errorf("github: decode response: %w", err)
		}
	}

	return nil
}

// handleError processes error responses and returns appropriate errors.
func (c *Client) handleError(resp *http.Response) error {
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		body = []byte("failed to read error body")
	}

	errData := &ErrorResponse{}
	if err := json.Unmarshal(body, errData); err != nil {
		// Fallback if error body doesn't match expected structure
		errData.Message = strings.TrimSpace(string(body))
	}

	// Detect rate limiting from 403 responses via header or message content.
	// GitHub returns 403 (not 429) for most rate limits, with X-RateLimit-Remaining: 0.
	errType := ""
	if resp.StatusCode == http.StatusForbidden &&
		(resp.Header.Get("X-RateLimit-Remaining") == "0" ||
			strings.Contains(strings.ToLower(errData.Message), "rate limit")) {
		errType = "RATE_LIMITED"
	}

	apiErr := &APIError{
		StatusCode: resp.StatusCode,
		Message:    errData.Message,
		Type:       errType,
		URL:        resp.Request.URL.String(),
	}

	return apiErr
}
