package main

import (
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

var (
	skillNamePattern    = regexp.MustCompile(`^[A-Za-z0-9]+(-[A-Za-z0-9]+)*$`)
	markdownLinkPattern = regexp.MustCompile(`\[[^\]]+\]\(([^)]+)\)`)
)

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string, stdout io.Writer) error {
	if len(args) != 1 {
		return errors.New("usage: scripts/validate-agent-skill.sh <skill-directory>")
	}
	skillDir := strings.TrimRight(args[0], string(os.PathSeparator))
	if skillDir == "" {
		skillDir = "."
	}
	if err := validateSkillDir(skillDir); err != nil {
		return err
	}
	_, err := fmt.Fprintf(stdout, "validated %s\n", skillDir)
	return err
}

func validateSkillDir(skillDir string) error {
	info, err := os.Stat(skillDir)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("skill directory not found: %s", skillDir)
		}
		return fmt.Errorf("stat skill directory %s: %w", skillDir, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("skill directory not found: %s", skillDir)
	}

	entries, err := os.ReadDir(skillDir)
	if err != nil {
		return fmt.Errorf("read skill directory %s: %w", skillDir, err)
	}
	if len(entries) != 1 || entries[0].Name() != "SKILL.md" || entries[0].IsDir() {
		names := make([]string, 0, len(entries))
		for _, entry := range entries {
			names = append(names, entry.Name())
		}
		sort.Strings(names)
		return fmt.Errorf("%s must contain only SKILL.md; found %s", skillDir, strings.Join(names, ", "))
	}

	skillFile := filepath.Join(skillDir, "SKILL.md")
	contentBytes, err := os.ReadFile(skillFile)
	if err != nil {
		return fmt.Errorf("read %s: %w", skillFile, err)
	}
	content := strings.ReplaceAll(string(contentBytes), "\r\n", "\n")
	metadata, body, err := parseFrontmatter(skillFile, content)
	if err != nil {
		return err
	}
	if err := validateMetadata(skillDir, skillFile, metadata); err != nil {
		return err
	}
	if err := validateRunnerContract(skillFile, body); err != nil {
		return err
	}
	if err := validateMarkdownLinks(skillDir, skillFile, body); err != nil {
		return err
	}
	if err := validateForbiddenGuidance(skillFile, content); err != nil {
		return err
	}
	return nil
}

func parseFrontmatter(skillFile string, content string) (map[string]string, string, error) {
	lines := strings.Split(content, "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return nil, "", fmt.Errorf("%s must start with YAML frontmatter delimited by ---", skillFile)
	}
	closingLine := -1
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			closingLine = i
			break
		}
	}
	if closingLine == -1 {
		return nil, "", fmt.Errorf("%s must include a closing --- line for YAML frontmatter", skillFile)
	}
	if closingLine == 1 {
		return nil, "", fmt.Errorf("%s frontmatter must contain required fields", skillFile)
	}
	metadata := map[string]string{}
	for i, line := range lines[1:closingLine] {
		if strings.TrimSpace(line) == "" {
			continue
		}
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			return nil, "", fmt.Errorf("%s frontmatter line %d must use key: value syntax", skillFile, i+2)
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if key == "" {
			return nil, "", fmt.Errorf("%s frontmatter line %d has an empty key", skillFile, i+2)
		}
		if _, exists := metadata[key]; exists {
			return nil, "", fmt.Errorf("%s frontmatter field %q must not be duplicated", skillFile, key)
		}
		metadata[key] = strings.Trim(value, `"'`)
	}
	return metadata, strings.Join(lines[closingLine+1:], "\n"), nil
}

func validateMetadata(skillDir string, skillFile string, metadata map[string]string) error {
	name := metadata["name"]
	if name == "" {
		return fmt.Errorf("%s frontmatter must define a non-empty name", skillFile)
	}
	if len([]rune(name)) > 64 {
		return fmt.Errorf("%s name must be 64 characters or fewer", skillFile)
	}
	if !skillNamePattern.MatchString(name) {
		return fmt.Errorf("%s name must use letters, numbers, and single hyphens only", skillFile)
	}
	parentDir := filepath.Base(skillDir)
	if !strings.EqualFold(name, parentDir) {
		return fmt.Errorf("%s name must match the parent directory (%q)", skillFile, parentDir)
	}
	for _, field := range []string{"description", "license", "compatibility"} {
		value := metadata[field]
		if value == "" {
			return fmt.Errorf("%s frontmatter must define a non-empty %s", skillFile, field)
		}
	}
	if len([]rune(metadata["description"])) > 1024 {
		return fmt.Errorf("%s description must be 1024 characters or fewer", skillFile)
	}
	if len([]rune(metadata["compatibility"])) > 500 {
		return fmt.Errorf("%s compatibility must be 500 characters or fewer", skillFile)
	}
	return nil
}

func validateRunnerContract(skillFile string, content string) error {
	required := []string{
		"openbrief config",
		"openbrief brief",
		"OPENBRIEF_DATABASE_PATH",
		"replace_sources",
		"NO_REPLY",
		"Do not maintain repo-local state files",
		"directly as a substitute for runner JSON",
		"Legacy Migration",
		"user explicitly points to",
		"draft OpenBrief sources and outlet policies",
		"apply only after approval",
		"delivery history, latest-seen state, run state",
		"inferred private configuration without user review",
	}
	for _, want := range required {
		if !strings.Contains(content, want) {
			return fmt.Errorf("%s missing required runner guidance %q", skillFile, want)
		}
	}
	return nil
}

func validateMarkdownLinks(skillDir string, skillFile string, content string) error {
	skillRoot, err := filepath.Abs(skillDir)
	if err != nil {
		return fmt.Errorf("resolve skill directory %s: %w", skillDir, err)
	}
	for _, match := range markdownLinkPattern.FindAllStringSubmatch(content, -1) {
		target := match[1]
		if shouldSkipLinkTarget(target) {
			continue
		}
		targetPath, err := filepath.Abs(filepath.Clean(filepath.Join(skillDir, target)))
		if err != nil {
			return fmt.Errorf("%s link target %q cannot be resolved: %w", skillFile, target, err)
		}
		rel, err := filepath.Rel(skillRoot, targetPath)
		if err != nil {
			return fmt.Errorf("%s link target %q cannot be compared with skill directory: %w", skillFile, target, err)
		}
		if rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
			return fmt.Errorf("%s link target %q escapes the skill directory", skillFile, target)
		}
		if _, err := os.Stat(targetPath); err != nil {
			return fmt.Errorf("%s link target %q is not installed with the skill: %w", skillFile, target, err)
		}
	}
	return nil
}

func shouldSkipLinkTarget(target string) bool {
	target = strings.Trim(target, "<>")
	if target == "" || strings.HasPrefix(target, "#") || filepath.IsAbs(target) {
		return true
	}
	if parsed, err := url.Parse(target); err == nil && parsed.Scheme != "" {
		return true
	}
	return false
}

func validateForbiddenGuidance(skillFile string, content string) error {
	forbiddenSubstrings := []string{
		"OPENBRIEF_DATA_DIR",
		"brief-fetch.ts",
		"BRIEF_PAYWALL_POLICY",
		"BRIEF_SOURCES",
		"home-openclaw",
		"/Volumes/",
		"/Users/",
		"migration/import tooling is available",
		"go run ./cmd/openbrief",
		"CLI fallback",
		"Generated Client Fallback",
		"inspect source files, generated files, repo files",
		"workspace backups, private run logs, or legacy brief scripts",
		"recovery/import from private historical artifacts",
		"recover, infer, or import private source inventory",
		"Private artifacts must not be used as authoritative production configuration",
		"Do not infer or recover them from private backups or old personal files",
	}
	for _, forbidden := range forbiddenSubstrings {
		if strings.Contains(content, forbidden) {
			return fmt.Errorf("%s contains forbidden product guidance %q", skillFile, forbidden)
		}
	}
	return nil
}
