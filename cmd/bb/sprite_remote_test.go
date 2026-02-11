package main

import (
	"strings"
	"testing"
)

func TestBuildEnvArgs(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		env     map[string]string
		want    []string
		wantErr string
	}{
		{
			name: "multiple vars sorted alphabetically",
			env: map[string]string{
				"OPENROUTER_API_KEY":  "sk-or-test",
				"ANTHROPIC_AUTH_TOKEN": "tok-test",
			},
			want: []string{"-env", "ANTHROPIC_AUTH_TOKEN=tok-test,OPENROUTER_API_KEY=sk-or-test"},
		},
		{
			name: "nil map returns nil",
			env:  nil,
			want: nil,
		},
		{
			name: "empty map returns nil",
			env:  map[string]string{},
			want: nil,
		},
		{
			name: "single var",
			env:  map[string]string{"KEY": "value"},
			want: []string{"-env", "KEY=value"},
		},
		{
			name:    "comma in value returns error",
			env:     map[string]string{"KEY": "a,b"},
			wantErr: "contains a comma",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := buildEnvArgs(tc.env)

			if tc.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tc.wantErr)
				}
				if !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("expected error containing %q, got %q", tc.wantErr, err.Error())
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(got) != len(tc.want) {
				t.Fatalf("expected %d args, got %d: %v", len(tc.want), len(got), got)
			}
			for i := range tc.want {
				if got[i] != tc.want[i] {
					t.Fatalf("arg[%d]: expected %q, got %q", i, tc.want[i], got[i])
				}
			}
		})
	}
}
