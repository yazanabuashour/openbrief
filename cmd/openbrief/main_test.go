package main

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

func TestVersionCommand(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"--version"}, strings.NewReader(""), &stdout, &stderr)
	if code != 0 {
		t.Fatalf("code = %d stderr = %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "openbrief ") {
		t.Fatalf("stdout = %q", stdout.String())
	}
}

func TestConfigInitWithDBFlag(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "custom", "openbrief.sqlite")
	var stdout, stderr bytes.Buffer
	code := run([]string{"config", "--db", dbPath}, strings.NewReader(`{"action":"init"}`), &stdout, &stderr)
	if code != 0 {
		t.Fatalf("code = %d stderr = %s", code, stderr.String())
	}
	var result struct {
		Paths struct {
			DatabasePath string `json:"database_path"`
		} `json:"paths"`
		Summary string `json:"summary"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("decode stdout: %v\n%s", err, stdout.String())
	}
	if result.Paths.DatabasePath != dbPath {
		t.Fatalf("DatabasePath = %q, want %q", result.Paths.DatabasePath, dbPath)
	}
}

func TestBriefRejectsUnknownField(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"brief", "--db", filepath.Join(t.TempDir(), "openbrief.sqlite")}, strings.NewReader(`{"action":"validate","extra":true}`), &stdout, &stderr)
	if code == 0 {
		t.Fatal("command succeeded with unknown field")
	}
	if !strings.Contains(stderr.String(), "unknown field") {
		t.Fatalf("stderr = %q", stderr.String())
	}
}
