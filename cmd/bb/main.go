package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/misty-step/bitterblossom/internal/config"
	"github.com/misty-step/bitterblossom/internal/dispatch"
	"github.com/misty-step/bitterblossom/internal/monitor"
	"github.com/misty-step/bitterblossom/internal/sprite"
)

var version = "dev"

func main() {
	if err := run(context.Background(), os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string, out io.Writer) error {
	if len(args) == 0 || args[0] == "version" {
		_, err := fmt.Fprintf(out, "bb version %s\n", version)
		return err
	}

	switch args[0] {
	case "run-task":
		return runTask(ctx, args[1:], out)
	case "check-fleet":
		return checkFleet(ctx, args[1:], out)
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func runTask(ctx context.Context, args []string, out io.Writer) error {
	if len(args) < 3 {
		return errors.New("usage: bb run-task <sprite> <repo|owner/repo> <issue-number> [persona-role]")
	}

	issueNumber, err := strconv.Atoi(args[2])
	if err != nil {
		return fmt.Errorf("invalid issue number %q: %w", args[2], err)
	}

	personaRole := "sprite"
	if len(args) > 3 {
		personaRole = args[3]
	}

	svc := dispatch.NewService(sprite.NewCLI(os.Getenv("SPRITE_CLI")))
	result, err := svc.RunIssueTask(ctx, dispatch.DispatchRequest{
		Sprite:      args[0],
		Repo:        args[1],
		IssueNumber: issueNumber,
		PersonaRole: personaRole,
	})
	if err != nil {
		return err
	}

	_, err = fmt.Fprintf(
		out,
		"Dispatched %s to %s (pid=%d)\nLog: %s\n",
		result.Sprite,
		result.Task,
		result.PID,
		result.LogPath,
	)
	return err
}

func checkFleet(ctx context.Context, args []string, out io.Writer) error {
	fs := flag.NewFlagSet("check-fleet", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	all := fs.Bool("all", false, "check all sprites from `sprite list`")
	compositionPath := fs.String("composition", "compositions/v1.yaml", "composition yaml path")
	if err := fs.Parse(args); err != nil {
		return err
	}

	targets := fs.Args()
	if !*all && len(targets) == 0 {
		composition, err := config.LoadComposition(*compositionPath)
		if err != nil {
			return fmt.Errorf("loading default sprite targets: %w", err)
		}
		targets = config.SpriteNames(composition)
	}

	svc := monitor.NewService(sprite.NewCLI(os.Getenv("SPRITE_CLI")))
	report, err := svc.CheckFleet(ctx, monitor.FleetRequest{Sprites: targets, All: *all})
	if err != nil {
		return err
	}

	if _, err := fmt.Fprintf(out, "%-12s %-10s %-30s %-20s %s\n", "SPRITE", "STATUS", "TASK", "STARTED", "RUNTIME"); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(out, "%-12s %-10s %-30s %-20s %s\n", "------", "------", "----", "-------", "-------"); err != nil {
		return err
	}

	var failures []string
	for _, spriteStatus := range report.Sprites {
		if _, err := fmt.Fprintf(
			out,
			"%-12s %-10s %-30s %-20s %s\n",
			spriteStatus.Sprite,
			spriteStatus.State,
			spriteStatus.Task,
			spriteStatus.Started,
			spriteStatus.Runtime,
		); err != nil {
			return err
		}
		if spriteStatus.Error != "" {
			failures = append(failures, fmt.Sprintf("%s: %s", spriteStatus.Sprite, spriteStatus.Error))
		}
	}

	if len(failures) == 0 {
		return nil
	}
	_, err = fmt.Fprintf(out, "\nWarnings:\n- %s\n", strings.Join(failures, "\n- "))
	return err
}
