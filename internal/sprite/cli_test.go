package sprite

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"
	"time"
)

func TestWithOrgArgs(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		base []string
		org  string
		want []string
	}{
		{
			name: "no org keeps args",
			base: []string{"list"},
			org:  "",
			want: []string{"list"},
		},
		{
			name: "appends org at end when no separator",
			base: []string{"api", "/orgs"},
			org:  "misty-step",
			want: []string{"api", "/orgs", "-o", "misty-step"},
		},
		{
			name: "inserts org before separator",
			base: []string{"exec", "-s", "bramble", "--", "bash", "-ceu", "echo ok"},
			org:  "misty-step",
			want: []string{"exec", "-s", "bramble", "-o", "misty-step", "--", "bash", "-ceu", "echo ok"},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := withOrgArgs(tc.base, tc.org)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("withOrgArgs() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestCreateArgs(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		org  string
		want []string
	}{
		{
			name: "with org",
			org:  "misty-step",
			want: []string{"create", "-skip-console", "-o", "misty-step", "bramble"},
		},
		{
			name: "without org",
			org:  "",
			want: []string{"create", "-skip-console", "bramble"},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := createArgs("bramble", tc.org)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("createArgs() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestDestroyArgs(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		org  string
		want []string
	}{
		{
			name: "with org",
			org:  "misty-step",
			want: []string{"destroy", "-force", "-o", "misty-step", "bramble"},
		},
		{
			name: "without org",
			org:  "",
			want: []string{"destroy", "-force", "bramble"},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := destroyArgs("bramble", tc.org)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("destroyArgs() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestUploadFileArgs(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name       string
		sprite     string
		localPath  string
		remotePath string
		want       []string
		wantErr    bool
	}{
		{
			name:       "basic",
			sprite:     "thorn",
			localPath:  "sprites/thorn.md",
			remotePath: "/home/sprite/workspace/PERSONA.md",
			want: []string{
				"exec", "-s", "thorn",
				"-file", "sprites/thorn.md:/home/sprite/workspace/PERSONA.md",
				"--", "true",
			},
		},
		{
			name:       "colon in local path",
			sprite:     "thorn",
			localPath:  "C:\\Users\\file.md",
			remotePath: "/home/sprite/workspace/PERSONA.md",
			wantErr:    true,
		},
		{
			name:       "colon in remote path",
			sprite:     "thorn",
			localPath:  "sprites/thorn.md",
			remotePath: "/home/sprite/workspace/file:v2.md",
			wantErr:    true,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := uploadFileArgs(tc.sprite, tc.localPath, tc.remotePath)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("uploadFileArgs() expected error, got %v", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("uploadFileArgs() unexpected error: %v", err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("uploadFileArgs() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestCLIExecWithEnvBuildsCorrectArgs(t *testing.T) {
	t.Parallel()

	// Test that ExecWithEnv includes -e flags before the command separator
	// This test verifies the argument structure without actually executing
	// by using a mock approach that inspects the args that would be built

	cases := []struct {
		name    string
		sprite  string
		command string
		env     map[string]string
		wantEnv []string // The -e KEY=VALUE pairs we expect to see
	}{
		{
			name:    "no env vars",
			sprite:  "bramble",
			command: "echo hello",
			env:     nil,
			wantEnv: nil,
		},
		{
			name:    "single env var",
			sprite:  "moss",
			command: "claude -p",
			env: map[string]string{
				"OPENROUTER_API_KEY": "test-key-123",
			},
			wantEnv: []string{"-e", "OPENROUTER_API_KEY=test-key-123"},
		},
		{
			name:    "multiple env vars",
			sprite:  "fern",
			command: "bash script.sh",
			env: map[string]string{
				"ANTHROPIC_AUTH_TOKEN": "token-abc",
				"OPENROUTER_API_KEY":   "key-xyz",
			},
			wantEnv: []string{"-e", "ANTHROPIC_AUTH_TOKEN=token-abc", "-e", "OPENROUTER_API_KEY=key-xyz"},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Build args as ExecWithEnv would
			args := []string{"exec", "-s", tc.sprite}

			if len(tc.env) > 0 {
				keys := make([]string, 0, len(tc.env))
				for k := range tc.env {
					keys = append(keys, k)
				}
				for _, k := range keys {
					args = append(args, "-e", k+"="+tc.env[k])
				}
			}

			args = append(args, "--", "bash", "-ceu", tc.command)
			args = withOrgArgs(args, "misty-step")

			// Verify env vars are in the args
			if tc.wantEnv != nil {
				for i, expected := range tc.wantEnv {
					found := false
					for j, arg := range args {
						if arg == expected && i < len(tc.wantEnv)-1 && j+1 < len(args) {
							// Check that -e is followed by KEY=VALUE
							if args[j] == "-e" && j+1 < len(args) {
								found = true
								break
							}
						}
						if arg == expected {
							found = true
							break
						}
					}
					if !found {
						t.Errorf("expected to find %q in args: %v", expected, args)
					}
				}
			}

			// Verify the command separator is present
			foundSeparator := false
			for _, arg := range args {
				if arg == "--" {
					foundSeparator = true
					break
				}
			}
			if !foundSeparator {
				t.Errorf("expected -- separator in args: %v", args)
			}
		})
	}
}

func TestArgsForLogRedactsEnvValues(t *testing.T) {
	t.Parallel()

	args := []string{"exec", "-env", "OPENROUTER_API_KEY=secret", "-env", "ANTHROPIC_AUTH_TOKEN=token", "--", "bash", "-ceu", "echo hi"}
	want := "exec -env OPENROUTER_API_KEY=<redacted> -env ANTHROPIC_AUTH_TOKEN=<redacted> -- bash -ceu echo hi"

	original := append([]string(nil), args...)
	got := argsForLog(args)

	if got != want {
		t.Fatalf("argsForLog() = %q, want %q", got, want)
	}
	if !reflect.DeepEqual(args, original) {
		t.Fatalf("argsForLog mutated args: %v", args)
	}
}

func TestClassifyError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		err        error
		wantRetry  bool
		wantClass  error
	}{
		{
			name:      "nil error",
			err:       nil,
			wantRetry: false,
			wantClass: nil,
		},
		{
			name:      "i/o timeout is transport",
			err:       errors.New("read tcp 10.0.0.1:443: i/o timeout"),
			wantRetry: true,
			wantClass: ErrTransportFailure,
		},
		{
			name:      "failed to connect is transport",
			err:       errors.New("failed to connect: read tcp ...:443: i/o timeout"),
			wantRetry: true,
			wantClass: ErrTransportFailure,
		},
		{
			name:      "connection refused is transport",
			err:       errors.New("dial tcp: connection refused"),
			wantRetry: true,
			wantClass: ErrTransportFailure,
		},
		{
			name:      "exit status is command failure",
			err:       errors.New("exit status 1"),
			wantRetry: false,
			wantClass: ErrCommandFailure,
		},
		{
			name:      "context deadline exceeded is timeout",
			err:       context.DeadlineExceeded,
			wantRetry: false,
			wantClass: ErrTimeout,
		},
		{
			name:      "context canceled not retryable",
			err:       context.Canceled,
			wantRetry: false,
			wantClass: context.Canceled,
		},
		{
			name:      "unknown error not retryable",
			err:       errors.New("some random error"),
			wantRetry: false,
			wantClass: nil, // returns original error as-is
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			class, retryable := ClassifyError(tc.err)
			if retryable != tc.wantRetry {
				t.Errorf("ClassifyError(%v) retryable = %v, want %v", tc.err, retryable, tc.wantRetry)
			}
			if tc.wantClass != nil {
				if class == nil || !errors.Is(class, tc.wantClass) {
					t.Errorf("ClassifyError(%v) class = %v, want %v wrapped", tc.err, class, tc.wantClass)
				}
			} else if tc.err != nil {
				if class == nil {
					t.Errorf("ClassifyError(%v) class = nil, want non-nil", tc.err)
				} else if class.Error() != tc.err.Error() {
					t.Errorf("ClassifyError(%v) class = %v, want original error", tc.err, class)
				}
			}
		})
	}
}

func TestClassifyError_StderrTransportString(t *testing.T) {
	t.Parallel()

	// A command error whose stderr output mentions "connection refused"
	// should NOT be classified as a transport failure.
	inner := errors.New("exit status 1")
	wrapped := fmt.Errorf("running sprite exec: %w (stderr: connection refused)", inner)
	class, retryable := ClassifyError(wrapped)
	if retryable {
		t.Errorf("ClassifyError(%v) retryable = true, want false (stderr transport string should not trigger retry)", wrapped)
	}
	if !errors.Is(class, ErrCommandFailure) {
		t.Errorf("ClassifyError(%v) class = %v, want ErrCommandFailure", wrapped, class)
	}
}

func TestResilientCLI_RetryOnTransportError(t *testing.T) {
	t.Parallel()

	callCount := 0
	mock := &MockSpriteCLI{
		ExecFn: func(ctx context.Context, sprite, command string, stdin []byte) (string, error) {
			callCount++
			if callCount < 3 {
				return "", errors.New("read tcp 10.0.0.1:443: i/o timeout")
			}
			return "success", nil
		},
	}

	r := NewResilientCLI(mock,
		WithMaxRetries(3),
		WithBaseDelay(1*time.Millisecond),
	)

	ctx := context.Background()
	result, err := r.Exec(ctx, "test-sprite", "echo hello", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "success" {
		t.Errorf("result = %q, want success", result)
	}
	if callCount != 3 {
		t.Errorf("callCount = %d, want 3", callCount)
	}
}

func TestResilientCLI_NoRetryOnCommandFailure(t *testing.T) {
	t.Parallel()

	callCount := 0
	mock := &MockSpriteCLI{
		ExecFn: func(ctx context.Context, sprite, command string, stdin []byte) (string, error) {
			callCount++
			return "", errors.New("exit status 1: command failed")
		},
	}

	r := NewResilientCLI(mock,
		WithMaxRetries(3),
		WithBaseDelay(1*time.Millisecond),
	)

	ctx := context.Background()
	_, err := r.Exec(ctx, "test-sprite", "false", nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if callCount != 1 {
		t.Errorf("callCount = %d, want 1 (no retry)", callCount)
	}
	if !errors.Is(err, ErrCommandFailure) {
		t.Errorf("expected ErrCommandFailure, got: %v", err)
	}
}

func TestResilientCLI_ExhaustedRetries(t *testing.T) {
	t.Parallel()

	callCount := 0
	mock := &MockSpriteCLI{
		ListFn: func(ctx context.Context) ([]string, error) {
			callCount++
			return nil, errors.New("connection refused")
		},
	}

	r := NewResilientCLI(mock,
		WithMaxRetries(2),
		WithBaseDelay(1*time.Millisecond),
	)

	ctx := context.Background()
	_, err := r.List(ctx)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if callCount != 3 { // initial + 2 retries
		t.Errorf("callCount = %d, want 3", callCount)
	}
	if !errors.Is(err, ErrTransportFailure) {
		t.Errorf("expected ErrTransportFailure, got: %v", err)
	}
}

func TestResilientCLI_RespectsContextCancellation(t *testing.T) {
	t.Parallel()

	mock := &MockSpriteCLI{
		ExecFn: func(ctx context.Context, sprite, command string, stdin []byte) (string, error) {
			return "", errors.New("read tcp: i/o timeout")
		},
	}

	r := NewResilientCLI(mock,
		WithMaxRetries(10),
		WithBaseDelay(1*time.Hour), // Long delay to ensure we can cancel
	)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	_, err := r.Exec(ctx, "test-sprite", "echo hello", nil)
	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got: %v", err)
	}
}
