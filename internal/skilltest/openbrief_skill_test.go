package skilltest_test

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestOpenBriefSkillPayloadContainsOnlySkillMarkdown(t *testing.T) {
	t.Parallel()

	entries, err := os.ReadDir(openBriefSkillDir(t))
	if err != nil {
		t.Fatalf("read skill dir: %v", err)
	}
	if len(entries) != 1 || entries[0].Name() != "SKILL.md" || entries[0].IsDir() {
		t.Fatalf("skill payload entries = %v, want exactly SKILL.md", entries)
	}
}

func TestOpenBriefSkillUsesInstalledRunnerAndDBOnlyConfig(t *testing.T) {
	t.Parallel()

	content, err := os.ReadFile(filepath.Join(openBriefSkillDir(t), "SKILL.md"))
	if err != nil {
		t.Fatalf("read skill: %v", err)
	}
	text := string(content)
	for _, want := range []string{
		"openbrief config",
		"openbrief brief",
		"OPENBRIEF_DATABASE_PATH",
		"run_brief",
		"record_delivery",
		"replace_sources",
		"github_release",
		"url_canonicalization",
		"outlet_extraction",
		"suppressed_policy",
		"NO_REPLY",
		"Do not run `openbrief --help`",
		"Do not maintain repo-local state files",
		"perform a production OpenBrief task by bypassing the installed runner",
		"directly as a substitute for runner JSON",
		"Allowed Contexts",
		"migration design may inspect repository files",
		"Legacy Migration",
		"user explicitly points to",
		"draft OpenBrief sources and outlet policies",
		"apply only after approval",
		"delivery history, latest-seen state, run state",
		"inferred private configuration without user review",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("skill missing %q", want)
		}
	}
	for _, forbidden := range []string{
		"OPENBRIEF_DATA_DIR",
		"brief-fetch.ts",
		"BRIEF_PAYWALL_POLICY",
		"BRIEF_SOURCES",
		"home-openclaw",
		"/Volumes/",
		"/Users/",
		"migration/import tooling is available",
		"go run ./cmd/openbrief",
		"inspect source files, generated files, repo files",
		"workspace backups, private run logs, or legacy brief scripts",
		"recovery/import from private historical artifacts",
		"recover, infer, or import private source inventory",
		"Private artifacts must not be used as authoritative production configuration",
		"Do not infer or recover them from private backups or old personal files",
	} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("skill contains forbidden text %q", forbidden)
		}
	}
}

func openBriefSkillDir(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve test path")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", "skills", "openbrief"))
}
