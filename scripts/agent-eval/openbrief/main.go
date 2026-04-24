package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const (
	modelName       = "gpt-5.4-mini"
	reasoningEffort = "medium"
)

type scenarioTurn struct {
	Prompt string
}

type scenario struct {
	ID    string
	Turns []scenarioTurn
}

type runOptions struct {
	RunRoot  string
	Scenario string
}

type runResult struct {
	RunRoot        string      `json:"run_root"`
	CodexHome      string      `json:"codex_home"`
	ScenarioCount  int         `json:"scenario_count"`
	SessionFiles   int         `json:"new_session_files"`
	ScenarioResult []jobResult `json:"scenario_results"`
	ElapsedSeconds float64     `json:"elapsed_seconds"`
}

type jobResult struct {
	ScenarioID string  `json:"scenario_id"`
	RunDir     string  `json:"run_dir"`
	Database   string  `json:"database"`
	Passed     bool    `json:"passed"`
	Error      string  `json:"error,omitempty"`
	Seconds    float64 `json:"seconds"`
}

var scenarios = []scenario{
	{
		ID: "empty-config-rejects-run-brief",
		Turns: []scenarioTurn{{
			Prompt: "Run an OpenBrief brief from a fresh empty configuration and report the production runner result.",
		}},
	},
	{
		ID: "rss-source-first-run-candidate",
		Turns: []scenarioTurn{{
			Prompt: "Configure an RSS source for https://github.blog/feed/ with key github-blog, section technology, and threshold medium. Then run an OpenBrief brief and report the JSON-derived brief.",
		}},
	},
	{
		ID: "github-release-source-must-include",
		Turns: []scenarioTurn{{
			Prompt: "Configure a GitHub release source for repository openai/codex with key codex-releases, section releases, and threshold always. Then run an OpenBrief brief and report the JSON-derived brief.",
		}},
	},
	{
		ID: "repeat-run-no-new-items",
		Turns: []scenarioTurn{
			{Prompt: "Configure an RSS source for https://github.blog/feed/ with key github-blog, section technology, and threshold medium. Run an OpenBrief brief and record any delivered message when required."},
			{Prompt: "Run OpenBrief again without changing configuration and report the production runner result."},
		},
	},
	{
		ID: "record-delivery-suppresses-repeats",
		Turns: []scenarioTurn{{
			Prompt: "Configure an RSS source for https://github.blog/feed/ with key github-blog, section technology, and threshold medium. Run an OpenBrief brief, record the delivered message when required, then run the brief again and report whether repeats were suppressed.",
		}},
	},
	{
		ID: "feed-failure-health-footnote",
		Turns: []scenarioTurn{{
			Prompt: "Configure an RSS source with key broken-feed, label Broken Feed, URL https://127.0.0.1:1/no-feed.xml, section technology, and threshold medium. Run an OpenBrief brief and report the health footnote from the JSON result.",
		}},
	},
	{
		ID: "feed-recovery-resolves-warning",
		Turns: []scenarioTurn{
			{Prompt: "Configure an RSS source with key changing-feed, label Changing Feed, URL https://127.0.0.1:1/no-feed.xml, section technology, and threshold medium. Run an OpenBrief brief and report the JSON result."},
			{Prompt: "Replace the changing-feed source URL with https://github.blog/feed/. Run OpenBrief again and report the JSON-derived result."},
		},
	},
	{
		ID: "invalid-source-config-rejects",
		Turns: []scenarioTurn{{
			Prompt: "Try to configure an OpenBrief source with an invalid key Bad/Key and report the production runner rejection without inspecting repo files or SQLite.",
		}},
	},
	{
		ID: "routine-agent-hygiene",
		Turns: []scenarioTurn{{
			Prompt: "Run a normal OpenBrief configuration inspection. Do not inspect SQLite, source files, .openclaw, workspace backups, repo files, or environment variables.",
		}},
	},
}

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
	fmt.Fprintln(w, "usage: go run ./scripts/agent-eval/openbrief run [--run-root path] [--scenario id]")
}

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
	return failedScenarioError(results)
}

func parseRunOptions(args []string) (runOptions, error) {
	fs := flag.NewFlagSet("openbrief agent-eval run", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var options runOptions
	fs.StringVar(&options.RunRoot, "run-root", "", "directory for copied repos, raw logs, caches, and isolated Codex home")
	fs.StringVar(&options.Scenario, "scenario", "", "scenario ID to run")
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
	for turnIndex, turn := range current.Turns {
		args := codexArgsForTurn(runRepo, runDir, current, turn, turnIndex+1, sessionID)
		logPath := filepath.Join(runDir, fmt.Sprintf("turn-%d.jsonl", turnIndex+1))
		out, err := runCodex(ctx, runDir, dbPath, args)
		if writeErr := os.WriteFile(logPath, out, 0o600); writeErr != nil && err == nil {
			err = writeErr
		}
		if err != nil {
			return finishResult(result, start, err)
		}
		if turnIndex == 0 && len(current.Turns) > 1 {
			sessionID = parseSessionID(out)
			if sessionID == "" {
				return finishResult(result, start, errors.New("multi-turn scenario did not emit a Codex session id"))
			}
		}
	}
	result.Passed = true
	return finishResult(result, start, nil)
}

func finishResult(result jobResult, start time.Time, err error) jobResult {
	result.Seconds = roundSeconds(time.Since(start).Seconds())
	if err != nil {
		result.Error = err.Error()
	}
	return result
}

func codexArgsForTurn(runRepo string, runDir string, currentScenario scenario, turn scenarioTurn, turnIndex int, sessionID string) []string {
	baseConfig := []string{
		"-m", modelName,
		"-c", fmt.Sprintf("model_reasoning_effort=%q", reasoningEffort),
		"-c", "shell_environment_policy.inherit=all",
	}
	if len(currentScenario.Turns) == 1 {
		args := []string{
			"exec",
			"--json",
			"--ephemeral",
			"--full-auto",
			"--skip-git-repo-check",
			"--ignore-user-config",
			"-C", runRepo,
			"--add-dir", runDir,
		}
		args = append(args, baseConfig...)
		return append(args, turn.Prompt)
	}
	if turnIndex == 1 {
		args := []string{
			"exec",
			"--json",
			"--full-auto",
			"--skip-git-repo-check",
			"--ignore-user-config",
			"-C", runRepo,
			"--add-dir", runDir,
		}
		args = append(args, baseConfig...)
		return append(args, turn.Prompt)
	}
	args := []string{
		"exec",
		"-C", runRepo,
		"--add-dir", runDir,
		"resume",
		"--json",
		"--full-auto",
		"--skip-git-repo-check",
		"--ignore-user-config",
	}
	args = append(args, baseConfig...)
	args = append(args, sessionID, turn.Prompt)
	return args
}

func runCodex(ctx context.Context, runDir string, dbPath string, args []string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "codex", args...)
	cmd.Env = evalEnv(runDir, dbPath)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return out, fmt.Errorf("codex %s: %w", strings.Join(args, " "), err)
	}
	return out, nil
}

func evalEnv(runDir string, dbPath string) []string {
	env := evalCodexEnv(filepath.Dir(runDir))
	env = envWithout(env, "OPENBRIEF_DATA_DIR")
	env = envWithOverride(env, "OPENBRIEF_DATABASE_PATH", dbPath)
	env = envWithOverride(env, "GOCACHE", filepath.Join(runDir, "gocache"))
	env = envWithOverride(env, "GOMODCACHE", filepath.Join(runDir, "gomodcache"))
	env = envWithOverride(env, "TMPDIR", filepath.Join(runDir, "tmp"))
	env = envWithOverride(env, "PATH", filepath.Join(runDir, "bin")+string(os.PathListSeparator)+os.Getenv("PATH"))
	return env
}

func envWithout(env []string, key string) []string {
	prefix := key + "="
	out := make([]string, 0, len(env))
	for _, entry := range env {
		if !strings.HasPrefix(entry, prefix) {
			out = append(out, entry)
		}
	}
	return out
}

func evalCodexEnv(runRoot string) []string {
	return envWithOverride(os.Environ(), "CODEX_HOME", evalCodexHome(runRoot))
}

func envWithOverride(env []string, key string, value string) []string {
	prefix := key + "="
	out := make([]string, 0, len(env)+1)
	for _, entry := range env {
		if !strings.HasPrefix(entry, prefix) {
			out = append(out, entry)
		}
	}
	return append(out, prefix+value)
}

func evalCodexHome(runRoot string) string {
	return filepath.Join(runRoot, "codex-home")
}

func sourceCodexHome() (string, error) {
	if home := os.Getenv("CODEX_HOME"); home != "" {
		return home, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".codex"), nil
}

func setupEvalCodexHome(runRoot string) error {
	sourceHome, err := sourceCodexHome()
	if err != nil {
		return err
	}
	return setupEvalCodexHomeFromSource(runRoot, sourceHome)
}

func setupEvalCodexHomeFromSource(runRoot string, sourceHome string) error {
	sourceAuth := filepath.Join(sourceHome, "auth.json")
	authBytes, err := os.ReadFile(sourceAuth)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("missing Codex auth at %s; run codex login before running evals", sourceAuth)
		}
		return err
	}
	codexHome := evalCodexHome(runRoot)
	if err := os.RemoveAll(codexHome); err != nil {
		return err
	}
	if err := os.MkdirAll(codexHome, 0o700); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(codexHome, "auth.json"), authBytes, 0o600)
}

func buildProductionBinary(ctx context.Context, runRepo string, runDir string) error {
	binDir := filepath.Join(runDir, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, "mise", "exec", "--", "go", "build", "-o", filepath.Join(binDir, "openbrief"), "./cmd/openbrief")
	cmd.Dir = runRepo
	cmd.Env = evalEnv(runDir, filepath.Join(runRepo, "openbrief.sqlite"))
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("build openbrief: %w\n%s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func copyRepo(src string, dst string) error {
	return filepath.WalkDir(src, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return os.MkdirAll(dst, 0o755)
		}
		if shouldSkipRepoPath(rel, entry) {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		target := filepath.Join(dst, rel)
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return os.MkdirAll(target, info.Mode().Perm())
		}
		if info.Mode()&os.ModeType != 0 {
			return nil
		}
		return copyFile(path, target, info.Mode().Perm())
	})
}

func shouldSkipRepoPath(rel string, entry fs.DirEntry) bool {
	first := strings.Split(rel, string(os.PathSeparator))[0]
	switch first {
	case ".git", ".beads", ".agents":
		return true
	}
	if rel == "AGENTS.md" {
		return true
	}
	if !entry.IsDir() && isIgnoredDatabaseFile(rel) {
		return true
	}
	if entry.IsDir() && rel == filepath.Join("scripts", "agent-eval") {
		return true
	}
	return false
}

func isIgnoredDatabaseFile(rel string) bool {
	name := filepath.Base(rel)
	return strings.HasSuffix(name, ".sqlite") ||
		strings.Contains(name, ".sqlite-") ||
		strings.HasSuffix(name, ".db") ||
		strings.Contains(name, ".db-")
}

func installEvalSkill(runRepo string) error {
	source := filepath.Join(runRepo, "skills", "openbrief", "SKILL.md")
	target := filepath.Join(runRepo, ".agents", "skills", "openbrief", "SKILL.md")
	return copyFile(source, target, 0o644)
}

func copyFile(src string, dst string, perm fs.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, perm)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}

func parseSessionID(out []byte) string {
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var event struct {
			SessionID string `json:"session_id"`
			ID        string `json:"id"`
			Type      string `json:"type"`
		}
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			continue
		}
		if event.SessionID != "" {
			return event.SessionID
		}
		if event.Type == "session" && event.ID != "" {
			return event.ID
		}
	}
	return ""
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
		result.RunDir = strings.ReplaceAll(result.RunDir, runRoot, "<run-root>")
		result.Database = strings.ReplaceAll(result.Database, runRoot, "<run-root>")
		result.Error = strings.ReplaceAll(result.Error, runRoot, "<run-root>")
		out = append(out, result)
	}
	return out
}

func failedScenarioError(results []jobResult) error {
	for _, result := range results {
		if !result.Passed {
			return errors.New("one or more agent eval scenarios failed")
		}
	}
	return nil
}

func repoRoot() (string, error) {
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("resolve repo root: %w", err)
	}
	return filepath.Abs(strings.TrimSpace(string(out)))
}

func isWithin(child string, parent string) bool {
	rel, err := filepath.Rel(parent, child)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator)))
}

func roundSeconds(value float64) float64 {
	return float64(int(value*100+0.5)) / 100
}
