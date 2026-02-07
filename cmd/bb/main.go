package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/misty-step/bitterblossom/internal/agent"
	"github.com/misty-step/bitterblossom/internal/clients"
	"github.com/misty-step/bitterblossom/internal/fleet"
	"github.com/misty-step/bitterblossom/internal/health"
	"github.com/misty-step/bitterblossom/internal/logs"
	"github.com/misty-step/bitterblossom/internal/prs"
	"github.com/misty-step/bitterblossom/internal/watchdog"
)

func main() {
	if err := run(context.Background(), os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "ERROR:", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string) error {
	if len(args) == 0 {
		usage()
		return nil
	}

	runner := clients.ExecRunner{}
	logger := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))

	spriteBin := envOr("SPRITE_CLI", filepath.Join(os.Getenv("HOME"), ".local/bin/sprite"))
	spriteClient := clients.NewSpriteCLI(runner, spriteBin)
	flyClient := clients.NewFlyCLI(runner, envOr("FLY_CLI", "fly"))
	ghClient := clients.NewGHCLI(runner, envOr("GH_CLI", "gh"))
	gitClient := clients.NewGitCLI(runner, envOr("GIT_BIN", "git"))
	org := envOr("FLY_ORG", "misty-step")

	switch args[0] {
	case "agent":
		return runAgent(ctx, logger, runner, gitClient, args[1:])
	case "watch":
		return runWatch(ctx, logger, spriteClient, org, args[1:])
	case "fleet":
		return runFleet(ctx, spriteClient, flyClient, org, args[1:])
	case "health":
		return runHealth(ctx, spriteClient, org, args[1:])
	case "status":
		return runStatus(ctx, spriteClient, org, args[1:])
	case "prs":
		return runPRs(ctx, ghClient, args[1:])
	case "logs":
		return runLogs(ctx, spriteClient, org, args[1:])
	case "help", "--help", "-h":
		usage()
		return nil
	default:
		usage()
		return fmt.Errorf("unknown subcommand %q", args[0])
	}
}

func runAgent(ctx context.Context, logger *slog.Logger, runner clients.Runner, git clients.GitClient, args []string) error {
	fs := flag.NewFlagSet("agent", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	cfg := agent.Config{}
	fs.StringVar(&cfg.SpriteName, "sprite", envOr("SPRITE_NAME", "sprite"), "sprite name for event metadata")
	fs.StringVar(&cfg.Workspace, "workspace", "/home/sprite/workspace", "workspace path")
	fs.StringVar(&cfg.PromptFile, "prompt-file", "PROMPT.md", "prompt file name")
	fs.StringVar(&cfg.LogFile, "log-file", "", "log file path (default: <workspace>/ralph.log)")
	fs.StringVar(&cfg.EventFile, "event-file", "", "event file path (default: <workspace>/sprite-agent-events.ndjson)")
	fs.StringVar(&cfg.ClaudeCommand, "claude-cmd", "claude -p --permission-mode bypassPermissions", "claude invocation command")
	fs.IntVar(&cfg.MaxIterations, "max-iterations", 50, "max loop iterations (0 for unlimited)")
	fs.DurationVar(&cfg.RestartDelay, "restart-delay", 5*time.Second, "delay before restarting Claude after exit")
	fs.DurationVar(&cfg.HeartbeatEvery, "heartbeat", 30*time.Second, "heartbeat emit interval")
	fs.DurationVar(&cfg.GitScanEvery, "git-scan", 45*time.Second, "git progress scan interval")
	fs.DurationVar(&cfg.AutoPushEvery, "auto-push-interval", 2*time.Minute, "auto-push interval")
	fs.BoolVar(&cfg.AutoPush, "auto-push", true, "auto-push commits ahead of origin")
	fs.BoolVar(&cfg.DryRun, "dry-run", false, "emit actions without running mutating operations")
	fs.BoolVar(&cfg.StopOnTaskSignal, "stop-on-signal", true, "stop when TASK_COMPLETE or BLOCKED.md appears")
	fs.StringVar(&cfg.PushRemote, "push-remote", "origin", "remote for auto-push")
	if err := fs.Parse(args); err != nil {
		return err
	}

	return agent.RunDaemon(ctx, cfg, agent.Dependencies{
		Runner: runner,
		Git:    git,
		Logger: logger,
		Stdout: os.Stdout,
	})
}

func runWatch(ctx context.Context, logger *slog.Logger, sprite clients.SpriteClient, defaultOrg string, args []string) error {
	fs := flag.NewFlagSet("watch", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	cfg := watchdog.Config{}
	fs.StringVar(&cfg.Org, "org", defaultOrg, "Fly/Sprite org")
	fs.StringVar(&cfg.ActiveAgentsFile, "active-agents", "/tmp/active-agents.txt", "active agents tracker file")
	fs.BoolVar(&cfg.DryRun, "dry-run", false, "show mutating operations without executing")
	fs.BoolVar(&cfg.EnableRedispatch, "redispatch", true, "auto-redispatch dead sprites with active tasks")
	fs.BoolVar(&cfg.ConfirmRedispatch, "confirm-redispatch", false, "required to execute redispatch operations")
	fs.BoolVar(&cfg.AutoPushOnDone, "auto-push", true, "auto-push unpushed commits when TASK_COMPLETE is set")
	fs.IntVar(&cfg.StaleMinutes, "stale-minutes", 30, "minutes without changes before stale alert")
	fs.StringVar(&cfg.MarkerFile, "marker-file", "/tmp/watchdog-marker", "local marker file path")
	fs.BoolVar(&cfg.JSONOutput, "json", false, "JSON output")
	if err := fs.Parse(args); err != nil {
		return err
	}

	r := watchdog.Runner{Sprite: sprite, Log: logger, Out: os.Stdout}
	_, err := r.Run(ctx, cfg)
	if errors.Is(err, watchdog.ErrNeedsAttention) {
		return err
	}
	return err
}

func runFleet(ctx context.Context, sprite clients.SpriteClient, fly clients.FlyClient, defaultOrg string, args []string) error {
	fs := flag.NewFlagSet("fleet", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	cfg := fleet.FleetConfig{}
	defaultEvents := filepath.Join(os.Getenv("HOME"), ".openclaw/workspace/infra/sprite-events.ndjson")
	fs.StringVar(&cfg.Org, "org", defaultOrg, "Fly/Sprite org")
	fs.StringVar(&cfg.EventsFile, "events-file", defaultEvents, "NDJSON event log path")
	fs.DurationVar(&cfg.MaxAge, "max-age", 20*time.Minute, "max event age before fallback")
	fs.BoolVar(&cfg.JSONOutput, "json", false, "JSON output")
	if err := fs.Parse(args); err != nil {
		return err
	}
	service := fleet.FleetService{Sprite: sprite, Fly: fly, Out: os.Stdout}
	_, err := service.Run(ctx, cfg)
	return err
}

func runHealth(ctx context.Context, sprite clients.SpriteClient, defaultOrg string, args []string) error {
	fs := flag.NewFlagSet("health", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	cfg := health.Config{}
	fs.StringVar(&cfg.Org, "org", defaultOrg, "Fly/Sprite org")
	fs.BoolVar(&cfg.JSONOutput, "json", false, "JSON output")
	fs.IntVar(&cfg.StaleThresholdMin, "stale-minutes", 30, "minutes without file changes before stale")
	fs.StringVar(&cfg.ActiveAgentsFile, "active-agents", "/tmp/active-agents.txt", "active agents tracker file")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() > 0 {
		cfg.Sprite = fs.Arg(0)
	}
	checker := health.Checker{Sprite: sprite, Out: os.Stdout}
	_, err := checker.Run(ctx, cfg)
	return err
}

func runStatus(ctx context.Context, sprite clients.SpriteClient, defaultOrg string, args []string) error {
	fs := flag.NewFlagSet("status", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	cfg := fleet.StatusConfig{}
	repoRoot, _ := os.Getwd()
	cfg.CompositionPath = filepath.Join(repoRoot, "compositions", "v1.yaml")
	cfg.SpritesDir = filepath.Join(repoRoot, "sprites")
	fs.StringVar(&cfg.Org, "org", defaultOrg, "Fly/Sprite org")
	fs.StringVar(&cfg.CompositionPath, "composition", cfg.CompositionPath, "composition YAML path")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() > 1 {
		return fmt.Errorf("only one sprite name can be provided")
	}
	if fs.NArg() == 1 {
		cfg.TargetSprite = fs.Arg(0)
	}
	service := fleet.StatusService{Sprite: sprite, Out: os.Stdout}
	return service.Run(ctx, cfg)
}

func runPRs(ctx context.Context, gh clients.GitHubClient, args []string) error {
	fs := flag.NewFlagSet("prs", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	cfg := prs.Config{}
	fs.StringVar(&cfg.Org, "org", "misty-step", "GitHub org")
	fs.StringVar(&cfg.Author, "author", "kaylee-mistystep", "PR author")
	fs.IntVar(&cfg.PerPage, "per-page", 50, "max PRs to fetch")
	fs.BoolVar(&cfg.DryRun, "dry-run", true, "required for mutating paths; default true")
	fs.BoolVar(&cfg.JSONOnly, "json-only", true, "emit JSON")
	if err := fs.Parse(args); err != nil {
		return err
	}
	s := prs.Shepherd{GH: gh, Out: os.Stdout}
	_, err := s.Run(ctx, cfg)
	return err
}

func runLogs(ctx context.Context, sprite clients.SpriteClient, defaultOrg string, args []string) error {
	fs := flag.NewFlagSet("logs", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	cfg := logs.Config{}
	fs.StringVar(&cfg.Org, "org", defaultOrg, "Fly/Sprite org")
	fs.BoolVar(&cfg.All, "all", false, "tail all sprites")
	fs.BoolVar(&cfg.Brief, "brief", false, "show only 5 lines")
	fs.IntVar(&cfg.Lines, "n", 50, "number of lines")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() > 0 {
		cfg.Sprite = fs.Arg(0)
	}
	v := logs.Viewer{Sprite: sprite, Out: os.Stdout}
	return v.Run(ctx, cfg)
}

func usage() {
	fmt.Fprintln(os.Stderr, "Usage: bb <subcommand> [options]")
	fmt.Fprintln(os.Stderr)
	fmt.Fprintln(os.Stderr, "Subcommands:")
	fmt.Fprintln(os.Stderr, "  agent   On-sprite supervisor daemon")
	fmt.Fprintln(os.Stderr, "  watch   Active fleet watchdog (signals, stale/dead detection, redispatch)")
	fmt.Fprintln(os.Stderr, "  fleet   Event-log fleet status with sprite/fly fallback")
	fmt.Fprintln(os.Stderr, "  health  Deep health checks for sprites")
	fmt.Fprintln(os.Stderr, "  status  Fleet overview and per-sprite detail")
	fmt.Fprintln(os.Stderr, "  prs     PR shepherd monitoring")
	fmt.Fprintln(os.Stderr, "  logs    Tail sprite logs")
}

func envOr(name, fallback string) string {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback
	}
	return value
}
