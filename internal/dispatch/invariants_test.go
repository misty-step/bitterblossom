package dispatch

import "testing"

func TestRequireOneShotInvariants(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		command string
		wantErr bool
	}{
		{
			name:    "ok",
			command: "cat PROMPT.md | claude -p --dangerously-skip-permissions --permission-mode bypassPermissions --verbose --output-format stream-json",
			wantErr: false,
		},
		{
			name:    "missing claude print",
			command: "cat PROMPT.md | claude --dangerously-skip-permissions --verbose --output-format stream-json",
			wantErr: true,
		},
		{
			name:    "missing dangerously skip",
			command: "cat PROMPT.md | claude -p --permission-mode bypassPermissions --verbose --output-format stream-json",
			wantErr: true,
		},
		{
			name:    "missing verbose",
			command: "cat PROMPT.md | claude -p --dangerously-skip-permissions --output-format stream-json",
			wantErr: true,
		},
		{
			name:    "missing output format",
			command: "cat PROMPT.md | claude -p --dangerously-skip-permissions --verbose",
			wantErr: true,
		},
		{
			name:    "reordered flags ok",
			command: "cat PROMPT.md | claude -p --output-format stream-json --verbose --dangerously-skip-permissions",
			wantErr: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := requireOneShotInvariants(tc.command)
			if tc.wantErr && err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("expected no error, got %v", err)
			}
		})
	}
}
