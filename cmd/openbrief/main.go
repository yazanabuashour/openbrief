package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"runtime/debug"

	"github.com/yazanabuashour/openbrief/internal/runclient"
	"github.com/yazanabuashour/openbrief/internal/runner"
)

var version string

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

func run(args []string, stdin io.Reader, stdout io.Writer, stderr io.Writer) int {
	if len(args) == 0 {
		usage(stderr)
		return 2
	}

	switch args[0] {
	case "help", "-h", "--help":
		usage(stdout)
		return 0
	case "version", "--version":
		writeVersion(stdout)
		return 0
	case "config":
		return runConfig(args[1:], stdin, stdout, stderr)
	case "brief":
		return runBrief(args[1:], stdin, stdout, stderr)
	default:
		_, _ = fmt.Fprintf(stderr, "unknown openbrief command %q\n", args[0])
		usage(stderr)
		return 2
	}
}

func runConfig(args []string, stdin io.Reader, stdout io.Writer, stderr io.Writer) int {
	config, ok := parseConfig("config", args, stderr)
	if !ok {
		return 2
	}
	var request runner.ConfigTaskRequest
	if err := decodeRequest(stdin, &request); err != nil {
		_, _ = fmt.Fprintf(stderr, "decode config request: %v\n", err)
		return 1
	}
	result, err := runner.RunConfigTask(context.Background(), config, request)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "run config task: %v\n", err)
		return 1
	}
	if err := json.NewEncoder(stdout).Encode(result); err != nil {
		_, _ = fmt.Fprintf(stderr, "encode config result: %v\n", err)
		return 1
	}
	return 0
}

func runBrief(args []string, stdin io.Reader, stdout io.Writer, stderr io.Writer) int {
	config, ok := parseConfig("brief", args, stderr)
	if !ok {
		return 2
	}
	var request runner.BriefTaskRequest
	if err := decodeRequest(stdin, &request); err != nil {
		_, _ = fmt.Fprintf(stderr, "decode brief request: %v\n", err)
		return 1
	}
	result, err := runner.RunBriefTask(context.Background(), config, request)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "run brief task: %v\n", err)
		return 1
	}
	if err := json.NewEncoder(stdout).Encode(result); err != nil {
		_, _ = fmt.Fprintf(stderr, "encode brief result: %v\n", err)
		return 1
	}
	return 0
}

func parseConfig(name string, args []string, stderr io.Writer) (runclient.Config, bool) {
	fs := flag.NewFlagSet("openbrief "+name, flag.ContinueOnError)
	fs.SetOutput(stderr)
	databasePath := fs.String("db", "", "OpenBrief SQLite database path")
	if err := fs.Parse(args); err != nil {
		return runclient.Config{}, false
	}
	if fs.NArg() != 0 {
		_, _ = fmt.Fprintf(stderr, "unexpected positional arguments: %v\n", fs.Args())
		return runclient.Config{}, false
	}
	return runclient.Config{DatabasePath: *databasePath}, true
}

func decodeRequest[T any](stdin io.Reader, request *T) error {
	decoder := json.NewDecoder(stdin)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(request); err != nil {
		return err
	}
	var extra json.RawMessage
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return errors.New("multiple JSON values are not supported")
		}
		return err
	}
	return nil
}

func writeVersion(w io.Writer) {
	info, ok := readBuildInfo()
	_, _ = fmt.Fprintf(w, "openbrief %s\n", resolvedVersion(version, info, ok))
}

func readBuildInfo() (*debug.BuildInfo, bool) {
	return debug.ReadBuildInfo()
}

func resolvedVersion(linkerVersion string, info *debug.BuildInfo, ok bool) string {
	if linkerVersion != "" {
		return linkerVersion
	}
	if ok && info != nil && info.Main.Version != "" && info.Main.Version != "(devel)" {
		return info.Main.Version
	}
	return "dev"
}

func usage(w io.Writer) {
	_, _ = fmt.Fprintln(w, "usage: openbrief <version|config|brief> [--db path]")
	_, _ = fmt.Fprintln(w, "       openbrief config [--db path] < request.json")
	_, _ = fmt.Fprintln(w, "       openbrief brief [--db path] < request.json")
}
