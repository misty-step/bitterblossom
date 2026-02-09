package fly

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	// DefaultBaseURL is the Sprites API endpoint (sprites.dev).
	DefaultBaseURL = "https://api.sprites.dev/v1"
)

var (
	// ErrMissingToken indicates no API token was provided.
	ErrMissingToken = errors.New("missing API token")
	// ErrMissingApp indicates an empty app name in a request.
	ErrMissingApp = errors.New("missing app")
	// ErrMissingMachineID indicates an empty machine id in a request.
	ErrMissingMachineID = errors.New("missing machine id")
	// ErrMissingCommand indicates no command was supplied for exec.
	ErrMissingCommand = errors.New("missing exec command")
	// ErrMockNotImplemented indicates no behavior is configured for a mock method.
	ErrMockNotImplemented = errors.New("mock method not implemented")
)

// MachineClient defines the operations needed by fleet reconciliation.
type MachineClient interface {
	Create(ctx context.Context, req CreateRequest) (Machine, error)
	Destroy(ctx context.Context, app, machineID string) error
	List(ctx context.Context, app string) ([]Machine, error)
	Status(ctx context.Context, app, machineID string) (Machine, error)
	Exec(ctx context.Context, app, machineID string, req ExecRequest) (ExecResult, error)
}

// Machine is the subset of Fly machine metadata the reconciler needs.
type Machine struct {
	ID       string            `json:"id"`
	Name     string            `json:"name"`
	State    string            `json:"state"`
	Region   string            `json:"region,omitempty"`
	ImageRef string            `json:"image_ref,omitempty"`
	Metadata map[string]string `json:"metadata,omitempty"`
}

// CreateRequest is the payload for machine creation.
type CreateRequest struct {
	App      string            `json:"app"`
	Name     string            `json:"name"`
	Region   string            `json:"region,omitempty"`
	Config   map[string]any    `json:"config,omitempty"`
	Metadata map[string]string `json:"metadata,omitempty"`
}

// ExecRequest describes a command execution on a machine.
type ExecRequest struct {
	Command        []string `json:"command"`
	TimeoutSeconds int      `json:"timeout_seconds,omitempty"`
}

// ExecResult captures command execution output.
type ExecResult struct {
	ExitCode int    `json:"exit_code"`
	Stdout   string `json:"stdout,omitempty"`
	Stderr   string `json:"stderr,omitempty"`
}

// APIError captures non-success HTTP responses.
type APIError struct {
	StatusCode int
	Body       string
}

func (e APIError) Error() string {
	if strings.TrimSpace(e.Body) == "" {
		return fmt.Sprintf("fly api error: status %d", e.StatusCode)
	}
	return fmt.Sprintf("fly api error: status %d: %s", e.StatusCode, strings.TrimSpace(e.Body))
}

// Client is the Sprites API implementation.
type Client struct {
	token      string
	baseURL    string
	httpClient *http.Client
	maxRetries int
	baseDelay  time.Duration
	sleep      func(time.Duration)
}

// Option customizes a Client.
type Option func(*Client)

// WithHTTPClient overrides the default HTTP client.
func WithHTTPClient(httpClient *http.Client) Option {
	return func(client *Client) {
		if httpClient != nil {
			client.httpClient = httpClient
		}
	}
}

// WithBaseURL overrides the default Sprites API URL.
func WithBaseURL(baseURL string) Option {
	return func(client *Client) {
		if strings.TrimSpace(baseURL) != "" {
			client.baseURL = strings.TrimRight(baseURL, "/")
		}
	}
}

// WithRetry configures retry behavior for transient failures.
func WithRetry(maxRetries int, baseDelay time.Duration) Option {
	return func(client *Client) {
		if maxRetries >= 0 {
			client.maxRetries = maxRetries
		}
		if baseDelay > 0 {
			client.baseDelay = baseDelay
		}
	}
}

// WithSleepFn overrides the sleep function (useful for tests).
func WithSleepFn(sleepFn func(time.Duration)) Option {
	return func(client *Client) {
		if sleepFn != nil {
			client.sleep = sleepFn
		}
	}
}

// NewClient constructs a Sprites API client.
func NewClient(token string, opts ...Option) (*Client, error) {
	if strings.TrimSpace(token) == "" {
		return nil, ErrMissingToken
	}

	client := &Client{
		token:      token,
		baseURL:    DefaultBaseURL,
		httpClient: &http.Client{Timeout: 15 * time.Second},
		maxRetries: 3,
		baseDelay:  250 * time.Millisecond,
		sleep:      time.Sleep,
	}
	for _, opt := range opts {
		opt(client)
	}
	return client, nil
}

// Create creates a machine in the given app.
func (c *Client) Create(ctx context.Context, req CreateRequest) (Machine, error) {
	if strings.TrimSpace(req.App) == "" {
		return Machine{}, ErrMissingApp
	}
	if strings.TrimSpace(req.Name) == "" {
		return Machine{}, errors.New("missing machine name")
	}

	path := fmt.Sprintf("/apps/%s/machines", url.PathEscape(req.App))
	payload := map[string]any{}
	if req.Name != "" {
		payload["name"] = req.Name
	}
	if req.Region != "" {
		payload["region"] = req.Region
	}
	if len(req.Config) > 0 {
		payload["config"] = req.Config
	}
	if len(req.Metadata) > 0 {
		payload["metadata"] = req.Metadata
	}

	var machine Machine
	if err := c.doJSON(ctx, http.MethodPost, path, payload, &machine); err != nil {
		return Machine{}, err
	}
	return machine, nil
}

// Destroy deletes a machine.
func (c *Client) Destroy(ctx context.Context, app, machineID string) error {
	if strings.TrimSpace(app) == "" {
		return ErrMissingApp
	}
	if strings.TrimSpace(machineID) == "" {
		return ErrMissingMachineID
	}

	path := fmt.Sprintf("/apps/%s/machines/%s?force=true", url.PathEscape(app), url.PathEscape(machineID))
	return c.doJSON(ctx, http.MethodDelete, path, nil, nil)
}

// List returns all machines for an app.
func (c *Client) List(ctx context.Context, app string) ([]Machine, error) {
	if strings.TrimSpace(app) == "" {
		return nil, ErrMissingApp
	}

	path := fmt.Sprintf("/apps/%s/machines", url.PathEscape(app))
	body, err := c.doRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}

	machines := []Machine{}
	if err := json.Unmarshal(body, &machines); err == nil {
		return machines, nil
	}

	var wrapped struct {
		Machines []Machine `json:"machines"`
	}
	if err := json.Unmarshal(body, &wrapped); err != nil {
		return nil, fmt.Errorf("decode machines list: %w", err)
	}
	return wrapped.Machines, nil
}

// Status fetches current state for one machine.
func (c *Client) Status(ctx context.Context, app, machineID string) (Machine, error) {
	if strings.TrimSpace(app) == "" {
		return Machine{}, ErrMissingApp
	}
	if strings.TrimSpace(machineID) == "" {
		return Machine{}, ErrMissingMachineID
	}

	path := fmt.Sprintf("/apps/%s/machines/%s", url.PathEscape(app), url.PathEscape(machineID))
	var machine Machine
	if err := c.doJSON(ctx, http.MethodGet, path, nil, &machine); err != nil {
		return Machine{}, err
	}
	return machine, nil
}

// Exec runs a command on a machine.
func (c *Client) Exec(ctx context.Context, app, machineID string, req ExecRequest) (ExecResult, error) {
	if strings.TrimSpace(app) == "" {
		return ExecResult{}, ErrMissingApp
	}
	if strings.TrimSpace(machineID) == "" {
		return ExecResult{}, ErrMissingMachineID
	}
	if len(req.Command) == 0 {
		return ExecResult{}, ErrMissingCommand
	}

	path := fmt.Sprintf("/apps/%s/machines/%s/exec", url.PathEscape(app), url.PathEscape(machineID))
	var result ExecResult
	if err := c.doJSON(ctx, http.MethodPost, path, req, &result); err != nil {
		return ExecResult{}, err
	}
	return result, nil
}

func (c *Client) doJSON(ctx context.Context, method, path string, payload any, out any) error {
	body, err := c.doRequest(ctx, method, path, payload)
	if err != nil {
		return err
	}
	if out == nil || len(body) == 0 {
		return nil
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	return nil
}

func (c *Client) doRequest(ctx context.Context, method, path string, payload any) ([]byte, error) {
	var requestBody []byte
	var err error
	if payload != nil {
		requestBody, err = json.Marshal(payload)
		if err != nil {
			return nil, fmt.Errorf("marshal payload: %w", err)
		}
	}

	var lastErr error
	for attempt := 0; attempt <= c.maxRetries; attempt++ {
		bodyReader := io.Reader(nil)
		if len(requestBody) > 0 {
			bodyReader = bytes.NewReader(requestBody)
		}

		request, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, bodyReader)
		if err != nil {
			return nil, err
		}
		request.Header.Set("Authorization", "Bearer "+c.token)
		request.Header.Set("Accept", "application/json")
		if payload != nil {
			request.Header.Set("Content-Type", "application/json")
		}

		response, err := c.httpClient.Do(request)
		if err != nil {
			lastErr = err
			if attempt < c.maxRetries && isTransientError(err) {
				c.sleep(backoffDelay(c.baseDelay, attempt))
				continue
			}
			return nil, err
		}

		responseBody, readErr := io.ReadAll(response.Body)
		closeErr := response.Body.Close()
		if readErr != nil {
			return nil, readErr
		}
		if closeErr != nil {
			return nil, closeErr
		}

		if response.StatusCode >= 200 && response.StatusCode < 300 {
			return responseBody, nil
		}

		apiErr := APIError{StatusCode: response.StatusCode, Body: string(responseBody)}
		lastErr = apiErr
		if attempt < c.maxRetries && isTransientStatus(response.StatusCode) {
			c.sleep(backoffDelay(c.baseDelay, attempt))
			continue
		}
		return nil, apiErr
	}

	if lastErr == nil {
		lastErr = errors.New("request failed")
	}
	return nil, lastErr
}

func isTransientError(err error) bool {
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return false
	}
	var netErr net.Error
	if errors.As(err, &netErr) {
		return true
	}
	return true
}

func isTransientStatus(status int) bool {
	switch status {
	case http.StatusRequestTimeout, http.StatusTooManyRequests, http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		return true
	default:
		return status >= 500
	}
}

func backoffDelay(base time.Duration, attempt int) time.Duration {
	delay := base
	for i := 0; i < attempt; i++ {
		delay *= 2
		if delay > 4*time.Second {
			return 4 * time.Second
		}
	}
	return delay
}

// MockMachineClient is an injectable fake for tests.
type MockMachineClient struct {
	CreateFn  func(ctx context.Context, req CreateRequest) (Machine, error)
	DestroyFn func(ctx context.Context, app, machineID string) error
	ListFn    func(ctx context.Context, app string) ([]Machine, error)
	StatusFn  func(ctx context.Context, app, machineID string) (Machine, error)
	ExecFn    func(ctx context.Context, app, machineID string, req ExecRequest) (ExecResult, error)
}

// Create invokes CreateFn or returns ErrMockNotImplemented.
func (m *MockMachineClient) Create(ctx context.Context, req CreateRequest) (Machine, error) {
	if m.CreateFn == nil {
		return Machine{}, ErrMockNotImplemented
	}
	return m.CreateFn(ctx, req)
}

// Destroy invokes DestroyFn or returns ErrMockNotImplemented.
func (m *MockMachineClient) Destroy(ctx context.Context, app, machineID string) error {
	if m.DestroyFn == nil {
		return ErrMockNotImplemented
	}
	return m.DestroyFn(ctx, app, machineID)
}

// List invokes ListFn or returns ErrMockNotImplemented.
func (m *MockMachineClient) List(ctx context.Context, app string) ([]Machine, error) {
	if m.ListFn == nil {
		return nil, ErrMockNotImplemented
	}
	return m.ListFn(ctx, app)
}

// Status invokes StatusFn or returns ErrMockNotImplemented.
func (m *MockMachineClient) Status(ctx context.Context, app, machineID string) (Machine, error) {
	if m.StatusFn == nil {
		return Machine{}, ErrMockNotImplemented
	}
	return m.StatusFn(ctx, app, machineID)
}

// Exec invokes ExecFn or returns ErrMockNotImplemented.
func (m *MockMachineClient) Exec(ctx context.Context, app, machineID string, req ExecRequest) (ExecResult, error) {
	if m.ExecFn == nil {
		return ExecResult{}, ErrMockNotImplemented
	}
	return m.ExecFn(ctx, app, machineID, req)
}
