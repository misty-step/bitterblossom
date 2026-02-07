package main

import (
	"fmt"
	"os"
)

var version = "dev"

func main() {
	if _, err := fmt.Fprintf(os.Stdout, "bb version %s\n", version); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}
