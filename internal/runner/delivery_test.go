package runner

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/yazanabuashour/openbrief/internal/runclient"
	"github.com/yazanabuashour/openbrief/internal/storage/sqlite"
)

func TestRSSRunUpdatesStateAndRepeatNoReply(t *testing.T) {
	ctx := context.Background()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(rssFixture("First item", "https://example.com/first", "guid-1")))
	}))
	defer server.Close()

	cfg := testConfig(t)
	configureSource(t, cfg, Source{
		Key:       "example",
		Label:     "Example",
		Kind:      sqlite.SourceKindRSS,
		URL:       server.URL,
		Section:   "technology",
		Threshold: sqlite.ThresholdMedium,
		Enabled:   true,
	})

	first, err := RunBriefTask(ctx, cfg, BriefTaskRequest{Action: BriefActionRun})
	if err != nil {
		t.Fatalf("first RunBriefTask: %v", err)
	}
	if first.Rejected || len(first.Candidates) != 1 || first.Candidates[0].Title != "First item" {
		t.Fatalf("first = %+v", first)
	}
	if len(first.PreviousBriefs) != 0 {
		t.Fatalf("first PreviousBriefs = %+v, want empty", first.PreviousBriefs)
	}
	second, err := RunBriefTask(ctx, cfg, BriefTaskRequest{Action: BriefActionRun})
	if err != nil {
		t.Fatalf("second RunBriefTask: %v", err)
	}
	if second.Summary != "NO_REPLY" || len(second.Candidates) != 0 {
		t.Fatalf("second = %+v", second)
	}
}

func TestEvalFileFeedRunUpdatesState(t *testing.T) {
	t.Setenv("OPENBRIEF_EVAL_ALLOW_FILE_URLS", "1")
	ctx := context.Background()
	feedPath := filepath.Join(t.TempDir(), "feed.xml")
	if err := os.WriteFile(feedPath, []byte(rssFixture("File item", "https://example.com/file", "file-guid")), 0o644); err != nil {
		t.Fatalf("write feed fixture: %v", err)
	}

	cfg := testConfig(t)
	configureSource(t, cfg, Source{
		Key:       "file-feed",
		Label:     "File Feed",
		Kind:      sqlite.SourceKindRSS,
		URL:       "file://" + feedPath,
		Section:   "technology",
		Threshold: sqlite.ThresholdMedium,
		Enabled:   true,
	})

	result, err := RunBriefTask(ctx, cfg, BriefTaskRequest{Action: BriefActionRun})
	if err != nil {
		t.Fatalf("RunBriefTask: %v", err)
	}
	if result.Rejected || len(result.Candidates) != 1 || result.Candidates[0].Title != "File item" {
		t.Fatalf("result = %+v", result)
	}
}

func TestRecordDeliverySuppressesRecentItem(t *testing.T) {
	ctx := context.Background()
	guid := "guid-1"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(rssFixture("Same item", "https://example.com/same", guid)))
	}))
	defer server.Close()

	cfg := testConfig(t)
	configureSource(t, cfg, Source{
		Key:       "example",
		Label:     "Example",
		Kind:      sqlite.SourceKindRSS,
		URL:       server.URL,
		Section:   "technology",
		Threshold: sqlite.ThresholdMedium,
		Enabled:   true,
	})

	first, err := RunBriefTask(ctx, cfg, BriefTaskRequest{Action: BriefActionRun})
	if err != nil {
		t.Fatalf("first RunBriefTask: %v", err)
	}
	_, err = RunBriefTask(ctx, cfg, BriefTaskRequest{
		Action:  BriefActionRecordDelivery,
		RunID:   first.RunID,
		Message: "- [Same item](<https://example.com/same>)",
	})
	if err != nil {
		t.Fatalf("record delivery: %v", err)
	}

	guid = "guid-2"
	second, err := RunBriefTask(ctx, cfg, BriefTaskRequest{Action: BriefActionRun})
	if err != nil {
		t.Fatalf("second RunBriefTask: %v", err)
	}
	if len(second.Candidates) != 0 || len(second.Suppressed) != 1 || second.Suppressed[0].Reason != "recently_sent" {
		t.Fatalf("second = %+v", second)
	}
}

func TestRunBriefIncludesPreviousTwoBriefs(t *testing.T) {
	ctx := context.Background()
	current := 1
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(rssFixture(
			fmt.Sprintf("History story %d", current),
			fmt.Sprintf("https://example.com/history-%d", current),
			fmt.Sprintf("history-guid-%d", current),
		)))
	}))
	defer server.Close()

	cfg := testConfig(t)
	configureSource(t, cfg, Source{
		Key:       "history",
		Label:     "History",
		Kind:      sqlite.SourceKindRSS,
		URL:       server.URL,
		Section:   "technology",
		Threshold: sqlite.ThresholdMedium,
		Enabled:   true,
	})

	for i := 1; i <= 3; i++ {
		current = i
		result, err := RunBriefTask(ctx, cfg, BriefTaskRequest{Action: BriefActionRun})
		if err != nil {
			t.Fatalf("run %d RunBriefTask: %v", i, err)
		}
		message := fmt.Sprintf("- [History story %d](<https://example.com/history-%d>)", i, i)
		_, err = RunBriefTask(ctx, cfg, BriefTaskRequest{
			Action:  BriefActionRecordDelivery,
			RunID:   result.RunID,
			Message: message,
		})
		if err != nil {
			t.Fatalf("record delivery %d: %v", i, err)
		}
	}

	current = 4
	result, err := RunBriefTask(ctx, cfg, BriefTaskRequest{Action: BriefActionRun})
	if err != nil {
		t.Fatalf("fourth RunBriefTask: %v", err)
	}
	if len(result.Candidates) != 1 || result.Candidates[0].Title != "History story 4" {
		t.Fatalf("fourth result candidates = %+v", result.Candidates)
	}
	if len(result.PreviousBriefs) != 2 {
		t.Fatalf("PreviousBriefs = %+v, want 2", result.PreviousBriefs)
	}
	if result.PreviousBriefs[0].Message != "- [History story 3](<https://example.com/history-3>)" ||
		result.PreviousBriefs[1].Message != "- [History story 2](<https://example.com/history-2>)" {
		t.Fatalf("PreviousBriefs = %+v", result.PreviousBriefs)
	}
	if result.PreviousBriefs[0].DeliveredAt == "" || result.PreviousBriefs[1].DeliveredAt == "" {
		t.Fatalf("PreviousBriefs missing delivered_at: %+v", result.PreviousBriefs)
	}
}

func TestRecordDeliveryStoresNoReplyWithoutSentItems(t *testing.T) {
	ctx := context.Background()
	cfg := testConfig(t)

	_, err := RunBriefTask(ctx, cfg, BriefTaskRequest{
		Action:  BriefActionRecordDelivery,
		RunID:   "run-no-reply",
		Message: "NO_REPLY",
	})
	if err != nil {
		t.Fatalf("record NO_REPLY delivery: %v", err)
	}

	rt, err := runclient.Open(ctx, cfg)
	if err != nil {
		t.Fatalf("open runtime: %v", err)
	}
	defer func() {
		_ = rt.Close()
	}()
	deliveries, err := rt.Store().RecentDeliveries(ctx, 2)
	if err != nil {
		t.Fatalf("RecentDeliveries: %v", err)
	}
	if len(deliveries) != 1 || deliveries[0].RunID != "run-no-reply" || deliveries[0].Message != "NO_REPLY" {
		t.Fatalf("deliveries = %+v", deliveries)
	}
	sent, err := rt.Store().RecentSentItems(ctx, time.Time{})
	if err != nil {
		t.Fatalf("RecentSentItems: %v", err)
	}
	if len(sent) != 0 {
		t.Fatalf("sent items = %+v, want none", sent)
	}
}

func TestAlwaysReportBypassesRecentSuppression(t *testing.T) {
	ctx := context.Background()
	guid := "guid-1"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(rssFixture("Always story", "https://example.com/always", guid)))
	}))
	defer server.Close()

	cfg := testConfig(t)
	configureSources(t, cfg, []Source{{
		Key:          "always",
		Label:        "Always",
		Kind:         sqlite.SourceKindRSS,
		URL:          server.URL,
		Section:      "blogs",
		Threshold:    sqlite.ThresholdMedium,
		Enabled:      true,
		AlwaysReport: true,
	}})

	first, err := RunBriefTask(ctx, cfg, BriefTaskRequest{Action: BriefActionRun})
	if err != nil {
		t.Fatalf("first RunBriefTask: %v", err)
	}
	_, err = RunBriefTask(ctx, cfg, BriefTaskRequest{
		Action:  BriefActionRecordDelivery,
		RunID:   first.RunID,
		Message: "- [Always story](<https://example.com/always>)",
	})
	if err != nil {
		t.Fatalf("record delivery: %v", err)
	}
	guid = "guid-2"
	second, err := RunBriefTask(ctx, cfg, BriefTaskRequest{Action: BriefActionRun})
	if err != nil {
		t.Fatalf("second RunBriefTask: %v", err)
	}
	if len(second.MustInclude) != 1 || len(second.SuppressedRecent) != 0 {
		t.Fatalf("second = %+v", second)
	}
}

func TestGitHubReleaseSourceIsMustInclude(t *testing.T) {
	ctx := context.Background()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode([]map[string]any{{
			"tag_name":     "v1.2.3",
			"name":         "v1.2.3",
			"html_url":     "https://github.com/acme/tool/releases/tag/v1.2.3",
			"published_at": "2026-04-23T00:00:00Z",
			"draft":        false,
			"prerelease":   false,
		}})
	}))
	defer server.Close()

	cfg := testConfig(t)
	configureSource(t, cfg, Source{
		Key:       "tool",
		Label:     "Tool",
		Kind:      sqlite.SourceKindGitHubRelease,
		URL:       server.URL,
		Repo:      "acme/tool",
		Section:   "releases",
		Threshold: sqlite.ThresholdAlways,
		Enabled:   true,
	})

	result, err := RunBriefTask(ctx, cfg, BriefTaskRequest{Action: BriefActionRun})
	if err != nil {
		t.Fatalf("RunBriefTask: %v", err)
	}
	if len(result.MustInclude) != 1 || result.MustInclude[0].Title != "acme/tool v1.2.3" {
		t.Fatalf("result = %+v", result)
	}
}

func TestParseDeliveryMessage(t *testing.T) {
	items := parseDeliveryMessage("- [A](<https://example.com/a>)\nnot a bullet\n- [B](<https://example.com/b>)")
	if len(items) != 2 || items[0].Title != "A" || items[1].URL != "https://example.com/b" {
		t.Fatalf("items = %+v", items)
	}
}
