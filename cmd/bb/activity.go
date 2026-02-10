package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
)

type activityOptions struct {
	Org       string
	SpriteCLI string
	Format    string
	Commits   int
	Timeout   time.Duration
}

type spriteRemote interface {
	Exec(ctx context.Context, sprite, command string, stdin []byte) (string, error)
}

type activityDeps struct {
	newRemote func(binary, org string) spriteRemote
}

type gitActivity struct {
	Sprite       string   `json:"sprite"`
	Branch       string   `json:"branch"`
	RemoteBranch string   `json:"remote_branch,omitempty"`
	Ahead        int      `json:"ahead"`
	Behind       int      `json:"behind"`
	Commits      []commit `json:"commits"`
	Staged       []string `json:"staged"`
	Unstaged     []string `json:"unstaged"`
	Untracked    []string `json:"untracked"`
	Error        string   `json:"error,omitempty"`
}

type commit struct {
	Hash    string `json:"hash"`
	Message string `json:"message"`
}

func defaultActivityDeps() activityDeps {
	return activityDeps{
		newRemote: func(binary, org string) spriteRemote {
			return newSpriteCLIRemote(binary, org)
		},
	}
}

func newActivityCmd() *cobra.Command {
	return newActivityCmdWithDeps(defaultActivityDeps())
}

func newActivityCmdWithDeps(deps activityDeps) *cobra.Command {
	opts := activityOptions{
		Org:       defaultOrg(),
		SpriteCLI: defaultSpriteCLIPath(),
		Format:    "text",
		Commits:   10,
		Timeout:   30 * time.Second,
	}

	command := &cobra.Command{
		Use:   "activity <sprite>",
		Short: "Show git activity for a sprite (branch, commits, files changed)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			format := strings.ToLower(strings.TrimSpace(opts.Format))
			if format != "json" && format != "text" {
				return errors.New("--format must be json or text")
			}

			spriteName := strings.TrimSpace(args[0])
			if spriteName == "" {
				return errors.New("sprite name is required")
			}

			runCtx, cancel := context.WithTimeout(cmd.Context(), opts.Timeout)
			defer cancel()

			remote := deps.newRemote(opts.SpriteCLI, opts.Org)
			activity, err := fetchGitActivity(runCtx, remote, spriteName, opts.Commits)
			if err != nil {
				return &exitError{Code: 1, Err: err}
			}

			if format == "json" {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				if err := enc.Encode(activity); err != nil {
					return &exitError{Code: 1, Err: err}
				}
				return nil
			}

			return writeActivityText(cmd.OutOrStdout(), activity)
		},
	}

	command.Flags().StringVar(&opts.Org, "org", opts.Org, "Sprites organization")
	command.Flags().StringVar(&opts.SpriteCLI, "sprite-cli", opts.SpriteCLI, "Path to sprite CLI")
	command.Flags().StringVar(&opts.Format, "format", opts.Format, "Output format: json|text")
	command.Flags().IntVar(&opts.Commits, "commits", opts.Commits, "Number of recent commits to show")
	command.Flags().DurationVar(&opts.Timeout, "timeout", opts.Timeout, "Command timeout")

	return command
}

func fetchGitActivity(ctx context.Context, remote spriteRemote, sprite string, commitCount int) (gitActivity, error) {
	activity := gitActivity{Sprite: sprite}

	// Get current branch
	branchOut, err := remote.Exec(ctx, sprite, "git -C /mnt/persist rev-parse --abbrev-ref HEAD", nil)
	if err != nil {
		activity.Error = fmt.Sprintf("failed to get branch: %v", err)
		return activity, nil
	}
	activity.Branch = strings.TrimSpace(branchOut)

	// Get remote tracking branch
	remoteOut, err := remote.Exec(ctx, sprite, "git -C /mnt/persist rev-parse --abbrev-ref --symbolic-full-name @{u} 2>/dev/null || echo ''", nil)
	if err == nil {
		activity.RemoteBranch = strings.TrimSpace(remoteOut)
	}

	// Get ahead/behind counts
	if activity.RemoteBranch != "" {
		aheadBehindOut, err := remote.Exec(ctx, sprite, "git -C /mnt/persist rev-list --left-right --count HEAD...@{u} 2>/dev/null || echo '0\t0'", nil)
		if err == nil {
			parts := strings.Fields(strings.TrimSpace(aheadBehindOut))
			if len(parts) >= 2 {
				fmt.Sscanf(parts[0], "%d", &activity.Ahead)
				fmt.Sscanf(parts[1], "%d", &activity.Behind)
			}
		}
	}

	// Get recent commits
	logOut, err := remote.Exec(ctx, sprite, fmt.Sprintf("git -C /mnt/persist log -n %d --pretty=format:%%h%%n%%s%%n%%n", commitCount), nil)
	if err == nil {
		lines := strings.Split(strings.TrimSpace(logOut), "\n")
		activity.Commits = make([]commit, 0, commitCount)
		for i := 0; i < len(lines); i++ {
			hash := strings.TrimSpace(lines[i])
			if hash == "" {
				continue
			}
			// Next line should be the message
			if i+1 < len(lines) {
				msg := strings.TrimSpace(lines[i+1])
				activity.Commits = append(activity.Commits, commit{
					Hash:    hash,
					Message: msg,
				})
				i++ // Skip message line
				// Skip empty separator line if present
				if i+1 < len(lines) && strings.TrimSpace(lines[i+1]) == "" {
					i++
				}
			}
		}
	}

	// Get staged files
	stagedOut, err := remote.Exec(ctx, sprite, "git -C /mnt/persist diff --cached --name-only", nil)
	if err == nil {
		activity.Staged = parseFileList(stagedOut)
	}

	// Get unstaged files
	unstagedOut, err := remote.Exec(ctx, sprite, "git -C /mnt/persist diff --name-only", nil)
	if err == nil {
		activity.Unstaged = parseFileList(unstagedOut)
	}

	// Get untracked files
	untrackedOut, err := remote.Exec(ctx, sprite, "git -C /mnt/persist ls-files --others --exclude-standard", nil)
	if err == nil {
		activity.Untracked = parseFileList(untrackedOut)
	}

	return activity, nil
}

func parseFileList(output string) []string {
	lines := strings.Split(strings.TrimSpace(output), "\n")
	result := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			result = append(result, line)
		}
	}
	return result
}

func writeActivityText(out io.Writer, activity gitActivity) error {
	if activity.Error != "" {
		_, err := fmt.Fprintf(out, "Error: %s\n", activity.Error)
		return err
	}

	if _, err := fmt.Fprintf(out, "=== Git Activity: %s ===\n\n", activity.Sprite); err != nil {
		return err
	}

	// Branch info
	if _, err := fmt.Fprintf(out, "Branch: %s\n", activity.Branch); err != nil {
		return err
	}
	if activity.RemoteBranch != "" {
		if _, err := fmt.Fprintf(out, "Tracking: %s\n", activity.RemoteBranch); err != nil {
			return err
		}
		if activity.Ahead > 0 || activity.Behind > 0 {
			if _, err := fmt.Fprintf(out, "Status: %d ahead, %d behind\n", activity.Ahead, activity.Behind); err != nil {
				return err
			}
		} else {
			if _, err := fmt.Fprintln(out, "Status: up to date"); err != nil {
				return err
			}
		}
	}
	if _, err := fmt.Fprintln(out); err != nil {
		return err
	}

	// Recent commits
	if len(activity.Commits) > 0 {
		if _, err := fmt.Fprintf(out, "Recent Commits (%d):\n", len(activity.Commits)); err != nil {
			return err
		}
		tw := tabwriter.NewWriter(out, 2, 2, 2, ' ', 0)
		for _, c := range activity.Commits {
			if _, err := fmt.Fprintf(tw, "  %s\t%s\n", c.Hash, c.Message); err != nil {
				return err
			}
		}
		if err := tw.Flush(); err != nil {
			return err
		}
		if _, err := fmt.Fprintln(out); err != nil {
			return err
		}
	}

	// File changes
	totalChanges := len(activity.Staged) + len(activity.Unstaged) + len(activity.Untracked)
	if totalChanges > 0 {
		if _, err := fmt.Fprintf(out, "File Changes (%d):\n", totalChanges); err != nil {
			return err
		}

		if len(activity.Staged) > 0 {
			if _, err := fmt.Fprintf(out, "  Staged (%d):\n", len(activity.Staged)); err != nil {
				return err
			}
			for _, file := range activity.Staged {
				if _, err := fmt.Fprintf(out, "    + %s\n", file); err != nil {
					return err
				}
			}
		}

		if len(activity.Unstaged) > 0 {
			if _, err := fmt.Fprintf(out, "  Unstaged (%d):\n", len(activity.Unstaged)); err != nil {
				return err
			}
			for _, file := range activity.Unstaged {
				if _, err := fmt.Fprintf(out, "    M %s\n", file); err != nil {
					return err
				}
			}
		}

		if len(activity.Untracked) > 0 {
			if _, err := fmt.Fprintf(out, "  Untracked (%d):\n", len(activity.Untracked)); err != nil {
				return err
			}
			for _, file := range activity.Untracked {
				if _, err := fmt.Fprintf(out, "    ? %s\n", file); err != nil {
					return err
				}
			}
		}
	} else {
		if _, err := fmt.Fprintln(out, "Working tree clean"); err != nil {
			return err
		}
	}

	return nil
}
