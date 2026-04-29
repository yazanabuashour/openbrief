package main

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

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
