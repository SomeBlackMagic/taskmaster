package main

import (
	"fmt"
	"os"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: taskmaster <command> [args...]")
		os.Exit(1)
	}
	// TODO(etap2): wire up runner.Runner and start execution
	fmt.Fprintln(os.Stderr, "not implemented yet")
	os.Exit(1)
}
