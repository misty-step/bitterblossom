package github

import "time"

// Issue represents a GitHub issue as returned by the REST API.
type Issue struct {
	ID          int64      `json:"id"`
	Number      int        `json:"number"`
	Title       string     `json:"title"`
	Body        string     `json:"body"`
	State       string     `json:"state"`
	StateReason string     `json:"state_reason,omitempty"`
	URL         string     `json:"url"`
	HTMLURL     string     `json:"html_url"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	ClosedAt    *time.Time `json:"closed_at,omitempty"`
	Labels      []Label    `json:"labels"`
	Assignees   []User     `json:"assignees"`
	User        User       `json:"user"`
}

// Closed returns whether the issue is closed, derived from State.
func (i *Issue) Closed() bool {
	return i.State == "closed"
}

// Label represents a GitHub issue label.
type Label struct {
	ID          int64  `json:"id,omitempty"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Color       string `json:"color,omitempty"`
	URL         string `json:"url,omitempty"`
}

// User represents a GitHub user.
type User struct {
	ID        int64  `json:"id"`
	Login     string `json:"login"`
	Type      string `json:"type"` // "User", "Organization", "Bot"
	AvatarURL string `json:"avatar_url,omitempty"`
	HTMLURL   string `json:"html_url,omitempty"`
}

// ErrorResponse represents GitHub's error response structure.
type ErrorResponse struct {
	Message          string       `json:"message"`
	Errors           []FieldError `json:"errors,omitempty"`
	DocumentationURL string       `json:"documentation_url,omitempty"`
}

// FieldError represents field-level validation errors.
type FieldError struct {
	Resource string `json:"resource"`
	Field    string `json:"field"`
	Code     string `json:"code"`
	Message  string `json:"message,omitempty"`
}
