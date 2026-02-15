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
	"sync"
	"time"

	"github.com/misty-step/bitterblossom/internal/claude"
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
	// DefaultMaxConcurrentUploads is the default number of concurrent skill file uploads.
	// Conservative default to avoid overwhelming the remote host while providing throughput benefits.
	DefaultMaxConcurrentUploads = 3

	// ProbeTimeout is how long to wait for a sprite connectivity probe.
	// 15 seconds accommodates sleeping sprites (Fly.io auto-sleeps after 30s idle,
	// wake takes several seconds). Short enough to fail fast vs the old 45s-per-step cascade.
	ProbeTimeout = 15 * time.Second

	// Signal file names written by agents to indicate task completion or blocking.
	// Both extensions are checked because agents may write either variant.
	SignalTaskComplete   = "TASK_COMPLETE"
	SignalTaskCompleteMD = "TASK_COMPLETE.md"
	SignalBlocked        = "BLOCKED.md"
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

// ErrSpriteUnreachable indicates the sprite is not responding to connectivity probes.
type ErrSpriteUnreachable struct {
	Sprite string
	Cause  error
}

func (e *ErrSpriteUnreachable) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("sprite %q is not responding (%v)", e.Sprite, e.Cause)
	}
	return fmt.Sprintf("sprite %q is not responding", e.Sprite)
}

func (e *ErrSpriteUnreachable) Unwrap() error {
	return e.Cause
}

// RemoteClient runs remote commands on sprites and uploads files.
type RemoteClient interface {
	Exec(ctx context.Context, sprite, remoteCommand string, stdin []byte) (string, error)
	ExecWithEnv(ctx context.Context, sprite, remoteCommand string, stdin []byte, env map[string]string) (string, error)
	Upload(ctx context.Context, sprite, remotePath string, content []byte) error
	List(ctx context.Context) ([]string, error)
	// ProbeConnectivity checks if a sprite is reachable with a short timeout.
	// Returns nil if the sprite responds, or an error if unreachable.
	ProbeConnectivity(ctx context.Context, sprite string) error
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
	// AllowOrphan bypasses the orphan sprite check. When false (default),
	// dispatch to sprites not in the loaded composition is rejected.
	AllowOrphan bool
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
	StepRegistryLookup     StepKind = "registry_lookup"
	StepProvision          StepKind = "provision"
	StepProbeConnectivity  StepKind = "probe_connectivity" // preflight: fast check sprite reachable
	StepValidateEnv        StepKind = "validate_env"
	StepValidateWorkspace  StepKind = "validate_workspace"
	StepCleanSignals       StepKind = "clean_signals"
	StepUploadScaffold     StepKind = "upload_scaffold"
	StepValidateIssue      StepKind = "validate_issue"
	StepSetupRepo          StepKind = "setup_repo"
	StepUploadSkills       StepKind = "upload_skills"
	StepUploadPrompt       StepKind = "upload_prompt"
	StepWriteStatus        StepKind = "write_status"
	StepEnsureProxy        StepKind = "ensure_proxy"
	StepStartAgent         StepKind = "start_agent"
)

// Plan is the rendered execution plan for dry-run or execute mode.
type Plan struct {
	Sprite string     `json:"sprite"`
	Mode   string     `json:"mode"`
	Steps  []PlanStep `json:"steps"`
}

// WorkDelta captures the work produced by an agent execution.
type WorkDelta struct {
	// Commits is the number of new commits created.
	Commits int `json:"commits,omitempty"`
	// PRs is the number of pull requests created.
	PRs int `json:"prs,omitempty"`
	// HasChanges is true if any work was produced (commits or PRs).
	HasChanges bool `json:"has_changes,omitempty"`
	// DirtyFiles is the number of uncommitted changed files (staged + unstaged).
	// Non-zero when the agent modified files but didn't commit.
	DirtyFiles int `json:"dirty_files,omitempty"`
	// VerificationFailed is true when work delta calculation failed (e.g., I/O timeout).
	// This is distinct from HasChanges=false — it means we couldn't verify the outcome.
	VerificationFailed bool `json:"verification_failed,omitempty"`
	// VerificationError holds the error message when VerificationFailed is true.
	VerificationError string `json:"verification_error,omitempty"`
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
	// LogPath is the path to the agent output log on the sprite (oneshot mode only).
	LogPath string `json:"log_path,omitempty"`
	// Work captures the work delta produced by the agent (commits, PRs).
	Work WorkDelta `json:"work,omitempty"`
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
	// ScaffoldDir is the local path to the base/ directory containing CLAUDE.md,
	// settings.json, hooks/, etc. If empty, scaffolding is skipped.
	ScaffoldDir string
	// MaxConcurrentUploads controls the number of concurrent skill file uploads.
	// 0 or negative uses DefaultMaxConcurrentUploads (3). Higher values increase
	// throughput but may overwhelm the remote host.
	MaxConcurrentUploads int
}

type provisionInfo struct {
	Persona       string
	ConfigVersion string
}

// Service executes dispatch plans.
type Service struct {
	remote               RemoteClient
	fly                  fly.MachineClient
	app                  string
	workspace            string
	maxRalphIterations   int
	provisionConfig      map[string]any
	logger               *slog.Logger
	now                  func() time.Time
	ralphTemplate        string
	provisionHints       map[string]provisionInfo
	envVars              map[string]string
	registryPath         string
	registryRequired     bool
	proxyLifecycle       *proxy.Lifecycle
	eventLogger          EventLogger
	scaffoldDir          string
	maxConcurrentUploads int
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

	maxConcurrentUploads := cfg.MaxConcurrentUploads
	if maxConcurrentUploads <= 0 {
		maxConcurrentUploads = DefaultMaxConcurrentUploads
	}

	svc := &Service{
		remote:               cfg.Remote,
		fly:                  cfg.Fly,
		app:                  strings.TrimSpace(cfg.App),
		workspace:            workspace,
		maxRalphIterations:   maxIterations,
		provisionConfig:      copyMap(cfg.ProvisionConfig),
		logger:               logger,
		now:                  now,
		ralphTemplate:        template,
		provisionHints:       hints,
		envVars:              copyStringMap(cfg.EnvVars),
		registryPath:         registryPath,
		registryRequired:     cfg.RegistryRequired,
		eventLogger:          cfg.EventLogger,
		scaffoldDir:          strings.TrimSpace(cfg.ScaffoldDir),
		maxConcurrentUploads: maxConcurrentUploads,
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

	// Orphan sprite check: if a composition is loaded and the sprite exists
	// remotely but is not in the composition, it's an orphan. Orphan sprites
	// lack persistent workspace volumes — dispatches run in void. (See #347.)
	if !provisionNeeded && !prepared.AllowOrphan && len(s.provisionHints) > 0 {
		if _, inComposition := s.provisionHints[prepared.Sprite]; !inComposition {
			names := make([]string, 0, len(s.provisionHints))
			for name := range s.provisionHints {
				names = append(names, name)
			}
			sort.Strings(names)
			return Result{}, &ErrOrphanSprite{Sprite: prepared.Sprite, Composition: names}
		}
	}

	plan := s.buildPlan(prepared, provisionNeeded)
	result := Result{
		Plan:       plan,
		Executed:   prepared.Execute,
		State:      StatePending,
		PromptPath: prepared.PromptPath,
		StartedAt:  prepared.StartedAt,
		Task:       prepared.TaskLabel,
		LogPath:    prepared.LogPath,
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

	// Preflight: fast connectivity probe before entering pipeline (see #357)
	// This catches unreachable sprites quickly instead of burning 45s per step.
	if err := s.remote.ProbeConnectivity(ctx, prepared.Sprite); err != nil {
		// Don't wrap user cancellation as unreachable sprite
		if errors.Is(err, context.Canceled) {
			return fail("probe_connectivity", fmt.Errorf("dispatch: cancelled during connectivity probe: %w", err))
		}
		return fail("probe_connectivity", &ErrSpriteUnreachable{Sprite: prepared.Sprite, Cause: err})
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

	s.logger.Info("dispatch clean signals", "sprite", prepared.Sprite)
	if err := s.cleanSignals(ctx, prepared.Sprite); err != nil {
		return fail("clean_signals", fmt.Errorf("dispatch: clean stale signals: %w", err))
	}

	if s.scaffoldDir != "" {
		s.logger.Info("dispatch scaffold", "sprite", prepared.Sprite, "base", s.scaffoldDir)
		if err := s.scaffold(ctx, prepared.Sprite); err != nil {
			return fail("scaffold", fmt.Errorf("dispatch: scaffold environment: %w", err))
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

	// Capture HEAD SHA before agent starts for work delta tracking
	var preExecSHA string
	if prepared.Repo.RepoDir != "" {
		sha, err := s.captureHeadSHA(ctx, prepared.Sprite, prepared.Repo.RepoDir)
		if err != nil {
			s.logger.Warn("failed to capture pre-exec HEAD SHA", "sprite", prepared.Sprite, "error", err)
		} else {
			preExecSHA = sha
			s.logger.Info("captured pre-exec HEAD SHA", "sprite", prepared.Sprite, "sha", preExecSHA)
		}
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
		// Calculate work delta for oneshot mode
		if prepared.Repo.RepoDir != "" && preExecSHA != "" {
			work, err := s.calculateWorkDelta(ctx, prepared.Sprite, prepared.Repo.RepoDir, preExecSHA)
			if err != nil {
				s.logger.Error("work delta verification failed", "sprite", prepared.Sprite, "error", err)
				result.Work = WorkDelta{
					VerificationFailed: true,
					VerificationError:  err.Error(),
				}
			} else {
				result.Work = work
				s.logger.Info("calculated work delta", "sprite", prepared.Sprite, "commits", work.Commits, "prs", work.PRs, "has_changes", work.HasChanges, "dirty_files", work.DirtyFiles)
			}
		}
		logEvent(&pkgevents.DoneEvent{
			Meta: pkgevents.Meta{
				TS:         s.now().UTC(),
				SpriteName: prepared.Sprite,
				EventKind:  pkgevents.KindDone,
				Issue:      prepared.Issue,
			},
		})
		// Update STATUS.json to reflect completion (fixes #367 - stale status)
		if err := s.writeFinalStatus(ctx, prepared, result, "completed", 0); err != nil {
			s.logger.Warn("failed to write final status", "sprite", prepared.Sprite, "error", err)
		}
	}
	return result, nil
}

type statusFile struct {
	Repo      string `json:"repo,omitempty"`
	Started   string `json:"started,omitempty"`
	Completed string `json:"completed,omitempty"`
	Mode      string `json:"mode,omitempty"`
	Task      string `json:"task,omitempty"`
	Status    string `json:"status,omitempty"`
	ExitCode  int    `json:"exit_code,omitempty"`
}

// writeFinalStatus updates STATUS.json with completion information.
// This ensures watchdog reports the correct state after dispatch finishes (#367).
func (s *Service) writeFinalStatus(ctx context.Context, prepared preparedRequest, result Result, status string, exitCode int) error {
	finalStatus := statusFile{
		Repo:      prepared.Repo.Slug,
		Started:   prepared.StartedAt.Format(time.RFC3339),
		Completed: s.now().UTC().Format(time.RFC3339),
		Mode:      prepared.Mode,
		Task:      prepared.TaskLabel,
		Status:    status,
		ExitCode:  exitCode,
	}
	statusBytes, err := json.Marshal(finalStatus)
	if err != nil {
		return fmt.Errorf("marshal final status: %w", err)
	}
	if err := s.remote.Upload(ctx, prepared.Sprite, s.workspace+"/STATUS.json", append(statusBytes, '\n')); err != nil {
		return fmt.Errorf("upload final status: %w", err)
	}
	return nil
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
	// Preflight connectivity probe before entering pipeline (see #357)
	steps = append(steps, PlanStep{
		Kind:        StepProbeConnectivity,
		Description: fmt.Sprintf("probe connectivity to sprite %q (%s timeout)", req.Sprite, ProbeTimeout),
	})
	if !req.AllowAnthropicDirect {
		steps = append(steps, PlanStep{
			Kind:        StepValidateEnv,
			Description: "verify ANTHROPIC_API_KEY is not set to a direct key",
		})
	}
	if len(s.provisionHints) > 0 && !req.AllowOrphan {
		steps = append(steps, PlanStep{
			Kind:        StepValidateWorkspace,
			Description: fmt.Sprintf("verify sprite %q is in composition (orphan check)", req.Sprite),
		})
	}
	steps = append(steps, PlanStep{
		Kind:        StepCleanSignals,
		Description: fmt.Sprintf("remove stale signal files from %s", s.workspace),
	})
	if s.scaffoldDir != "" {
		steps = append(steps, PlanStep{
			Kind:        StepUploadScaffold,
			Description: fmt.Sprintf("upload base CLAUDE.md, persona, hooks, settings to %s", s.workspace),
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
	AllowOrphan          bool
	MachineID            string
	// LogPath is the path to the agent output log on the sprite (oneshot mode only).
	LogPath string
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
	logPath := s.workspace + "/logs/oneshot-" + startedAt.Format("20060102-150405") + ".log"
	startCommand := buildOneShotScript(s.workspace, promptPath, logPath)
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
		AllowOrphan:          req.AllowOrphan,
		MachineID:            machineID,
		LogPath:              logPath,
	}, nil
}

func (s *Service) cleanSignals(ctx context.Context, sprite string) error {
	// Only remove signal files, not PID files. The agent start scripts
	// (buildOneShotScript, buildStartRalphScript) need agent.pid and ralph.pid
	// to kill stale processes before launching new ones.
	// Also remove PR_URL to prevent stale URLs from causing false positive completion
	// detection (see PR #318).
	script := fmt.Sprintf(
		"rm -f %[1]s/TASK_COMPLETE %[1]s/TASK_COMPLETE.md %[1]s/BLOCKED.md %[1]s/BLOCKED %[1]s/PR_URL",
		shellutil.Quote(s.workspace),
	)
	_, err := s.remote.Exec(ctx, sprite, script, nil)
	return err
}

func (s *Service) scaffold(ctx context.Context, sprite string) error {
	if s.scaffoldDir == "" {
		return nil
	}

	// Ensure target directories exist before uploading
	mkdirScript := fmt.Sprintf("mkdir -p %s/.claude/hooks", shellutil.Quote(s.workspace))
	if _, err := s.remote.Exec(ctx, sprite, mkdirScript, nil); err != nil {
		return fmt.Errorf("scaffold mkdir: %w", err)
	}

	// Upload base/CLAUDE.md -> $WORKSPACE/CLAUDE.md
	claudeMD := filepath.Join(s.scaffoldDir, "CLAUDE.md")
	if content, err := os.ReadFile(claudeMD); err == nil {
		if err := s.remote.Upload(ctx, sprite, s.workspace+"/CLAUDE.md", content); err != nil {
			return fmt.Errorf("upload CLAUDE.md: %w", err)
		}
	}

	// Upload persona: sprites/<sprite-name>.md -> $WORKSPACE/PERSONA.md
	personaDir := filepath.Join(filepath.Dir(s.scaffoldDir), "sprites")
	personaMD := filepath.Join(personaDir, sprite+".md")
	if content, err := os.ReadFile(personaMD); err == nil {
		if err := s.remote.Upload(ctx, sprite, s.workspace+"/PERSONA.md", content); err != nil {
			return fmt.Errorf("upload PERSONA.md: %w", err)
		}
	}

	// Upload base/settings.json -> $WORKSPACE/.claude/settings.json
	settingsJSON := filepath.Join(s.scaffoldDir, "settings.json")
	if content, err := os.ReadFile(settingsJSON); err == nil {
		if err := s.remote.Upload(ctx, sprite, s.workspace+"/.claude/settings.json", content); err != nil {
			return fmt.Errorf("upload settings.json: %w", err)
		}
	}

	// Upload hooks from base/hooks/ -> $WORKSPACE/.claude/hooks/
	hooksDir := filepath.Join(s.scaffoldDir, "hooks")
	if entries, err := os.ReadDir(hooksDir); err == nil {
		for _, entry := range entries {
			if entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
				continue
			}
			content, err := os.ReadFile(filepath.Join(hooksDir, entry.Name()))
			if err != nil {
				continue
			}
			remotePath := s.workspace + "/.claude/hooks/" + entry.Name()
			if err := s.remote.Upload(ctx, sprite, remotePath, content); err != nil {
				return fmt.Errorf("upload hook %s: %w", entry.Name(), err)
			}
		}
	}

	// Upload shell export for Claude flags (single source of truth for shell scripts).
	// This file can be sourced by sprite-agent.sh and other shell scripts.
	flagsExport := []byte(claude.ShellExport())
	if err := s.remote.Upload(ctx, sprite, s.workspace+"/.claude/flags.sh", flagsExport); err != nil {
		return fmt.Errorf("upload flags.sh: %w", err)
	}

	// Create MEMORY.md and LEARNINGS.md if they don't exist (preserve across dispatches).
	// Combined into one remote call to save a network round-trip.
	ws := shellutil.Quote(s.workspace)
	initScript := fmt.Sprintf(
		"test -f %[1]s/MEMORY.md || printf '# MEMORY\\n' > %[1]s/MEMORY.md; "+
			"test -f %[1]s/LEARNINGS.md || printf '# LEARNINGS\\n' > %[1]s/LEARNINGS.md",
		ws,
	)
	if _, err := s.remote.Exec(ctx, sprite, initScript, nil); err != nil {
		return fmt.Errorf("init memory/learnings: %w", err)
	}

	return nil
}

// uploadWork represents a single file upload task.
type uploadWork struct {
	index      int
	localPath  string
	remotePath string
}

// uploadResult captures the outcome of a single upload.
type uploadResult struct {
	index int
	err   error
}

func (s *Service) uploadSkills(ctx context.Context, sprite string, skills []preparedSkill) error {
	// Collect all files to upload with their original index for deterministic ordering.
	var total int
	for _, skill := range skills {
		total += len(skill.Files)
	}
	if total == 0 {
		return nil
	}

	work := make([]uploadWork, 0, total)
	idx := 0
	for _, skill := range skills {
		for _, file := range skill.Files {
			work = append(work, uploadWork{
				index:      idx,
				localPath:  file.LocalPath,
				remotePath: file.RemotePath,
			})
			idx++
		}
	}

	// Pre-validate all files (symlink check) before starting uploads.
	// This ensures we fail fast on validation errors before any network operations.
	for _, w := range work {
		info, err := os.Lstat(w.localPath)
		if err != nil {
			return fmt.Errorf("stat %q: %w", w.localPath, err)
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return fmt.Errorf("read %q: skill file must be a regular non-symlink file", w.localPath)
		}
	}

	// Use bounded concurrency for uploads.
	// Sequential execution when maxConcurrentUploads == 1 for simplicity.
	if s.maxConcurrentUploads <= 1 {
		return s.uploadSkillsSequential(ctx, sprite, work)
	}

	return s.uploadSkillsConcurrent(ctx, sprite, work)
}

// uploadSkillsSequential performs uploads one at a time.
// Used when concurrency is disabled or for fallback.
func (s *Service) uploadSkillsSequential(ctx context.Context, sprite string, work []uploadWork) error {
	for _, w := range work {
		if err := s.doUpload(ctx, sprite, w); err != nil {
			return err
		}
	}
	return nil
}

// uploadSkillsConcurrent performs uploads with bounded concurrency.
// Uses worker pool pattern with fail-fast behavior on first error.
func (s *Service) uploadSkillsConcurrent(ctx context.Context, sprite string, work []uploadWork) error {
	numWorkers := s.maxConcurrentUploads
	if numWorkers > len(work) {
		numWorkers = len(work)
	}

	// Create cancellable context for fail-fast.
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	workCh := make(chan uploadWork, len(work))
	resultCh := make(chan uploadResult, len(work))

	// Start workers.
	var wg sync.WaitGroup
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for w := range workCh {
				err := s.doUpload(ctx, sprite, w)
				resultCh <- uploadResult{index: w.index, err: err}
				// Fail-fast: stop processing if context is cancelled.
				if err != nil {
					return
				}
			}
		}()
	}

	// Queue work.
	go func() {
		for _, w := range work {
			select {
			case workCh <- w:
			case <-ctx.Done():
				close(workCh)
				return
			}
		}
		close(workCh)
	}()

	// Close result channel when all workers exit.
	go func() {
		wg.Wait()
		close(resultCh)
	}()

	// Collect results, maintaining deterministic ordering by index.
	var firstErr error

	for result := range resultCh {
		if result.err != nil && firstErr == nil {
			firstErr = result.err
			// Cancel remaining work by cancelling context.
			// Note: We continue reading results to ensure clean worker exit.
			cancel()
		}
	}

	return firstErr
}

// doUpload performs a single file upload.
func (s *Service) doUpload(ctx context.Context, sprite string, w uploadWork) error {
	content, err := os.ReadFile(w.localPath)
	if err != nil {
		return fmt.Errorf("read %q: %w", w.localPath, err)
	}
	if err := s.remote.Upload(ctx, sprite, w.remotePath, content); err != nil {
		return fmt.Errorf("upload %q to %q: %w", w.localPath, w.remotePath, err)
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
	seen := map[string]string{}        // skill name -> canonical path
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
		// Configure git credentials BEFORE any git operations that need auth
		buildGitConfigScript(workspace),
		"START_TIME=$(date +%s)",
		"if [ -d " + shellutil.Quote(repoDir) + " ]; then",
		"  echo \"[setup] pulling latest for " + shellutil.Quote(repoDir) + "...\"",
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
		"  echo \"[setup] cloning " + shellutil.Quote(cloneURL) + " (first time, may take a few minutes)...\"",
		"  gh repo clone " + shellutil.Quote(cloneURL) + " " + shellutil.Quote(repoDir) + " >/dev/null 2>&1 || git clone " + shellutil.Quote(cloneURL) + " " + shellutil.Quote(repoDir) + " >/dev/null 2>&1",
		"fi",
		"END_TIME=$(date +%s)",
		"ELAPSED=$((END_TIME - START_TIME))",
		"echo \"[setup] repo ready (${ELAPSED}s)\"",
	}, "\n")
}

// buildGitConfigScript generates a script to configure git credentials using GITHUB_TOKEN.
// This allows agents to commit and push without interactive authentication.
// The token is stored in git's credential store so subsequent git operations use it automatically.
func buildGitConfigScript(workspace string) string {
	return strings.Join([]string{
		"# Configure git credentials for non-interactive push/commit",
		"GITHUB_TOKEN=\"${GITHUB_TOKEN:-${GH_TOKEN:-}}\"",
		"if [ -n \"$GITHUB_TOKEN\" ]; then",
		"  # Extract GitHub user from gh CLI if available, otherwise use 'sprite'",
		"  GH_USER=\"$(gh api user -q .login 2>/dev/null || echo 'sprite')\"",
		"  # Ensure credential store directory exists",
		"  mkdir -p \"$HOME/.config/git\"",
		"  # Store credentials for github.com",
		"  printf 'protocol=https\\nhost=github.com\\nusername=%s\\npassword=%s\\n\\n' \"$GH_USER\" \"$GITHUB_TOKEN\" | git credential-store --file=\"$HOME/.git-credentials\" store",
		"  # Configure git to use the credential store",
		"  git config --global credential.helper 'store --file=\"$HOME/.git-credentials\"'",
		"  git config --global user.email \"${GH_USER}@sprites.dev\"",
		"  git config --global user.name \"${GH_USER}\"",
		"  echo \"[setup] git credentials configured for $GH_USER\"",
		"else",
		"  echo \"[setup] warning: GITHUB_TOKEN not set, git push may fail\"",
		"fi",
	}, "\n")
}

// buildSetupRepoScriptWithGitConfig is a variant that explicitly includes git credential setup.
// This is used for testing and ensures the git config is always included.
func buildSetupRepoScriptWithGitConfig(workspace, cloneURL, repoDir string) string {
	return buildSetupRepoScript(workspace, cloneURL, repoDir)
}

// buildOneShotScript generates a shell script for one-shot (non-Ralph) dispatch.
//
// Invariants:
//   - Stale signal files (TASK_COMPLETE, TASK_COMPLETE.md, BLOCKED.md) MUST be
//     removed before agent start. Without this, the --wait polling loop may detect
//     markers from a previous dispatch and report false success. (See PR #280.)
//   - The proxy startup is best-effort: if it fails, the agent runs with direct
//     connection. This avoids blocking dispatch on proxy infrastructure.
//   - PTY wrapping (script -qefc) is required because Claude Code expects a TTY.
//     Falls back to raw pipe on systems without script(1).
//   - --output-format stream-json enables structured output parsing by the
//     watchdog and polling systems.
//   - Output is captured to logPath for diagnostics. This addresses the "zero effect"
//     issue where agents exit cleanly but produce no observable changes.
func buildOneShotScript(workspace, promptPath, logPath string) string {
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

	// Log file for agent output capture (issue #278).
	// Truncated each dispatch so only the latest run's output is kept.
	logFile := shellutil.Quote(workspace + "/logs/agent-oneshot.log")

	return strings.Join([]string{
		"set -euo pipefail",
		"mkdir -p " + shellutil.Quote(workspace),
		"mkdir -p " + shellutil.Quote(filepath.Dir(logPath)),
		"cd " + shellutil.Quote(workspace),
		"rm -f " + SignalTaskComplete + " " + SignalTaskCompleteMD + " " + SignalBlocked,
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
		"# Capture output for diagnostics (addresses issue #278, #294 - zero effect debugging)",
		"AGENT_LOG=" + logFile,
		"echo '[oneshot] starting at '$(date -Iseconds) > " + shellutil.Quote(logPath),
		"echo '[oneshot] prompt: " + shellutil.Quote(promptPath) + "' >> " + shellutil.Quote(logPath),
		"if command -v script >/dev/null 2>&1; then",
		"  script -qefc " + shellutil.Quote("cat "+shellutil.Quote(promptPath)+" | claude "+claude.FlagSetWithPrefix()) + " /dev/null 2>&1 | tee \"$AGENT_LOG\"",
		"else",
		"  cat " + shellutil.Quote(promptPath) + " | claude "+claude.FlagSetWithPrefix()+" 2>&1 | tee \"$AGENT_LOG\"",
		"fi",
		"EXIT_CODE=$?",
		"echo '[oneshot] exited with code ' $EXIT_CODE ' at ' $(date -Iseconds) >> " + shellutil.Quote(logPath),
		"rm -f " + shellutil.Quote(promptPath),
		"exit $EXIT_CODE",
	}, "\n")
}

// buildStartRalphScript generates a shell script to start the Ralph (multi-iteration) agent loop.
//
// Invariants:
//   - Same stale-signal cleanup as buildOneShotScript (see PR #280).
//   - Kills any previously running agent/ralph processes to prevent zombie accumulation.
//   - Claude flags are validated both in this script (via case statements) and in
//     the agent binary (if it's a shell script, via grep). Belt-and-suspenders because
//     missing --dangerously-skip-permissions causes a blocking permissions prompt on
//     a headless sprite, and missing --output-format stream-json breaks structured parsing.
func buildStartRalphScript(workspace, sprite string, maxIterations int, webhookURL string, maxTokens int, maxTimeSec int) string {
	lines := []string{
		"set -euo pipefail",
		"WORKSPACE_DIR=" + shellutil.Quote(workspace),
		"mkdir -p \"$WORKSPACE_DIR/logs\"",
		"rm -f \"$WORKSPACE_DIR/" + SignalTaskComplete + "\" \"$WORKSPACE_DIR/" + SignalTaskCompleteMD + "\" \"$WORKSPACE_DIR/" + SignalBlocked + "\"",
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

	envParts := "SPRITE_NAME=" + shellutil.Quote(sprite)
	if strings.TrimSpace(webhookURL) != "" {
		envParts += " SPRITE_WEBHOOK_URL=" + shellutil.Quote(webhookURL)
	}
	envParts += " MAX_ITERATIONS=" + strconv.Itoa(maxIterations)
	if maxTokens > 0 {
		envParts += " MAX_TOKENS=" + strconv.Itoa(maxTokens)
	}
	if maxTimeSec > 0 {
		envParts += " MAX_TIME_SEC=" + strconv.Itoa(maxTimeSec)
	}
	lines = append(lines,
		"nohup env "+envParts+" BB_CLAUDE_FLAGS=\"$REQUIRED_CLAUDE_FLAGS\" \"$AGENT_BIN\" >/dev/null 2>&1 &",
	)
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

// captureHeadSHA captures the current HEAD SHA of the repo on the sprite.
func (s *Service) captureHeadSHA(ctx context.Context, sprite, repoDir string) (string, error) {
	script := fmt.Sprintf(
		"cd %s && git rev-parse HEAD 2>/dev/null || echo ''",
		shellutil.Quote(s.workspace+"/"+repoDir),
	)
	output, err := s.remote.Exec(ctx, sprite, script, nil)
	if err != nil {
		return "", fmt.Errorf("capture HEAD SHA: %w", err)
	}
	sha := strings.TrimSpace(output)
	// Validate SHA format (40 hex characters for git SHA)
	if sha == "" || len(sha) != 40 {
		return "", fmt.Errorf("invalid HEAD SHA format: %q", sha)
	}
	return sha, nil
}

// calculateWorkDelta calculates the work produced by comparing pre and post SHAs.
func (s *Service) calculateWorkDelta(ctx context.Context, sprite, repoDir, preExecSHA string) (WorkDelta, error) {
	repoPath := s.workspace + "/" + repoDir
	quotedPath := shellutil.Quote(repoPath)

	// Capture post-exec SHA
	postScript := fmt.Sprintf("cd %s && git rev-parse HEAD 2>/dev/null || echo ''", quotedPath)
	postOutput, err := s.remote.Exec(ctx, sprite, postScript, nil)
	if err != nil {
		return WorkDelta{}, fmt.Errorf("capture post-exec HEAD SHA: %w", err)
	}
	postExecSHA := strings.TrimSpace(postOutput)
	if postExecSHA == "" {
		return WorkDelta{}, errors.New("empty post-exec HEAD SHA")
	}

	// If SHA hasn't changed, no new commits — but check for uncommitted changes.
	if preExecSHA == postExecSHA {
		dirtyScript := fmt.Sprintf("cd %s && git status --porcelain 2>/dev/null | wc -l", quotedPath)
		dirtyOutput, dirtyErr := s.remote.Exec(ctx, sprite, dirtyScript, nil)
		dirtyFiles := 0
		if dirtyErr == nil {
			dirtyFiles, _ = strconv.Atoi(strings.TrimSpace(dirtyOutput))
		}
		return WorkDelta{Commits: 0, PRs: 0, HasChanges: false, DirtyFiles: dirtyFiles}, nil
	}

	// Count commits between pre and post SHAs
	countScript := fmt.Sprintf(
		"cd %s && git rev-list --count %s..%s 2>/dev/null || echo '0'",
		quotedPath, preExecSHA, postExecSHA,
	)
	countOutput, err := s.remote.Exec(ctx, sprite, countScript, nil)
	if err != nil {
		return WorkDelta{}, fmt.Errorf("count commits: %w", err)
	}
	commitCount, _ := strconv.Atoi(strings.TrimSpace(countOutput))

	// Check for PR_URL file to detect PR creation
	prScript := fmt.Sprintf(
		"cat %s/PR_URL 2>/dev/null | tr -d '[:space:]' || echo ''",
		shellutil.Quote(s.workspace),
	)
	prOutput, err := s.remote.Exec(ctx, sprite, prScript, nil)
	prURL := ""
	if err == nil {
		prURL = strings.TrimSpace(prOutput)
	}

	return WorkDelta{
		Commits:    commitCount,
		PRs:        boolToInt(prURL != ""),
		HasChanges: commitCount > 0 || prURL != "",
	}, nil
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
