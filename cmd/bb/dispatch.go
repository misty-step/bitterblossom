package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	dispatchsvc "github.com/misty-step/bitterblossom/internal/dispatch"
	"github.com/misty-step/bitterblossom/pkg/fly"
	"github.com/spf13/cobra"
)

type dispatchOptions struct {
	Repo                 string
	PromptFile           string
	Ralph                bool
	Execute              bool
	DryRun               bool
	JSON                 bool
	App                  string
	Token                string
	APIURL               string
	Org                  string
	SpriteCLI            string
	CompositionPath      string
	MaxIterations        int
	MaxTokens            int
	MaxTime              time.Duration
	WebhookURL           string
	AllowAnthropicDirect bool
}

type dispatchDeps struct {
	readFile     func(path string) ([]byte, error)
	newFlyClient func(token, apiURL string) (fly.MachineClient, error)
	newRemote    func(binary, org string) *spriteCLIRemote
	newService   func(cfg dispatchsvc.Config) (dispatchRunner, error)
}

type dispatchRunner interface {
	Run(ctx context.Context, req dispatchsvc.Request) (dispatchsvc.Result, error)
}

func defaultDispatchDeps() dispatchDeps {
	return dispatchDeps{
		readFile: os.ReadFile,
		newFlyClient: func(token, apiURL string) (fly.MachineClient, error) {
			return fly.NewClient(token, fly.WithBaseURL(apiURL))
		},
		newRemote: newSpriteCLIRemote,
		newService: func(cfg dispatchsvc.Config) (dispatchRunner, error) {
			return dispatchsvc.NewService(cfg)
		},
	}
}

func newDispatchCmd() *cobra.Command {
	return newDispatchCmdWithDeps(defaultDispatchDeps())
}

func newDispatchCmdWithDeps(deps dispatchDeps) *cobra.Command {
	opts := dispatchOptions{
		DryRun:          true,
		App:             strings.TrimSpace(os.Getenv("FLY_APP")),
		Token:           defaultFlyToken(),
		APIURL:          fly.DefaultBaseURL,
		Org:             strings.TrimSpace(os.Getenv("FLY_ORG")),
		SpriteCLI:       strings.TrimSpace(os.Getenv("SPRITE_CLI")),
		CompositionPath: "compositions/v1.yaml",
		MaxIterations:   dispatchsvc.DefaultMaxRalphIterations,
		MaxTokens:       dispatchsvc.DefaultMaxTokens,
		MaxTime:         dispatchsvc.DefaultMaxTime,
		WebhookURL:      strings.TrimSpace(os.Getenv("SPRITE_WEBHOOK_URL")),
	}

	command := &cobra.Command{
		Use:   "dispatch <sprite> [prompt]",
		Short: "Dispatch a task prompt to a sprite (dry-run by default)",
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return errors.New("dispatch: sprite name is required")
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if opts.Execute {
				opts.DryRun = false
			}
			if !opts.Execute && cmd.Flags().Changed("dry-run") && !opts.DryRun {
				return errors.New("dispatch: --dry-run=false requires --execute")
			}

			prompt, err := resolveDispatchPrompt(args, opts, deps)
			if err != nil {
				return err
			}

			appMissing := strings.TrimSpace(opts.App) == ""
			tokenMissing := strings.TrimSpace(opts.Token) == ""
			if appMissing || tokenMissing {
				return errors.New("Error: FLY_APP and FLY_API_TOKEN are required for sprite operations.\n  export FLY_APP=your-app\n  export FLY_API_TOKEN=your-token")
			}

			flyClient, err := deps.newFlyClient(opts.Token, opts.APIURL)
			if err != nil {
				return err
			}
			remote := deps.newRemote(opts.SpriteCLI, opts.Org)
			service, err := deps.newService(dispatchsvc.Config{
				Remote:             remote,
				Fly:                flyClient,
				App:                opts.App,
				Workspace:          dispatchsvc.DefaultWorkspace,
				CompositionPath:    opts.CompositionPath,
				RalphTemplatePath:  "scripts/ralph-prompt-template.md",
				MaxRalphIterations: opts.MaxIterations,
			})
			if err != nil {
				return err
			}

			result, err := service.Run(contextOrBackground(cmd.Context()), dispatchsvc.Request{
				Sprite:               args[0],
				Prompt:               prompt,
				Repo:                 opts.Repo,
				Ralph:                opts.Ralph,
				Execute:              opts.Execute,
				WebhookURL:           opts.WebhookURL,
				AllowAnthropicDirect: opts.AllowAnthropicDirect,
				MaxTokens:            opts.MaxTokens,
				MaxTime:              opts.MaxTime,
			})
			if err != nil {
				return err
			}

			return renderDispatchResult(cmd, result, opts.JSON)
		},
	}

	command.Flags().StringVar(&opts.Repo, "repo", "", "Repo to clone/pull before dispatch (org/repo or URL)")
	command.Flags().StringVar(&opts.PromptFile, "file", "", "Read prompt from a file")
	command.Flags().BoolVar(&opts.Ralph, "ralph", false, "Start persistent Ralph loop instead of one-shot")
	command.Flags().BoolVar(&opts.Execute, "execute", false, "Execute dispatch actions (default is dry-run)")
	command.Flags().BoolVar(&opts.DryRun, "dry-run", true, "Preview dispatch plan without side effects")
	command.Flags().BoolVar(&opts.JSON, "json", false, "Emit JSON output")
	command.Flags().StringVar(&opts.App, "app", opts.App, "Sprites app name")
	command.Flags().StringVar(&opts.Token, "token", opts.Token, "API token (or FLY_API_TOKEN/FLY_TOKEN)")
	command.Flags().StringVar(&opts.APIURL, "api-url", opts.APIURL, "Sprites API base URL")
	command.Flags().StringVar(&opts.Org, "org", opts.Org, "Sprite org passed to sprite CLI")
	command.Flags().StringVar(&opts.SpriteCLI, "sprite-cli", opts.SpriteCLI, "Sprite CLI binary path")
	command.Flags().StringVar(&opts.CompositionPath, "composition", opts.CompositionPath, "Composition YAML used for provisioning metadata")
	command.Flags().IntVar(&opts.MaxIterations, "max-iterations", opts.MaxIterations, "Ralph loop iteration safety cap")
	command.Flags().IntVar(&opts.MaxTokens, "max-tokens", opts.MaxTokens, "Ralph stuck-loop token safety cap (only with --ralph)")
	command.Flags().DurationVar(&opts.MaxTime, "max-time", opts.MaxTime, "Ralph stuck-loop runtime safety cap (only with --ralph)")
	command.Flags().StringVar(&opts.WebhookURL, "webhook-url", opts.WebhookURL, "Optional sprite-agent webhook URL")
	command.Flags().BoolVar(&opts.AllowAnthropicDirect, "allow-anthropic-direct", false, "Allow dispatch even if sprite has a real ANTHROPIC_API_KEY")

	return command
}

func resolveDispatchPrompt(args []string, opts dispatchOptions, deps dispatchDeps) (string, error) {
	if strings.TrimSpace(opts.PromptFile) != "" {
		content, err := deps.readFile(opts.PromptFile)
		if err != nil {
			return "", err
		}
		prompt := strings.TrimSpace(string(content))
		if prompt == "" {
			return "", errors.New("dispatch: prompt file is empty")
		}
		return prompt, nil
	}

	if len(args) < 2 {
		return "", errors.New("dispatch: prompt is required when --file is not set")
	}
	prompt := strings.TrimSpace(strings.Join(args[1:], " "))
	if prompt == "" {
		return "", errors.New("dispatch: prompt cannot be empty")
	}
	return prompt, nil
}

func renderDispatchResult(cmd *cobra.Command, result dispatchsvc.Result, jsonMode bool) error {
	if jsonMode {
		encoded, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			return err
		}
		_, err = fmt.Fprintln(cmd.OutOrStdout(), string(encoded))
		return err
	}

	if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Sprite: %s\n", result.Plan.Sprite); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Mode: %s\n", result.Plan.Mode); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(cmd.OutOrStdout(), "Plan:"); err != nil {
		return err
	}
	for _, step := range result.Plan.Steps {
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "- [%s] %s\n", step.Kind, step.Description); err != nil {
			return err
		}
	}

	if !result.Executed {
		_, err := fmt.Fprintln(cmd.OutOrStdout(), "Dry run only. Re-run with --execute to apply.")
		return err
	}

	if _, err := fmt.Fprintf(cmd.OutOrStdout(), "State: %s\n", result.State); err != nil {
		return err
	}
	if result.AgentPID > 0 {
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Agent PID: %d\n", result.AgentPID); err != nil {
			return err
		}
	}
	if strings.TrimSpace(result.CommandOutput) != "" {
		if _, err := fmt.Fprintln(cmd.OutOrStdout(), "Output:"); err != nil {
			return err
		}
		if _, err := fmt.Fprintln(cmd.OutOrStdout(), strings.TrimSpace(result.CommandOutput)); err != nil {
			return err
		}
	}
	return nil
}

func contextOrBackground(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}
	return ctx
}
