package main

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestRunVersion(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	if err := run(context.Background(), []string{"version"}, &out, &bytes.Buffer{}); err != nil {
		t.Fatalf("run(version) error = %v", err)
	}
	if !strings.Contains(out.String(), "bb version") {
		t.Fatalf("version output = %q", out.String())
	}
}

func TestRunUsageAndUnknownCommand(t *testing.T) {
	t.Parallel()

	var usage bytes.Buffer
	if err := run(context.Background(), nil, &usage, &bytes.Buffer{}); err != nil {
		t.Fatalf("run() error = %v", err)
	}
	if !strings.Contains(usage.String(), "Usage:") {
		t.Fatalf("usage output = %q", usage.String())
	}

	if err := run(context.Background(), []string{"wat"}, &bytes.Buffer{}, &bytes.Buffer{}); err == nil {
		t.Fatal("run(unknown) expected error")
	}
}
