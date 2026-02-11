package shellutil

import "testing"

func TestQuote(t *testing.T) {
	t.Parallel()
	tests := []struct {
		input string
		want  string
	}{
		{"simple", "'simple'"},
		{"with spaces", "'with spaces'"},
		{"with'quote", `'with'"'"'quote'`},
		{"with\"double", `'with"double'`},
		{"", "''"},
		{"$var", "'$var'"},
		{"`cmd`", "'`cmd`'"},
		{"a;b&&c|d", "'a;b&&c|d'"},
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			t.Parallel()
			got := Quote(tc.input)
			if got != tc.want {
				t.Errorf("Quote(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}
