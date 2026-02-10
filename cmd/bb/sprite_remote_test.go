package main

import "testing"

func TestSpriteExecArgsAlwaysAllocatesTTY(t *testing.T) {
	args := spriteExecArgs("myorg", "bramble", "echo hi")
	found := false
	for _, arg := range args {
		if arg == "-tty" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected -tty in args, got %v", args)
	}
}
