package preflight

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"strings"

	"github.com/misty-step/bitterblossom/internal/lib"
)

type Status string

const (
	StatusPass Status = "pass"
	StatusWarn Status = "warn"
	StatusFail Status = "fail"
)

type CheckResult struct {
	Name    string
	Status  Status
	Message string
}

type SpriteReport struct {
	Sprite   string
	Checks   []CheckResult
	Failures int
	Warnings int
}

type FleetReport struct {
	Reports   []SpriteReport
	Failures  int
	Warnings  int
	Checked   int
	Succeeded int
}

type Service struct {
	Logger *slog.Logger
	Sprite *lib.SpriteCLI
}

func NewService(logger *slog.Logger, sprite *lib.SpriteCLI) *Service {
	return &Service{Logger: logger, Sprite: sprite}
}

func (s *Service) CheckAll(ctx context.Context) (FleetReport, error) {
	names, err := s.Sprite.List(ctx)
	if err != nil {
		return FleetReport{}, err
	}
	if len(names) == 0 {
		return FleetReport{}, fmt.Errorf("no sprites found")
	}

	report := FleetReport{}
	for _, name := range names {
		spriteReport, err := s.CheckSprite(ctx, name)
		if err != nil {
			return FleetReport{}, err
		}
		report.Reports = append(report.Reports, spriteReport)
		report.Checked++
		report.Failures += spriteReport.Failures
		report.Warnings += spriteReport.Warnings
		if spriteReport.Failures == 0 {
			report.Succeeded++
		}
	}
	return report, nil
}

func (s *Service) CheckSprite(ctx context.Context, spriteName string) (SpriteReport, error) {
	if err := lib.ValidateSpriteName(spriteName); err != nil {
		return SpriteReport{}, err
	}

	report := SpriteReport{Sprite: spriteName}
	add := func(name string, status Status, msg string) {
		report.Checks = append(report.Checks, CheckResult{Name: name, Status: status, Message: msg})
		switch status {
		case StatusFail:
			report.Failures++
		case StatusWarn:
			report.Warnings++
		}
	}

	exists, err := s.Sprite.Exists(ctx, spriteName)
	if err != nil {
		return SpriteReport{}, err
	}
	if !exists {
		add("sprite_exists", StatusFail, fmt.Sprintf("Sprite %q does not exist on Fly.io", spriteName))
		return report, nil
	}
	add("sprite_exists", StatusPass, "Sprite exists")

	response, err := s.Sprite.Exec(ctx, spriteName, false, "echo", "alive")
	if err != nil {
		add("sprite_responsive", StatusFail, fmt.Sprintf("Sprite unreachable: %v", err))
		return report, nil
	}
	if strings.Contains(response.Stdout, "alive") {
		add("sprite_responsive", StatusPass, "Sprite responsive")
	} else {
		add("sprite_responsive", StatusFail, fmt.Sprintf("Sprite unreachable: %s%s", response.Stdout, response.Stderr))
		return report, nil
	}

	claude, err := s.Sprite.Exec(ctx, spriteName, false, "bash", "-c", "claude --version 2>/dev/null || echo MISSING")
	if err != nil {
		add("claude_installed", StatusFail, fmt.Sprintf("Claude Code check failed: %v", err))
	} else if strings.Contains(claude.Stdout, "MISSING") {
		add("claude_installed", StatusFail, "Claude Code not installed")
	} else {
		line := strings.TrimSpace(firstLine(claude.Stdout))
		add("claude_installed", StatusPass, fmt.Sprintf("Claude Code installed: %s", line))
	}

	gitCred, err := s.Sprite.Exec(ctx, spriteName, false, "bash", "-c", "git config --global credential.helper 2>/dev/null || echo MISSING")
	if err != nil {
		add("git_credential_helper", StatusFail, fmt.Sprintf("git credentials check failed: %v", err))
	} else if strings.Contains(gitCred.Stdout, "store") {
		add("git_credential_helper", StatusPass, "Git credential helper: store")
	} else {
		add("git_credential_helper", StatusFail, fmt.Sprintf("Git credentials NOT configured (credential.helper=%s)", strings.TrimSpace(gitCred.Stdout)))
	}

	gitCredFile, err := s.Sprite.Exec(ctx, spriteName, false, "bash", "-c", "test -s /home/sprite/.git-credentials && echo EXISTS || echo MISSING")
	if err != nil {
		add("git_credential_file", StatusFail, fmt.Sprintf("git credentials file check failed: %v", err))
	} else if strings.Contains(gitCredFile.Stdout, "EXISTS") {
		add("git_credential_file", StatusPass, "Git credentials file exists")
	} else {
		add("git_credential_file", StatusFail, "Git credentials file MISSING or empty (/home/sprite/.git-credentials)")
	}

	pushTestCmd := "cd /tmp && rm -rf preflight-test && mkdir preflight-test && cd preflight-test && git init -q && git remote add origin https://github.com/misty-step/cerberus.git && git ls-remote origin HEAD >/dev/null 2>&1 && echo PASS || echo FAIL"
	pushTest, err := s.Sprite.Exec(ctx, spriteName, false, "bash", "-c", pushTestCmd)
	if err != nil {
		add("git_remote_access", StatusFail, fmt.Sprintf("Git remote access check failed: %v", err))
	} else if strings.Contains(pushTest.Stdout, "PASS") {
		add("git_remote_access", StatusPass, "Git remote access verified")
	} else {
		add("git_remote_access", StatusFail, "Git remote access FAILED (cannot reach GitHub)")
	}

	hasClaudeMD, err := s.Sprite.Exec(ctx, spriteName, false, "bash", "-c", "test -f /home/sprite/workspace/CLAUDE.md && echo YES || echo NO")
	if err != nil {
		add("workspace_claude_md", StatusWarn, fmt.Sprintf("CLAUDE.md check failed: %v", err))
	} else if strings.Contains(hasClaudeMD.Stdout, "YES") {
		add("workspace_claude_md", StatusPass, "CLAUDE.md present")
	} else {
		add("workspace_claude_md", StatusWarn, "CLAUDE.md missing (sprite may lack instructions)")
	}

	diskAvail, err := s.Sprite.Exec(ctx, spriteName, false, "bash", "-c", "df -h /home/sprite | tail -1 | awk '{print $4}'")
	if err != nil {
		add("disk_space", StatusFail, fmt.Sprintf("Disk check failed: %v", err))
	} else {
		add("disk_space", StatusPass, fmt.Sprintf("Disk available: %s", strings.TrimSpace(diskAvail.Stdout)))
	}

	gitUser, err := s.Sprite.Exec(ctx, spriteName, false, "bash", "-c", "git config --global user.name 2>/dev/null || echo MISSING")
	if err != nil {
		add("git_user", StatusWarn, fmt.Sprintf("Git user check failed: %v", err))
	} else if strings.Contains(gitUser.Stdout, "MISSING") {
		add("git_user", StatusWarn, "Git user.name not configured")
	} else {
		add("git_user", StatusPass, fmt.Sprintf("Git user: %s", strings.TrimSpace(gitUser.Stdout)))
	}

	claudeCount, err := s.Sprite.Exec(ctx, spriteName, false, "bash", "-c", "pgrep -c claude 2>/dev/null || echo 0")
	if err != nil {
		add("stale_claude_processes", StatusWarn, fmt.Sprintf("Claude process check failed: %v", err))
	} else {
		count := parseCount(claudeCount.Stdout)
		if count > 0 {
			add("stale_claude_processes", StatusWarn, fmt.Sprintf("Claude already running (%d processes) — kill before redispatch", count))
		} else {
			add("stale_claude_processes", StatusPass, "No stale Claude processes")
		}
	}

	return report, nil
}

func parseCount(raw string) int {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return 0
	}
	count, err := strconv.Atoi(trimmed)
	if err != nil {
		return 0
	}
	return count
}

func firstLine(text string) string {
	if idx := strings.IndexByte(text, '\n'); idx >= 0 {
		return text[:idx]
	}
	return text
}
