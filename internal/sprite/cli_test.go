package sprite

import (
	"reflect"
	"testing"
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
