package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func runCommand(args []string, stdout io.Writer) error {
	options, err := parseRunOptions(args)
	if err != nil {
		return err
	}
	repoRoot, err := repoRoot()
	if err != nil {
		return err
	}
	runRoot := options.RunRoot
	if runRoot == "" {
		runRoot, err = os.MkdirTemp("", "openbrief-agent-eval-*")
		if err != nil {
			return fmt.Errorf("create run root: %w", err)
		}
	} else if err := os.MkdirAll(runRoot, 0o755); err != nil {
		return fmt.Errorf("create run root: %w", err)
	}
	runRoot, err = filepath.Abs(runRoot)
	if err != nil {
		return fmt.Errorf("absolute run root: %w", err)
	}
	if isWithin(runRoot, repoRoot) {
		return fmt.Errorf("run root must be outside the repository: %s", runRoot)
	}
	if err := setupEvalCodexHome(runRoot); err != nil {
		return fmt.Errorf("prepare eval Codex home: %w", err)
	}
	if err := installEvalCodexSkill(repoRoot, runRoot); err != nil {
		return fmt.Errorf("install eval Codex skill: %w", err)
	}
	selected, err := selectScenarios(options.Scenario)
	if err != nil {
		return err
	}

	marker := time.Now()
	start := time.Now()
	results := make([]jobResult, 0, len(selected))
	for _, current := range selected {
		results = append(results, runScenario(context.Background(), repoRoot, runRoot, current))
	}
	report := runResult{
		RunRoot:        "<run-root>",
		CodexHome:      filepath.Join("<run-root>", "codex-home"),
		ScenarioCount:  len(selected),
		SessionFiles:   countNewSessionFiles(marker, runRoot),
		ScenarioResult: scrubResults(runRoot, results),
		ElapsedSeconds: roundSeconds(time.Since(start).Seconds()),
	}
	encoder := json.NewEncoder(stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(report); err != nil {
		return err
	}
	if options.ReportDir != "" {
		if err := writeReducedReports(options.ReportDir, options.ReportName, report); err != nil {
			return err
		}
	}
	return failedScenarioError(results)
}

func parseRunOptions(args []string) (runOptions, error) {
	fs := flag.NewFlagSet("openbrief agent-eval run", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var options runOptions
	fs.StringVar(&options.RunRoot, "run-root", "", "directory for copied repos, raw logs, caches, and isolated Codex home")
	fs.StringVar(&options.Scenario, "scenario", "", "scenario ID to run")
	fs.StringVar(&options.ReportDir, "report-dir", "", "optional directory for reduced JSON and Markdown reports")
	fs.StringVar(&options.ReportName, "report-name", "openbrief-v0.1.0-final", "report basename when --report-dir is set")
	if err := fs.Parse(args); err != nil {
		return runOptions{}, err
	}
	if fs.NArg() != 0 {
		return runOptions{}, fmt.Errorf("unexpected positional arguments: %v", fs.Args())
	}
	return options, nil
}

func selectScenarios(id string) ([]scenario, error) {
	if strings.TrimSpace(id) == "" {
		return scenarios, nil
	}
	for _, current := range scenarios {
		if current.ID == id {
			return []scenario{current}, nil
		}
	}
	return nil, fmt.Errorf("unknown scenario %q", id)
}

func runScenario(ctx context.Context, repoRoot string, runRoot string, current scenario) jobResult {
	start := time.Now()
	runDir := filepath.Join(runRoot, current.ID)
	runRepo := filepath.Join(runDir, "repo")
	dbPath := filepath.Join(runRepo, "openbrief.sqlite")
	result := jobResult{
		ScenarioID: current.ID,
		RunDir:     runDir,
		Database:   dbPath,
	}
	if err := os.RemoveAll(runDir); err != nil {
		return finishResult(result, start, err)
	}
	current, err := prepareScenarioFixtures(current, runDir)
	if err != nil {
		return finishResult(result, start, err)
	}
	if err := copyRepo(repoRoot, runRepo); err != nil {
		return finishResult(result, start, err)
	}
	if err := installEvalSkill(runRepo); err != nil {
		return finishResult(result, start, err)
	}
	if err := buildProductionBinary(ctx, runRepo, runDir); err != nil {
		return finishResult(result, start, err)
	}
	var sessionID string
	var metrics scenarioMetrics
	var finalMessage string
	for turnIndex, turn := range current.Turns {
		args := codexArgsForTurn(runRepo, runDir, current, turn, turnIndex+1, sessionID)
		logPath := filepath.Join(runDir, fmt.Sprintf("turn-%d.jsonl", turnIndex+1))
		out, err := runCodex(ctx, runDir, dbPath, args)
		if writeErr := os.WriteFile(logPath, out, 0o600); writeErr != nil && err == nil {
			err = writeErr
		}
		parsed := parseCodexOutput(out)
		metrics = mergeMetrics(metrics, parsed.Metrics)
		if parsed.FinalMessage != "" {
			finalMessage = parsed.FinalMessage
		}
		if err != nil {
			result.Metrics = metrics
			result.FinalMessage = finalMessage
			return finishResult(result, start, err)
		}
		if turnIndex == 0 && len(current.Turns) > 1 {
			sessionID = parsed.SessionID
			if sessionID == "" {
				result.Metrics = metrics
				result.FinalMessage = finalMessage
				return finishResult(result, start, errors.New("multi-turn scenario did not emit a Codex session id"))
			}
		}
	}
	verification := verifyScenario(dbPath, current.ID, finalMessage, metrics)
	result.Metrics = metrics
	result.Verification = verification
	result.FinalMessage = finalMessage
	result.Passed = verification.Passed
	if !verification.Passed {
		return finishResult(result, start, errors.New(verification.Details))
	}
	return finishResult(result, start, nil)
}

func finishResult(result jobResult, start time.Time, err error) jobResult {
	result.Seconds = roundSeconds(time.Since(start).Seconds())
	if err != nil {
		result.Error = err.Error()
	}
	return result
}
