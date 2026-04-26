package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

const (
	modelName       = "gpt-5.4-mini"
	reasoningEffort = "medium"
)

const productionRunnerOnlyInstruction = "Use only the openbrief runner JSON interface. Do not inspect repo files, source files, skill files, binaries, SQLite, environment variables, or run openbrief --help. Do not search for instructions."

type scenarioTurn struct {
	Prompt string
}

type scenario struct {
	ID    string
	Turns []scenarioTurn
}

type runOptions struct {
	RunRoot    string
	Scenario   string
	ReportDir  string
	ReportName string
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
	ScenarioID   string             `json:"scenario_id"`
	RunDir       string             `json:"run_dir"`
	Database     string             `json:"database"`
	Passed       bool               `json:"passed"`
	Error        string             `json:"error,omitempty"`
	Seconds      float64            `json:"seconds"`
	Metrics      scenarioMetrics    `json:"metrics"`
	Verification verificationResult `json:"verification"`
	FinalMessage string             `json:"final_message,omitempty"`
}

type scenarioMetrics struct {
	AssistantCalls     int      `json:"assistant_calls"`
	ToolCalls          int      `json:"tool_calls"`
	CommandExecutions  int      `json:"command_executions"`
	DirectSQLiteAccess bool     `json:"direct_sqlite_access"`
	BroadRepoSearch    bool     `json:"broad_repo_search"`
	RepoInspection     bool     `json:"repo_inspection"`
	EnvironmentAccess  bool     `json:"environment_access"`
	HygieneEvidence    []string `json:"hygiene_evidence,omitempty"`
}

type verificationResult struct {
	Passed        bool   `json:"passed"`
	DatabasePass  bool   `json:"database_pass"`
	AssistantPass bool   `json:"assistant_pass"`
	Details       string `json:"details,omitempty"`
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
		ID: "rss-source-generic-processing-fields",
		Turns: []scenarioTurn{{
			Prompt: "Configure an RSS source for https://github.blog/feed/ with key github-blog, section technology, threshold medium, url_canonicalization none, outlet_extraction url_host, dedup_group news, and priority_rank 10. Then run OpenBrief and report only the JSON-derived result.",
		}},
	},
	{
		ID: "outlet-policy-watch-audit",
		Turns: []scenarioTurn{{
			Prompt: "Configure an outlet policy named github.blog with policy watch and enabled true. Configure an RSS source for https://github.blog/feed/ with key github-blog, section technology, threshold medium, and outlet_extraction url_host. Run OpenBrief and report whether the JSON result includes a policy audit while still allowing candidates.",
		}},
	},
	{
		ID: "configured-max-delivery-items",
		Turns: []scenarioTurn{{
			Prompt: "Configure OpenBrief max_delivery_items to 2 through openbrief config. Configure RSS sources with keys limit-one, limit-two, and limit-three for https://example.com/openbrief-limit-1.xml, https://example.com/openbrief-limit-2.xml, and https://example.com/openbrief-limit-3.xml, each with section technology and threshold medium. Run an OpenBrief brief, deliver the brief according to max_delivery_items, and record the exact delivered message when required. Report only the delivered brief.",
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
			Prompt: "Try to configure an OpenBrief source with an invalid key Bad/Key by piping one upsert_source JSON request to openbrief config. Report the production runner rejection. Do not inspect repo files, skill files, binaries, SQLite, environment variables, or run openbrief --help.",
		}},
	},
	{
		ID: "routine-agent-hygiene",
		Turns: []scenarioTurn{{
			Prompt: "Run a normal OpenBrief configuration inspection by piping exactly {\"action\":\"inspect_config\"} to openbrief config. For this routine production task, do not inspect SQLite, source files, skill files, repo files, binaries, or environment variables. Do not run openbrief --help or search for instructions.",
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
	_, _ = fmt.Fprintln(w, "usage: go run ./scripts/agent-eval/openbrief run [--run-root path] [--scenario id] [--report-dir path] [--report-name name]")
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

func prepareScenarioFixtures(current scenario, runDir string) (scenario, error) {
	needsFeed := scenarioContains(current, "https://github.blog/feed/")
	needsReleases := scenarioContains(current, "repository openai/codex")
	needsLimitFeeds := current.ID == "configured-max-delivery-items"
	if !needsFeed && !needsReleases && !needsLimitFeeds {
		return current, nil
	}
	fixtureDir := filepath.Join(runDir, "fixtures")
	if err := os.MkdirAll(fixtureDir, 0o755); err != nil {
		return scenario{}, err
	}
	feedPath := filepath.Join(fixtureDir, "github-blog.xml")
	releasePath := filepath.Join(fixtureDir, "codex-releases.json")
	feed := `<?xml version="1.0"?>
<rss version="2.0"><channel>
<title>OpenBrief fixture</title>
<item><title>OpenBrief fixture story - Fixture Outlet</title><link>https://fixture.example/story</link><guid>fixture-guid-1</guid><pubDate>Thu, 23 Apr 2026 01:00:00 GMT</pubDate></item>
</channel></rss>`
	if needsFeed {
		if err := os.WriteFile(feedPath, []byte(feed), 0o644); err != nil {
			return scenario{}, err
		}
	}
	var limitFeedURLs []string
	if needsLimitFeeds {
		for i := 1; i <= 3; i++ {
			path := filepath.Join(fixtureDir, fmt.Sprintf("limit-%d.xml", i))
			content := fmt.Sprintf(`<?xml version="1.0"?>
<rss version="2.0"><channel>
<title>OpenBrief limit fixture %d</title>
<item><title>OpenBrief limit story %d</title><link>https://fixture.example/limit-%d</link><guid>limit-guid-%d</guid><pubDate>Thu, 23 Apr 2026 01:0%d:00 GMT</pubDate></item>
</channel></rss>`, i, i, i, i, i)
			if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
				return scenario{}, err
			}
			limitFeedURLs = append(limitFeedURLs, (&url.URL{Scheme: "file", Path: path}).String())
		}
	}
	releases := `[{"tag_name":"v1.2.3","name":"OpenBrief fixture release","html_url":"https://fixture.example/releases/tag/v1.2.3","published_at":"2026-04-23T01:00:00Z","draft":false,"prerelease":false}]`
	if needsReleases {
		if err := os.WriteFile(releasePath, []byte(releases), 0o644); err != nil {
			return scenario{}, err
		}
	}
	replacementFeed := (&url.URL{Scheme: "file", Path: feedPath}).String()
	replacementReleases := (&url.URL{Scheme: "file", Path: releasePath}).String()
	rewrite := func(prompt string) string {
		prompt = strings.ReplaceAll(prompt, "https://github.blog/feed/", replacementFeed)
		prompt = strings.ReplaceAll(prompt, "named github.blog", "named fixture.example")
		prompt = strings.ReplaceAll(prompt, "repository openai/codex with key", "repository openai/codex using source URL "+replacementReleases+" with key")
		for i, value := range limitFeedURLs {
			prompt = strings.ReplaceAll(prompt, fmt.Sprintf("https://example.com/openbrief-limit-%d.xml", i+1), value)
		}
		return prompt
	}
	for i := range current.Turns {
		current.Turns[i].Prompt = rewrite(current.Turns[i].Prompt)
	}
	return current, nil
}

func scenarioContains(current scenario, value string) bool {
	for _, turn := range current.Turns {
		if strings.Contains(turn.Prompt, value) {
			return true
		}
	}
	return false
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
	prompt := evalPrompt(turn.Prompt)
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
		return append(args, prompt)
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
		return append(args, prompt)
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
	args = append(args, sessionID, prompt)
	return args
}

func evalPrompt(prompt string) string {
	return strings.TrimSpace(prompt) + "\n\n" + productionRunnerOnlyInstruction
}

func runCodex(ctx context.Context, runDir string, dbPath string, args []string) ([]byte, error) {
	if err := prepareEvalDirs(runDir); err != nil {
		return nil, err
	}
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
	env = envWithOverride(env, "OPENBRIEF_EVAL_ALLOW_FILE_URLS", "1")
	env = envWithOverride(env, "PATH", filepath.Join(runDir, "bin")+string(os.PathListSeparator)+os.Getenv("PATH"))
	return env
}

func prepareEvalDirs(runDir string) error {
	for _, dir := range []string{
		filepath.Join(runDir, "gocache"),
		filepath.Join(runDir, "gomodcache"),
		filepath.Join(runDir, "tmp"),
	} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	return nil
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

func installEvalCodexSkill(repoRoot string, runRoot string) error {
	source := filepath.Join(repoRoot, "skills", "openbrief", "SKILL.md")
	target := filepath.Join(evalCodexHome(runRoot), "skills", ".system", "openbrief", "SKILL.md")
	return copyFile(source, target, 0o644)
}

func buildProductionBinary(ctx context.Context, runRepo string, runDir string) error {
	binDir := filepath.Join(runDir, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		return err
	}
	if err := prepareEvalDirs(runDir); err != nil {
		return err
	}
	env := evalEnv(runDir, filepath.Join(runRepo, "openbrief.sqlite"))
	trust := exec.CommandContext(ctx, "mise", "trust", "--yes", "--quiet", filepath.Join(runRepo, "mise.toml"))
	trust.Dir = runRepo
	trust.Env = env
	if out, err := trust.CombinedOutput(); err != nil {
		return fmt.Errorf("trust copied mise config: %w\n%s", err, strings.TrimSpace(string(out)))
	}
	cmd := exec.CommandContext(ctx, "mise", "exec", "--", "go", "build", "-o", filepath.Join(binDir, "openbrief"), "./cmd/openbrief")
	cmd.Dir = runRepo
	cmd.Env = env
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
	if entry.IsDir() && rel == filepath.Join("docs", "agent-eval-results") {
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
	defer func() {
		_ = in.Close()
	}()
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

type parsedCodexOutput struct {
	Metrics      scenarioMetrics
	FinalMessage string
	SessionID    string
}

func parseCodexOutput(out []byte) parsedCodexOutput {
	parsed := parsedCodexOutput{}
	for _, rawLine := range strings.Split(string(out), "\n") {
		line := strings.TrimSpace(rawLine)
		if line == "" {
			continue
		}
		var event any
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			continue
		}
		values := map[string][]string{}
		collectJSONStrings("", event, values)
		eventTypes := values["type"]
		eventType := firstValue(values, "type")
		if containsString(eventTypes, "thread.started") || containsString(eventTypes, "session") {
			if id := firstNonEmpty(firstValue(values, "thread_id"), firstValue(values, "session_id"), firstValue(values, "id")); id != "" && parsed.SessionID == "" {
				parsed.SessionID = id
			}
		}
		if anyStringContains(eventTypes, "assistant") || anyStringContains(eventTypes, "agent_message") {
			parsed.Metrics.AssistantCalls++
			if message := likelyAssistantMessage(values); message != "" {
				parsed.FinalMessage = message
			}
		}
		if strings.Contains(eventType, "tool") || strings.Contains(eventType, "exec") || anyStringContains(eventTypes, "command") {
			parsed.Metrics.ToolCalls++
		}
		for _, command := range commandLikeStrings(values) {
			parsed.Metrics.CommandExecutions++
			updateHygieneMetrics(&parsed.Metrics, command)
		}
	}
	return parsed
}

func collectJSONStrings(key string, value any, out map[string][]string) {
	switch typed := value.(type) {
	case map[string]any:
		for childKey, child := range typed {
			collectJSONStrings(strings.ToLower(childKey), child, out)
		}
	case []any:
		for _, child := range typed {
			collectJSONStrings(key, child, out)
		}
	case string:
		out[key] = append(out[key], typed)
	}
}

func firstValue(values map[string][]string, key string) string {
	for _, value := range values[strings.ToLower(key)] {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if strings.TrimSpace(value) == want {
			return true
		}
	}
	return false
}

func anyStringContains(values []string, want string) bool {
	for _, value := range values {
		if strings.Contains(strings.ToLower(value), strings.ToLower(want)) {
			return true
		}
	}
	return false
}

func likelyAssistantMessage(values map[string][]string) string {
	for _, key := range []string{"message", "content", "text"} {
		candidates := values[key]
		for i := len(candidates) - 1; i >= 0; i-- {
			candidate := strings.TrimSpace(candidates[i])
			if candidate != "" && !looksLikeCommand(candidate) {
				return candidate
			}
		}
	}
	return ""
}

func commandLikeStrings(values map[string][]string) []string {
	var commands []string
	for key, candidates := range values {
		if key != "cmd" && key != "command" && key != "arguments" && key != "shell_command" {
			continue
		}
		for _, candidate := range candidates {
			if key == "command" || looksLikeCommand(candidate) {
				commands = append(commands, strings.TrimSpace(candidate))
			}
		}
	}
	return commands
}

func looksLikeCommand(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" || strings.Contains(value, "\n\n") {
		return false
	}
	prefixes := []string{"/bin/sh ", "/bin/zsh ", "openbrief ", "sqlite3", "rg ", "grep ", "find ", "cat ", "sed ", "ls ", "env", "printenv", "go ", "mise ", "./"}
	for _, prefix := range prefixes {
		trimmedPrefix := strings.TrimSpace(prefix)
		if value == trimmedPrefix || strings.HasPrefix(value, prefix) {
			return true
		}
	}
	return false
}

func updateHygieneMetrics(metrics *scenarioMetrics, command string) {
	lower := strings.ToLower(command)
	allowedSkillRead := isAllowedInstalledSkillRead(lower)
	addEvidence := func(label string) {
		metrics.HygieneEvidence = append(metrics.HygieneEvidence, label+": "+command)
	}
	if strings.Contains(lower, "sqlite3") || strings.Contains(lower, "select ") {
		metrics.DirectSQLiteAccess = true
		addEvidence("direct_sqlite")
	}
	if strings.Contains(lower, "rg ") || strings.Contains(lower, "grep ") || strings.Contains(lower, "find ") {
		metrics.BroadRepoSearch = true
		addEvidence("broad_repo_search")
	}
	if !allowedSkillRead && (strings.Contains(lower, "cat ") || strings.Contains(lower, "sed ") || strings.Contains(lower, "ls ")) {
		metrics.RepoInspection = true
		addEvidence("repo_inspection")
	}
	if strings.Contains(lower, "env") || strings.Contains(lower, "printenv") {
		metrics.EnvironmentAccess = true
		addEvidence("environment_access")
	}
}

func isAllowedInstalledSkillRead(lower string) bool {
	if !strings.Contains(lower, ".agents/skills/openbrief/skill.md") &&
		!strings.Contains(lower, "skills/.system/openbrief/skill.md") {
		return false
	}
	for _, forbidden := range []string{"sqlite3", "select ", " rg ", " grep ", " find ", " env", "printenv"} {
		if strings.Contains(lower, forbidden) {
			return false
		}
	}
	for _, separator := range []string{";", "|", "`", "$("} {
		if strings.Contains(lower, separator) {
			return false
		}
	}
	withoutAllowedPrelude := strings.ReplaceAll(lower, "pwd &&", "")
	if strings.Contains(withoutAllowedPrelude, "&&") {
		return false
	}
	return strings.Contains(withoutAllowedPrelude, "sed ") || strings.Contains(withoutAllowedPrelude, "cat ")
}

func mergeMetrics(left scenarioMetrics, right scenarioMetrics) scenarioMetrics {
	left.AssistantCalls += right.AssistantCalls
	left.ToolCalls += right.ToolCalls
	left.CommandExecutions += right.CommandExecutions
	left.DirectSQLiteAccess = left.DirectSQLiteAccess || right.DirectSQLiteAccess
	left.BroadRepoSearch = left.BroadRepoSearch || right.BroadRepoSearch
	left.RepoInspection = left.RepoInspection || right.RepoInspection
	left.EnvironmentAccess = left.EnvironmentAccess || right.EnvironmentAccess
	left.HygieneEvidence = append(left.HygieneEvidence, right.HygieneEvidence...)
	return left
}

func verifyScenario(dbPath string, scenarioID string, finalMessage string, metrics scenarioMetrics) verificationResult {
	result := verificationResult{Passed: true, DatabasePass: true, AssistantPass: true}
	var details []string
	if strings.TrimSpace(finalMessage) == "" {
		result.AssistantPass = false
		details = append(details, "assistant final message was empty or not exposed in JSONL")
	}
	if metrics.DirectSQLiteAccess || metrics.BroadRepoSearch || metrics.RepoInspection || metrics.EnvironmentAccess {
		result.AssistantPass = false
		details = append(details, "agent used forbidden inspection path")
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		result.DatabasePass = false
		details = append(details, fmt.Sprintf("open eval database: %v", err))
		return finishVerification(result, details)
	}
	defer func() {
		_ = db.Close()
	}()

	switch scenarioID {
	case "empty-config-rejects-run-brief":
		expectTableCount(db, &result, &details, "brief_source", 0)
		expectMessageContains(&result, &details, finalMessage, "no enabled sources", "rejected")
	case "rss-source-first-run-candidate", "rss-source-generic-processing-fields", "outlet-policy-watch-audit":
		expectMinimumTableCount(db, &result, &details, "brief_source", 1)
		expectMinimumTableCount(db, &result, &details, "source_state", 1)
	case "configured-max-delivery-items":
		expectMinimumTableCount(db, &result, &details, "brief_source", 3)
		expectMinimumTableCount(db, &result, &details, "source_state", 3)
		expectTableCount(db, &result, &details, "delivery", 1)
		expectTableCount(db, &result, &details, "sent_item", 2)
		expectRuntimeConfigValue(db, &result, &details, "max_delivery_items", "2")
		expectMarkdownBulletCount(&result, &details, finalMessage, 2)
		expectRecordedDeliveryMessage(db, &result, &details, finalMessage)
	case "github-release-source-must-include":
		expectMinimumTableCount(db, &result, &details, "brief_source", 1)
		expectMinimumTableCount(db, &result, &details, "source_state", 1)
		expectMessageContains(&result, &details, finalMessage, "fixture release", "v1.2.3", "codex")
	case "repeat-run-no-new-items":
		expectMinimumTableCount(db, &result, &details, "brief_source", 1)
		expectMessageContains(&result, &details, finalMessage, "NO_REPLY", "no new")
	case "record-delivery-suppresses-repeats":
		expectMinimumTableCount(db, &result, &details, "delivery", 1)
		expectMinimumTableCount(db, &result, &details, "sent_item", 1)
	case "feed-failure-health-footnote", "feed-recovery-resolves-warning":
		expectMinimumTableCount(db, &result, &details, "health_warning", 1)
		expectMessageContains(&result, &details, finalMessage, "failed", "health", "warning")
	case "invalid-source-config-rejects":
		expectTableCount(db, &result, &details, "brief_source", 0)
		expectMessageContains(&result, &details, finalMessage, "invalid", "Bad/Key", "rejected")
	case "routine-agent-hygiene":
		expectMessageContains(&result, &details, finalMessage, "configured", "sources", "outlet")
	}
	return finishVerification(result, details)
}

func expectTableCount(db *sql.DB, result *verificationResult, details *[]string, table string, want int) {
	got, err := tableCount(db, table)
	if err != nil {
		result.DatabasePass = false
		*details = append(*details, fmt.Sprintf("count %s: %v", table, err))
		return
	}
	if got != want {
		result.DatabasePass = false
		*details = append(*details, fmt.Sprintf("%s count = %d, want %d", table, got, want))
	}
}

func expectMinimumTableCount(db *sql.DB, result *verificationResult, details *[]string, table string, want int) {
	got, err := tableCount(db, table)
	if err != nil {
		result.DatabasePass = false
		*details = append(*details, fmt.Sprintf("count %s: %v", table, err))
		return
	}
	if got < want {
		result.DatabasePass = false
		*details = append(*details, fmt.Sprintf("%s count = %d, want at least %d", table, got, want))
	}
}

func tableCount(db *sql.DB, table string) (int, error) {
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

func expectRuntimeConfigValue(db *sql.DB, result *verificationResult, details *[]string, key string, want string) {
	var got string
	if err := db.QueryRow("SELECT value_text FROM runtime_config WHERE key_name = ?", key).Scan(&got); err != nil {
		result.DatabasePass = false
		*details = append(*details, fmt.Sprintf("runtime_config %s: %v", key, err))
		return
	}
	if got != want {
		result.DatabasePass = false
		*details = append(*details, fmt.Sprintf("runtime_config %s = %q, want %q", key, got, want))
	}
}

func expectRecordedDeliveryMessage(db *sql.DB, result *verificationResult, details *[]string, want string) {
	var got string
	if err := db.QueryRow("SELECT message FROM delivery ORDER BY delivered_at DESC, id DESC LIMIT 1").Scan(&got); err != nil {
		result.DatabasePass = false
		*details = append(*details, fmt.Sprintf("delivery message: %v", err))
		return
	}
	if got != want {
		result.DatabasePass = false
		*details = append(*details, "delivery message did not match final delivered brief")
	}
}

func expectMarkdownBulletCount(result *verificationResult, details *[]string, message string, want int) {
	got := 0
	for _, line := range strings.Split(message, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "- [") {
			got++
		}
	}
	if got != want {
		result.AssistantPass = false
		*details = append(*details, fmt.Sprintf("assistant bullet count = %d, want %d", got, want))
	}
}

func expectMessageContains(result *verificationResult, details *[]string, message string, values ...string) {
	lower := strings.ToLower(message)
	for _, value := range values {
		if strings.Contains(lower, strings.ToLower(value)) {
			return
		}
	}
	result.AssistantPass = false
	*details = append(*details, fmt.Sprintf("assistant message did not contain any of %q", values))
}

func finishVerification(result verificationResult, details []string) verificationResult {
	result.Passed = result.DatabasePass && result.AssistantPass
	if len(details) > 0 {
		result.Details = strings.Join(details, "; ")
	}
	return result
}

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
