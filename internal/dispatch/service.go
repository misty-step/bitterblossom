package dispatch

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/misty-step/bitterblossom/internal/lib"
)

const defaultMaxRalphIterations = 50

// Service handles sprite task dispatch and Ralph loop lifecycle.
type Service struct {
	Logger             *slog.Logger
	Sprite             *lib.SpriteCLI
	Paths              lib.Paths
	Workspace          string
	MaxRalphIterations int
}

type Status struct {
	RalphStatus string
	Signals     string
	RecentLog   string
	MemoryTail  string
}

func NewService(logger *slog.Logger, sprite *lib.SpriteCLI, paths lib.Paths, maxRalphIterations int) *Service {
	if maxRalphIterations <= 0 {
		maxRalphIterations = defaultMaxRalphIterations
	}
	return &Service{
		Logger:             logger,
		Sprite:             sprite,
		Paths:              paths,
		Workspace:          filepath.ToSlash(filepath.Join(lib.DefaultRemoteHome, "workspace")),
		MaxRalphIterations: maxRalphIterations,
	}
}

func (s *Service) GenerateRalphPrompt(task, repo, sprite string) (string, error) {
	templatePath := s.Paths.RalphTemplatePath()
	content, err := os.ReadFile(templatePath)
	if err != nil {
		return "", fmt.Errorf("read Ralph prompt template %s: %w", templatePath, err)
	}

	rendered := string(content)
	rendered = strings.ReplaceAll(rendered, "{{TASK_DESCRIPTION}}", task)
	if strings.TrimSpace(repo) == "" {
		repo = "OWNER/REPO"
	}
	rendered = strings.ReplaceAll(rendered, "{{REPO}}", repo)
	if strings.TrimSpace(sprite) == "" {
		sprite = "sprite"
	}
	rendered = strings.ReplaceAll(rendered, "{{SPRITE_NAME}}", sprite)
	return rendered, nil
}

func (s *Service) SetupRepo(ctx context.Context, spriteName, repo string) error {
	if err := lib.ValidateRepoRef(repo); err != nil {
		return err
	}
	repoDir := path.Base(repo)
	remoteCmd := fmt.Sprintf("cd %s && if [ -d '%s' ]; then cd '%s' && git fetch origin && git pull --ff-only; else gh repo clone '%s'; fi", s.Workspace, repoDir, repoDir, repo)
	_, err := s.Sprite.Exec(ctx, spriteName, true, "bash", "-c", remoteCmd)
	return err
}

func (s *Service) UploadPrompt(ctx context.Context, spriteName, prompt, remotePath string) error {
	tmp, err := os.CreateTemp("", "bb-dispatch-prompt-*.md")
	if err != nil {
		return fmt.Errorf("create temp prompt file: %w", err)
	}
	tmpPath := tmp.Name()
	defer func() {
		_ = os.Remove(tmpPath)
	}()
	if _, err := tmp.WriteString(prompt); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write temp prompt file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("flush temp prompt file: %w", err)
	}

	_, err = s.Sprite.ExecWithFile(ctx, spriteName, tmpPath, remotePath, true, "echo", "prompt uploaded")
	return err
}

func (s *Service) DispatchOneShot(ctx context.Context, spriteName, prompt, repo string) (string, error) {
	if err := lib.ValidateSpriteName(spriteName); err != nil {
		return "", err
	}
	if strings.TrimSpace(prompt) == "" {
		return "", &lib.ValidationError{Field: "prompt", Message: "is required"}
	}

	if strings.TrimSpace(repo) != "" {
		if err := s.SetupRepo(ctx, spriteName, repo); err != nil {
			return "", err
		}
	}

	remotePrompt := filepath.ToSlash(filepath.Join(s.Workspace, ".dispatch-prompt.md"))
	if err := s.UploadPrompt(ctx, spriteName, prompt, remotePrompt); err != nil {
		return "", err
	}

	remoteCmd := fmt.Sprintf("cd %s && cat .dispatch-prompt.md | claude -p --permission-mode bypassPermissions 2>&1 | grep -v '^\\$'; rm -f .dispatch-prompt.md", s.Workspace)
	result, err := s.Sprite.Exec(ctx, spriteName, true, "bash", "-c", remoteCmd)
	if err != nil {
		return "", err
	}
	return result.Stdout, nil
}

func (s *Service) StartRalph(ctx context.Context, spriteName, prompt, repo string) error {
	if err := lib.ValidateSpriteName(spriteName); err != nil {
		return err
	}
	if strings.TrimSpace(prompt) == "" {
		return &lib.ValidationError{Field: "prompt", Message: "is required"}
	}

	renderedPrompt, err := s.GenerateRalphPrompt(prompt, repo, spriteName)
	if err != nil {
		return err
	}

	remotePrompt := filepath.ToSlash(filepath.Join(s.Workspace, "PROMPT.md"))
	if err := s.UploadPrompt(ctx, spriteName, renderedPrompt, remotePrompt); err != nil {
		return err
	}

	if strings.TrimSpace(repo) != "" {
		if err := s.SetupRepo(ctx, spriteName, repo); err != nil {
			return err
		}
	}

	ralphScript, err := s.buildRalphLoopScript()
	if err != nil {
		return err
	}
	tmpScript, err := os.CreateTemp("", "bb-ralph-loop-*.sh")
	if err != nil {
		return fmt.Errorf("create temp Ralph script: %w", err)
	}
	tmpPath := tmpScript.Name()
	defer func() {
		_ = os.Remove(tmpPath)
	}()
	if _, err := tmpScript.WriteString(ralphScript); err != nil {
		_ = tmpScript.Close()
		return fmt.Errorf("write temp Ralph script: %w", err)
	}
	if err := tmpScript.Close(); err != nil {
		return fmt.Errorf("flush temp Ralph script: %w", err)
	}

	remoteScript := filepath.ToSlash(filepath.Join(s.Workspace, "ralph-loop.sh"))
	if _, err := s.Sprite.ExecWithFile(ctx, spriteName, tmpPath, remoteScript, true, "chmod", "+x", remoteScript); err != nil {
		return err
	}

	startCmd := fmt.Sprintf("cd %s && nohup bash ralph-loop.sh > /dev/null 2>&1 & echo $! > ralph.pid && echo \"PID: $(cat ralph.pid)\"", s.Workspace)
	_, err = s.Sprite.Exec(ctx, spriteName, true, "bash", "-c", startCmd)
	return err
}

func (s *Service) buildRalphLoopScript() (string, error) {
	if s.MaxRalphIterations <= 0 {
		return "", &lib.ValidationError{Field: "max-ralph-iterations", Message: "must be > 0"}
	}
	script := fmt.Sprintf(`#!/bin/bash
set -uo pipefail

WORKSPACE=%q
LOG="$WORKSPACE/ralph.log"
ITERATION=0
MAX_ITERATIONS=%d

echo "[ralph] Starting loop at $(date -u +%%Y-%%m-%%dT%%H:%%M:%%SZ) (max $MAX_ITERATIONS iterations)" | tee -a "$LOG"

while true; do
    ITERATION=$((ITERATION + 1))
    echo "" >> "$LOG"
    echo "[ralph] === Iteration $ITERATION / $MAX_ITERATIONS at $(date -u +%%Y-%%m-%%dT%%H:%%M:%%SZ) ===" | tee -a "$LOG"

    if [ "$ITERATION" -gt "$MAX_ITERATIONS" ]; then
        echo "[ralph] Hit max iterations ($MAX_ITERATIONS). Stopping." | tee -a "$LOG"
        break
    fi

    if [ -f "$WORKSPACE/TASK_COMPLETE" ]; then
        echo "[ralph] Task marked complete. Stopping." | tee -a "$LOG"
        break
    fi

    if [ -f "$WORKSPACE/BLOCKED.md" ]; then
        echo "[ralph] Task blocked. See BLOCKED.md. Stopping." | tee -a "$LOG"
        break
    fi

    cd "$WORKSPACE"
    cat PROMPT.md | claude -p --permission-mode bypassPermissions >> "$LOG" 2>&1

    EXIT_CODE=$?
    echo "[ralph] Claude exited with code $EXIT_CODE at $(date -u +%%Y-%%m-%%dT%%H:%%M:%%SZ)" | tee -a "$LOG"
    echo "[ralph] heartbeat: iteration=$ITERATION exit=$EXIT_CODE ts=$(date -u +%%Y-%%m-%%dT%%H:%%M:%%SZ)" >> "$LOG"

    sleep 5
done

echo "[ralph] Loop ended after $ITERATION iterations at $(date -u +%%Y-%%m-%%dT%%H:%%M:%%SZ)" | tee -a "$LOG"
`, s.Workspace, s.MaxRalphIterations)
	return script, nil
}

func (s *Service) StopRalph(ctx context.Context, spriteName string) error {
	if err := lib.ValidateSpriteName(spriteName); err != nil {
		return err
	}
	stopCmd := fmt.Sprintf("if [ -f %s/ralph.pid ]; then PID=$(cat %s/ralph.pid); kill $PID 2>/dev/null || true; pkill -f ralph-loop.sh 2>/dev/null || true; pkill -f 'claude -p' 2>/dev/null || true; rm -f %s/ralph.pid; echo 'Ralph loop stopped'; else echo 'No Ralph loop running (no PID file)'; fi", s.Workspace, s.Workspace, s.Workspace)
	_, err := s.Sprite.Exec(ctx, spriteName, true, "bash", "-c", stopCmd)
	return err
}

func (s *Service) CheckStatus(ctx context.Context, spriteName string) (Status, error) {
	if err := lib.ValidateSpriteName(spriteName); err != nil {
		return Status{}, err
	}

	ralphCmd := fmt.Sprintf("if [ -f %s/ralph.pid ] && kill -0 $(cat %s/ralph.pid) 2>/dev/null; then echo 'RUNNING (PID '$(cat %s/ralph.pid)')'; else echo 'NOT RUNNING'; fi", s.Workspace, s.Workspace, s.Workspace)
	ralph, err := s.Sprite.Exec(ctx, spriteName, false, "bash", "-c", ralphCmd)
	if err != nil {
		return Status{}, err
	}

	signalsCmd := fmt.Sprintf("[ -f %[1]s/TASK_COMPLETE ] && echo 'STATUS: TASK COMPLETE' && cat %[1]s/TASK_COMPLETE; [ -f %[1]s/BLOCKED.md ] && echo 'STATUS: BLOCKED' && cat %[1]s/BLOCKED.md; [ ! -f %[1]s/TASK_COMPLETE ] && [ ! -f %[1]s/BLOCKED.md ] && echo 'STATUS: Working'", s.Workspace)
	signals, err := s.Sprite.Exec(ctx, spriteName, false, "bash", "-c", signalsCmd)
	if err != nil {
		return Status{}, err
	}

	recentLog, _ := s.Sprite.Exec(ctx, spriteName, false, "tail", "-20", filepath.ToSlash(filepath.Join(s.Workspace, "ralph.log")))
	memoryTail, _ := s.Sprite.Exec(ctx, spriteName, false, "tail", "-10", filepath.ToSlash(filepath.Join(s.Workspace, "MEMORY.md")))

	return Status{
		RalphStatus: strings.TrimSpace(ralph.Stdout),
		Signals:     strings.TrimSpace(signals.Stdout),
		RecentLog:   strings.TrimSpace(recentLog.Stdout),
		MemoryTail:  strings.TrimSpace(memoryTail.Stdout),
	}, nil
}
