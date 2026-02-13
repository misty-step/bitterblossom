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
type Client struct {
	token      string
	baseURL    string
	httpClient *http.Client
	maxRetries int
	baseDelay  time.Duration
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

// WithRetry configures retry behavior.
func WithRetry(maxRetries int, baseDelay time.Duration) Option {
	return func(c *Client) {
		c.maxRetries = maxRetries
		c.baseDelay = baseDelay
	}
}

// NewClient creates a new GitHub API client.
func NewClient(opts ...Option) *Client {
	c := &Client{
		baseURL:    DefaultBaseURL,
		httpClient: &http.Client{Timeout: DefaultTimeout},
		maxRetries: 3,
		baseDelay:  time.Second,
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

// ListIssues fetches issues matching the provided options.
func (c *Client) ListIssues(ctx context.Context, owner, repo string, opts *IssueListOptions) ([]Issue, error) {
	if err := c.validateRepo(owner, repo); err != nil {
		return nil, err
	}

	endpoint := fmt.Sprintf("/repos/%s/%s/issues", owner, repo)
	if opts == nil {
		opts = &IssueListOptions{}
	}

	// Build query parameters
	params := url.Values{}
	if opts.State != "" {
		params.Set("state", opts.State)
	} else {
		params.Set("state", "open")
	}
	if len(opts.Labels) > 0 {
		params.Set("labels", strings.Join(opts.Labels, ","))
	}
	if opts.Assignee != "" {
		params.Set("assignee", opts.Assignee)
	}
	if opts.Sort != "" {
		params.Set("sort", opts.Sort)
	}
	if opts.Direction != "" {
		params.Set("direction", opts.Direction)
	}
	if opts.PerPage > 0 {
		params.Set("per_page", fmt.Sprintf("%d", opts.PerPage))
	}
	if opts.Page > 0 {
		params.Set("page", fmt.Sprintf("%d", opts.Page))
	}

	if query := params.Encode(); query != "" {
		endpoint = endpoint + "?" + query
	}

	req, err := c.newRequest(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}

	var issues []Issue
	if err := c.do(req, &issues); err != nil {
		return nil, err
	}

	return issues, nil
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

	apiErr := &APIError{
		StatusCode: resp.StatusCode,
		Message:    errData.Message,
		URL:        resp.Request.URL.String(),
	}

	return apiErr
}
