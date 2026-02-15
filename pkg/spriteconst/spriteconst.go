// Package spriteconst provides shared constants and types for sprite operations
// across the bitterblossom codebase.
package spriteconst

// DefaultWorkspace is the default path on sprites where prompts and status
// artifacts are written. This constant is used by dispatch, watchdog, and
// monitoring components.
const DefaultWorkspace = "/home/sprite/workspace"

// StatusFile represents the on-disk STATUS.json artifact that tracks agent
// execution state. It is used by dispatch, watchdog, and monitor packages.
type StatusFile struct {
	Repo    string `json:"repo,omitempty"`
	Issue   int    `json:"issue,omitempty"`
	Started string `json:"started,omitempty"`
	Mode    string `json:"mode,omitempty"`
	Task    string `json:"task,omitempty"`
}
