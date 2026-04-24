package runner

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/yazanabuashour/openbrief/internal/runclient"
	"github.com/yazanabuashour/openbrief/internal/storage/sqlite"
)

func TestRunBriefRejectsNoEnabledSources(t *testing.T) {
	result, err := RunBriefTask(context.Background(), testConfig(t), BriefTaskRequest{Action: BriefActionRun})
	if err != nil {
		t.Fatalf("RunBriefTask: %v", err)
	}
	if !result.Rejected || result.RejectionReason != "no enabled sources configured" {
		t.Fatalf("result = %+v", result)
	}
}

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
	second, err := RunBriefTask(ctx, cfg, BriefTaskRequest{Action: BriefActionRun})
	if err != nil {
		t.Fatalf("second RunBriefTask: %v", err)
	}
	if second.Summary != "NO_REPLY" || len(second.Candidates) != 0 {
		t.Fatalf("second = %+v", second)
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

func TestHealthWarningNewAndResolved(t *testing.T) {
	ctx := context.Background()
	fail := true
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if fail {
			http.Error(w, "bad", http.StatusBadGateway)
			return
		}
		_, _ = w.Write([]byte(rssFixture("Recovered", "https://example.com/recovered", "guid-ok")))
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
	if first.HealthFootnote == "" || len(first.HealthDelta.NewWarnings) != 1 {
		t.Fatalf("first = %+v", first)
	}

	fail = false
	second, err := RunBriefTask(ctx, cfg, BriefTaskRequest{Action: BriefActionRun})
	if err != nil {
		t.Fatalf("second RunBriefTask: %v", err)
	}
	if second.HealthFootnote == "" || len(second.HealthDelta.ResolvedWarnings) != 1 {
		t.Fatalf("second = %+v", second)
	}
}

func TestParseDeliveryMessage(t *testing.T) {
	items := parseDeliveryMessage("- [A](<https://example.com/a>)\nnot a bullet\n- [B](<https://example.com/b>)")
	if len(items) != 2 || items[0].Title != "A" || items[1].URL != "https://example.com/b" {
		t.Fatalf("items = %+v", items)
	}
}

func TestParseFeedRejectsNonFeedXML(t *testing.T) {
	_, err := parseFeed([]byte(`<?xml version="1.0"?><error><message>bad</message></error>`))
	if err == nil {
		t.Fatal("parseFeed accepted non-feed XML")
	}
}

func testConfig(t *testing.T) runclient.Config {
	t.Helper()
	return runclient.Config{DatabasePath: filepath.Join(t.TempDir(), "data", "openbrief.sqlite")}
}

func configureSource(t *testing.T, cfg runclient.Config, source Source) {
	t.Helper()
	result, err := RunConfigTask(context.Background(), cfg, ConfigTaskRequest{
		Action:  ConfigActionReplaceSources,
		Sources: []Source{source},
	})
	if err != nil {
		t.Fatalf("RunConfigTask: %v", err)
	}
	if result.Rejected {
		t.Fatalf("config rejected: %s", result.RejectionReason)
	}
}

func rssFixture(title, link, guid string) string {
	return `<?xml version="1.0"?>
<rss version="2.0"><channel>
<item><title>` + title + `</title><link>` + link + `</link><guid>` + guid + `</guid><pubDate>Thu, 23 Apr 2026 00:00:00 GMT</pubDate></item>
</channel></rss>`
}
