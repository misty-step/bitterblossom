package dispatch

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/misty-step/bitterblossom/internal/fleet"
	"github.com/misty-step/bitterblossom/pkg/fly"
)

const (
	// DefaultWorkspace is where prompts and status artifacts are written on sprites.
	DefaultWorkspace = "/home/sprite/workspace"
	// DefaultMaxRalphIterations mirrors the shell-script safety cap.
	DefaultMaxRalphIterations = 50
)

var (
	spriteNamePattern = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)
	repoPartPattern   = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)
)

var (
	// ErrInvalidRequest indicates malformed dispatch input.
	ErrInvalidRequest = errors.New("dispatch: invalid request")
	// ErrInvalidStateTransition indicates a bug in dispatch state-machine progression.
	ErrInvalidStateTransition = errors.New("dispatch: invalid state transition")
)

// RemoteClient runs remote commands on sprites and uploads files.
type RemoteClient interface {
	Exec(ctx context.Context, sprite, remoteCommand string, stdin []byte) (string, error)
	Upload(ctx context.Context, sprite, remotePath string, content []byte) error
}

// Request describes a dispatch operation.
type Request struct {
	Sprite     string
	Prompt     string
	Repo       string
	Ralph      bool
	Execute    bool
	WebhookURL string
}

// PlanStep is one dry-run/execute step in the dispatch lifecycle.
type PlanStep struct {
	Kind        StepKind `json:"kind"`
	Description string   `json:"description"`
}

// StepKind identifies dispatch planning/execution phases.
type StepKind string

const (
	StepProvision    StepKind = "provision"
	StepSetupRepo    StepKind = "setup_repo"
	StepUploadPrompt StepKind = "upload_prompt"
	StepWriteStatus  StepKind = "write_status"
	StepStartAgent   StepKind = "start_agent"
)

// Plan is the rendered execution plan for dry-run or execute mode.
type Plan struct {
	Sprite string     `json:"sprite"`
	Mode   string     `json:"mode"`
	Steps  []PlanStep `json:"steps"`
}

// Result is returned from Run.
type Result struct {
	Plan          Plan          `json:"plan"`
	Executed      bool          `json:"executed"`
	State         DispatchState `json:"state"`
	Provisioned   bool          `json:"provisioned"`
	PromptPath    string        `json:"prompt_path"`
	AgentPID      int           `json:"agent_pid,omitempty"`
	CommandOutput string        `json:"command_output,omitempty"`
	StartedAt     time.Time     `json:"started_at,omitempty"`
	Task          string        `json:"task,omitempty"`
}

// Config wires dependencies for dispatching.
type Config struct {
	Remote             RemoteClient
	Fly                fly.MachineClient
	App                string
	Workspace          string
	CompositionPath    string
	RalphTemplatePath  string
	MaxRalphIterations int
	ProvisionConfig    map[string]any
	Logger             *slog.Logger
	Now                func() time.Time
}

type provisionInfo struct {
	Persona       string
	ConfigVersion string
}

// Service executes dispatch plans.
type Service struct {
	remote             RemoteClient
	fly                fly.MachineClient
	app                string
	workspace          string
	maxRalphIterations int
	provisionConfig    map[string]any
	logger             *slog.Logger
	now                func() time.Time
	ralphTemplate      string
	provisionHints     map[string]provisionInfo
}

// NewService constructs a dispatch service.
func NewService(cfg Config) (*Service, error) {
	if cfg.Remote == nil {
		return nil, errors.New("dispatch: remote client is required")
	}
	if cfg.Fly == nil {
		return nil, errors.New("dispatch: fly client is required")
	}
	if strings.TrimSpace(cfg.App) == "" {
		return nil, errors.New("dispatch: fly app is required")
	}
	workspace := strings.TrimSpace(cfg.Workspace)
	if workspace == "" {
		workspace = DefaultWorkspace
	}
	maxIterations := cfg.MaxRalphIterations
	if maxIterations <= 0 {
		maxIterations = DefaultMaxRalphIterations
	}
	logger := cfg.Logger
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	}
	now := cfg.Now
	if now == nil {
		now = time.Now
	}
	template := defaultRalphPromptTemplate
	if path := strings.TrimSpace(cfg.RalphTemplatePath); path != "" {
		content, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("dispatch: read ralph template: %w", err)
		}
		template = string(content)
	}

	hints, err := loadProvisionHints(strings.TrimSpace(cfg.CompositionPath))
	if err != nil {
		return nil, err
	}

	return &Service{
		remote:             cfg.Remote,
		fly:                cfg.Fly,
		app:                strings.TrimSpace(cfg.App),
		workspace:          workspace,
		maxRalphIterations: maxIterations,
		provisionConfig:    copyMap(cfg.ProvisionConfig),
		logger:             logger,
		now:                now,
		ralphTemplate:      template,
		provisionHints:     hints,
	}, nil
}

// Run executes a dispatch request or returns the dry-run plan.
func (s *Service) Run(ctx context.Context, req Request) (Result, error) {
	prepared, err := s.prepare(req)
	if err != nil {
		return Result{}, err
	}

	provisionNeeded, err := s.needsProvision(ctx, prepared.Sprite)
	if err != nil {
		return Result{}, fmt.Errorf("dispatch: determine provisioning need: %w", err)
	}

	plan := s.buildPlan(prepared, provisionNeeded)
	result := Result{
		Plan:       plan,
		Executed:   prepared.Execute,
		State:      StatePending,
		PromptPath: prepared.PromptPath,
		StartedAt:  prepared.StartedAt,
		Task:       prepared.TaskLabel,
	}
	if !prepared.Execute {
		return result, nil
	}

	state := StatePending
	transition := func(event DispatchEvent) error {
		next, err := advanceState(state, event)
		if err != nil {
			return fmt.Errorf("%w: %v", ErrInvalidStateTransition, err)
		}
		s.logger.Info("dispatch transition", "sprite", prepared.Sprite, "from", state, "event", event, "to", next)
		state = next
		result.State = next
		return nil
	}

	if provisionNeeded {
		if err := transition(EventProvisionRequired); err != nil {
			return Result{}, err
		}
		if err := s.provision(ctx, prepared); err != nil {
			if _, failErr := advanceState(state, EventFailure); failErr == nil {
				result.State = StateFailed
			}
			return Result{}, fmt.Errorf("dispatch: provision sprite %q: %w", prepared.Sprite, err)
		}
		result.Provisioned = true
		if err := transition(EventProvisionSucceeded); err != nil {
			return Result{}, err
		}
	} else if err := transition(EventMachineReady); err != nil {
		return Result{}, err
	}

	if prepared.Repo.CloneURL != "" {
		s.logger.Info("dispatch setup repo", "sprite", prepared.Sprite, "repo", prepared.Repo.CloneURL)
		if _, err := s.remote.Exec(ctx, prepared.Sprite, buildSetupRepoScript(s.workspace, prepared.Repo.CloneURL, prepared.Repo.RepoDir), nil); err != nil {
			result.State = StateFailed
			return Result{}, fmt.Errorf("dispatch: setup repo: %w", err)
		}
	}

	s.logger.Info("dispatch upload prompt", "sprite", prepared.Sprite, "path", prepared.PromptPath)
	if err := s.remote.Upload(ctx, prepared.Sprite, prepared.PromptPath, []byte(prepared.Prompt)); err != nil {
		result.State = StateFailed
		return Result{}, fmt.Errorf("dispatch: upload prompt: %w", err)
	}
	if err := transition(EventPromptUploaded); err != nil {
		return Result{}, err
	}

	statusBytes, err := json.Marshal(statusFile{
		Repo:    prepared.Repo.Slug,
		Started: prepared.StartedAt.Format(time.RFC3339),
		Mode:    prepared.Mode,
		Task:    prepared.TaskLabel,
	})
	if err != nil {
		result.State = StateFailed
		return Result{}, fmt.Errorf("dispatch: marshal status: %w", err)
	}
	if err := s.remote.Upload(ctx, prepared.Sprite, s.workspace+"/STATUS.json", append(statusBytes, '\n')); err != nil {
		result.State = StateFailed
		return Result{}, fmt.Errorf("dispatch: upload status: %w", err)
	}

	s.logger.Info("dispatch start agent", "sprite", prepared.Sprite, "mode", prepared.Mode)
	output, err := s.remote.Exec(ctx, prepared.Sprite, prepared.StartCommand, nil)
	if err != nil {
		result.State = StateFailed
		return Result{}, fmt.Errorf("dispatch: start agent: %w", err)
	}
	result.CommandOutput = strings.TrimSpace(output)
	if pid, ok := parsePID(output); ok {
		result.AgentPID = pid
	}

	if err := transition(EventAgentStarted); err != nil {
		return Result{}, err
	}
	if !prepared.Ralph {
		if err := transition(EventOneShotComplete); err != nil {
			return Result{}, err
		}
	}
	return result, nil
}

type statusFile struct {
	Repo    string `json:"repo,omitempty"`
	Started string `json:"started,omitempty"`
	Mode    string `json:"mode,omitempty"`
	Task    string `json:"task,omitempty"`
}

func (s *Service) provision(ctx context.Context, req preparedRequest) error {
	metadata := map[string]string{
		"managed_by": "bb.dispatch",
	}
	for key, value := range req.ProvisionMetadata {
		metadata[key] = value
	}
	_, err := s.fly.Create(ctx, fly.CreateRequest{
		App:      s.app,
		Name:     req.Sprite,
		Config:   copyMap(s.provisionConfig),
		Metadata: metadata,
	})
	return err
}

func (s *Service) needsProvision(ctx context.Context, sprite string) (bool, error) {
	machines, err := s.fly.List(ctx, s.app)
	if err != nil {
		return false, err
	}
	for _, machine := range machines {
		if strings.TrimSpace(machine.Name) == sprite {
			return false, nil
		}
	}
	return true, nil
}

func (s *Service) buildPlan(req preparedRequest, provisionNeeded bool) Plan {
	steps := make([]PlanStep, 0, 5)
	if provisionNeeded {
		steps = append(steps, PlanStep{
			Kind:        StepProvision,
			Description: fmt.Sprintf("create Fly machine for sprite %q", req.Sprite),
		})
	}
	if req.Repo.CloneURL != "" {
		steps = append(steps, PlanStep{
			Kind:        StepSetupRepo,
			Description: fmt.Sprintf("clone/pull repo %q in %s", req.Repo.CloneURL, s.workspace),
		})
	}
	steps = append(steps, PlanStep{
		Kind:        StepUploadPrompt,
		Description: fmt.Sprintf("upload prompt to %s", req.PromptPath),
	})
	steps = append(steps, PlanStep{
		Kind:        StepWriteStatus,
		Description: fmt.Sprintf("write status marker to %s/STATUS.json", s.workspace),
	})
	if req.Ralph {
		steps = append(steps, PlanStep{
			Kind:        StepStartAgent,
			Description: "start Ralph loop via sprite-agent",
		})
	} else {
		steps = append(steps, PlanStep{
			Kind:        StepStartAgent,
			Description: "run one-shot prompt with claude -p",
		})
	}

	mode := "dry-run"
	if req.Execute {
		mode = "execute"
	}
	return Plan{
		Sprite: req.Sprite,
		Mode:   mode,
		Steps:  steps,
	}
}

type preparedRequest struct {
	Request
	Sprite            string
	Repo              repoTarget
	Prompt            string
	PromptPath        string
	StartCommand      string
	StartedAt         time.Time
	Mode              string
	TaskLabel         string
	ProvisionMetadata map[string]string
}

type repoTarget struct {
	Slug     string
	CloneURL string
	RepoDir  string
}

func (s *Service) prepare(req Request) (preparedRequest, error) {
	sprite := strings.TrimSpace(req.Sprite)
	if !spriteNamePattern.MatchString(sprite) {
		return preparedRequest{}, fmt.Errorf("%w: invalid sprite name %q", ErrInvalidRequest, req.Sprite)
	}

	prompt := strings.TrimSpace(req.Prompt)
	if prompt == "" {
		return preparedRequest{}, fmt.Errorf("%w: prompt is required", ErrInvalidRequest)
	}

	repo, err := parseRepo(req.Repo)
	if err != nil {
		return preparedRequest{}, err
	}

	mode := "oneshot"
	promptPath := s.workspace + "/.dispatch-prompt.md"
	if req.Ralph {
		mode = "ralph"
		promptPath = s.workspace + "/PROMPT.md"
		prompt = renderRalphPrompt(s.ralphTemplate, prompt, repo.Slug, sprite)
	}

	startedAt := s.now().UTC()
	startCommand := buildOneShotScript(s.workspace, promptPath)
	if req.Ralph {
		startCommand = buildStartRalphScript(s.workspace, sprite, s.maxRalphIterations, req.WebhookURL)
	}

	taskLabel := prompt
	if repo.Slug != "" {
		taskLabel = repo.Slug + ": " + prompt
	}
	if len(taskLabel) > 220 {
		taskLabel = taskLabel[:217] + "..."
	}

	metadata := map[string]string{}
	if hint, ok := s.provisionHints[sprite]; ok {
		if hint.Persona != "" {
			metadata["persona"] = hint.Persona
		}
		if hint.ConfigVersion != "" {
			metadata["config_version"] = hint.ConfigVersion
		}
	}

	return preparedRequest{
		Request:           req,
		Sprite:            sprite,
		Repo:              repo,
		Prompt:            prompt,
		PromptPath:        promptPath,
		StartCommand:      startCommand,
		StartedAt:         startedAt,
		Mode:              mode,
		TaskLabel:         taskLabel,
		ProvisionMetadata: metadata,
	}, nil
}

func parseRepo(input string) (repoTarget, error) {
	repo := strings.TrimSpace(input)
	if repo == "" {
		return repoTarget{}, nil
	}

	if strings.HasPrefix(repo, "https://") {
		parsed, err := url.Parse(repo)
		if err != nil {
			return repoTarget{}, fmt.Errorf("%w: invalid repo url %q", ErrInvalidRequest, repo)
		}
		name := strings.TrimSuffix(path.Base(parsed.Path), ".git")
		if !repoPartPattern.MatchString(name) {
			return repoTarget{}, fmt.Errorf("%w: invalid repo name in %q", ErrInvalidRequest, repo)
		}
		segments := strings.Split(strings.Trim(strings.TrimSuffix(parsed.Path, ".git"), "/"), "/")
		slug := ""
		if len(segments) >= 2 {
			owner := segments[len(segments)-2]
			repoName := segments[len(segments)-1]
			if repoPartPattern.MatchString(owner) && repoPartPattern.MatchString(repoName) {
				slug = owner + "/" + repoName
			}
		}
		return repoTarget{
			Slug:     slug,
			CloneURL: repo,
			RepoDir:  name,
		}, nil
	}

	owner := "misty-step"
	repoName := repo
	if strings.Contains(repo, "/") {
		parts := strings.Split(repo, "/")
		if len(parts) != 2 {
			return repoTarget{}, fmt.Errorf("%w: repo %q must be owner/repo", ErrInvalidRequest, repo)
		}
		owner = strings.TrimSpace(parts[0])
		repoName = strings.TrimSpace(parts[1])
	}

	if !repoPartPattern.MatchString(owner) || !repoPartPattern.MatchString(repoName) {
		return repoTarget{}, fmt.Errorf("%w: repo %q contains invalid characters", ErrInvalidRequest, repo)
	}

	return repoTarget{
		Slug:     owner + "/" + repoName,
		CloneURL: "https://github.com/" + owner + "/" + repoName + ".git",
		RepoDir:  repoName,
	}, nil
}

func parsePID(output string) (int, bool) {
	for i := len(strings.Split(output, "\n")) - 1; i >= 0; i-- {
		line := strings.TrimSpace(strings.Split(output, "\n")[i])
		if line == "" {
			continue
		}
		line = strings.TrimPrefix(line, "PID:")
		line = strings.TrimSpace(line)
		pid, err := strconv.Atoi(line)
		if err == nil && pid > 0 {
			return pid, true
		}
	}
	return 0, false
}

func buildSetupRepoScript(workspace, cloneURL, repoDir string) string {
	return strings.Join([]string{
		"set -euo pipefail",
		"cd " + shellQuote(workspace),
		"if [ -d " + shellQuote(repoDir) + " ]; then",
		"  cd " + shellQuote(repoDir),
		"  git fetch origin >/dev/null 2>&1 || true",
		"  git pull --ff-only >/dev/null 2>&1 || true",
		"else",
		"  gh repo clone " + shellQuote(cloneURL) + " " + shellQuote(repoDir) + " >/dev/null 2>&1 || git clone " + shellQuote(cloneURL) + " " + shellQuote(repoDir) + " >/dev/null 2>&1",
		"fi",
	}, "\n")
}

func buildOneShotScript(workspace, promptPath string) string {
	return strings.Join([]string{
		"set -euo pipefail",
		"cd " + shellQuote(workspace),
		"cat " + shellQuote(promptPath) + " | claude -p --permission-mode bypassPermissions",
		"rm -f " + shellQuote(promptPath),
	}, "\n")
}

func buildStartRalphScript(workspace, sprite string, maxIterations int, webhookURL string) string {
	lines := []string{
		"set -euo pipefail",
		"WORKSPACE_DIR=" + shellQuote(workspace),
		"mkdir -p \"$WORKSPACE_DIR/logs\"",
		"rm -f \"$WORKSPACE_DIR/TASK_COMPLETE\" \"$WORKSPACE_DIR/BLOCKED.md\"",
		"if [ -f \"$WORKSPACE_DIR/agent.pid\" ] && kill -0 \"$(cat \"$WORKSPACE_DIR/agent.pid\")\" 2>/dev/null; then kill \"$(cat \"$WORKSPACE_DIR/agent.pid\")\" 2>/dev/null || true; fi",
		"if [ -f \"$WORKSPACE_DIR/ralph.pid\" ] && kill -0 \"$(cat \"$WORKSPACE_DIR/ralph.pid\")\" 2>/dev/null; then kill \"$(cat \"$WORKSPACE_DIR/ralph.pid\")\" 2>/dev/null || true; fi",
		"AGENT_BIN=\"$HOME/.local/bin/sprite-agent\"",
		"if [ ! -x \"$AGENT_BIN\" ]; then AGENT_BIN=\"$WORKSPACE_DIR/.sprite-agent.sh\"; fi",
		"if [ ! -x \"$AGENT_BIN\" ]; then echo \"sprite-agent not found\" >&2; exit 1; fi",
		"cd \"$WORKSPACE_DIR\"",
		"printf 'bb-%s-%s\\n' \"$(date -u +%Y%m%d-%H%M%S)\" " + shellQuote(sprite) + " > \"$WORKSPACE_DIR/.current-task-id\"",
	}
	if strings.TrimSpace(webhookURL) != "" {
		lines = append(lines,
			"nohup env SPRITE_NAME="+shellQuote(sprite)+" SPRITE_WEBHOOK_URL="+shellQuote(webhookURL)+" MAX_ITERATIONS="+strconv.Itoa(maxIterations)+" \"$AGENT_BIN\" >/dev/null 2>&1 &",
		)
	} else {
		lines = append(lines,
			"nohup env SPRITE_NAME="+shellQuote(sprite)+" MAX_ITERATIONS="+strconv.Itoa(maxIterations)+" \"$AGENT_BIN\" >/dev/null 2>&1 &",
		)
	}
	lines = append(lines,
		"PID=\"$!\"",
		"echo \"$PID\" > \"$WORKSPACE_DIR/agent.pid\"",
		"echo \"$PID\" > \"$WORKSPACE_DIR/ralph.pid\"",
		"echo \"PID: $PID\"",
	)
	return strings.Join(lines, "\n")
}

func renderRalphPrompt(template, task, repo, sprite string) string {
	content := template
	content = strings.ReplaceAll(content, "{{TASK_DESCRIPTION}}", task)
	if strings.TrimSpace(repo) == "" {
		repo = "OWNER/REPO"
	}
	content = strings.ReplaceAll(content, "{{REPO}}", repo)
	content = strings.ReplaceAll(content, "{{SPRITE_NAME}}", sprite)
	return content
}

func loadProvisionHints(path string) (map[string]provisionInfo, error) {
	if strings.TrimSpace(path) == "" {
		return map[string]provisionInfo{}, nil
	}
	composition, err := fleet.LoadComposition(path)
	if err != nil {
		return nil, fmt.Errorf("dispatch: load composition: %w", err)
	}
	version := ""
	if composition.Version > 0 {
		version = strconv.Itoa(composition.Version)
	}
	hints := make(map[string]provisionInfo, len(composition.Sprites))
	for _, spec := range composition.Sprites {
		hints[spec.Name] = provisionInfo{
			Persona:       spec.Persona.Name,
			ConfigVersion: version,
		}
	}
	return hints, nil
}

func copyMap(in map[string]any) map[string]any {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]any, len(in))
	keys := make([]string, 0, len(in))
	for key := range in {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		out[key] = in[key]
	}
	return out
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", `'"'"'`) + "'"
}
