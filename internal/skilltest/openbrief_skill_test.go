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
		"replace_sources",
		"set_brief_options",
		"max_delivery_items",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("skill missing %q", want)
		}
	}
}

func TestOpenBriefSkillPreservesPreviousBriefMessages(t *testing.T) {
	t.Parallel()

	content, err := os.ReadFile(filepath.Join(openBriefSkillDir(t), "SKILL.md"))
	if err != nil {
		t.Fatalf("read skill: %v", err)
	}
	text := string(content)
	for _, want := range []string{
		"`record_delivery` result includes `final_answer`",
		"ignore `run_brief.previous_briefs`",
		"latest three delivery records",
		"render that entry's `message` exactly as recorded",
		"Do not summarize, paraphrase, strip links",
		"Delivered 7 items, including",
		"Preserve Markdown links",
		"`NO_REPLY`",
		"health footnote",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("skill missing previous brief rendering rule %q", want)
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
