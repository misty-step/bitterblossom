package main

import (
	"fmt"
	"os"
)

var version = "dev"

func main() {
	if _, err := fmt.Fprintf(os.Stdout, "bb version %s\n", version); err != nil {
		os.Exit(1)
	}
}
