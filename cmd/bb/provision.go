package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/misty-step/bitterblossom/internal/contracts"
	"github.com/misty-step/bitterblossom/internal/lifecycle"
	"github.com/misty-step/bitterblossom/internal/sprite"
	"github.com/spf13/cobra"
)

type provisionOptions struct {
	Composition string
	All         bool
	Org         string
	SpriteCLI   string
	Timeout     time.Duration
}

type provisionDeps struct {
	getwd              func() (string, error)
	getenv             func(string) string
	newCLI             func(binary, org string) sprite.SpriteCLI
	resolveComposition func(path string) ([]string, error)
	resolveGitHubAuth  func(spriteName string, getenv func(string) string) (lifecycle.GitHubAuth, error)
	renderSettings     func(settingsPath, authToken string) (string, error)
	provision          func(ctx context.Context, cli sprite.SpriteCLI, cfg lifecycle.Config, opts lifecycle.ProvisionOpts) (lifecycle.ProvisionResult, error)
}

func defaultProvisionDeps() provisionDeps {
	return provisionDeps{
		getwd:  os.Getwd,
		getenv: os.Getenv,
		newCLI: func(binary, org string) sprite.SpriteCLI {
			cli := sprite.NewCLIWithOrg(binary, org)
			return cli
		},
		resolveComposition: resolveCompositionSprites,
		resolveGitHubAuth:  lifecycle.ResolveGitHubAuth,
		renderSettings:     lifecycle.RenderSettings,
		provision:          lifecycle.Provision,
	}
}

func newProvisionCmd() *cobra.Command {
	return newProvisionCmdWithDeps(defaultProvisionDeps())
}

func newProvisionCmdWithDeps(deps provisionDeps) *cobra.Command {
	opts := provisionOptions{
		Composition: defaultLifecycleComposition,
		Org:         defaultOrg(),
		SpriteCLI:   defaultSpriteCLIPath(),
		Timeout:     30 * time.Minute,
	}

	command := &cobra.Command{
		Use:   "provision <sprite-name>",
		Short: "Provision one sprite or the full composition",
		RunE: func(cmd *cobra.Command, args []string) error {
			names, err := resolveProvisionTargets(args, opts.All, opts.Composition, deps.resolveComposition)
			if err != nil {
				return err
			}

			rootDir, err := deps.getwd()
			if err != nil {
				return err
			}
			cfg := defaultLifecycleConfig(rootDir, opts.Org)

			settingsPath := filepath.Join(cfg.BaseDir, "settings.json")
			renderedSettings, err := deps.renderSettings(settingsPath, deps.getenv("ANTHROPIC_AUTH_TOKEN"))
			if err != nil {
				return err
			}
			defer func() {
				_ = os.Remove(renderedSettings)
			}()

			runCtx, cancel := context.WithTimeout(cmd.Context(), opts.Timeout)
			defer cancel()

			cli := deps.newCLI(opts.SpriteCLI, opts.Org)
			compositionLabel := strings.TrimSuffix(filepath.Base(opts.Composition), filepath.Ext(opts.Composition))

			results := make([]lifecycle.ProvisionResult, 0, len(names))
			for _, name := range names {
				auth, err := deps.resolveGitHubAuth(name, deps.getenv)
				if err != nil {
					return err
				}
				result, err := deps.provision(runCtx, cli, cfg, lifecycle.ProvisionOpts{
					Name:             name,
					CompositionLabel: compositionLabel,
					SettingsPath:     renderedSettings,
					GitHubAuth:       auth,
					BootstrapScript:  filepath.Join(cfg.RootDir, "scripts", "sprite-bootstrap.sh"),
					AgentScript:      filepath.Join(cfg.RootDir, "scripts", "sprite-agent.sh"),
				})
				if err != nil {
					return err
				}
				results = append(results, result)
			}

			return contracts.WriteJSON(cmd.OutOrStdout(), "provision", map[string]any{
				"results": results,
			})
		},
	}

	command.Flags().StringVar(&opts.Composition, "composition", opts.Composition, "Path to composition YAML")
	command.Flags().BoolVar(&opts.All, "all", false, "Provision all sprites from composition")
	command.Flags().StringVar(&opts.Org, "org", opts.Org, "Sprites organization")
	command.Flags().StringVar(&opts.SpriteCLI, "sprite-cli", opts.SpriteCLI, "Path to sprite CLI")
	command.Flags().DurationVar(&opts.Timeout, "timeout", opts.Timeout, "Command timeout")

	return command
}

func resolveProvisionTargets(
	args []string,
	all bool,
	compositionPath string,
	resolveComposition func(path string) ([]string, error),
) ([]string, error) {
	if all {
		if len(args) > 0 {
			return nil, errors.New("use either --all or explicit sprite names, not both")
		}
		return resolveComposition(compositionPath)
	}
	if len(args) == 0 {
		return nil, errors.New("sprite name is required (or pass --all)")
	}
	return args, nil
}
