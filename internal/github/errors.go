// Package github provides a typed GitHub API client for issue operations.
package github

import (
	"errors"
	"fmt"
)

// Common errors for GitHub API operations.
var (
	// ErrAuth indicates authentication/authorization failure (401/403).
	ErrAuth = errors.New("github: authentication failed")

	// ErrNotFound indicates the requested resource was not found (404).
	ErrNotFound = errors.New("github: resource not found")

	// ErrRateLimited indicates rate limit has been exceeded (429).
	ErrRateLimited = errors.New("github: rate limit exceeded")

	// ErrServer indicates a server-side error (5xx).
	ErrServer = errors.New("github: server error")

	// ErrInvalidRequest indicates a client-side request error (400, 422).
	ErrInvalidRequest = errors.New("github: invalid request")

	// ErrMissingToken indicates no API token was provided.
	ErrMissingToken = errors.New("github: missing API token")

	// ErrInvalidRepo indicates invalid repository format.
	ErrInvalidRepo = errors.New("github: invalid repository format")

	// ErrInvalidResponse indicates the API response could not be parsed.
	ErrInvalidResponse = errors.New("github: invalid response")
)

// APIError captures structured error information from GitHub API responses.
type APIError struct {
	StatusCode int
	Message    string
	Type       string // GitHub's error type (e.g., "NOT_FOUND", "RATE_LIMITED")
	URL        string // The request URL for debugging
}

func (e *APIError) Error() string {
	if e.Message != "" {
		return fmt.Sprintf("github api error %d (%s): %s", e.StatusCode, e.Type, e.Message)
	}
	return fmt.Sprintf("github api error %d", e.StatusCode)
}

// IsAuth returns true if this error represents an authentication failure.
func (e *APIError) IsAuth() bool {
	return e.StatusCode == 401
}

// IsNotFound returns true if this error represents a missing resource.
func (e *APIError) IsNotFound() bool {
	return e.StatusCode == 404
}

// IsRateLimited returns true if this error represents rate limiting.
func (e *APIError) IsRateLimited() bool {
	return e.StatusCode == 429 || e.StatusCode == 403 && e.Type == "RATE_LIMITED"
}

// IsServerError returns true if this error represents a server-side failure.
func (e *APIError) IsServerError() bool {
	return e.StatusCode >= 500
}

// Unwrap returns an appropriate sentinel error for classification.
func (e *APIError) Unwrap() error {
	switch {
	case e.IsAuth():
		return ErrAuth
	case e.IsNotFound():
		return ErrNotFound
	case e.IsRateLimited():
		return ErrRateLimited
	case e.IsServerError():
		return ErrServer
	case e.StatusCode >= 400:
		return ErrInvalidRequest
	default:
		return nil
	}
}

// IsAuth checks if an error is an authentication failure.
func IsAuth(err error) bool {
	return errors.Is(err, ErrAuth)
}

// IsNotFound checks if an error is a "not found" response.
func IsNotFound(err error) bool {
	return errors.Is(err, ErrNotFound)
}

// IsRateLimited checks if an error is rate limiting.
func IsRateLimited(err error) bool {
	return errors.Is(err, ErrRateLimited)
}
