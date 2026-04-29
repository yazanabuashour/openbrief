package main

import (
	"fmt"
	"io"
	"os"
)

const (
	modelName       = "gpt-5.4-mini"
	reasoningEffort = "medium"
)

func main() {
	if len(os.Args) < 2 {
		usage(os.Stderr)
		os.Exit(2)
	}
	switch os.Args[1] {
	case "run":
		if err := runCommand(os.Args[2:], os.Stdout); err != nil {
			fmt.Fprintf(os.Stderr, "agent eval failed: %v\n", err)
			os.Exit(1)
		}
	default:
		usage(os.Stderr)
		os.Exit(2)
	}
}

func usage(w io.Writer) {
	_, _ = fmt.Fprintln(w, "usage: go run ./scripts/agent-eval/openbrief run [--run-root path] [--scenario id] [--report-dir path] [--report-name name]")
}
