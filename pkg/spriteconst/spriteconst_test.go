package spriteconst

import (
	"encoding/json"
	"testing"
)

func TestDefaultWorkspace(t *testing.T) {
	if DefaultWorkspace != "/home/sprite/workspace" {
		t.Errorf("DefaultWorkspace = %q, want %q", DefaultWorkspace, "/home/sprite/workspace")
	}
}

func TestStatusFile_MarshalJSON(t *testing.T) {
	// Test with all fields populated
	sf := StatusFile{
		Repo:    "misty-step/test",
		Issue:   42,
		Started: "2024-01-15T10:30:00Z",
		Mode:    "github-issue",
		Task:    "implement feature",
	}

	data, err := json.Marshal(sf)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	// Verify all fields present
	var decoded map[string]interface{}
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if decoded["repo"] != "misty-step/test" {
		t.Errorf("repo field wrong: got %v", decoded["repo"])
	}
	if decoded["issue"] != float64(42) {
		t.Errorf("issue field wrong: got %v", decoded["issue"])
	}
	if decoded["started"] != "2024-01-15T10:30:00Z" {
		t.Errorf("started field wrong: got %v", decoded["started"])
	}
	if decoded["mode"] != "github-issue" {
		t.Errorf("mode field wrong: got %v", decoded["mode"])
	}
	if decoded["task"] != "implement feature" {
		t.Errorf("task field wrong: got %v", decoded["task"])
	}
}

func TestStatusFile_MarshalJSON_Omitempty(t *testing.T) {
	// Test with zero values - should be omitted
	sf := StatusFile{
		Repo: "misty-step/test",
	}

	data, err := json.Marshal(sf)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var decoded map[string]interface{}
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	// Repo should be present
	if decoded["repo"] != "misty-step/test" {
		t.Errorf("repo field wrong: got %v", decoded["repo"])
	}

	// Empty fields should be omitted due to omitempty
	if _, ok := decoded["issue"]; ok {
		t.Error("issue field should be omitted for zero value")
	}
	if _, ok := decoded["started"]; ok {
		t.Error("started field should be omitted for zero value")
	}
	if _, ok := decoded["mode"]; ok {
		t.Error("mode field should be omitted for zero value")
	}
	if _, ok := decoded["task"]; ok {
		t.Error("task field should be omitted for zero value")
	}
}

func TestStatusFile_UnmarshalJSON(t *testing.T) {
	// Test full unmarshal
	input := `{"repo":"misty-step/test","issue":42,"started":"2024-01-15T10:30:00Z","mode":"github-issue","task":"test task"}`

	var sf StatusFile
	if err := json.Unmarshal([]byte(input), &sf); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if sf.Repo != "misty-step/test" {
		t.Errorf("Repo = %q, want %q", sf.Repo, "misty-step/test")
	}
	if sf.Issue != 42 {
		t.Errorf("Issue = %d, want %d", sf.Issue, 42)
	}
	if sf.Started != "2024-01-15T10:30:00Z" {
		t.Errorf("Started = %q, want %q", sf.Started, "2024-01-15T10:30:00Z")
	}
	if sf.Mode != "github-issue" {
		t.Errorf("Mode = %q, want %q", sf.Mode, "github-issue")
	}
	if sf.Task != "test task" {
		t.Errorf("Task = %q, want %q", sf.Task, "test task")
	}
}

func TestStatusFile_UnmarshalJSON_Partial(t *testing.T) {
	// Test backward compatibility - only repo and started (no issue, mode, task)
	input := `{"repo":"misty-step/test","started":"2024-01-15T10:30:00Z"}`

	var sf StatusFile
	if err := json.Unmarshal([]byte(input), &sf); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if sf.Repo != "misty-step/test" {
		t.Errorf("Repo = %q, want %q", sf.Repo, "misty-step/test")
	}
	if sf.Issue != 0 {
		t.Errorf("Issue = %d, want 0", sf.Issue)
	}
	if sf.Started != "2024-01-15T10:30:00Z" {
		t.Errorf("Started = %q, want %q", sf.Started, "2024-01-15T10:30:00Z")
	}
}

func TestStatusFile_UnmarshalJSON_BackwardCompatibility(t *testing.T) {
	// Test monitor format (repo, issue, started)
	input := `{"repo":"misty-step/test","issue":123,"started":"2024-01-15T10:30:00Z"}`

	var sf StatusFile
	if err := json.Unmarshal([]byte(input), &sf); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if sf.Repo != "misty-step/test" {
		t.Errorf("Repo = %q, want %q", sf.Repo, "misty-step/test")
	}
	if sf.Issue != 123 {
		t.Errorf("Issue = %d, want 123", sf.Issue)
	}
	if sf.Started != "2024-01-15T10:30:00Z" {
		t.Errorf("Started = %q, want %q", sf.Started, "2024-01-15T10:30:00Z")
	}
}

func TestStatusFile_UnmarshalJSON_EmptyPayload(t *testing.T) {
	// Test empty object
	input := `{}`

	var sf StatusFile
	if err := json.Unmarshal([]byte(input), &sf); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if sf.Repo != "" {
		t.Errorf("Repo = %q, want empty", sf.Repo)
	}
	if sf.Issue != 0 {
		t.Errorf("Issue = %d, want 0", sf.Issue)
	}
}
