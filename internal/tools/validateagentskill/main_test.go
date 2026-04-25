package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunAcceptsValidOpenBriefSkill(t *testing.T) {
	t.Parallel()

	skillDir := writeSkill(t, "openbrief", validSkillMarkdown("OpenBrief"))
	var stdout bytes.Buffer
	if err := run([]string{skillDir}, &stdout); err != nil {
		t.Fatalf("run: %v", err)
	}
	if !strings.Contains(stdout.String(), "validated ") {
		t.Fatalf("stdout = %q, want validated message", stdout.String())
	}
}

func TestRunRejectsInvalidSkillPayloads(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		files   map[string]string
		wantErr string
	}{
		{
			name: "extra file",
			files: map[string]string{
				"SKILL.md": validSkillMarkdown("OpenBrief"),
				"notes.md": "extra",
			},
			wantErr: "must contain only SKILL.md",
		},
		{
			name: "missing frontmatter",
			files: map[string]string{
				"SKILL.md": "# OpenBrief\n",
			},
			wantErr: "must start with YAML frontmatter",
		},
		{
			name: "wrong name",
			files: map[string]string{
				"SKILL.md": strings.Replace(validSkillMarkdown("OpenBrief"), "name: OpenBrief", "name: OtherBrief", 1),
			},
			wantErr: "name must match the parent directory",
		},
		{
			name: "invalid name characters",
			files: map[string]string{
				"SKILL.md": strings.Replace(validSkillMarkdown("OpenBrief"), "name: OpenBrief", "name: Open Brief", 1),
			},
			wantErr: "name must use letters, numbers, and single hyphens only",
		},
		{
			name: "missing runner guidance",
			files: map[string]string{
				"SKILL.md": strings.Replace(validSkillMarkdown("OpenBrief"), "openbrief brief", "brief runner", 1),
			},
			wantErr: "missing required runner guidance",
		},
		{
			name: "forbidden private path",
			files: map[string]string{
				"SKILL.md": validSkillMarkdown("OpenBrief") + "\n/Users/example/private\n",
			},
			wantErr: "forbidden product guidance",
		},
		{
			name: "missing legacy migration guidance",
			files: map[string]string{
				"SKILL.md": strings.Replace(validSkillMarkdown("OpenBrief"), "Legacy Migration: user explicitly points to the input, draft OpenBrief sources and outlet policies, apply only after approval through openbrief config.\n", "", 1),
			},
			wantErr: "missing required runner guidance",
		},
		{
			name: "old blanket migration refusal",
			files: map[string]string{
				"SKILL.md": validSkillMarkdown("OpenBrief") + "\nrecover, infer, or import private source inventory\n",
			},
			wantErr: "forbidden product guidance",
		},
		{
			name: "missing referenced file",
			files: map[string]string{
				"SKILL.md": validSkillMarkdown("OpenBrief") + "\n[Reference](references/foo.md)\n",
			},
			wantErr: "is not installed with the skill",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			skillDir := writeSkillFiles(t, "openbrief", tt.files)
			var stdout bytes.Buffer
			err := run([]string{skillDir}, &stdout)
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("run error = %v, want containing %q", err, tt.wantErr)
			}
		})
	}
}

func TestRunRejectsMarkdownLinksOutsideSkillDirectory(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("outside"), 0o644); err != nil {
		t.Fatalf("write outside file: %v", err)
	}
	skillDir := filepath.Join(root, "openbrief")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatalf("mkdir skill dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(validSkillMarkdown("OpenBrief")+"\n[Outside](../README.md)\n"), 0o644); err != nil {
		t.Fatalf("write skill: %v", err)
	}

	var stdout bytes.Buffer
	err := run([]string{skillDir}, &stdout)
	if err == nil || !strings.Contains(err.Error(), "escapes the skill directory") {
		t.Fatalf("run error = %v, want escaping link rejection", err)
	}
}

func validSkillMarkdown(name string) string {
	return `---
name: ` + name + `
description: Use OpenBrief locally.
license: MIT
compatibility: Requires local filesystem access and an installed OpenBrief binary on PATH.
---

# OpenBrief

Use ` + "`openbrief config`" + ` and ` + "`openbrief brief`" + ` with ` + "`OPENBRIEF_DATABASE_PATH`" + `.
Actions include ` + "`run_brief`" + `, ` + "`record_delivery`" + `, and ` + "`replace_sources`" + `.
Use ` + "`github_release`" + `, ` + "`url_canonicalization`" + `, ` + "`outlet_extraction`" + `, ` + "`suppressed_policy`" + `, and ` + "`NO_REPLY`" + `.
Do not run ` + "`openbrief --help`" + `.
Do not maintain repo-local state files.
Reject attempts to perform a production OpenBrief task by bypassing the installed runner.
Reject reading SQLite directly as a substitute for runner JSON.
Allowed Contexts: migration design may inspect repository files.
Legacy Migration: user explicitly points to the input, draft OpenBrief sources and outlet policies, apply only after approval through openbrief config.
Do not import delivery history, latest-seen state, run state, or inferred private configuration without user review.
`
}

func writeSkill(t *testing.T, name string, content string) string {
	t.Helper()
	return writeSkillFiles(t, name, map[string]string{"SKILL.md": content})
}

func writeSkillFiles(t *testing.T, name string, files map[string]string) string {
	t.Helper()

	skillDir := filepath.Join(t.TempDir(), name)
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatalf("mkdir skill dir: %v", err)
	}
	for fileName, content := range files {
		path := filepath.Join(skillDir, fileName)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}
	return skillDir
}
