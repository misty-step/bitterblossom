package watchdog

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
)

type probe struct {
	ClaudeCount      int
	AgentRunning     bool
	HasComplete      bool
	HasBlocked       bool
	BlockedSummary   string
	Branch           string
	CommitsLast2h    int
	DirtyRepos       int
	AheadCommits     int
	HasPrompt        bool
	CurrentTaskID    string
	Status           statusFile
	SupervisorState  supervisorState
}

type statusFile struct {
	Repo    string `json:"repo,omitempty"`
	Issue   int    `json:"issue,omitempty"`
	Started string `json:"started,omitempty"`
	Mode    string `json:"mode,omitempty"`
	Task    string `json:"task,omitempty"`
}

type supervisorState struct {
	LastProgressAt time.Time `json:"last_progress_at,omitempty"`
	LastActivity   string    `json:"last_activity,omitempty"`
	Stalled        bool      `json:"stalled"`
}

func parseProbeOutput(output string) (probe, error) {
	values := map[string]string{}
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "__") {
			continue
		}
		parts := strings.SplitN(strings.TrimPrefix(line, "__"), "__", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])
		values[key] = value
	}

	claudeCount, err := parseInt(values["CLAUDE_COUNT"])
	if err != nil {
		return probe{}, fmt.Errorf("invalid claude count: %w", err)
	}
	commits, err := parseInt(values["COMMITS_LAST_2H"])
	if err != nil {
		return probe{}, fmt.Errorf("invalid commits_last_2h: %w", err)
	}
	dirtyRepos, err := parseInt(values["DIRTY_REPOS"])
	if err != nil {
		return probe{}, fmt.Errorf("invalid dirty_repos: %w", err)
	}
	aheadCommits, err := parseInt(values["AHEAD_COMMITS"])
	if err != nil {
		return probe{}, fmt.Errorf("invalid ahead_commits: %w", err)
	}

	blockedSummary, err := decodeB64(values["BLOCKED_B64"])
	if err != nil {
		return probe{}, fmt.Errorf("decode blocked summary: %w", err)
	}
	branch, err := decodeB64(values["BRANCH_B64"])
	if err != nil {
		return probe{}, fmt.Errorf("decode branch: %w", err)
	}
	taskID, err := decodeB64(values["TASK_ID_B64"])
	if err != nil {
		return probe{}, fmt.Errorf("decode task id: %w", err)
	}
	statusJSON, err := decodeB64(values["STATUS_B64"])
	if err != nil {
		return probe{}, fmt.Errorf("decode status json: %w", err)
	}
	supervisorStateJSON, err := decodeB64(values["SUPERVISOR_STATE_B64"])
	if err != nil {
		return probe{}, fmt.Errorf("decode supervisor state json: %w", err)
	}

	status := statusFile{}
	if strings.TrimSpace(statusJSON) != "" {
		if err := json.Unmarshal([]byte(statusJSON), &status); err != nil {
			return probe{}, fmt.Errorf("parse STATUS.json: %w", err)
		}
	}

	supervisorStateVal := supervisorState{}
	if strings.TrimSpace(supervisorStateJSON) != "" {
		if err := json.Unmarshal([]byte(supervisorStateJSON), &supervisorStateVal); err != nil {
			return probe{}, fmt.Errorf("parse supervisor state: %w", err)
		}
	}

	return probe{
		ClaudeCount:      claudeCount,
		AgentRunning:     strings.EqualFold(values["AGENT_RUNNING"], "yes"),
		HasComplete:      strings.EqualFold(values["HAS_COMPLETE"], "yes"),
		HasBlocked:       strings.EqualFold(values["HAS_BLOCKED"], "yes"),
		BlockedSummary:   strings.TrimSpace(blockedSummary),
		Branch:           strings.TrimSpace(branch),
		CommitsLast2h:    commits,
		DirtyRepos:       dirtyRepos,
		AheadCommits:     aheadCommits,
		HasPrompt:        strings.EqualFold(values["HAS_PROMPT"], "yes"),
		CurrentTaskID:    strings.TrimSpace(taskID),
		Status:           status,
		SupervisorState:  supervisorStateVal,
	}, nil
}

func parseInt(value string) (int, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0, err
	}
	if parsed < 0 {
		return 0, fmt.Errorf("negative value %d", parsed)
	}
	return parsed, nil
}

func decodeB64(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", nil
	}
	decoded, err := base64.StdEncoding.DecodeString(raw)
	if err == nil {
		return string(decoded), nil
	}
	decoded, err = base64.RawStdEncoding.DecodeString(raw)
	if err != nil {
		return "", err
	}
	return string(decoded), nil
}
