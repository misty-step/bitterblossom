package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"time"

	"github.com/misty-step/bitterblossom/internal/contracts"
	"github.com/misty-step/bitterblossom/internal/lifecycle"
	"github.com/misty-step/bitterblossom/internal/sprite"
	"github.com/spf13/cobra"
)

type syncOptions struct {
	Composition string
	BaseOnly    bool
	Org         string
	SpriteCLI   string
	Timeout     time.Duration
}

type syncDeps struct {
	getwd              func() (string, error)
	getenv             func(string) string
	newCLI             func(binary, org string) sprite.SpriteCLI
	resolveComposition func(path string) ([]string, error)
	renderSettings     func(settingsPath, authToken string) (string, error)
	sync               func(ctx context.Context, cli sprite.SpriteCLI, cfg lifecycle.Config, opts lifecycle.SyncOpts) error
}

func defaultSyncDeps() syncDeps {
	return syncDeps{
		getwd:  os.Getwd,
		getenv: os.Getenv,
		newCLI: func(binary, org string) sprite.SpriteCLI {
			cli := sprite.NewCLIWithOrg(binary, org)
			return cli
		},
		resolveComposition: resolveCompositionSprites,
		renderSettings:     lifecycle.RenderSettings,
		sync:               lifecycle.Sync,
	}
}

func newSyncCmd() *cobra.Command {
	return newSyncCmdWithDeps(defaultSyncDeps())
}

func newSyncCmdWithDeps(deps syncDeps) *cobra.Command {
	opts := syncOptions{
		Composition: defaultLifecycleComposition,
		Org:         defaultOrg(),
		SpriteCLI:   defaultSpriteCLIPath(),
		Timeout:     30 * time.Minute,
	}

	command := &cobra.Command{
		Use:   "sync [sprite-name ...]",
		Short: "Sync base config and persona definitions to sprites",
		RunE: func(cmd *cobra.Command, args []string) error {
			names := args
			if len(names) == 0 {
				resolved, err := deps.resolveComposition(opts.Composition)
				if err != nil {
					return err
				}
				names = resolved
			}

			rootDir, err := deps.getwd()
			if err != nil {
				return err
			}
			cfg := defaultLifecycleConfig(rootDir, opts.Org)

			settingsPath := filepath.Join(cfg.BaseDir, "settings.json")
			authToken := resolveLifecycleAuthToken(deps.getenv)
			if authToken == "" {
				return errors.New("sync: OPENROUTER_API_KEY is required (ANTHROPIC_AUTH_TOKEN is accepted as a legacy fallback)")
			}
			renderedSettings, err := deps.renderSettings(settingsPath, authToken)
			if err != nil {
				return err
			}
			defer func() {
				_ = os.Remove(renderedSettings)
			}()

			runCtx, cancel := context.WithTimeout(cmd.Context(), opts.Timeout)
			defer cancel()

			cli := deps.newCLI(opts.SpriteCLI, opts.Org)
			for _, name := range names {
				if err := deps.sync(runCtx, cli, cfg, lifecycle.SyncOpts{
					Name:         name,
					SettingsPath: renderedSettings,
					BaseOnly:     opts.BaseOnly,
				}); err != nil {
					return err
				}
			}

			return contracts.WriteJSON(cmd.OutOrStdout(), "sync", map[string]any{
				"sprites":   names,
				"base_only": opts.BaseOnly,
			})
		},
	}

	command.Flags().StringVar(&opts.Composition, "composition", opts.Composition, "Path to composition YAML")
	command.Flags().BoolVar(&opts.BaseOnly, "base-only", false, "Only sync shared base config")
	command.Flags().StringVar(&opts.Org, "org", opts.Org, "Sprites organization")
	command.Flags().StringVar(&opts.SpriteCLI, "sprite-cli", opts.SpriteCLI, "Path to sprite CLI")
	command.Flags().DurationVar(&opts.Timeout, "timeout", opts.Timeout, "Command timeout")

	return command
}
