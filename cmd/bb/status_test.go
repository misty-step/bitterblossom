package main

import (
	"testing"
)

func TestParsePorcelainStatus_Empty(t *testing.T) {
	t.Parallel()

	lines := parsePorcelainStatus("")
	if lines != nil {
		t.Fatalf("expected nil for empty input, got %v", lines)
	}
}

func TestParsePorcelainStatus_Clean(t *testing.T) {
	t.Parallel()

	// A clean repo produces no porcelain output.
	lines := parsePorcelainStatus("")
	if len(lines) != 0 {
		t.Fatalf("expected 0 lines for clean repo, got %d: %v", len(lines), lines)
	}
}

func TestParsePorcelainStatus_ModifiedFile(t *testing.T) {
	t.Parallel()

	input := " M cmd/bb/status.go\n"
	lines := parsePorcelainStatus(input)
	if len(lines) != 1 {
		t.Fatalf("expected 1 line, got %d: %v", len(lines), lines)
	}
	if lines[0] != " M cmd/bb/status.go" {
		t.Errorf("line[0] = %q, want %q", lines[0], " M cmd/bb/status.go")
	}
}

func TestParsePorcelainStatus_UntrackedFile(t *testing.T) {
	t.Parallel()

	input := "?? newfile.go\n"
	lines := parsePorcelainStatus(input)
	if len(lines) != 1 {
		t.Fatalf("expected 1 line, got %d: %v", len(lines), lines)
	}
	if lines[0] != "?? newfile.go" {
		t.Errorf("line[0] = %q, want %q", lines[0], "?? newfile.go")
	}
}

func TestParsePorcelainStatus_MultipleFiles(t *testing.T) {
	t.Parallel()

	input := " M cmd/bb/dispatch.go\n?? cmd/bb/newfile.go\nA  cmd/bb/added.go\n"
	lines := parsePorcelainStatus(input)
	if len(lines) != 3 {
		t.Fatalf("expected 3 lines, got %d: %v", len(lines), lines)
	}
	want := []string{
		" M cmd/bb/dispatch.go",
		"?? cmd/bb/newfile.go",
		"A  cmd/bb/added.go",
	}
	for i, w := range want {
		if lines[i] != w {
			t.Errorf("line[%d] = %q, want %q", i, lines[i], w)
		}
	}
}

func TestParsePorcelainStatus_NoTrailingNewline(t *testing.T) {
	t.Parallel()

	// Some environments may omit trailing newline.
	input := " M cmd/bb/status.go"
	lines := parsePorcelainStatus(input)
	if len(lines) != 1 {
		t.Fatalf("expected 1 line, got %d: %v", len(lines), lines)
	}
	if lines[0] != " M cmd/bb/status.go" {
		t.Errorf("line[0] = %q, want %q", lines[0], " M cmd/bb/status.go")
	}
}

func TestParsePorcelainStatus_StagedAndUnstaged(t *testing.T) {
	t.Parallel()

	// MM means staged modification + unstaged modification.
	input := "MM cmd/bb/status.go\n"
	lines := parsePorcelainStatus(input)
	if len(lines) != 1 {
		t.Fatalf("expected 1 line, got %d: %v", len(lines), lines)
	}
	if lines[0] != "MM cmd/bb/status.go" {
		t.Errorf("line[0] = %q, want %q", lines[0], "MM cmd/bb/status.go")
	}
}

func TestParsePorcelainStatus_CountMatchesLines(t *testing.T) {
	t.Parallel()

	input := " M a.go\n?? b.go\nD  c.go\n"
	lines := parsePorcelainStatus(input)
	if len(lines) != 3 {
		t.Errorf("expected 3 lines (matching dirty count), got %d", len(lines))
	}
}
