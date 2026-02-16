package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	sprites "github.com/superfly/sprites-go"

	"github.com/spf13/cobra"
)

func newDispatchCmd() *cobra.Command {
	var (
		repo          string
		timeout       time.Duration
		maxIterations int
		harness       string
		model         string
	)

	cmd := &cobra.Command{
		Use:   "dispatch <sprite> <prompt>",
		Short: "Dispatch a task to a sprite via the ralph loop",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			spriteName := args[0]
			prompt := args[1]

			if harness != "claude" && harness != "opencode" {
				return fmt.Errorf("--harness must be 'claude' or 'opencode', got %q", harness)
			}

			return runDispatch(cmd.Context(), spriteName, prompt, repo, maxIterations, timeout, harness, model)
		},
	}

	cmd.Flags().StringVar(&repo, "repo", "", "GitHub repo (owner/repo)")
	cmd.Flags().DurationVar(&timeout, "timeout", 30*time.Minute, "Max wall-clock time for the ralph loop")
	cmd.Flags().IntVar(&maxIterations, "max-iterations", 50, "Max ralph loop iterations")
	cmd.Flags().StringVar(&harness, "harness", "claude", "Agent harness: claude or opencode")
	cmd.Flags().StringVar(&model, "model", "", "Model for opencode harness (e.g. moonshotai/kimi-k2.5)")
	_ = cmd.MarkFlagRequired("repo")

	return cmd
}

func runDispatch(ctx context.Context, spriteName, prompt, repo string, maxIter int, timeout time.Duration, harness, model string) error {
	// Validate credentials
	token, err := spriteToken()
	if err != nil {
		return err
	}
	ghToken, err := requireEnv("GITHUB_TOKEN")
	if err != nil {
		return err
	}

	// LLM auth is handled by settings.json on the sprite (baked in during setup).
	// Dispatch only validates that GITHUB_TOKEN is set for git operations.

	client := sprites.New(token)
	defer func() { _ = client.Close() }()
	s := client.Sprite(spriteName)

	// 1. Probe connectivity (15s)
	_, _ = fmt.Fprintf(os.Stderr, "probing %s...\n", spriteName)
	probeCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	if _, err := s.CommandContext(probeCtx, "echo", "ok").Output(); err != nil {
		return fmt.Errorf("sprite %q unreachable: %w", spriteName, err)
	}

	// 2. Check that setup was run (ralph.sh must exist)
	ralphScript := "/home/sprite/workspace/.ralph.sh"
	checkCtx, checkCancel := context.WithTimeout(ctx, 10*time.Second)
	defer checkCancel()
	if _, err := s.CommandContext(checkCtx, "test", "-f", ralphScript).Output(); err != nil {
		return fmt.Errorf("sprite %q not configured — run: bb setup %s --repo %s", spriteName, spriteName, repo)
	}

	// 3. Kill stale agent processes from prior dispatches
	// Without this, concurrent claude processes compete for resources and hang.
	killCtx, killCancel := context.WithTimeout(ctx, 10*time.Second)
	defer killCancel()
	_, _ = s.CommandContext(killCtx, "bash", "-c", "pkill -9 -f 'ralph\\.sh|claude|opencode' 2>/dev/null; sleep 1").Output()

	// 4. Repo sync (pull latest on default branch)
	repoName := path.Base(repo)
	workspace := "/home/sprite/workspace/" + repoName

	_, _ = fmt.Fprintf(os.Stderr, "syncing repo %s...\n", repo)
	syncScript := fmt.Sprintf(
		`git config --global --add safe.directory %s 2>/dev/null; export GH_TOKEN=%q && cd %s && git checkout master 2>/dev/null || git checkout main 2>/dev/null; git pull --ff-only 2>&1`,
		workspace, ghToken, workspace,
	)
	syncCmd := s.CommandContext(ctx, "bash", "-c", syncScript)
	if out, err := syncCmd.Output(); err != nil {
		return fmt.Errorf("repo sync failed: %w\n%s", err, out)
	}

	// 5. Clean stale signals
	cleanScript := fmt.Sprintf(
		"rm -f %s/TASK_COMPLETE %s/TASK_COMPLETE.md %s/BLOCKED.md",
		workspace, workspace, workspace,
	)
	_, _ = s.CommandContext(ctx, "bash", "-c", cleanScript).Output()

	// 6. Render and upload prompt
	rendered, err := renderPrompt(prompt, repo, spriteName)
	if err != nil {
		return fmt.Errorf("render prompt: %w", err)
	}

	promptPath := workspace + "/.dispatch-prompt.md"
	if err := s.Filesystem().WriteFileContext(ctx, promptPath, []byte(rendered), 0644); err != nil {
		return fmt.Errorf("upload prompt: %w", err)
	}

	// 7. Run ralph loop — foreground, streaming
	_, _ = fmt.Fprintf(os.Stderr, "starting ralph loop (max %d iterations, %s timeout, harness=%s)...\n", maxIter, timeout, harness)

	// Only pass operational env vars — LLM auth comes from settings.json (claude) or env var (opencode).
	totalSec := int(timeout.Seconds())
	iterSec := 900 // default per-iteration timeout
	if totalSec < iterSec {
		iterSec = totalSec // cap per-iteration at total timeout (#389)
	}
	ralphEnv := fmt.Sprintf(
		`export MAX_ITERATIONS=%d MAX_TIME_SEC=%d ITER_TIMEOUT_SEC=%d WORKSPACE=%q GH_TOKEN=%q AGENT_HARNESS=%q AGENT_MODEL=%q LEFTHOOK=0`,
		maxIter, totalSec, iterSec, workspace, ghToken, harness, model,
	)

	// OpenCode needs OPENROUTER_API_KEY in env; Claude Code uses settings.json on sprite.
	if harness == "opencode" {
		orKey := os.Getenv("OPENROUTER_API_KEY")
		if orKey == "" {
			return fmt.Errorf("OPENROUTER_API_KEY must be set for opencode harness")
		}
		ralphEnv += fmt.Sprintf(` OPENROUTER_API_KEY=%q`, orKey)
	}

	ralphEnv += fmt.Sprintf(` && exec bash %s`, ralphScript)

	ralphCtx, ralphCancel := context.WithTimeout(ctx, timeout+5*time.Minute) // grace period beyond ralph's own timeout
	defer ralphCancel()

	ralphCmd := s.CommandContext(ralphCtx, "bash", "-c", ralphEnv)
	ralphCmd.Dir = workspace
	ralphCmd.SetTTY(true)

	monitor := newDispatchOutputMonitor(os.Stderr, 45*time.Second)
	defer monitor.stop()
	monitor.start()

	stdout := monitor.wrap(os.Stdout)
	stderr := monitor.wrap(os.Stderr)
	ralphCmd.Stdout = stdout
	ralphCmd.Stderr = stderr
	ralphCmd.TextMessageHandler = newDispatchTextMessageHandler(stdout, stderr)

	ralphErr := ralphCmd.Run()

	// 8. Verify work produced
	_, _ = fmt.Fprintf(os.Stderr, "\n=== work produced ===\n")
	verifyScript := fmt.Sprintf(
		`cd %s && echo "--- commits ---" && git log --oneline origin/master..HEAD 2>/dev/null || git log --oneline origin/main..HEAD 2>/dev/null; echo "--- PRs ---" && gh pr list --json url,title 2>/dev/null || echo "(gh not available)"`,
		workspace,
	)
	verifyScript = fmt.Sprintf(`export GH_TOKEN=%q && %s`, ghToken, verifyScript)
	verifyCmd := s.CommandContext(ctx, "bash", "-c", verifyScript)
	verifyCmd.Stdout = os.Stderr
	verifyCmd.Stderr = os.Stderr
	_ = verifyCmd.Run()

	// 9. Return appropriate exit code
	if ralphErr != nil {
		if exitErr, ok := ralphErr.(*sprites.ExitError); ok {
			code := exitErr.ExitCode()
			switch code {
			case 0:
				return nil
			case 2:
				return &exitError{Code: 2, Err: fmt.Errorf("agent blocked — check BLOCKED.md on sprite")}
			default:
				return &exitError{Code: code, Err: fmt.Errorf("ralph exited %d", code)}
			}
		}
		return fmt.Errorf("ralph failed: %w", ralphErr)
	}
	return nil
}

// renderPrompt reads the local ralph-prompt-template.md and substitutes placeholders.
func renderPrompt(taskDescription, repo, spriteName string) (string, error) {
	tmpl, err := os.ReadFile("scripts/ralph-prompt-template.md")
	if err != nil {
		return "", fmt.Errorf("read template: %w (are you running from the repo root?)", err)
	}

	rendered := string(tmpl)
	rendered = strings.ReplaceAll(rendered, "{{TASK_DESCRIPTION}}", taskDescription)
	rendered = strings.ReplaceAll(rendered, "{{REPO}}", repo)
	rendered = strings.ReplaceAll(rendered, "{{SPRITE_NAME}}", spriteName)

	return rendered, nil
}

type activityWriter struct {
	out  io.Writer
	mark func()
}

func (w *activityWriter) Write(p []byte) (int, error) {
	n, err := w.out.Write(p)
	if n > 0 && w.mark != nil {
		w.mark()
	}
	return n, err
}

type dispatchOutputMonitor struct {
	out             io.Writer
	silentThreshold time.Duration

	lastActivityUnixNano atomic.Int64
	stopCh               chan struct{}
	stopOnce             sync.Once
}

func newDispatchOutputMonitor(out io.Writer, silentThreshold time.Duration) *dispatchOutputMonitor {
	if silentThreshold <= 0 {
		silentThreshold = 45 * time.Second
	}

	m := &dispatchOutputMonitor{
		out:             out,
		silentThreshold: silentThreshold,
		stopCh:          make(chan struct{}),
	}
	m.lastActivityUnixNano.Store(time.Now().UnixNano())
	return m
}

func (m *dispatchOutputMonitor) wrap(out io.Writer) io.Writer {
	return &activityWriter{
		out:  out,
		mark: m.markActivity,
	}
}

func (m *dispatchOutputMonitor) markActivity() {
	m.lastActivityUnixNano.Store(time.Now().UnixNano())
}

func (m *dispatchOutputMonitor) start() {
	go m.loop()
}

func (m *dispatchOutputMonitor) stop() {
	m.stopOnce.Do(func() {
		close(m.stopCh)
	})
}

func (m *dispatchOutputMonitor) loop() {
	ticker := time.NewTicker(m.silentThreshold)
	defer ticker.Stop()

	for {
		select {
		case <-m.stopCh:
			return
		case <-ticker.C:
			last := time.Unix(0, m.lastActivityUnixNano.Load())
			silentFor := time.Since(last)
			if silentFor < m.silentThreshold {
				continue
			}
			_, _ = fmt.Fprintf(m.out, "[dispatch] no remote output for %s; still running...\n", silentFor.Round(time.Second))
		}
	}
}

func newDispatchTextMessageHandler(stdout, stderr io.Writer) func([]byte) {
	return func(data []byte) {
		if len(data) == 0 || bytes.HasPrefix(data, []byte("control:")) {
			return
		}

		trim := bytes.TrimSpace(data)
		if len(trim) == 0 {
			return
		}

		if trim[0] != '{' {
			_, _ = stdout.Write(data)
			if data[len(data)-1] != '\n' {
				_, _ = stdout.Write([]byte{'\n'})
			}
			return
		}

		var msg struct {
			Type  string `json:"type"`
			Data  string `json:"data,omitempty"`
			Error string `json:"error,omitempty"`
		}

		if err := json.Unmarshal(data, &msg); err != nil {
			_, _ = stdout.Write(data)
			if data[len(data)-1] != '\n' {
				_, _ = stdout.Write([]byte{'\n'})
			}
			return
		}

		switch msg.Type {
		case "stdout", "info":
			if msg.Data != "" {
				_, _ = io.WriteString(stdout, msg.Data)
			}
		case "stderr":
			if msg.Data != "" {
				_, _ = io.WriteString(stderr, msg.Data)
			}
		case "error":
			payload := msg.Error
			if payload == "" {
				payload = msg.Data
			}
			if payload != "" {
				_, _ = io.WriteString(stderr, payload)
			}
		}
	}
}
