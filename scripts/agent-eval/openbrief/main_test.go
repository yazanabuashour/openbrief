package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestEvalCodexEnvOverridesCodexHome(t *testing.T) {
	runRoot := t.TempDir()
	t.Setenv("CODEX_HOME", filepath.Join(t.TempDir(), "personal-codex"))

	env := strings.Join(evalCodexEnv(runRoot), "\n")
	want := "CODEX_HOME=" + filepath.Join(runRoot, "codex-home")
	if !strings.Contains(env, want) {
		t.Fatalf("evalCodexEnv() missing %q in %s", want, env)
	}
	if strings.Contains(env, "personal-codex") {
		t.Fatalf("evalCodexEnv() kept personal CODEX_HOME: %s", env)
	}
}

func TestEvalEnvUsesDatabasePathAndIsolatedCodexHome(t *testing.T) {
	runRoot := t.TempDir()
	runDir := filepath.Join(runRoot, "scenario")
	dbPath := filepath.Join(runDir, "repo", "openbrief.sqlite")
	t.Setenv("OPENBRIEF_DATA_DIR", filepath.Join(t.TempDir(), "retired-data-dir"))

	env := strings.Join(evalEnv(runDir, dbPath), "\n")
	for _, want := range []string{
		"CODEX_HOME=" + filepath.Join(runRoot, "codex-home"),
		"OPENBRIEF_DATABASE_PATH=" + dbPath,
		"GOCACHE=" + filepath.Join(runDir, "gocache"),
		"GOMODCACHE=" + filepath.Join(runDir, "gomodcache"),
		"TMPDIR=" + filepath.Join(runDir, "tmp"),
	} {
		if !strings.Contains(env, want) {
			t.Fatalf("evalEnv() missing %q in %s", want, env)
		}
	}
	if strings.Contains(env, "OPENBRIEF_DATA_DIR=") {
		t.Fatalf("evalEnv() contains retired data-dir env: %s", env)
	}
}

func TestFailedScenarioErrorReportsAnyFailure(t *testing.T) {
	if err := failedScenarioError([]jobResult{{Passed: true}, {Passed: true}}); err != nil {
		t.Fatalf("failedScenarioError() = %v, want nil", err)
	}
	if err := failedScenarioError([]jobResult{{Passed: true}, {Passed: false}}); err == nil {
		t.Fatal("failedScenarioError() = nil, want error")
	}
}

func TestCopyRepoSkipsIgnoredDatabaseFiles(t *testing.T) {
	src := t.TempDir()
	dst := filepath.Join(t.TempDir(), "repo")
	files := map[string]string{
		"keep.txt":              "keep",
		"openbrief.sqlite":      "db",
		"openbrief.sqlite-wal":  "wal",
		"openbrief.sqlite-shm":  "shm",
		"local.db":              "db",
		"local.db-shm":          "shm",
		"nested/keep.md":        "keep",
		"nested/cache.sqlite":   "db",
		"nested/cache.sqlite-2": "db",
	}
	for rel, content := range files {
		path := filepath.Join(src, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	if err := copyRepo(src, dst); err != nil {
		t.Fatalf("copyRepo: %v", err)
	}
	for _, rel := range []string{"keep.txt", "nested/keep.md"} {
		if _, err := os.Stat(filepath.Join(dst, rel)); err != nil {
			t.Fatalf("expected copied %s: %v", rel, err)
		}
	}
	for _, rel := range []string{
		"openbrief.sqlite",
		"openbrief.sqlite-wal",
		"openbrief.sqlite-shm",
		"local.db",
		"local.db-shm",
		"nested/cache.sqlite",
		"nested/cache.sqlite-2",
	} {
		if _, err := os.Stat(filepath.Join(dst, rel)); !os.IsNotExist(err) {
			t.Fatalf("expected skipped %s: stat error = %v", rel, err)
		}
	}
}

func TestCodexArgsRequireIgnoreUserConfig(t *testing.T) {
	single := scenario{ID: "single", Turns: []scenarioTurn{{Prompt: "single prompt"}}}
	singleArgs := codexArgsForTurn("run-root/single/repo", "run-root/single", single, single.Turns[0], 1, "")
	if !containsArg(singleArgs, "--ephemeral") {
		t.Fatalf("single args missing --ephemeral: %v", singleArgs)
	}
	if !containsArg(singleArgs, "--ignore-user-config") {
		t.Fatalf("single args missing --ignore-user-config: %v", singleArgs)
	}

	multi := scenario{ID: "multi", Turns: []scenarioTurn{{Prompt: "first"}, {Prompt: "second"}}}
	firstArgs := codexArgsForTurn("run-root/multi/repo", "run-root/multi", multi, multi.Turns[0], 1, "")
	if containsArg(firstArgs, "--ephemeral") {
		t.Fatalf("first multi-turn args must persist the session: %v", firstArgs)
	}
	if !containsArg(firstArgs, "--ignore-user-config") {
		t.Fatalf("first multi-turn args missing --ignore-user-config: %v", firstArgs)
	}

	resumeArgs := codexArgsForTurn("run-root/multi/repo", "run-root/multi", multi, multi.Turns[1], 2, "session-123")
	if containsArg(resumeArgs, "--ephemeral") {
		t.Fatalf("resume args must not be ephemeral: %v", resumeArgs)
	}
	if !containsArg(resumeArgs, "--ignore-user-config") {
		t.Fatalf("resume args missing --ignore-user-config: %v", resumeArgs)
	}
	if resumeArgs[len(resumeArgs)-2] != "session-123" || resumeArgs[len(resumeArgs)-1] != "second" {
		t.Fatalf("resume args must end with session id and prompt: %v", resumeArgs)
	}
}

func TestSetupEvalCodexHomeCopiesOnlyAuth(t *testing.T) {
	sourceHome := filepath.Join(t.TempDir(), "source-codex")
	if err := os.MkdirAll(filepath.Join(sourceHome, "sessions"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sourceHome, "auth.json"), []byte(`{"token":"secret"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sourceHome, "config.toml"), []byte("model = \"custom\""), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sourceHome, "sessions", "session.jsonl"), []byte("session"), 0o644); err != nil {
		t.Fatal(err)
	}

	runRoot := t.TempDir()
	if err := setupEvalCodexHomeFromSource(runRoot, sourceHome); err != nil {
		t.Fatalf("setupEvalCodexHomeFromSource: %v", err)
	}
	codexHome := evalCodexHome(runRoot)
	authPath := filepath.Join(codexHome, "auth.json")
	authBytes, err := os.ReadFile(authPath)
	if err != nil {
		t.Fatalf("read copied auth: %v", err)
	}
	if string(authBytes) != `{"token":"secret"}` {
		t.Fatalf("auth content = %q, want copied source auth", authBytes)
	}
	info, err := os.Lstat(authPath)
	if err != nil {
		t.Fatalf("lstat auth: %v", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		t.Fatal("auth copy must not be a symlink")
	}
	for _, unwanted := range []string{"config.toml", filepath.Join("sessions", "session.jsonl")} {
		if _, err := os.Stat(filepath.Join(codexHome, unwanted)); !os.IsNotExist(err) {
			t.Fatalf("unexpected copied %s: stat error = %v", unwanted, err)
		}
	}
	homeInfo, err := os.Stat(codexHome)
	if err != nil {
		t.Fatalf("stat eval codex home: %v", err)
	}
	if homeInfo.Mode().Perm()&0o077 != 0 {
		t.Fatalf("eval codex home permissions = %v, want no group/other access", homeInfo.Mode().Perm())
	}
}

func TestSetupEvalCodexHomeRequiresAuth(t *testing.T) {
	err := setupEvalCodexHomeFromSource(t.TempDir(), t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "run codex login") {
		t.Fatalf("setupEvalCodexHomeFromSource() error = %v, want login guidance", err)
	}
}

func TestCountNewSessionFilesUsesEvalCodexHome(t *testing.T) {
	runRoot := t.TempDir()
	sessionsDir := filepath.Join(evalCodexHome(runRoot), "sessions")
	if err := os.MkdirAll(sessionsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	marker := time.Now()
	oldPath := filepath.Join(sessionsDir, "old.jsonl")
	newPath := filepath.Join(sessionsDir, "new.jsonl")
	otherPath := filepath.Join(sessionsDir, "other.jsonl")
	personalPath := filepath.Join(t.TempDir(), ".codex", "sessions", "new.jsonl")
	for path, content := range map[string]string{
		oldPath:      runRoot,
		newPath:      runRoot,
		otherPath:    "different run root",
		personalPath: runRoot,
	} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Chtimes(oldPath, marker.Add(-time.Hour), marker.Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(newPath, marker.Add(time.Hour), marker.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(otherPath, marker.Add(time.Hour), marker.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(personalPath, marker.Add(time.Hour), marker.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}

	if got := countNewSessionFiles(marker, runRoot); got != 1 {
		t.Fatalf("countNewSessionFiles() = %d, want 1", got)
	}
}

func containsArg(args []string, want string) bool {
	for _, arg := range args {
		if arg == want {
			return true
		}
	}
	return false
}
