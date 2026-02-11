package monitor

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/misty-step/bitterblossom/internal/shellutil"
)

const workspace = "/home/sprite/workspace"

// Executor reads remote state from sprites.
type Executor interface {
	Exec(ctx context.Context, sprite, remoteCommand string, stdin []byte) (string, error)
	List(ctx context.Context) ([]string, error)
}

// TaskState is the workflow state for one sprite assignment.
type TaskState string

const (
	TaskStateRunning TaskState = "RUNNING"
	TaskStateDone    TaskState = "DONE"
	TaskStateIdle    TaskState = "IDLE"
	TaskStateError   TaskState = "ERROR"
)

// TaskStatus summarizes active or recent work for one sprite.
type TaskStatus struct {
	Sprite  string
	State   TaskState
	Task    string
	Started string
	Runtime string
	Error   string
}

// FleetRequest controls sprite selection.
type FleetRequest struct {
	Sprites []string
	All     bool
}

// FleetReport is the result table for check-fleet.
type FleetReport struct {
	Sprites []TaskStatus
}

// Monitor checks fleet task status.
type Monitor interface {
	CheckFleet(ctx context.Context, req FleetRequest) (FleetReport, error)
}

// Service implements Monitor over sprite CLI.
type Service struct {
	exec Executor
	now  func() time.Time
}

// NewService constructs a monitor service.
func NewService(exec Executor) *Service {
	if exec == nil {
		panic("monitor.NewService: exec cannot be nil")
	}
	return &Service{
		exec: exec,
		now:  time.Now,
	}
}

// CheckFleet collects status from each target sprite.
func (s *Service) CheckFleet(ctx context.Context, req FleetRequest) (FleetReport, error) {
	targets, err := s.resolveTargets(ctx, req)
	if err != nil {
		return FleetReport{}, err
	}

	statuses := make([]TaskStatus, 0, len(targets))
	for _, sprite := range targets {
		statuses = append(statuses, s.querySprite(ctx, sprite))
	}
	return FleetReport{Sprites: statuses}, nil
}

func (s *Service) resolveTargets(ctx context.Context, req FleetRequest) ([]string, error) {
	if req.All {
		sprites, err := s.exec.List(ctx)
		if err != nil {
			return nil, fmt.Errorf("listing sprites: %w", err)
		}
		return sortedUnique(sprites), nil
	}
	if len(req.Sprites) == 0 {
		return nil, fmt.Errorf("no sprites provided")
	}
	return sortedUnique(req.Sprites), nil
}

func (s *Service) querySprite(ctx context.Context, sprite string) TaskStatus {
	output, err := s.exec.Exec(ctx, sprite, probeScript(), nil)
	if err != nil {
		return TaskStatus{
			Sprite:  sprite,
			State:   TaskStateError,
			Task:    "-",
			Started: "-",
			Runtime: "-",
			Error:   err.Error(),
		}
	}

	fileStatus, alive, parseErr := parseProbeOutput(output)
	if parseErr != nil {
		return TaskStatus{
			Sprite:  sprite,
			State:   TaskStateError,
			Task:    "-",
			Started: "-",
			Runtime: "-",
			Error:   parseErr.Error(),
		}
	}

	state := TaskStateIdle
	switch {
	case alive:
		state = TaskStateRunning
	case fileStatus.Repo != "":
		state = TaskStateDone
	}

	task := "-"
	if fileStatus.Repo != "" {
		task = fmt.Sprintf("%s#%d", fileStatus.Repo, fileStatus.Issue)
	}

	started := "-"
	runtime := "-"
	if fileStatus.Started != "" {
		started = fileStatus.Started
		if parsed, err := time.Parse(time.RFC3339, fileStatus.Started); err == nil {
			delta := s.now().UTC().Sub(parsed)
			if delta > 0 {
				runtime = delta.Round(time.Second).String()
			}
		}
	}

	return TaskStatus{
		Sprite:  sprite,
		State:   state,
		Task:    task,
		Started: started,
		Runtime: runtime,
	}
}

type statusFile struct {
	Repo    string `json:"repo"`
	Issue   int    `json:"issue"`
	Started string `json:"started"`
}

func parseProbeOutput(output string) (statusFile, bool, error) {
	var (
		fileStatus statusFile
		stateLine  string
	)
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		switch {
		case strings.HasPrefix(line, "__STATUS_JSON__"):
			payload := strings.TrimPrefix(line, "__STATUS_JSON__")
			if payload == "" {
				payload = "{}"
			}
			if err := json.Unmarshal([]byte(payload), &fileStatus); err != nil {
				return statusFile{}, false, fmt.Errorf("invalid STATUS.json payload: %w", err)
			}
		case strings.HasPrefix(line, "__AGENT_STATE__"):
			stateLine = strings.TrimPrefix(line, "__AGENT_STATE__")
		}
	}
	if stateLine == "" {
		return statusFile{}, false, fmt.Errorf("missing agent state marker")
	}
	return fileStatus, stateLine == "alive", nil
}

func probeScript() string {
	return strings.Join([]string{
		"STATUS_PATH=" + shellutil.Quote(workspace+"/STATUS.json"),
		"PID_PATH=" + shellutil.Quote(workspace+"/AGENT_PID"),
		"if [ -f \"$STATUS_PATH\" ]; then",
		"  STATUS_JSON=\"$(tr -d '\\n' < \"$STATUS_PATH\")\"",
		"else",
		"  STATUS_JSON='{}'",
		"fi",
		"echo \"__STATUS_JSON__${STATUS_JSON}\"",
		"AGENT_STATE=dead",
		"if [ -f \"$PID_PATH\" ]; then",
		"  PID=\"$(cat \"$PID_PATH\")\"",
		"  if kill -0 \"$PID\" 2>/dev/null; then",
		"    AGENT_STATE=alive",
		"  fi",
		"elif pgrep -f 'claude -p' >/dev/null 2>&1; then",
		"  AGENT_STATE=alive",
		"fi",
		"echo \"__AGENT_STATE__${AGENT_STATE}\"",
	}, "\n")
}

func sortedUnique(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(values))
	uniq := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		uniq = append(uniq, trimmed)
	}
	sort.Strings(uniq)
	return uniq
}

