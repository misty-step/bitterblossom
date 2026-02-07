package dispatch

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/misty-step/bitterblossom/internal/contracts"
)

const workspace = "/home/sprite/workspace"

var (
	spriteNamePattern = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)
	repoPartPattern   = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)
)

// Executor runs remote commands on sprites.
type Executor interface {
	Exec(ctx context.Context, sprite, remoteCommand string, stdin []byte) (string, error)
}

// DispatchRequest contains the fields needed to launch an issue task.
type DispatchRequest struct {
	Sprite      string
	Repo        string
	IssueNumber int
	PersonaRole string
}

// Dispatcher fans work out to a sprite.
type Dispatcher interface {
	RunIssueTask(ctx context.Context, req DispatchRequest) (contracts.DispatchResult, error)
}

// Service dispatches issue-driven tasks to sprites.
type Service struct {
	exec Executor
	now  func() time.Time
}

// NewService constructs a dispatch service.
func NewService(exec Executor) *Service {
	if exec == nil {
		panic("dispatch.NewService: exec cannot be nil")
	}
	return &Service{
		exec: exec,
		now:  time.Now,
	}
}

// RunIssueTask writes TASK.md, launches Claude, and records STATUS.json.
func (s *Service) RunIssueTask(ctx context.Context, req DispatchRequest) (contracts.DispatchResult, error) {
	sprite := strings.TrimSpace(req.Sprite)
	if !spriteNamePattern.MatchString(sprite) {
		return contracts.DispatchResult{}, fmt.Errorf("invalid sprite name %q", req.Sprite)
	}

	owner, repoName, err := parseRepo(req.Repo)
	if err != nil {
		return contracts.DispatchResult{}, err
	}
	if req.IssueNumber <= 0 {
		return contracts.DispatchResult{}, fmt.Errorf("issue number must be greater than zero")
	}

	personaRole := strings.TrimSpace(req.PersonaRole)
	if personaRole == "" {
		personaRole = "sprite"
	}

	repoSlug := owner + "/" + repoName
	prompt := buildPrompt(sprite, personaRole, repoSlug, req.IssueNumber)
	if _, err := s.exec.Exec(ctx, sprite, "cat > "+workspace+"/TASK.md", []byte(prompt)); err != nil {
		return contracts.DispatchResult{}, fmt.Errorf("uploading task prompt: %w", err)
	}

	logFile := fmt.Sprintf("%s-%s-%d.log", sprite, repoName, req.IssueNumber)
	launchScript := buildLaunchScript(repoSlug, repoName, req.IssueNumber, logFile)
	pidOutput, err := s.exec.Exec(ctx, sprite, launchScript, nil)
	if err != nil {
		return contracts.DispatchResult{}, fmt.Errorf("starting task agent: %w", err)
	}

	pid, err := parsePID(pidOutput)
	if err != nil {
		return contracts.DispatchResult{}, fmt.Errorf("parsing task pid: %w", err)
	}

	return contracts.DispatchResult{
		Sprite:    sprite,
		Task:      fmt.Sprintf("%s#%d", repoSlug, req.IssueNumber),
		StartedAt: s.now().UTC(),
		PID:       pid,
		LogPath:   workspace + "/" + logFile,
	}, nil
}

func parseRepo(input string) (string, string, error) {
	repo := strings.TrimSpace(input)
	if repo == "" {
		return "", "", fmt.Errorf("repo is required")
	}
	if strings.Contains(repo, "://") {
		return "", "", fmt.Errorf("repo %q must be in owner/repo form", repo)
	}

	owner := "misty-step"
	repoName := repo
	if strings.Contains(repo, "/") {
		parts := strings.Split(repo, "/")
		if len(parts) != 2 {
			return "", "", fmt.Errorf("repo %q must be in owner/repo form", repo)
		}
		owner = strings.TrimSpace(parts[0])
		repoName = strings.TrimSpace(parts[1])
	}

	if !repoPartPattern.MatchString(owner) || !repoPartPattern.MatchString(repoName) {
		return "", "", fmt.Errorf("repo %q contains invalid characters", repo)
	}
	return owner, repoName, nil
}

func parsePID(output string) (int, error) {
	last := ""
	for _, line := range strings.Split(output, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		last = trimmed
	}
	if last == "" {
		return 0, fmt.Errorf("pid output was empty")
	}
	pid, err := strconv.Atoi(last)
	if err != nil {
		return 0, fmt.Errorf("invalid pid %q", last)
	}
	return pid, nil
}

func buildPrompt(sprite, personaRole, repoSlug string, issue int) string {
	return fmt.Sprintf(`You are %s, a %s sprite in the Fae Court.

Read your persona at /home/sprite/workspace/PERSONA.md for your working philosophy and approach.

## Your Assignment

GitHub issue #%d in %s:

`+"```"+`
gh issue view %d --repo %s
`+"```"+`

## Execution Protocol

1. Read the issue. Specs and acceptance criteria are in the issue.
2. Clone the repo if needed: git clone https://github.com/%s.git
3. Create a branch with a descriptive name based on the issue.
4. Implement the solution following CLAUDE.md guidance.
5. Write tests for edge cases and error paths.
6. Open a PR referencing the issue with clear rationale.
`, sprite, personaRole, issue, repoSlug, issue, repoSlug, repoSlug)
}

func buildLaunchScript(repoSlug, repoName string, issue int, logFile string) string {
	return strings.Join([]string{
		"set -euo pipefail",
		"cd " + shellQuote(workspace),
		"if [ -d " + shellQuote(repoName) + " ]; then",
		"  cd " + shellQuote(repoName),
		"  git pull --ff-only >/dev/null 2>&1 || true",
		"else",
		"  git clone " + shellQuote("https://github.com/"+repoSlug+".git") + " " + shellQuote(repoName) + " >/dev/null 2>&1 || true",
		"  cd " + shellQuote(repoName),
		"fi",
		"cat " + shellQuote(workspace+"/TASK.md") + " | claude -p --permission-mode bypassPermissions > " + shellQuote(workspace+"/"+logFile) + " 2>&1 &",
		"AGENT_PID=$!",
		"echo \"$AGENT_PID\" > " + shellQuote(workspace+"/AGENT_PID"),
		"printf '{\"repo\":\"%s\",\"issue\":%d,\"started\":\"%s\"}\\n' " + shellQuote(repoSlug) + " " + strconv.Itoa(issue) + " \"$(date -u +%Y-%m-%dT%H:%M:%SZ)\" > " + shellQuote(workspace+"/STATUS.json"),
		"echo \"$AGENT_PID\"",
	}, "\n")
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", `'"'"'`) + "'"
}
