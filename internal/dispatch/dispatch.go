package dispatch

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	storeevents "github.com/misty-step/bitterblossom/internal/events"
	"github.com/misty-step/bitterblossom/internal/fleet"
	"github.com/misty-step/bitterblossom/internal/proxy"
	"github.com/misty-step/bitterblossom/internal/registry"
	"github.com/misty-step/bitterblossom/internal/shellutil"
	pkgevents "github.com/misty-step/bitterblossom/pkg/events"
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

	// DefaultMaxSkillMounts is the default maximum number of --skill mounts per dispatch.
	DefaultMaxSkillMounts = 10
	// DefaultMaxFilesPerSkill is the default maximum number of files per skill.
	DefaultMaxFilesPerSkill = 100
	// DefaultMaxBytesPerSkill is the default maximum total bytes per skill (10MB).
	DefaultMaxBytesPerSkill = 10 * 1024 * 1024
	// DefaultMaxFileSize is the default maximum size for a single skill file (1MB).
	DefaultMaxFileSize = 1024 * 1024
)

var (
	spriteNamePattern = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)
	repoPartPattern   = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)
	// skillNamePattern validates skill directory names: lowercase alphanumeric with hyphens.
	// This is stricter than repoPartPattern and decouples skill naming from repo naming.
	skillNamePattern = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)
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

// EventLogger persists structured lifecycle events.
type EventLogger interface {
	Log(event pkgevents.Event) error
}

type noopEventLogger struct{}

func (noopEventLogger) Log(pkgevents.Event) error { return nil }

// Request describes a dispatch operation.
type Request struct {
	Sprite               string
	Prompt               string
	Repo                 string
	Skills               []string
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
	StepUploadSkills   StepKind = "upload_skills"
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
	// EventLogger receives dispatch lifecycle events. Nil uses default local logger.
	EventLogger EventLogger
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
	eventLogger        EventLogger
}

// NewService constructs a dispatch service.
func NewService(cfg Config) (*Service, error) {
	if cfg.Remote == nil {
		return nil, errors.New("dispatch: remote client is required")
	}
	// Fly client and app are optional — only required when provisioning new sprites.
	// Validated at provisioning time in provision().
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
		eventLogger:        cfg.EventLogger,
	}

	if svc.eventLogger == nil {
		defaultLogger, eventErr := storeevents.NewLogger(storeevents.LoggerConfig{})
		if eventErr != nil {
			logger.Warn("dispatch events disabled", "error", eventErr)
			svc.eventLogger = noopEventLogger{}
		} else {
			svc.eventLogger = defaultLogger
		}
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
	logEvent := func(event pkgevents.Event) {
		if event == nil || s.eventLogger == nil {
			return
		}
		if err := s.eventLogger.Log(event); err != nil {
			s.logger.Warn("dispatch event log failed", "sprite", prepared.Sprite, "event", event.Kind(), "error", err)
		}
	}
	logError := func(code string, inErr error) {
		if inErr == nil {
			return
		}
		logEvent(&pkgevents.ErrorEvent{
			Meta: pkgevents.Meta{
				TS:         s.now().UTC(),
				SpriteName: prepared.Sprite,
				EventKind:  pkgevents.KindError,
				Issue:      prepared.Issue,
			},
			Code:    code,
			Message: inErr.Error(),
		})
	}
	fail := func(code string, inErr error) (Result, error) {
		result.State = StateFailed
		logError(code, inErr)
		return result, inErr
	}
	transition := func(event DispatchEvent) error {
		from := state
		next, err := advanceState(state, event)
		if err != nil {
			return fmt.Errorf("%w: %v", ErrInvalidStateTransition, err)
		}
		s.logger.Info("dispatch transition", "sprite", prepared.Sprite, "from", state, "event", event, "to", next)
		state = next
		result.State = next
		logEvent(&pkgevents.ProgressEvent{
			Meta: pkgevents.Meta{
				TS:         s.now().UTC(),
				SpriteName: prepared.Sprite,
				EventKind:  pkgevents.KindProgress,
				Issue:      prepared.Issue,
			},
			Activity: "dispatch_transition",
			Detail:   fmt.Sprintf("from=%s event=%s to=%s", from, event, next),
		})
		return nil
	}
	logEvent(&pkgevents.DispatchEvent{
		Meta: pkgevents.Meta{
			TS:         prepared.StartedAt,
			SpriteName: prepared.Sprite,
			EventKind:  pkgevents.KindDispatch,
			Issue:      prepared.Issue,
		},
		Task: prepared.TaskLabel,
		Repo: prepared.Repo.Slug,
	})

	if provisionNeeded {
		if err := transition(EventProvisionRequired); err != nil {
			return fail("state_transition", err)
		}
		machineID, err := s.provision(ctx, prepared)
		if err != nil {
			return fail("provision", fmt.Errorf("dispatch: provision sprite %q: %w", prepared.Sprite, err))
		}
		if strings.TrimSpace(machineID) != "" {
			prepared.MachineID = machineID
		}
		result.Provisioned = true
		if err := transition(EventProvisionSucceeded); err != nil {
			return fail("state_transition", err)
		}
	} else if err := transition(EventMachineReady); err != nil {
		return fail("state_transition", err)
	}

	if !prepared.AllowAnthropicDirect {
		s.logger.Info("dispatch validate env", "sprite", prepared.Sprite)
		keyOutput, err := s.remote.Exec(ctx, prepared.Sprite, "printenv ANTHROPIC_API_KEY 2>/dev/null || true", nil)
		if err != nil {
			return fail("validate_env", fmt.Errorf("dispatch: check sprite env: %w", err))
		}
		env := map[string]string{}
		if key := strings.TrimSpace(keyOutput); key != "" {
			env["ANTHROPIC_API_KEY"] = key
		}
		if err := ValidateNoDirectAnthropic(env, false); err != nil {
			return fail("validate_env", err)
		}
	}

	if prepared.Repo.CloneURL != "" {
		s.logger.Info("dispatch setup repo", "sprite", prepared.Sprite, "repo", prepared.Repo.CloneURL)
		if _, err := s.remote.Exec(ctx, prepared.Sprite, buildSetupRepoScript(s.workspace, prepared.Repo.CloneURL, prepared.Repo.RepoDir), nil); err != nil {
			return fail("setup_repo", fmt.Errorf("dispatch: setup repo: %w", err))
		}
	}

	if len(prepared.Skills) > 0 {
		s.logger.Info("dispatch upload skills", "sprite", prepared.Sprite, "count", len(prepared.Skills))
		if err := s.uploadSkills(ctx, prepared.Sprite, prepared.Skills); err != nil {
			return fail("upload_skills", fmt.Errorf("dispatch: upload skills: %w", err))
		}
	}

	s.logger.Info("dispatch upload prompt", "sprite", prepared.Sprite, "path", prepared.PromptPath)
	if err := s.remote.Upload(ctx, prepared.Sprite, prepared.PromptPath, []byte(prepared.Prompt)); err != nil {
		return fail("upload_prompt", fmt.Errorf("dispatch: upload prompt: %w", err))
	}
	if err := transition(EventPromptUploaded); err != nil {
		return fail("state_transition", err)
	}

	statusBytes, err := json.Marshal(statusFile{
		Repo:    prepared.Repo.Slug,
		Started: prepared.StartedAt.Format(time.RFC3339),
		Mode:    prepared.Mode,
		Task:    prepared.TaskLabel,
	})
	if err != nil {
		return fail("marshal_status", fmt.Errorf("dispatch: marshal status: %w", err))
	}
	if err := s.remote.Upload(ctx, prepared.Sprite, s.workspace+"/STATUS.json", append(statusBytes, '\n')); err != nil {
		return fail("upload_status", fmt.Errorf("dispatch: upload status: %w", err))
	}

	// Ensure proxy is running if OPENROUTER_API_KEY is configured
	execEnvVars := copyStringMap(s.envVars)
	if openRouterKey, ok := s.envVars["OPENROUTER_API_KEY"]; ok && openRouterKey != "" {
		s.logger.Info("dispatch ensure proxy", "sprite", prepared.Sprite)
		proxyURL, err := s.proxyLifecycle.EnsureProxy(ctx, prepared.Sprite, openRouterKey)
		if err != nil {
			return fail("ensure_proxy", fmt.Errorf("dispatch: ensure proxy: %w", err))
		}
		s.logger.Info("dispatch proxy ready", "sprite", prepared.Sprite, "url", proxyURL)
		// Set proxy environment variables for the agent
		execEnvVars["ANTHROPIC_BASE_URL"] = proxyURL
		execEnvVars["ANTHROPIC_API_KEY"] = "proxy-mode"
	}

	// Pre-dispatch secret scan: ensure no credentials leaked into command args.
	if err := ValidateCommandNoSecrets(prepared.StartCommand, "start command"); err != nil {
		return fail("validate_command", err)
	}

	s.logger.Info("dispatch start agent", "sprite", prepared.Sprite, "mode", prepared.Mode)
	output, err := s.remote.ExecWithEnv(ctx, prepared.Sprite, prepared.StartCommand, nil, execEnvVars)
	if err != nil {
		return fail("start_agent", fmt.Errorf("dispatch: start agent: %w", err))
	}
	result.CommandOutput = strings.TrimSpace(output)
	if pid, ok := parsePID(output); ok {
		result.AgentPID = pid
	}

	if err := transition(EventAgentStarted); err != nil {
		return fail("state_transition", err)
	}
	if !prepared.Ralph {
		if err := transition(EventOneShotComplete); err != nil {
			return fail("state_transition", err)
		}
		logEvent(&pkgevents.DoneEvent{
			Meta: pkgevents.Meta{
				TS:         s.now().UTC(),
				SpriteName: prepared.Sprite,
				EventKind:  pkgevents.KindDone,
				Issue:      prepared.Issue,
			},
		})
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
	if s.fly == nil || strings.TrimSpace(s.app) == "" {
		return "", errors.New("dispatch: provisioning requires Fly app name and API token (set --app/--token or FLY_APP/FLY_API_TOKEN)")
	}
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
		if err := s.registerSprite(ctx, req.Sprite, machine.ID); err != nil {
			// Log the error but don't fail the provision - the sprite exists
			s.logger.Warn("failed to register sprite in registry", "sprite", req.Sprite, "machine_id", machine.ID, "error", err)
		} else {
			s.logger.Info("registered sprite in registry", "sprite", req.Sprite, "machine_id", machine.ID)
		}
	}

	return machine.ID, nil
}

// registerSprite adds a sprite to the registry.
func (s *Service) registerSprite(ctx context.Context, name, machineID string) error {
	return registry.WithLockedRegistry(ctx, s.registryPath, func(reg *registry.Registry) error {
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
	steps := make([]PlanStep, 0, 8)

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
	if len(req.Skills) > 0 {
		steps = append(steps, PlanStep{
			Kind:        StepUploadSkills,
			Description: fmt.Sprintf("upload %d skill package(s) into %s/skills", len(req.Skills), s.workspace),
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

	if _, hasOpenRouterKey := s.envVars["OPENROUTER_API_KEY"]; hasOpenRouterKey {
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
	Skills               []preparedSkill
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

type preparedSkill struct {
	Name       string
	LocalRoot  string
	RemoteRoot string
	PromptPath string
	Files      []skillFile
}

type skillFile struct {
	LocalPath  string
	RemotePath string
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
	taskPrompt := prompt

	repo, err := parseRepo(req.Repo)
	if err != nil {
		return preparedRequest{}, err
	}
	skills, err := resolveSkillMounts(req.Skills, s.workspace)
	if err != nil {
		return preparedRequest{}, err
	}
	prompt = appendSkillInstructions(prompt, skills)

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

	taskLabel := taskPrompt
	if repo.Slug != "" {
		taskLabel = repo.Slug + ": " + taskPrompt
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
		Skills:               skills,
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

func (s *Service) uploadSkills(ctx context.Context, sprite string, skills []preparedSkill) error {
	for _, skill := range skills {
		for _, file := range skill.Files {
			info, err := os.Lstat(file.LocalPath)
			if err != nil {
				return fmt.Errorf("stat %q: %w", file.LocalPath, err)
			}
			if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
				return fmt.Errorf("read %q: skill file must be a regular non-symlink file", file.LocalPath)
			}

			content, err := os.ReadFile(file.LocalPath)
			if err != nil {
				return fmt.Errorf("read %q: %w", file.LocalPath, err)
			}
			if err := s.remote.Upload(ctx, sprite, file.RemotePath, content); err != nil {
				return fmt.Errorf("upload %q to %q: %w", file.LocalPath, file.RemotePath, err)
			}
		}
	}
	return nil
}

func appendSkillInstructions(prompt string, skills []preparedSkill) string {
	if len(skills) == 0 {
		return prompt
	}

	lines := []string{
		strings.TrimSpace(prompt),
		"",
		"Required skills:",
	}
	for _, skill := range skills {
		lines = append(lines, fmt.Sprintf("- Follow the skill at %s", skill.PromptPath))
	}
	lines = append(lines, "Treat these skills as required workflow constraints for this task.")
	return strings.Join(lines, "\n")
}

type resolveSkillLimits struct {
	MaxMounts        int
	MaxFilesPerSkill int
	MaxBytesPerSkill int64
	MaxFileSize      int64
}

var defaultResolveSkillLimits = resolveSkillLimits{
	MaxMounts:        DefaultMaxSkillMounts,
	MaxFilesPerSkill: DefaultMaxFilesPerSkill,
	MaxBytesPerSkill: DefaultMaxBytesPerSkill,
	MaxFileSize:      DefaultMaxFileSize,
}

func resolveSkillMounts(paths []string, workspace string) ([]preparedSkill, error) {
	return resolveSkillMountsWithLimits(paths, workspace, defaultResolveSkillLimits)
}

func resolveSkillMountsWithLimits(paths []string, workspace string, limits resolveSkillLimits) ([]preparedSkill, error) {
	if len(paths) == 0 {
		return nil, nil
	}

	if len(paths) > limits.MaxMounts {
		return nil, fmt.Errorf("%w: too many --skill mounts: %d (max %d)", ErrInvalidRequest, len(paths), limits.MaxMounts)
	}

	mounts := make([]preparedSkill, 0, len(paths))
	seen := map[string]string{}       // skill name -> canonical path
	seenCanonical := map[string]bool{} // canonical path -> exists (for duplicate detection)

	for _, raw := range paths {
		input := strings.TrimSpace(raw)
		if input == "" {
			continue
		}

		absPath, err := filepath.Abs(input)
		if err != nil {
			return nil, fmt.Errorf("%w: resolve skill path %q: %v", ErrInvalidRequest, input, err)
		}

		// Canonicalize path for duplicate detection (evaluates symlinks)
		canonicalPath, err := filepath.EvalSymlinks(absPath)
		if err != nil {
			// If EvalSymlinks fails, fall back to absolute path but still check for duplicates
			canonicalPath = absPath
		}
		canonicalPath = filepath.Clean(canonicalPath)

		info, err := os.Stat(absPath)
		if err != nil {
			return nil, fmt.Errorf("%w: skill path %q: %v", ErrInvalidRequest, input, err)
		}

		skillRoot := absPath
		if !info.IsDir() {
			if filepath.Base(absPath) != "SKILL.md" {
				return nil, fmt.Errorf("%w: skill path %q must be a directory or SKILL.md file", ErrInvalidRequest, input)
			}
			skillRoot = filepath.Dir(absPath)
			// Re-canonicalize since skillRoot changed
			canonicalPath, err = filepath.EvalSymlinks(skillRoot)
			if err != nil {
				canonicalPath = skillRoot
			}
			canonicalPath = filepath.Clean(canonicalPath)
		}

		skillName := filepath.Base(skillRoot)
		if !skillNamePattern.MatchString(skillName) {
			return nil, fmt.Errorf("%w: invalid skill directory name %q (must match %s)", ErrInvalidRequest, skillName, skillNamePattern.String())
		}

		// Check for duplicate skill names
		if previous, exists := seen[skillName]; exists {
			if previous == canonicalPath {
				continue
			}
			return nil, fmt.Errorf("%w: duplicate skill name %q from %q and %q", ErrInvalidRequest, skillName, previous, canonicalPath)
		}

		// Check for canonical path duplicates (same skill mounted via different paths)
		if seenCanonical[canonicalPath] {
			return nil, fmt.Errorf("%w: skill at %q already mounted (detected via canonical path)", ErrInvalidRequest, input)
		}

		skillDoc := filepath.Join(skillRoot, "SKILL.md")
		if _, err := os.Stat(skillDoc); err != nil {
			return nil, fmt.Errorf("%w: skill %q missing SKILL.md", ErrInvalidRequest, input)
		}

		files := make([]skillFile, 0, 16)
		var totalBytes int64
		if err := filepath.WalkDir(skillRoot, func(filePath string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() {
				return nil
			}
			if entry.Type()&fs.ModeSymlink != 0 {
				return fmt.Errorf("skill %q contains symlink %q", input, filePath)
			}

			info, err := entry.Info()
			if err != nil {
				return err
			}
			if !info.Mode().IsRegular() {
				return fmt.Errorf("skill %q contains non-regular file %q", input, filePath)
			}

			// Check individual file size limit
			if info.Size() > limits.MaxFileSize {
				return fmt.Errorf("skill %q file %q exceeds max file size: %d bytes (max %d)", input, filePath, info.Size(), limits.MaxFileSize)
			}

			// Check total bytes limit
			totalBytes += info.Size()
			if totalBytes > limits.MaxBytesPerSkill {
				return fmt.Errorf("skill %q exceeds total size limit: %d bytes (max %d)", input, totalBytes, limits.MaxBytesPerSkill)
			}

			relPath, err := filepath.Rel(skillRoot, filePath)
			if err != nil {
				return err
			}
			relPath = filepath.ToSlash(relPath)

			files = append(files, skillFile{
				LocalPath:  filePath,
				RemotePath: path.Join(workspace, "skills", skillName, relPath),
			})

			// Short-circuit file count inside walk to avoid traversing huge directories.
			if len(files) > limits.MaxFilesPerSkill {
				return fmt.Errorf("skill %q contains more than %d files", input, limits.MaxFilesPerSkill)
			}
			return nil
		}); err != nil {
			return nil, fmt.Errorf("%w: scan skill %q: %v", ErrInvalidRequest, input, err)
		}

		sort.Slice(files, func(i, j int) bool {
			return files[i].RemotePath < files[j].RemotePath
		})

		mounts = append(mounts, preparedSkill{
			Name:       skillName,
			LocalRoot:  skillRoot,
			RemoteRoot: path.Join(workspace, "skills", skillName),
			PromptPath: "./skills/" + skillName + "/SKILL.md",
			Files:      files,
		})
		seen[skillName] = canonicalPath
		seenCanonical[canonicalPath] = true
	}

	return mounts, nil
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
		"mkdir -p " + shellutil.Quote(workspace),
		"cd " + shellutil.Quote(workspace),
		"if [ -d " + shellutil.Quote(repoDir) + " ]; then",
		"  cd " + shellutil.Quote(repoDir),
		// Reset to clean state: discard changes, checkout default branch, pull latest.
		// This prevents stale feature branches from polluting new dispatches.
		"  git checkout -- . 2>/dev/null || true",
		"  git clean -fd 2>/dev/null || true",
		"  DEFAULT_BRANCH=$(git symbolic-ref refs/remotes/origin/HEAD 2>/dev/null | sed 's|refs/remotes/origin/||' || echo master)",
		"  git checkout \"$DEFAULT_BRANCH\" 2>/dev/null || git checkout master 2>/dev/null || git checkout main 2>/dev/null || true",
		"  git fetch origin >/dev/null 2>&1 || true",
		`  git reset --hard "origin/$(git rev-parse --abbrev-ref HEAD)" 2>/dev/null || true`,
		"else",
		"  gh repo clone " + shellutil.Quote(cloneURL) + " " + shellutil.Quote(repoDir) + " >/dev/null 2>&1 || git clone " + shellutil.Quote(cloneURL) + " " + shellutil.Quote(repoDir) + " >/dev/null 2>&1",
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
			envStr += fmt.Sprintf("%s=%s ", k, shellutil.Quote(env[k]))
		}
	}

	// Generate health check URL
	healthURL := proxy.HealthURL(port)
	// Local address for Claude Code
	baseURL := fmt.Sprintf("http://127.0.0.1:%s", port)

	// Log file for agent output capture (issue #278)
	logFile := shellutil.Quote(workspace + "/logs/agent-oneshot.log")

	return strings.Join([]string{
		"set -euo pipefail",
		"mkdir -p " + shellutil.Quote(workspace+"/logs"),
		"cd " + shellutil.Quote(workspace),
		"# Start anthropic proxy if available",
		"if [ -f " + shellutil.Quote(proxy.ProxyScriptPath) + " ] && [ -n \"${OPENROUTER_API_KEY:-}\" ] && command -v node >/dev/null 2>&1; then",
		"  PROXY_PID=\"\"",
		"  if [ -f " + shellutil.Quote(proxy.ProxyPIDFile) + " ]; then",
		"    PID_FROM_FILE=\"$(cat " + shellutil.Quote(proxy.ProxyPIDFile) + ")\"",
		"    if kill -0 \"$PID_FROM_FILE\" 2>/dev/null; then",
		"      PROXY_PID=\"$PID_FROM_FILE\"",
		"    fi",
		"  fi",
		"  if [ -z \"$PROXY_PID\" ]; then",
		"    echo '[proxy] starting anthropic-proxy...'",
		"    nohup env " + envStr + " node " + shellutil.Quote(proxy.ProxyScriptPath) + " >/dev/null 2>&1 &",
		"    sleep 1",
		"    for i in 1 2 3 4 5; do",
		"      if curl -s --max-time 2 " + shellutil.Quote(healthURL) + " >/dev/null 2>&1; then break; fi",
		"      sleep 0.5",
		"    done",
		"    if curl -s --max-time 2 " + shellutil.Quote(healthURL) + " >/dev/null 2>&1; then",
		"      echo '[proxy] proxy is healthy on :" + port + "'",
		"      export ANTHROPIC_BASE_URL=" + shellutil.Quote(baseURL),
		"      export ANTHROPIC_API_KEY=proxy-mode",
		"    else",
		"      echo '[proxy] warning: proxy failed to start, proceeding with direct connection'",
		"    fi",
		"  else",
		"    echo '[proxy] proxy already running on :" + port + "'",
		"    export ANTHROPIC_BASE_URL=" + shellutil.Quote(baseURL),
		"    export ANTHROPIC_API_KEY=proxy-mode",
		"  fi",
		"fi",
		"# Capture agent output to log file for diagnostics (issue #278)",
		"AGENT_LOG=" + logFile,
		"if command -v script >/dev/null 2>&1; then",
		"  script -qefc " + shellutil.Quote("cat "+shellutil.Quote(promptPath)+" | claude -p --dangerously-skip-permissions --permission-mode bypassPermissions --verbose --output-format stream-json") + " /dev/null 2>&1 | tee -a \"$AGENT_LOG\"",
		"else",
		"  cat " + shellutil.Quote(promptPath) + " | claude -p --dangerously-skip-permissions --permission-mode bypassPermissions --verbose --output-format stream-json 2>&1 | tee -a \"$AGENT_LOG\"",
		"fi",
		"rm -f " + shellutil.Quote(promptPath),
	}, "\n")
}

func buildStartRalphScript(workspace, sprite string, maxIterations int, webhookURL string, maxTokens int, maxTimeSec int) string {
	lines := []string{
		"set -euo pipefail",
		"WORKSPACE_DIR=" + shellutil.Quote(workspace),
		"mkdir -p \"$WORKSPACE_DIR/logs\"",
		"rm -f \"$WORKSPACE_DIR/TASK_COMPLETE\" \"$WORKSPACE_DIR/BLOCKED.md\"",
		"if [ -f \"$WORKSPACE_DIR/agent.pid\" ] && kill -0 \"$(cat \"$WORKSPACE_DIR/agent.pid\")\" 2>/dev/null; then kill \"$(cat \"$WORKSPACE_DIR/agent.pid\")\" 2>/dev/null || true; fi",
		"if [ -f \"$WORKSPACE_DIR/ralph.pid\" ] && kill -0 \"$(cat \"$WORKSPACE_DIR/ralph.pid\")\" 2>/dev/null; then kill \"$(cat \"$WORKSPACE_DIR/ralph.pid\")\" 2>/dev/null || true; fi",
		"AGENT_BIN=\"$HOME/.local/bin/sprite-agent\"",
		"if [ ! -x \"$AGENT_BIN\" ]; then AGENT_BIN=\"$WORKSPACE_DIR/.sprite-agent.sh\"; fi",
		"if [ ! -x \"$AGENT_BIN\" ]; then echo \"sprite-agent not found\" >&2; exit 1; fi",
		"REQUIRED_CLAUDE_FLAGS=" + shellutil.Quote("--dangerously-skip-permissions --permission-mode bypassPermissions --verbose --output-format stream-json"),
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
		"printf 'bb-%s-%s\\n' \"$(date -u +%Y%m%d-%H%M%S)\" " + shellutil.Quote(sprite) + " > \"$WORKSPACE_DIR/.current-task-id\"",
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
			"nohup env SPRITE_NAME="+shellutil.Quote(sprite)+" SPRITE_WEBHOOK_URL="+shellutil.Quote(webhookURL)+" MAX_ITERATIONS="+strconv.Itoa(maxIterations)+limits+" BB_CLAUDE_FLAGS=\"$REQUIRED_CLAUDE_FLAGS\" \"$AGENT_BIN\" >/dev/null 2>&1 &",
		)
	} else {
		lines = append(lines,
			"nohup env SPRITE_NAME="+shellutil.Quote(sprite)+" MAX_ITERATIONS="+strconv.Itoa(maxIterations)+limits+" BB_CLAUDE_FLAGS=\"$REQUIRED_CLAUDE_FLAGS\" \"$AGENT_BIN\" >/dev/null 2>&1 &",
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
