package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

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

func installEvalSkill(runRepo string) error {
	source := filepath.Join(runRepo, "skills", "openbrief", "SKILL.md")
	target := filepath.Join(runRepo, ".agents", "skills", "openbrief", "SKILL.md")
	return copyFile(source, target, 0o644)
}
