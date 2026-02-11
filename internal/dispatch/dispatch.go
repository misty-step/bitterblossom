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
	"github.com/misty-step/bitterblossom/internal/proxy"
	"github.com/misty-step/bitterblossom/internal/registry"
	"github.com/misty-step/bitterblossom/pkg/fly"
)

const (
	// DefaultWorkspace is where prompts and status artifacts are written on sprites.
	DefaultWorkspace = "/home/sprite/workspace"
	// DefaultMaxRalphIterations mirrors the shell-script safety cap.
	DefaultMaxRalphIterations = 50
	// DefaultMaxTokens is the default stuck-loop token safety cap for Ralph loops.
	DefaultMaxTokens = 200_000
	// DefaultMaxTime is the default stuck-loop runtime safety cap for Ralph loops.
	DefaultMaxTime = 30 * time.Minute
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
	ExecWithEnv(ctx context.Context, sprite, remoteCommand string, stdin []byte, env map[string]string) (string, error)
	Upload(ctx context.Context, sprite, remotePath string, content []byte) error
	List(ctx context.Context) ([]string, error)
}

// Request describes a dispatch operation.
type Request struct {
	Sprite               string
	Prompt               string
	Repo                 string
	Issue                int
	Ralph                bool
	Execute              bool
	WebhookURL           string
	AllowAnthropicDirect bool
	// MaxTokens / MaxTime apply only to Ralph mode (sprite-agent).
	MaxTokens int
	MaxTime   time.Duration
}

// PlanStep is one dry-run/execute step in the dispatch lifecycle.
type PlanStep struct {
	Kind        StepKind `json:"kind"`
	Description string   `json:"description"`
}

// StepKind identifies dispatch planning/execution phases.
type StepKind string

const (
	StepRegistryLookup StepKind = "registry_lookup"
	StepProvision      StepKind = "provision"
	StepValidateEnv    StepKind = "validate_env"
	StepValidateIssue  StepKind = "validate_issue"
	StepSetupRepo      StepKind = "setup_repo"
	StepUploadPrompt   StepKind = "upload_prompt"
	StepWriteStatus    StepKind = "write_status"
	StepEnsureProxy    StepKind = "ensure_proxy"
	StepStartAgent     StepKind = "start_agent"
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
	// EnvVars are environment variables to pass to sprite exec commands.
	// These are typically auth tokens like OPENROUTER_API_KEY and ANTHROPIC_AUTH_TOKEN.
	EnvVars map[string]string
	// RegistryPath is the path to the sprite registry file.
	// If empty, the default path (~/.config/bb/registry.toml) is used.
	RegistryPath string
	// RegistryRequired enforces that sprites must exist in the registry.
	// When true, dispatch will fail if the sprite is not found in the registry.
	RegistryRequired bool
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
	envVars            map[string]string
	registryPath       string
	registryRequired   bool
	proxyLifecycle     *proxy.Lifecycle
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

	registryPath := strings.TrimSpace(cfg.RegistryPath)
	if registryPath == "" && cfg.RegistryRequired {
		registryPath = registry.DefaultPath()
	}

	svc := &Service{
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
		envVars:            copyStringMap(cfg.EnvVars),
		registryPath:       registryPath,
		registryRequired:   cfg.RegistryRequired,
	}

	// Initialize proxy lifecycle manager
	svc.proxyLifecycle = proxy.NewLifecycle(cfg.Remote)

	return svc, nil
}

// Run executes a dispatch request or returns the dry-run plan.
func (s *Service) Run(ctx context.Context, req Request) (Result, error) {
	prepared, err := s.prepare(req)
	if err != nil {
		return Result{}, err
	}

	provisionNeeded, err := s.needsProvision(ctx, prepared.Sprite, prepared.MachineID)
	if err != nil {
		return Result{}, fmt.Errorf("dispatch: determine provisioning need: %w", err)
	}

	remoteSprite := prepared.Sprite
	if strings.TrimSpace(prepared.MachineID) != "" {
		remoteSprite = strings.TrimSpace(prepared.MachineID)
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
		machineID, err := s.provision(ctx, prepared)
		if err != nil {
			if _, failErr := advanceState(state, EventFailure); failErr == nil {
				result.State = StateFailed
			}
			return result, fmt.Errorf("dispatch: provision sprite %q: %w", prepared.Sprite, err)
		}
		if strings.TrimSpace(machineID) != "" {
			prepared.MachineID = machineID
			remoteSprite = machineID
		}
		result.Provisioned = true
		if err := transition(EventProvisionSucceeded); err != nil {
			return Result{}, err
		}
	} else if err := transition(EventMachineReady); err != nil {
		return Result{}, err
	}

	if !prepared.AllowAnthropicDirect {
		s.logger.Info("dispatch validate env", "sprite", prepared.Sprite)
		keyOutput, err := s.remote.Exec(ctx, remoteSprite, "printenv ANTHROPIC_API_KEY 2>/dev/null || true", nil)
		if err != nil {
			result.State = StateFailed
			return result, fmt.Errorf("dispatch: check sprite env: %w", err)
		}
		env := map[string]string{}
		if key := strings.TrimSpace(keyOutput); key != "" {
			env["ANTHROPIC_API_KEY"] = key
		}
		if err := ValidateNoDirectAnthropic(env, false); err != nil {
			result.State = StateFailed
			return result, err
		}
	}

	if prepared.Repo.CloneURL != "" {
		s.logger.Info("dispatch setup repo", "sprite", prepared.Sprite, "repo", prepared.Repo.CloneURL)
		if _, err := s.remote.Exec(ctx, remoteSprite, buildSetupRepoScript(s.workspace, prepared.Repo.CloneURL, prepared.Repo.RepoDir), nil); err != nil {
			result.State = StateFailed
			return result, fmt.Errorf("dispatch: setup repo: %w", err)
		}
	}

	s.logger.Info("dispatch upload prompt", "sprite", prepared.Sprite, "path", prepared.PromptPath)
	if err := s.remote.Upload(ctx, remoteSprite, prepared.PromptPath, []byte(prepared.Prompt)); err != nil {
		result.State = StateFailed
		return result, fmt.Errorf("dispatch: upload prompt: %w", err)
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
		return result, fmt.Errorf("dispatch: marshal status: %w", err)
	}
	if err := s.remote.Upload(ctx, remoteSprite, s.workspace+"/STATUS.json", append(statusBytes, '\n')); err != nil {
		result.State = StateFailed
		return result, fmt.Errorf("dispatch: upload status: %w", err)
	}

	// Ensure proxy is running if OPENROUTER_API_KEY is configured
	execEnvVars := copyStringMap(s.envVars)
	if openRouterKey, ok := s.envVars["OPENROUTER_API_KEY"]; ok && openRouterKey != "" {
		s.logger.Info("dispatch ensure proxy", "sprite", prepared.Sprite)
		proxyURL, err := s.proxyLifecycle.EnsureProxy(ctx, remoteSprite, openRouterKey)
		if err != nil {
			result.State = StateFailed
			return result, fmt.Errorf("dispatch: ensure proxy: %w", err)
		}
		s.logger.Info("dispatch proxy ready", "sprite", prepared.Sprite, "url", proxyURL)
		// Set proxy environment variables for the agent
		execEnvVars["ANTHROPIC_BASE_URL"] = proxyURL
		execEnvVars["ANTHROPIC_API_KEY"] = "proxy-mode"
	}

	s.logger.Info("dispatch start agent", "sprite", prepared.Sprite, "mode", prepared.Mode)
	output, err := s.remote.ExecWithEnv(ctx, remoteSprite, prepared.StartCommand, nil, execEnvVars)
	if err != nil {
		result.State = StateFailed
		return result, fmt.Errorf("dispatch: start agent: %w", err)
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

func (s *Service) provision(ctx context.Context, req preparedRequest) (string, error) {
	metadata := map[string]string{
		"managed_by": "bb.dispatch",
	}
	for key, value := range req.ProvisionMetadata {
		metadata[key] = value
	}
	machine, err := s.fly.Create(ctx, fly.CreateRequest{
		App:      s.app,
		Name:     req.Sprite,
		Config:   copyMap(s.provisionConfig),
		Metadata: metadata,
	})
	if err != nil {
		return "", err
	}

	// Register the sprite in the registry if a registry path is configured
	if s.registryPath != "" {
		if err := s.registerSprite(req.Sprite, machine.ID); err != nil {
			// Log the error but don't fail the provision - the sprite exists
			s.logger.Warn("failed to register sprite in registry", "sprite", req.Sprite, "machine_id", machine.ID, "error", err)
		} else {
			s.logger.Info("registered sprite in registry", "sprite", req.Sprite, "machine_id", machine.ID)
		}
	}

	return machine.ID, nil
}

// registerSprite adds a sprite to the registry.
func (s *Service) registerSprite(name, machineID string) error {
	return registry.WithLockedRegistry(s.registryPath, func(reg *registry.Registry) error {
		reg.Register(name, machineID)
		return nil
	})
}

func (s *Service) needsProvision(ctx context.Context, sprite string, machineID string) (bool, error) {
	// prepare() already performed registry resolution. If we have a machine ID, provisioning is not needed.
	if machineID != "" {
		s.logger.Debug("sprite found in registry", "sprite", sprite, "machine_id", machineID)
		return false, nil
	}

	// Check if sprite exists using sprite CLI instead of Fly API
	// This avoids 404 errors when the Fly app doesn't exist or API issues occur
	sprites, err := s.remote.List(ctx)
	if err != nil {
		return false, fmt.Errorf("list sprites: %w", err)
	}
	for _, name := range sprites {
		if strings.TrimSpace(name) == sprite {
			return false, nil
		}
	}
	return true, nil
}

func (s *Service) buildPlan(req preparedRequest, provisionNeeded bool) Plan {
	steps := make([]PlanStep, 0, 7)

	// Add registry lookup step if registry is configured
	if s.registryPath != "" || s.registryRequired {
		desc := fmt.Sprintf("lookup sprite %q in registry", req.Sprite)
		if req.MachineID != "" {
			desc = fmt.Sprintf("lookup sprite %q in registry (found: %s)", req.Sprite, req.MachineID)
		} else if s.registryRequired {
			desc = fmt.Sprintf("lookup sprite %q in registry (required)", req.Sprite)
		}
		steps = append(steps, PlanStep{
			Kind:        StepRegistryLookup,
			Description: desc,
		})
	}

	if req.Issue > 0 {
		steps = append(steps, PlanStep{
			Kind:        StepValidateIssue,
			Description: fmt.Sprintf("validate GitHub issue #%d is ready for dispatch", req.Issue),
		})
	}
	if provisionNeeded {
		steps = append(steps, PlanStep{
			Kind:        StepProvision,
			Description: fmt.Sprintf("create Fly machine for sprite %q", req.Sprite),
		})
	}
	if !req.AllowAnthropicDirect {
		steps = append(steps, PlanStep{
			Kind:        StepValidateEnv,
			Description: "verify ANTHROPIC_API_KEY is not set to a direct key",
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

	// Add proxy ensure step if we have OPENROUTER_API_KEY
	hasOpenRouterKey := false
	for key := range s.envVars {
		if key == "OPENROUTER_API_KEY" {
			hasOpenRouterKey = true
			break
		}
	}
	if hasOpenRouterKey {
		steps = append(steps, PlanStep{
			Kind:        StepEnsureProxy,
			Description: "ensure anthropic proxy is running on sprite",
		})
	}

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
	Sprite               string
	Repo                 repoTarget
	Prompt               string
	PromptPath           string
	StartCommand         string
	StartedAt            time.Time
	Mode                 string
	TaskLabel            string
	ProvisionMetadata    map[string]string
	AllowAnthropicDirect bool
	MachineID            string
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

	// Resolve sprite from registry if registry is configured
	var machineID string
	if s.registryPath != "" || s.registryRequired {
		resolvedID, err := ResolveSprite(sprite, s.registryPath)
		if err != nil {
			if s.registryRequired {
				return preparedRequest{}, fmt.Errorf("dispatch: %w", err)
			}
			// Log the error but continue if registry is optional
			s.logger.Debug("registry lookup failed, proceeding without it", "sprite", sprite, "error", err)
		} else {
			machineID = resolvedID
			s.logger.Info("resolved sprite from registry", "sprite", sprite, "machine_id", machineID)
		}
	}

	prompt := strings.TrimSpace(req.Prompt)
	if prompt == "" && req.Issue > 0 {
		prompt = IssuePrompt(req.Issue, req.Repo)
	}
	if prompt == "" {
		return preparedRequest{}, fmt.Errorf("%w: prompt or --issue is required", ErrInvalidRequest)
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
		maxTokens := req.MaxTokens
		if maxTokens <= 0 {
			maxTokens = DefaultMaxTokens
		}
		maxTime := req.MaxTime
		if maxTime <= 0 {
			maxTime = DefaultMaxTime
		}
		startCommand = buildStartRalphScript(
			s.workspace,
			sprite,
			s.maxRalphIterations,
			req.WebhookURL,
			maxTokens,
			int(maxTime.Round(time.Second).Seconds()),
		)
	}
	if !req.Ralph {
		if err := requireOneShotInvariants(startCommand); err != nil {
			return preparedRequest{}, err
		}
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
		Request:              req,
		Sprite:               sprite,
		Repo:                 repo,
		Prompt:               prompt,
		PromptPath:           promptPath,
		StartCommand:         startCommand,
		StartedAt:            startedAt,
		Mode:                 mode,
		TaskLabel:            taskLabel,
		ProvisionMetadata:    metadata,
		AllowAnthropicDirect: req.AllowAnthropicDirect,
		MachineID:            machineID,
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
	lines := strings.Split(output, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
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
		"mkdir -p " + shellQuote(workspace),
		"cd " + shellQuote(workspace),
		"if [ -d " + shellQuote(repoDir) + " ]; then",
		"  cd " + shellQuote(repoDir),
		// Reset to clean state: discard changes, checkout default branch, pull latest.
		// This prevents stale feature branches from polluting new dispatches.
		"  git checkout -- . 2>/dev/null || true",
		"  git clean -fd 2>/dev/null || true",
		"  DEFAULT_BRANCH=$(git symbolic-ref refs/remotes/origin/HEAD 2>/dev/null | sed 's|refs/remotes/origin/||' || echo master)",
		"  git checkout \"$DEFAULT_BRANCH\" 2>/dev/null || git checkout master 2>/dev/null || git checkout main 2>/dev/null || true",
		"  git fetch origin >/dev/null 2>&1 || true",
		"  git reset --hard \"origin/$DEFAULT_BRANCH\" 2>/dev/null || true",
		"else",
		"  gh repo clone " + shellQuote(cloneURL) + " " + shellQuote(repoDir) + " >/dev/null 2>&1 || git clone " + shellQuote(cloneURL) + " " + shellQuote(repoDir) + " >/dev/null 2>&1",
		"fi",
	}, "\n")
}

func buildOneShotScript(workspace, promptPath string) string {
	port := strconv.Itoa(proxy.ProxyPort)
	env := proxy.StartEnv("", port, "${OPENROUTER_API_KEY}")
	env["PROXY_PID_FILE"] = proxy.ProxyPIDFile

	// Build env vars for proxy startup
	envKeys := make([]string, 0, len(env))
	for k := range env {
		envKeys = append(envKeys, k)
	}
	sort.Strings(envKeys)
	envStr := ""
	for _, k := range envKeys {
		// OPENROUTER_API_KEY uses bash variable expansion, don't quote it
		if k == "OPENROUTER_API_KEY" && env[k] == "${OPENROUTER_API_KEY}" {
			envStr += fmt.Sprintf("%s=%s ", k, env[k])
		} else {
			envStr += fmt.Sprintf("%s=%s ", k, shellQuote(env[k]))
		}
	}

	// Generate health check URL
	healthURL := proxy.HealthURL(port)
	// Local address for Claude Code
	baseURL := fmt.Sprintf("http://127.0.0.1:%s", port)

	return strings.Join([]string{
		"set -euo pipefail",
		"mkdir -p " + shellQuote(workspace),
		"cd " + shellQuote(workspace),
		"# Start anthropic proxy if available",
		"if [ -f " + shellQuote(proxy.ProxyScriptPath) + " ] && [ -n \"${OPENROUTER_API_KEY:-}\" ] && command -v node >/dev/null 2>&1; then",
		"  PROXY_PID=\"\"",
		"  if [ -f " + shellQuote(proxy.ProxyPIDFile) + " ]; then",
		"    PID_FROM_FILE=\"$(cat " + shellQuote(proxy.ProxyPIDFile) + ")\"",
		"    if kill -0 \"$PID_FROM_FILE\" 2>/dev/null; then",
		"      PROXY_PID=\"$PID_FROM_FILE\"",
		"    fi",
		"  fi",
		"  if [ -z \"$PROXY_PID\" ]; then",
		"    echo '[proxy] starting anthropic-proxy...'",
		"    nohup env " + envStr + " node " + shellQuote(proxy.ProxyScriptPath) + " >/dev/null 2>&1 &",
		"    sleep 1",
		"    for i in 1 2 3 4 5; do",
		"      if curl -s --max-time 2 " + shellQuote(healthURL) + " >/dev/null 2>&1; then break; fi",
		"      sleep 0.5",
		"    done",
		"    if curl -s --max-time 2 " + shellQuote(healthURL) + " >/dev/null 2>&1; then",
		"      echo '[proxy] proxy is healthy on :" + port + "'",
		"      export ANTHROPIC_BASE_URL=" + shellQuote(baseURL),
		"      export ANTHROPIC_API_KEY=proxy-mode",
		"    else",
		"      echo '[proxy] warning: proxy failed to start, proceeding with direct connection'",
		"    fi",
		"  else",
		"    echo '[proxy] proxy already running on :" + port + "'",
		"    export ANTHROPIC_BASE_URL=" + shellQuote(baseURL),
		"    export ANTHROPIC_API_KEY=proxy-mode",
		"  fi",
		"fi",
		"if command -v script >/dev/null 2>&1; then",
		"  script -qefc " + shellQuote("cat "+shellQuote(promptPath)+" | claude -p --dangerously-skip-permissions --permission-mode bypassPermissions --verbose --output-format stream-json") + " /dev/null",
		"else",
		"  cat " + shellQuote(promptPath) + " | claude -p --dangerously-skip-permissions --permission-mode bypassPermissions --verbose --output-format stream-json",
		"fi",
		"rm -f " + shellQuote(promptPath),
	}, "\n")
}

func buildStartRalphScript(workspace, sprite string, maxIterations int, webhookURL string, maxTokens int, maxTimeSec int) string {
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
		"REQUIRED_CLAUDE_FLAGS=" + shellQuote("--dangerously-skip-permissions --permission-mode bypassPermissions --verbose --output-format stream-json"),
		"case \" $REQUIRED_CLAUDE_FLAGS \" in *\" --dangerously-skip-permissions \"*) ;; *) echo \"missing --dangerously-skip-permissions\" >&2; exit 1 ;; esac",
		"case \" $REQUIRED_CLAUDE_FLAGS \" in *\" --verbose \"*) ;; *) echo \"missing --verbose\" >&2; exit 1 ;; esac",
		"case \" $REQUIRED_CLAUDE_FLAGS \" in *\" --output-format stream-json \"*) ;; *) echo \"missing --output-format stream-json\" >&2; exit 1 ;; esac",
		// If sprite-agent is a script, validate it contains the required flags.
		// If it's a compiled binary, we can't introspect; rely on runtime checks in the agent itself.
		"if [ \"$(head -c 2 \"$AGENT_BIN\" 2>/dev/null || true)\" = \"#!\" ]; then",
		"  if ! grep -q -- '--dangerously-skip-permissions' \"$AGENT_BIN\" 2>/dev/null; then echo \"sprite-agent missing --dangerously-skip-permissions\" >&2; exit 1; fi",
		"  if ! grep -q -- '--output-format stream-json' \"$AGENT_BIN\" 2>/dev/null; then echo \"sprite-agent missing --output-format stream-json\" >&2; exit 1; fi",
		"fi",
		"cd \"$WORKSPACE_DIR\"",
		"printf 'bb-%s-%s\\n' \"$(date -u +%Y%m%d-%H%M%S)\" " + shellQuote(sprite) + " > \"$WORKSPACE_DIR/.current-task-id\"",
	}

	limits := ""
	if maxTokens > 0 {
		limits += " MAX_TOKENS=" + strconv.Itoa(maxTokens)
	}
	if maxTimeSec > 0 {
		limits += " MAX_TIME_SEC=" + strconv.Itoa(maxTimeSec)
	}
	if strings.TrimSpace(webhookURL) != "" {
		lines = append(lines,
			"nohup env SPRITE_NAME="+shellQuote(sprite)+" SPRITE_WEBHOOK_URL="+shellQuote(webhookURL)+" MAX_ITERATIONS="+strconv.Itoa(maxIterations)+limits+" BB_CLAUDE_FLAGS=\"$REQUIRED_CLAUDE_FLAGS\" \"$AGENT_BIN\" >/dev/null 2>&1 &",
		)
	} else {
		lines = append(lines,
			"nohup env SPRITE_NAME="+shellQuote(sprite)+" MAX_ITERATIONS="+strconv.Itoa(maxIterations)+limits+" BB_CLAUDE_FLAGS=\"$REQUIRED_CLAUDE_FLAGS\" \"$AGENT_BIN\" >/dev/null 2>&1 &",
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

func copyStringMap(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", `'"'"'`) + "'"
}
