package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func writeReducedReports(reportDir string, reportName string, report runResult) error {
	if strings.TrimSpace(reportName) == "" {
		return errors.New("report name is required")
	}
	if err := os.MkdirAll(reportDir, 0o755); err != nil {
		return err
	}
	jsonPath := filepath.Join(reportDir, reportName+".json")
	mdPath := filepath.Join(reportDir, reportName+".md")
	if err := writeJSON(jsonPath, report); err != nil {
		return err
	}
	return os.WriteFile(mdPath, []byte(markdownReport(reportName, report)), 0o644)
}

func writeJSON(path string, value any) error {
	content, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	content = append(content, '\n')
	return os.WriteFile(path, content, 0o644)
}

func markdownReport(reportName string, report runResult) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# OpenBrief Agent Eval %s\n\n", reportName)
	fmt.Fprintf(&b, "Harness: `codex exec --json --full-auto from throwaway run directories; single-turn scenarios use --ephemeral, multi-turn scenarios resume a persisted eval session with explicit writable eval roots`.\n\n")
	fmt.Fprintf(&b, "- Run root: `%s`\n", report.RunRoot)
	fmt.Fprintf(&b, "- Isolated Codex home: `%s`\n", report.CodexHome)
	fmt.Fprintf(&b, "- Scenarios: `%d`\n", report.ScenarioCount)
	fmt.Fprintf(&b, "- New session files: `%d`\n", report.SessionFiles)
	fmt.Fprintf(&b, "- Elapsed seconds: `%.2f`\n\n", report.ElapsedSeconds)
	fmt.Fprintf(&b, "## Results\n\n")
	fmt.Fprintf(&b, "| Scenario | Passed | Assistant | Database | Tools | Commands | Hygiene |\n")
	fmt.Fprintf(&b, "| --- | --- | --- | --- | ---: | ---: | --- |\n")
	for _, result := range report.ScenarioResult {
		hygiene := "clean"
		if result.Metrics.DirectSQLiteAccess || result.Metrics.BroadRepoSearch || result.Metrics.RepoInspection || result.Metrics.EnvironmentAccess {
			hygiene = "review"
		}
		fmt.Fprintf(&b, "| `%s` | `%t` | `%t` | `%t` | `%d` | `%d` | `%s` |\n", result.ScenarioID, result.Passed, result.Verification.AssistantPass, result.Verification.DatabasePass, result.Metrics.ToolCalls, result.Metrics.CommandExecutions, hygiene)
	}
	fmt.Fprintf(&b, "\n## Gate\n\n")
	if failedScenarioError(report.ScenarioResult) == nil {
		fmt.Fprintf(&b, "Recommendation: `ship_openbrief_runner_production`.\n")
	} else {
		fmt.Fprintf(&b, "Recommendation: `continue_openbrief_production_hardening`.\n")
	}
	fmt.Fprintf(&b, "\nRaw Codex logs, copied repositories, local SQLite databases, caches, and isolated session stores are intentionally not committed. Reduced artifacts use `<run-root>` placeholders.\n")
	return b.String()
}

func countNewSessionFiles(marker time.Time, runRoot string) int {
	sessionsDir := filepath.Join(evalCodexHome(runRoot), "sessions")
	count := 0
	_ = filepath.WalkDir(sessionsDir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil || !info.ModTime().After(marker) {
			return nil
		}
		content, err := os.ReadFile(path)
		if err == nil && strings.Contains(string(content), runRoot) {
			count++
		}
		return nil
	})
	return count
}

func scrubResults(runRoot string, results []jobResult) []jobResult {
	out := make([]jobResult, 0, len(results))
	for _, result := range results {
		result.RunDir = scrubRunRoot(result.RunDir, runRoot)
		result.Database = scrubRunRoot(result.Database, runRoot)
		result.Error = scrubRunRoot(result.Error, runRoot)
		result.FinalMessage = scrubRunRoot(result.FinalMessage, runRoot)
		result.Verification.Details = scrubRunRoot(result.Verification.Details, runRoot)
		for i, evidence := range result.Metrics.HygieneEvidence {
			result.Metrics.HygieneEvidence[i] = scrubRunRoot(evidence, runRoot)
		}
		out = append(out, result)
	}
	return out
}

func scrubRunRoot(value string, runRoot string) string {
	value = strings.ReplaceAll(value, "/private"+runRoot, "<run-root>")
	return strings.ReplaceAll(value, runRoot, "<run-root>")
}

func failedScenarioError(results []jobResult) error {
	for _, result := range results {
		if !result.Passed {
			return errors.New("one or more agent eval scenarios failed")
		}
	}
	return nil
}

func roundSeconds(value float64) float64 {
	return float64(int(value*100+0.5)) / 100
}
