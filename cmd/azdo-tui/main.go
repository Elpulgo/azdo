package main

import (
	"fmt"
	"os"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	// TODO: Initialize configuration
	// TODO: Initialize Azure DevOps client
	// TODO: Start TUI application
	fmt.Println("azdo TUI - Azure DevOps Terminal Interface")
	return nil
}
