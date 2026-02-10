package main

import (
	"context"
	"strings"
	"testing"
)

type mockSpriteCLIRemote struct {
	execFn func(ctx context.Context, sprite, command string, stdin []byte) (string, error)
}

func (m *mockSpriteCLIRemote) List(ctx context.Context) ([]string, error) {
	return nil, nil
}

func (m *mockSpriteCLIRemote) Exec(ctx context.Context, sprite, command string, stdin []byte) (string, error) {
	if m.execFn != nil {
		return m.execFn(ctx, sprite, command, stdin)
	}
	return "", nil
}

func (m *mockSpriteCLIRemote) Upload(ctx context.Context, sprite, remotePath string, content []byte) error {
	return nil
}

func TestFetchGitActivity(t *testing.T) {
	tests := []struct {
		name        string
		sprite      string
		commitCount int
		execFn      func(ctx context.Context, sprite, command string, stdin []byte) (string, error)
		wantBranch  string
		wantCommits int
		wantError   bool
	}{
		{
			name:        "successful fetch with commits",
			sprite:      "test-sprite",
			commitCount: 5,
			execFn: func(ctx context.Context, sprite, command string, stdin []byte) (string, error) {
				switch {
				case strings.Contains(command, "rev-parse --abbrev-ref HEAD"):
					return "main\n", nil
				case strings.Contains(command, "rev-parse --abbrev-ref --symbolic-full-name"):
					return "origin/main\n", nil
				case strings.Contains(command, "rev-list --left-right --count"):
					return "2\t1\n", nil
				case strings.Contains(command, "log"):
					return "abc123\nfeat: add feature\n\ndef456\nfix: bug fix\n\n", nil
				case strings.Contains(command, "diff --cached --name-only"):
					return "file1.go\nfile2.go\n", nil
				case strings.Contains(command, "diff --name-only") && !strings.Contains(command, "--cached"):
					return "file3.go\n", nil
				case strings.Contains(command, "ls-files --others --exclude-standard"):
					return "file4.go\n", nil
				default:
					return "", nil
				}
			},
			wantBranch:  "main",
			wantCommits: 2,
			wantError:   false,
		},
		{
			name:        "sprite with no remote tracking",
			sprite:      "test-sprite",
			commitCount: 5,
			execFn: func(ctx context.Context, sprite, command string, stdin []byte) (string, error) {
				switch {
				case strings.Contains(command, "rev-parse --abbrev-ref HEAD"):
					return "feature-branch\n", nil
				case strings.Contains(command, "rev-parse --abbrev-ref --symbolic-full-name"):
					return "\n", nil
				case strings.Contains(command, "log"):
					return "xyz789\nrefactor: cleanup\n\n", nil
				default:
					return "", nil
				}
			},
			wantBranch:  "feature-branch",
			wantCommits: 1,
			wantError:   false,
		},
		{
			name:        "clean working tree",
			sprite:      "test-sprite",
			commitCount: 3,
			execFn: func(ctx context.Context, sprite, command string, stdin []byte) (string, error) {
				switch {
				case strings.Contains(command, "rev-parse --abbrev-ref HEAD"):
					return "main\n", nil
				case strings.Contains(command, "log"):
					return "abc123\ninitial commit\n\n", nil
				default:
					return "", nil
				}
			},
			wantBranch:  "main",
			wantCommits: 1,
			wantError:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &mockSpriteCLIRemote{execFn: tt.execFn}
			ctx := context.Background()

			activity, err := fetchGitActivity(ctx, mock, tt.sprite, tt.commitCount)

			if tt.wantError && err == nil {
				t.Errorf("expected error but got none")
			}
			if !tt.wantError && err != nil {
				t.Errorf("unexpected error: %v", err)
			}

			if activity.Sprite != tt.sprite {
				t.Errorf("sprite = %q, want %q", activity.Sprite, tt.sprite)
			}

			if activity.Branch != tt.wantBranch {
				t.Errorf("branch = %q, want %q", activity.Branch, tt.wantBranch)
			}

			if len(activity.Commits) != tt.wantCommits {
				t.Errorf("commits count = %d, want %d", len(activity.Commits), tt.wantCommits)
			}
		})
	}
}

func TestParseFileList(t *testing.T) {
	tests := []struct {
		name   string
		output string
		want   []string
	}{
		{
			name:   "multiple files",
			output: "file1.go\nfile2.go\nfile3.go\n",
			want:   []string{"file1.go", "file2.go", "file3.go"},
		},
		{
			name:   "empty output",
			output: "",
			want:   []string{},
		},
		{
			name:   "output with whitespace",
			output: "  file1.go  \n\n  file2.go  \n",
			want:   []string{"file1.go", "file2.go"},
		},
		{
			name:   "single file",
			output: "main.go\n",
			want:   []string{"main.go"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseFileList(tt.output)
			if len(got) != len(tt.want) {
				t.Errorf("parseFileList() len = %d, want %d", len(got), len(tt.want))
				return
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("parseFileList()[%d] = %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestActivityCommand(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		wantErr    bool
		wantErrMsg string
	}{
		{
			name:    "valid sprite name",
			args:    []string{"test-sprite"},
			wantErr: false,
		},
		{
			name:       "no sprite name",
			args:       []string{},
			wantErr:    true,
			wantErrMsg: "accepts 1 arg(s), received 0",
		},
		{
			name:       "too many arguments",
			args:       []string{"sprite1", "sprite2"},
			wantErr:    true,
			wantErrMsg: "accepts 1 arg(s), received 2",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deps := activityDeps{
				newRemote: func(binary, org string) spriteRemote {
					return &mockSpriteCLIRemote{
						execFn: func(ctx context.Context, sprite, command string, stdin []byte) (string, error) {
							return "main\n", nil
						},
					}
				},
			}

			cmd := newActivityCmdWithDeps(deps)
			cmd.SetArgs(tt.args)

			err := cmd.Execute()
			if (err != nil) != tt.wantErr {
				t.Errorf("Execute() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.wantErr && tt.wantErrMsg != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErrMsg) {
					t.Errorf("Execute() error = %v, want error containing %q", err, tt.wantErrMsg)
				}
			}
		})
	}
}

func TestWriteActivityText(t *testing.T) {
	tests := []struct {
		name     string
		activity gitActivity
		want     []string // substrings that should be present in output
	}{
		{
			name: "complete activity",
			activity: gitActivity{
				Sprite:       "test-sprite",
				Branch:       "main",
				RemoteBranch: "origin/main",
				Ahead:        2,
				Behind:       1,
				Commits: []commit{
					{Hash: "abc123", Message: "feat: add feature"},
					{Hash: "def456", Message: "fix: bug fix"},
				},
				Staged:    []string{"file1.go", "file2.go"},
				Unstaged:  []string{"file3.go"},
				Untracked: []string{"file4.go"},
			},
			want: []string{
				"Git Activity: test-sprite",
				"Branch: main",
				"Tracking: origin/main",
				"2 ahead, 1 behind",
				"Recent Commits (2)",
				"abc123",
				"feat: add feature",
				"def456",
				"fix: bug fix",
				"File Changes (4)",
				"Staged (2)",
				"file1.go",
				"file2.go",
				"Unstaged (1)",
				"file3.go",
				"Untracked (1)",
				"file4.go",
			},
		},
		{
			name: "clean working tree",
			activity: gitActivity{
				Sprite: "test-sprite",
				Branch: "main",
				Commits: []commit{
					{Hash: "abc123", Message: "initial commit"},
				},
			},
			want: []string{
				"Git Activity: test-sprite",
				"Branch: main",
				"Working tree clean",
			},
		},
		{
			name: "error state",
			activity: gitActivity{
				Sprite: "test-sprite",
				Error:  "failed to connect",
			},
			want: []string{
				"Error: failed to connect",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf strings.Builder
			err := writeActivityText(&buf, tt.activity)
			if err != nil {
				t.Fatalf("writeActivityText() error = %v", err)
			}

			output := buf.String()
			for _, substr := range tt.want {
				if !strings.Contains(output, substr) {
					t.Errorf("output missing substring %q\noutput:\n%s", substr, output)
				}
			}
		})
	}
}
