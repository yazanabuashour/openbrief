package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPrepareScenarioFixturesRewritesGitHubBlogFeed(t *testing.T) {
	current := scenario{
		ID: "fixture",
		Turns: []scenarioTurn{{
			Prompt: "Configure https://github.blog/feed/ and outlet policy named github.blog.",
		}},
	}
	rewritten, err := prepareScenarioFixtures(current, t.TempDir())
	if err != nil {
		t.Fatalf("prepareScenarioFixtures: %v", err)
	}
	if strings.Contains(rewritten.Turns[0].Prompt, "https://github.blog/feed/") ||
		strings.Contains(rewritten.Turns[0].Prompt, "named github.blog") {
		t.Fatalf("prompt was not rewritten: %s", rewritten.Turns[0].Prompt)
	}
	if !strings.Contains(rewritten.Turns[0].Prompt, "file://") {
		t.Fatalf("prompt missing eval file feed URL: %s", rewritten.Turns[0].Prompt)
	}
}

func TestPrepareScenarioFixturesRewritesConfiguredMaxDeliveryFeeds(t *testing.T) {
	current := scenario{
		ID: "configured-max-delivery-items",
		Turns: []scenarioTurn{{
			Prompt: "Use https://example.com/openbrief-limit-1.xml and https://example.com/openbrief-limit-2.xml and https://example.com/openbrief-limit-3.xml.",
		}},
	}
	rewritten, err := prepareScenarioFixtures(current, t.TempDir())
	if err != nil {
		t.Fatalf("prepareScenarioFixtures: %v", err)
	}
	if strings.Contains(rewritten.Turns[0].Prompt, "https://example.com/openbrief-limit-") {
		t.Fatalf("prompt was not rewritten: %s", rewritten.Turns[0].Prompt)
	}
	if got := strings.Count(rewritten.Turns[0].Prompt, "file://"); got != 3 {
		t.Fatalf("file URL count = %d, want 3 in %s", got, rewritten.Turns[0].Prompt)
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
		filepath.Join("docs", "agent-eval-results", "previous.md"):     "previous report",
		filepath.Join("scripts", "agent-eval", "openbrief", "main.go"): "harness",
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
		filepath.Join("docs", "agent-eval-results", "previous.md"),
		filepath.Join("scripts", "agent-eval", "openbrief", "main.go"),
	} {
		if _, err := os.Stat(filepath.Join(dst, rel)); !os.IsNotExist(err) {
			t.Fatalf("expected skipped %s: stat error = %v", rel, err)
		}
	}
}
