package main

import (
	"bufio"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/misty-step/bitterblossom/internal/dispatch"
	"github.com/misty-step/bitterblossom/internal/lib"
	"github.com/misty-step/bitterblossom/internal/preflight"
	"github.com/misty-step/bitterblossom/internal/provision"
	bsync "github.com/misty-step/bitterblossom/internal/sync"
	"github.com/misty-step/bitterblossom/internal/teardown"
	"github.com/spf13/cobra"
)

type rootOptions struct {
	Root      string
	SpriteCLI string
	Org       string
	DryRun    bool
	LogLevel  string
}

type runtime struct {
	paths  lib.Paths
	runner lib.Runner
	sprite *lib.SpriteCLI
	logger *slog.Logger
}

func main() {
	if err := newRootCmd().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func newRootCmd() *cobra.Command {
	opts := &rootOptions{}

	cmd := &cobra.Command{
		Use:   "bb",
		Short: "Bitterblossom sprite lifecycle CLI",
	}
	cmd.SilenceUsage = true
	cmd.PersistentFlags().StringVar(&opts.Root, "root", ".", "Repository root")
	cmd.PersistentFlags().StringVar(&opts.SpriteCLI, "sprite-cli", envOrDefault("SPRITE_CLI", lib.DefaultSpriteCLI), "Path to sprite CLI")
	cmd.PersistentFlags().StringVar(&opts.Org, "org", envOrDefault("FLY_ORG", lib.DefaultOrg), "Fly.io organization")
	cmd.PersistentFlags().BoolVar(&opts.DryRun, "dry-run", false, "Show mutating operations without executing them")
	cmd.PersistentFlags().StringVar(&opts.LogLevel, "log-level", "info", "Log level: debug|info|warn|error")

	cmd.AddCommand(newDispatchCmd(opts))
	cmd.AddCommand(newProvisionCmd(opts))
	cmd.AddCommand(newBootstrapCmd(opts))
	cmd.AddCommand(newPreflightCmd(opts))
	cmd.AddCommand(newSyncCmd(opts))
	cmd.AddCommand(newTeardownCmd(opts))

	return cmd
}

func newDispatchCmd(opts *rootOptions) *cobra.Command {
	var useRalph bool
	var repo string
	var promptFile string
	var stop bool
	var status bool
	var skipPreflight bool
	maxRalphIterations := envInt("MAX_RALPH_ITERATIONS", 50)

	cmd := &cobra.Command{
		Use:   "dispatch <sprite> [flags] [prompt]",
		Short: "Dispatch a one-shot task or manage Ralph loops",
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) < 1 {
				return fmt.Errorf("sprite name is required")
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			rt, err := buildRuntime(opts)
			if err != nil {
				return err
			}
			ctx := cmd.Context()
			spriteName := args[0]
			if err := lib.ValidateSpriteName(spriteName); err != nil {
				return err
			}

			exists, err := rt.sprite.Exists(ctx, spriteName)
			if err != nil {
				return err
			}
			if !exists {
				return fmt.Errorf("sprite %q does not exist", spriteName)
			}

			if stop && status {
				return fmt.Errorf("use either --stop or --status")
			}

			dispatchSvc := dispatch.NewService(rt.logger, rt.sprite, rt.paths, maxRalphIterations)

			if stop {
				return dispatchSvc.StopRalph(ctx, spriteName)
			}
			if status {
				report, err := dispatchSvc.CheckStatus(ctx, spriteName)
				if err != nil {
					return err
				}
				fmt.Printf("=== Sprite: %s ===\n\n", spriteName)
				fmt.Printf("Ralph loop: %s\n", report.RalphStatus)
				if report.Signals != "" {
					fmt.Println(report.Signals)
				}
				fmt.Println()
				fmt.Println("Recent log:")
				if report.RecentLog == "" {
					fmt.Println("  (no log)")
				} else {
					fmt.Println(report.RecentLog)
				}
				fmt.Println()
				fmt.Println("MEMORY.md (last 10 lines):")
				if report.MemoryTail == "" {
					fmt.Println("  (no MEMORY.md)")
				} else {
					fmt.Println(report.MemoryTail)
				}
				return nil
			}

			if !skipPreflight {
				pf := preflight.NewService(rt.logger, rt.sprite)
				report, err := pf.CheckSprite(ctx, spriteName)
				if err != nil {
					return err
				}
				if report.Failures > 0 {
					return fmt.Errorf("preflight failed for %q (%d critical failures)", spriteName, report.Failures)
				}
			}

			prompt := strings.TrimSpace(strings.Join(args[1:], " "))
			if promptFile != "" {
				content, err := os.ReadFile(promptFile)
				if err != nil {
					return fmt.Errorf("read prompt file %s: %w", promptFile, err)
				}
				prompt = string(content)
			}
			if strings.TrimSpace(prompt) == "" {
				return &lib.ValidationError{Field: "prompt", Message: "is required"}
			}

			if useRalph {
				return dispatchSvc.StartRalph(ctx, spriteName, prompt, repo)
			}
			output, err := dispatchSvc.DispatchOneShot(ctx, spriteName, prompt, repo)
			if err != nil {
				return err
			}
			if output != "" {
				fmt.Print(output)
			}
			return nil
		},
	}

	cmd.Flags().BoolVar(&useRalph, "ralph", false, "Start Ralph loop mode")
	cmd.Flags().StringVar(&repo, "repo", "", "Repo to clone/pull (org/repo or https:// URL)")
	cmd.Flags().StringVar(&promptFile, "file", "", "Read prompt from file")
	cmd.Flags().BoolVar(&stop, "stop", false, "Stop a running Ralph loop")
	cmd.Flags().BoolVar(&status, "status", false, "Show sprite status and logs")
	cmd.Flags().BoolVar(&skipPreflight, "skip-preflight", false, "Skip pre-dispatch preflight checks")
	cmd.Flags().IntVar(&maxRalphIterations, "max-ralph-iterations", maxRalphIterations, "Safety cap for Ralph iterations")
	return cmd
}

func newProvisionCmd(opts *rootOptions) *cobra.Command {
	var all bool
	compositionDefault := envOrDefault("COMPOSITION", lib.DefaultComposition)
	var composition string

	cmd := &cobra.Command{
		Use:   "provision [flags] <sprite-name ...>",
		Short: "Provision sprites from definitions",
		RunE: func(cmd *cobra.Command, args []string) error {
			rt, err := buildRuntime(opts)
			if err != nil {
				return err
			}
			ctx := cmd.Context()
			provisionSvc := provision.NewService(rt.logger, rt.sprite, rt.runner, rt.paths, composition, opts.DryRun)

			targets, resolvedCompositionPath, err := provisionSvc.ResolveTargets(ctx, all, args)
			if err != nil {
				return err
			}

			settingsPath := rt.paths.BaseSettingsPath()
			cleanup := func() error { return nil }
			if !opts.DryRun {
				token := strings.TrimSpace(os.Getenv("ANTHROPIC_AUTH_TOKEN"))
				settingsPath, cleanup, err = provisionSvc.PrepareRenderedSettings(token)
				if err != nil {
					return err
				}
				defer func() {
					_ = cleanup()
				}()
			}

			githubToken := strings.TrimSpace(os.Getenv("GITHUB_TOKEN"))
			for _, name := range targets {
				if err := provisionSvc.ProvisionSprite(ctx, name, settingsPath, resolvedCompositionPath, githubToken); err != nil {
					return err
				}
				fmt.Printf("provisioned: %s\n", name)
			}
			return nil
		},
	}

	cmd.Flags().BoolVar(&all, "all", false, "Provision all sprites from current composition")
	cmd.Flags().StringVar(&composition, "composition", compositionDefault, "Composition YAML path")
	return cmd
}

func newBootstrapCmd(opts *rootOptions) *cobra.Command {
	var all bool
	compositionDefault := envOrDefault("COMPOSITION", lib.DefaultComposition)
	var composition string

	cmd := &cobra.Command{
		Use:   "bootstrap [flags] <sprite-name ...>",
		Short: "Idempotent sprite environment setup (config + credentials + checks)",
		RunE: func(cmd *cobra.Command, args []string) error {
			rt, err := buildRuntime(opts)
			if err != nil {
				return err
			}
			ctx := cmd.Context()
			provisionSvc := provision.NewService(rt.logger, rt.sprite, rt.runner, rt.paths, composition, opts.DryRun)

			targets, resolvedCompositionPath, err := provisionSvc.ResolveTargets(ctx, all, args)
			if err != nil {
				return err
			}

			settingsPath := rt.paths.BaseSettingsPath()
			if !opts.DryRun {
				token := strings.TrimSpace(os.Getenv("ANTHROPIC_AUTH_TOKEN"))
				renderedPath, cleanup, err := provisionSvc.PrepareRenderedSettings(token)
				if err != nil {
					return err
				}
				settingsPath = renderedPath
				defer func() {
					_ = cleanup()
				}()
			}

			compositionLabel := strings.TrimSuffix(filepath.Base(resolvedCompositionPath), filepath.Ext(resolvedCompositionPath))
			githubToken := strings.TrimSpace(os.Getenv("GITHUB_TOKEN"))
			for _, name := range targets {
				exists, err := rt.sprite.Exists(ctx, name)
				if err != nil {
					return err
				}
				if !exists {
					return fmt.Errorf("sprite %q does not exist (bootstrap does not create sprites)", name)
				}
				definitionPath := filepath.Join(rt.paths.SpritesDir, name+".md")
				if err := provisionSvc.BootstrapSprite(ctx, name, definitionPath, settingsPath, compositionLabel, githubToken); err != nil {
					return err
				}
				fmt.Printf("bootstrapped: %s\n", name)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&all, "all", false, "Bootstrap all sprites from current composition")
	cmd.Flags().StringVar(&composition, "composition", compositionDefault, "Composition YAML path")
	return cmd
}

func newPreflightCmd(opts *rootOptions) *cobra.Command {
	var all bool

	cmd := &cobra.Command{
		Use:   "preflight [flags] <sprite-name>",
		Short: "Run pre-dispatch sprite validation checks",
		RunE: func(cmd *cobra.Command, args []string) error {
			rt, err := buildRuntime(opts)
			if err != nil {
				return err
			}
			ctx := cmd.Context()
			service := preflight.NewService(rt.logger, rt.sprite)

			if all {
				if len(args) > 0 {
					return fmt.Errorf("do not pass sprite names when using --all")
				}
				report, err := service.CheckAll(ctx)
				if err != nil {
					return err
				}
				for _, spriteReport := range report.Reports {
					printPreflightReport(spriteReport)
				}
				printPreflightSummary(report.Failures, report.Warnings)
				if report.Failures > 0 {
					return fmt.Errorf("preflight failed: %d critical failures", report.Failures)
				}
				return nil
			}

			if len(args) != 1 {
				return fmt.Errorf("usage: bb preflight <sprite-name> or bb preflight --all")
			}
			report, err := service.CheckSprite(ctx, args[0])
			if err != nil {
				return err
			}
			printPreflightReport(report)
			printPreflightSummary(report.Failures, report.Warnings)
			if report.Failures > 0 {
				return fmt.Errorf("preflight failed: %d critical failures", report.Failures)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&all, "all", false, "Check all sprites")
	return cmd
}

func newSyncCmd(opts *rootOptions) *cobra.Command {
	var baseOnly bool
	compositionDefault := envOrDefault("COMPOSITION", lib.DefaultComposition)
	var composition string

	cmd := &cobra.Command{
		Use:   "sync [flags] [sprite-name ...]",
		Short: "Sync shared config and persona updates to sprites",
		RunE: func(cmd *cobra.Command, args []string) error {
			rt, err := buildRuntime(opts)
			if err != nil {
				return err
			}
			ctx := cmd.Context()
			syncSvc := bsync.NewService(rt.logger, rt.sprite, rt.paths, composition)

			targets, _, err := syncSvc.ResolveTargets(args)
			if err != nil {
				return err
			}

			settingsPath := rt.paths.BaseSettingsPath()
			if !opts.DryRun {
				token := strings.TrimSpace(os.Getenv("ANTHROPIC_AUTH_TOKEN"))
				renderedPath, cleanup, err := syncSvc.PrepareRenderedSettings(token)
				if err != nil {
					return err
				}
				settingsPath = renderedPath
				defer func() {
					_ = cleanup()
				}()
			}

			for _, name := range targets {
				if err := syncSvc.SyncSprite(ctx, name, settingsPath, baseOnly); err != nil {
					return err
				}
				fmt.Printf("synced: %s\n", name)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&baseOnly, "base-only", false, "Sync only shared base config, skip persona files")
	cmd.Flags().StringVar(&composition, "composition", compositionDefault, "Composition YAML path")
	return cmd
}

func newTeardownCmd(opts *rootOptions) *cobra.Command {
	var force bool

	cmd := &cobra.Command{
		Use:   "teardown [flags] <sprite-name>",
		Short: "Export archives and decommission a sprite",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			rt, err := buildRuntime(opts)
			if err != nil {
				return err
			}
			service := teardown.NewService(rt.logger, rt.sprite, rt.paths, opts.DryRun)
			confirm := func(prompt string) (bool, error) {
				if _, err := fmt.Fprint(os.Stdout, prompt); err != nil {
					return false, err
				}
				reader := bufio.NewReader(os.Stdin)
				line, err := reader.ReadString('\n')
				if err != nil {
					return false, err
				}
				line = strings.TrimSpace(line)
				return line == "y" || line == "Y", nil
			}

			archivePath, err := service.TeardownSprite(cmd.Context(), args[0], force, confirm)
			if err != nil {
				if errors.Is(err, teardown.ErrAborted) {
					fmt.Println("aborted")
					return nil
				}
				return err
			}
			fmt.Printf("archives saved to: %s\n", archivePath)
			return nil
		},
	}
	cmd.Flags().BoolVarP(&force, "force", "f", false, "Skip confirmation prompt")
	return cmd
}

func buildRuntime(opts *rootOptions) (runtime, error) {
	paths, err := lib.NewPaths(opts.Root)
	if err != nil {
		return runtime{}, err
	}
	logger := newLogger(opts.LogLevel)
	runner := &lib.ExecRunner{Logger: logger, DryRun: opts.DryRun}
	sprite := lib.NewSpriteCLI(runner, opts.SpriteCLI, opts.Org)
	return runtime{paths: paths, runner: runner, sprite: sprite, logger: logger}, nil
}

func newLogger(level string) *slog.Logger {
	logLevel := new(slog.LevelVar)
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "debug":
		logLevel.Set(slog.LevelDebug)
	case "warn":
		logLevel.Set(slog.LevelWarn)
	case "error":
		logLevel.Set(slog.LevelError)
	default:
		logLevel.Set(slog.LevelInfo)
	}
	handler := slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: logLevel})
	return slog.New(handler)
}

func printPreflightReport(report preflight.SpriteReport) {
	fmt.Printf("\n=== Preflight: %s ===\n", report.Sprite)
	for _, check := range report.Checks {
		prefix := "[PASS]"
		switch check.Status {
		case preflight.StatusWarn:
			prefix = "[WARN]"
		case preflight.StatusFail:
			prefix = "[FAIL]"
		}
		fmt.Printf("  %s %s\n", prefix, check.Message)
	}
}

func printPreflightSummary(failures, warnings int) {
	fmt.Println("\n-------------------------------------")
	if failures > 0 {
		fmt.Printf("PREFLIGHT FAILED: %d critical failures, %d warnings\n", failures, warnings)
		return
	}
	if warnings > 0 {
		fmt.Printf("PREFLIGHT PASSED with %d warnings\n", warnings)
		return
	}
	fmt.Println("PREFLIGHT PASSED: all checks green")
}

func envOrDefault(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func envInt(key string, fallback int) int {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(raw)
	if err != nil {
		return fallback
	}
	if parsed <= 0 {
		return fallback
	}
	return parsed
}
