package main

import (
	"database/sql"
	"fmt"
	"strings"

	_ "modernc.org/sqlite"
)

func verifyScenario(dbPath string, scenarioID string, finalMessage string, metrics scenarioMetrics) verificationResult {
	result := verificationResult{Passed: true, DatabasePass: true, AssistantPass: true}
	var details []string
	if strings.TrimSpace(finalMessage) == "" {
		result.AssistantPass = false
		details = append(details, "assistant final message was empty or not exposed in JSONL")
	}
	if metrics.DirectSQLiteAccess || metrics.BroadRepoSearch || metrics.RepoInspection || metrics.EnvironmentAccess {
		result.AssistantPass = false
		details = append(details, "agent used forbidden inspection path")
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		result.DatabasePass = false
		details = append(details, fmt.Sprintf("open eval database: %v", err))
		return finishVerification(result, details)
	}
	defer func() {
		_ = db.Close()
	}()

	switch scenarioID {
	case "empty-config-rejects-run-brief":
		expectTableCount(db, &result, &details, "brief_source", 0)
		expectMessageContains(&result, &details, finalMessage, "no enabled sources", "rejected")
	case "rss-source-first-run-candidate", "rss-source-generic-processing-fields", "outlet-policy-watch-audit":
		expectMinimumTableCount(db, &result, &details, "brief_source", 1)
		expectMinimumTableCount(db, &result, &details, "source_state", 1)
	case "configured-max-delivery-items":
		expectMinimumTableCount(db, &result, &details, "brief_source", 3)
		expectMinimumTableCount(db, &result, &details, "source_state", 3)
		expectTableCount(db, &result, &details, "delivery", 1)
		expectTableCount(db, &result, &details, "sent_item", 2)
		expectRuntimeConfigValue(db, &result, &details, "max_delivery_items", "2")
		expectMarkdownBulletCount(&result, &details, finalMessage, 2)
		expectRecordedDeliveryMessage(db, &result, &details, finalMessage)
	case "github-release-source-must-include":
		expectMinimumTableCount(db, &result, &details, "brief_source", 1)
		expectMinimumTableCount(db, &result, &details, "source_state", 1)
		expectMessageContains(&result, &details, finalMessage, "fixture release", "v1.2.3", "codex")
	case "repeat-run-no-new-items":
		expectMinimumTableCount(db, &result, &details, "brief_source", 1)
		expectMessageContains(&result, &details, finalMessage, "NO_REPLY", "no new")
	case "record-delivery-suppresses-repeats":
		expectMinimumTableCount(db, &result, &details, "delivery", 1)
		expectMinimumTableCount(db, &result, &details, "sent_item", 1)
	case "feed-failure-health-footnote", "feed-recovery-resolves-warning":
		expectMinimumTableCount(db, &result, &details, "health_warning", 1)
		expectMessageContains(&result, &details, finalMessage, "failed", "health", "warning")
	case "invalid-source-config-rejects":
		expectTableCount(db, &result, &details, "brief_source", 0)
		expectMessageContains(&result, &details, finalMessage, "invalid", "Bad/Key", "rejected")
	case "routine-agent-hygiene":
		expectMessageContains(&result, &details, finalMessage, "configured", "sources", "outlet")
	}
	return finishVerification(result, details)
}

func expectTableCount(db *sql.DB, result *verificationResult, details *[]string, table string, want int) {
	got, err := tableCount(db, table)
	if err != nil {
		result.DatabasePass = false
		*details = append(*details, fmt.Sprintf("count %s: %v", table, err))
		return
	}
	if got != want {
		result.DatabasePass = false
		*details = append(*details, fmt.Sprintf("%s count = %d, want %d", table, got, want))
	}
}

func expectMinimumTableCount(db *sql.DB, result *verificationResult, details *[]string, table string, want int) {
	got, err := tableCount(db, table)
	if err != nil {
		result.DatabasePass = false
		*details = append(*details, fmt.Sprintf("count %s: %v", table, err))
		return
	}
	if got < want {
		result.DatabasePass = false
		*details = append(*details, fmt.Sprintf("%s count = %d, want at least %d", table, got, want))
	}
}

func tableCount(db *sql.DB, table string) (int, error) {
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

func expectRuntimeConfigValue(db *sql.DB, result *verificationResult, details *[]string, key string, want string) {
	var got string
	if err := db.QueryRow("SELECT value_text FROM runtime_config WHERE key_name = ?", key).Scan(&got); err != nil {
		result.DatabasePass = false
		*details = append(*details, fmt.Sprintf("runtime_config %s: %v", key, err))
		return
	}
	if got != want {
		result.DatabasePass = false
		*details = append(*details, fmt.Sprintf("runtime_config %s = %q, want %q", key, got, want))
	}
}

func expectRecordedDeliveryMessage(db *sql.DB, result *verificationResult, details *[]string, want string) {
	var got string
	if err := db.QueryRow("SELECT message FROM delivery ORDER BY delivered_at DESC, id DESC LIMIT 1").Scan(&got); err != nil {
		result.DatabasePass = false
		*details = append(*details, fmt.Sprintf("delivery message: %v", err))
		return
	}
	if got != want {
		result.DatabasePass = false
		*details = append(*details, "delivery message did not match final delivered brief")
	}
}

func expectMarkdownBulletCount(result *verificationResult, details *[]string, message string, want int) {
	got := 0
	for _, line := range strings.Split(message, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "- [") {
			got++
		}
	}
	if got != want {
		result.AssistantPass = false
		*details = append(*details, fmt.Sprintf("assistant bullet count = %d, want %d", got, want))
	}
}

func expectMessageContains(result *verificationResult, details *[]string, message string, values ...string) {
	lower := strings.ToLower(message)
	for _, value := range values {
		if strings.Contains(lower, strings.ToLower(value)) {
			return
		}
	}
	result.AssistantPass = false
	*details = append(*details, fmt.Sprintf("assistant message did not contain any of %q", values))
}

func finishVerification(result verificationResult, details []string) verificationResult {
	result.Passed = result.DatabasePass && result.AssistantPass
	if len(details) > 0 {
		result.Details = strings.Join(details, "; ")
	}
	return result
}
