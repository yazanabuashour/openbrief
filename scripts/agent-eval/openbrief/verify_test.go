package main

import (
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"
)

func TestParseRunOptionsSupportsReportOutput(t *testing.T) {
	options, err := parseRunOptions([]string{
		"--run-root", "run-root",
		"--scenario", "routine-agent-hygiene",
		"--report-dir", "docs/agent-eval-results",
		"--report-name", "openbrief-v0.1.0-final",
	})
	if err != nil {
		t.Fatalf("parseRunOptions: %v", err)
	}
	if options.ReportDir != "docs/agent-eval-results" || options.ReportName != "openbrief-v0.1.0-final" {
		t.Fatalf("options = %+v", options)
	}
}

func TestExpectMarkdownBulletCount(t *testing.T) {
	result := verificationResult{AssistantPass: true}
	var details []string
	expectMarkdownBulletCount(&result, &details, "- [One](<https://example.com/1>)\n- [Two](<https://example.com/2>)", 2)
	if !result.AssistantPass || len(details) != 0 {
		t.Fatalf("result = %+v details = %v", result, details)
	}
	expectMarkdownBulletCount(&result, &details, "- [One](<https://example.com/1>)", 2)
	if result.AssistantPass {
		t.Fatal("expected bullet count mismatch to fail assistant pass")
	}
}

func TestExpectRecordedDeliveryMessage(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer func() {
		_ = db.Close()
	}()
	if _, err := db.Exec(`CREATE TABLE delivery (id INTEGER PRIMARY KEY AUTOINCREMENT, message TEXT NOT NULL, delivered_at TEXT NOT NULL)`); err != nil {
		t.Fatalf("create delivery: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO delivery (message, delivered_at) VALUES (?, ?)`, "- [One](<https://example.com/1>)", "2026-04-23T01:00:00Z"); err != nil {
		t.Fatalf("insert delivery: %v", err)
	}

	result := verificationResult{DatabasePass: true}
	var details []string
	expectRecordedDeliveryMessage(db, &result, &details, "- [One](<https://example.com/1>)")
	if !result.DatabasePass || len(details) != 0 {
		t.Fatalf("result = %+v details = %v", result, details)
	}
	expectRecordedDeliveryMessage(db, &result, &details, "- [Two](<https://example.com/2>)")
	if result.DatabasePass {
		t.Fatal("expected delivery mismatch to fail database pass")
	}
}
