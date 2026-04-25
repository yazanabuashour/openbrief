package runner

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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

func TestOutletPolicyBlocksBeforeNewItemSelection(t *testing.T) {
	ctx := context.Background()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`<?xml version="1.0"?>
<rss version="2.0"><channel>
<item><title>Blocked story - Blocked Outlet</title><link>https://example.com/blocked</link><guid>blocked</guid><pubDate>Thu, 23 Apr 2026 01:00:00 GMT</pubDate></item>
<item><title>Open story - Open Outlet</title><link>https://example.com/open</link><guid>open</guid><pubDate>Thu, 23 Apr 2026 00:00:00 GMT</pubDate></item>
</channel></rss>`))
	}))
	defer server.Close()

	cfg := testConfig(t)
	configureSources(t, cfg, []Source{{
		Key:              "news",
		Label:            "News",
		Kind:             sqlite.SourceKindRSS,
		URL:              server.URL,
		Section:          "technology",
		Threshold:        sqlite.ThresholdMedium,
		Enabled:          true,
		OutletExtraction: sqlite.OutletExtractionTitleSuffix,
	}})
	configureOutletPolicies(t, cfg, []sqlite.OutletPolicy{{
		Name:    "Blocked Outlet",
		Policy:  "block",
		Enabled: true,
	}})

	result, err := RunBriefTask(ctx, cfg, BriefTaskRequest{Action: BriefActionRun})
	if err != nil {
		t.Fatalf("RunBriefTask: %v", err)
	}
	if len(result.Candidates) != 1 || result.Candidates[0].Title != "Open story - Open Outlet" {
		t.Fatalf("candidates = %+v", result.Candidates)
	}
	if len(result.SuppressedPolicy) != 1 || result.SuppressedPolicy[0].Outlet != "Blocked Outlet" {
		t.Fatalf("suppressed policy = %+v", result.SuppressedPolicy)
	}
}

func TestOutletPolicyWatchAuditsButKeepsCandidate(t *testing.T) {
	ctx := context.Background()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(rssFixture("Watched story - Watch Outlet", "https://example.com/watch", "watch-guid")))
	}))
	defer server.Close()

	cfg := testConfig(t)
	configureSources(t, cfg, []Source{{
		Key:              "news",
		Label:            "News",
		Kind:             sqlite.SourceKindRSS,
		URL:              server.URL,
		Section:          "technology",
		Threshold:        sqlite.ThresholdMedium,
		Enabled:          true,
		OutletExtraction: sqlite.OutletExtractionTitleSuffix,
	}})
	configureOutletPolicies(t, cfg, []sqlite.OutletPolicy{{
		Name:    "Watch Outlet",
		Policy:  "watch",
		Enabled: true,
	}})

	result, err := RunBriefTask(ctx, cfg, BriefTaskRequest{Action: BriefActionRun})
	if err != nil {
		t.Fatalf("RunBriefTask: %v", err)
	}
	if len(result.Candidates) != 1 {
		t.Fatalf("candidates = %+v", result.Candidates)
	}
	if len(result.SuppressedPolicy) != 1 || result.SuppressedPolicy[0].Policy != "watch" {
		t.Fatalf("suppressed policy = %+v", result.SuppressedPolicy)
	}
}

func TestSameRunTopicDedupKeepsPreferredSource(t *testing.T) {
	ctx := context.Background()
	serverA := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(rssFixture("Sony raises PlayStation 5 prices in US", "https://example.com/a", "a-guid")))
	}))
	defer serverA.Close()
	serverB := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(rssFixture("Sony raises PlayStation 5 prices in US", "https://example.com/b", "b-guid")))
	}))
	defer serverB.Close()

	cfg := testConfig(t)
	configureSources(t, cfg, []Source{
		{
			Key:          "preferred",
			Label:        "Preferred",
			Kind:         sqlite.SourceKindRSS,
			URL:          serverA.URL,
			Section:      "technology",
			Threshold:    sqlite.ThresholdMedium,
			Enabled:      true,
			DedupGroup:   "news",
			PriorityRank: 1,
		},
		{
			Key:          "generic",
			Label:        "Generic",
			Kind:         sqlite.SourceKindRSS,
			URL:          serverB.URL,
			Section:      "technology",
			Threshold:    sqlite.ThresholdMedium,
			Enabled:      true,
			DedupGroup:   "news",
			PriorityRank: 5,
		},
	})

	result, err := RunBriefTask(ctx, cfg, BriefTaskRequest{Action: BriefActionRun})
	if err != nil {
		t.Fatalf("RunBriefTask: %v", err)
	}
	if len(result.Candidates) != 1 || result.Candidates[0].SourceKey != "preferred" {
		t.Fatalf("candidates = %+v", result.Candidates)
	}
	if len(result.Suppressed) != 1 || result.Suppressed[0].Reason != "same_run_duplicate" {
		t.Fatalf("suppressed = %+v", result.Suppressed)
	}
}

func TestBlankDedupGroupDoesNotCollapseAcrossSources(t *testing.T) {
	ctx := context.Background()
	serverA := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(rssFixture("Shared headline", "https://example.com/a", "a-guid")))
	}))
	defer serverA.Close()
	serverB := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(rssFixture("Shared headline", "https://example.com/b", "b-guid")))
	}))
	defer serverB.Close()

	cfg := testConfig(t)
	configureSources(t, cfg, []Source{
		{
			Key:       "first",
			Label:     "First",
			Kind:      sqlite.SourceKindRSS,
			URL:       serverA.URL,
			Section:   "technology",
			Threshold: sqlite.ThresholdMedium,
			Enabled:   true,
		},
		{
			Key:       "second",
			Label:     "Second",
			Kind:      sqlite.SourceKindRSS,
			URL:       serverB.URL,
			Section:   "technology",
			Threshold: sqlite.ThresholdMedium,
			Enabled:   true,
		},
	})

	result, err := RunBriefTask(ctx, cfg, BriefTaskRequest{Action: BriefActionRun})
	if err != nil {
		t.Fatalf("RunBriefTask: %v", err)
	}
	if len(result.Candidates) != 2 || len(result.Suppressed) != 0 {
		t.Fatalf("result = %+v", result)
	}
}

func TestFeedBurnerRedirectCanonicalization(t *testing.T) {
	ctx := context.Background()
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/feed":
			_, _ = w.Write([]byte(rssFixture("Redirected story", server.URL+"/redirect", "redirect-guid")))
		case "/redirect":
			http.Redirect(w, r, server.URL+"/final", http.StatusFound)
		case "/final":
			_, _ = w.Write([]byte("ok"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	cfg := testConfig(t)
	configureSources(t, cfg, []Source{{
		Key:                 "feed",
		Label:               "Feed",
		Kind:                sqlite.SourceKindRSS,
		URL:                 server.URL + "/feed",
		Section:             "technology",
		Threshold:           sqlite.ThresholdMedium,
		Enabled:             true,
		URLCanonicalization: sqlite.URLCanonicalizationFeedBurnerRedirect,
	}})

	result, err := RunBriefTask(ctx, cfg, BriefTaskRequest{Action: BriefActionRun})
	if err != nil {
		t.Fatalf("RunBriefTask: %v", err)
	}
	if len(result.Candidates) != 1 || result.Candidates[0].URL != server.URL+"/final" {
		t.Fatalf("candidates = %+v", result.Candidates)
	}
}

func TestURLHostOutletExtractionUsesCanonicalURL(t *testing.T) {
	ctx := context.Background()
	publisher := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}))
	defer publisher.Close()
	publisherURL := strings.Replace(publisher.URL, "127.0.0.1", "localhost", 1)
	var feed *httptest.Server
	feed = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/feed":
			_, _ = w.Write([]byte(rssFixture("Redirected publisher story", feed.URL+"/redirect", "redirect-guid")))
		case "/redirect":
			http.Redirect(w, r, publisherURL+"/story", http.StatusFound)
		default:
			http.NotFound(w, r)
		}
	}))
	defer feed.Close()

	cfg := testConfig(t)
	configureSources(t, cfg, []Source{{
		Key:                 "feed",
		Label:               "Feed",
		Kind:                sqlite.SourceKindRSS,
		URL:                 feed.URL + "/feed",
		Section:             "technology",
		Threshold:           sqlite.ThresholdMedium,
		Enabled:             true,
		URLCanonicalization: sqlite.URLCanonicalizationFeedBurnerRedirect,
		OutletExtraction:    sqlite.OutletExtractionURLHost,
	}})
	configureOutletPolicies(t, cfg, []sqlite.OutletPolicy{{
		Name:    "localhost",
		Policy:  "block",
		Enabled: true,
	}})

	result, err := RunBriefTask(ctx, cfg, BriefTaskRequest{Action: BriefActionRun})
	if err != nil {
		t.Fatalf("RunBriefTask: %v", err)
	}
	if len(result.Candidates) != 0 || len(result.SuppressedPolicy) != 1 {
		t.Fatalf("result = %+v", result)
	}
	if result.SuppressedPolicy[0].Outlet != "localhost" {
		t.Fatalf("suppressed policy = %+v", result.SuppressedPolicy)
	}
}

func TestGoogleNewsArticleURLResolverWithCustomEndpoints(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/articles/article-id":
			_, _ = w.Write([]byte(`<c-wiz data-n-a-sg="signature" data-n-a-ts="1775407930"></c-wiz>`))
		case "/batch":
			_ = r.ParseForm()
			if !strings.Contains(r.Form.Get("f.req"), "article-id") {
				t.Fatalf("batch body missing article id: %s", r.Form.Get("f.req"))
			}
			_, _ = w.Write([]byte(")]}'\n\n[[\"wrb.fr\",\"Fbv4je\",\"[null,\\\"https://publisher.example/story\\\"]\"]]"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	resolved, err := newFetcher().resolveGoogleNewsArticleURLWithEndpoints(
		context.Background(),
		"https://news.google.com/rss/articles/article-id?hl=en-US",
		server.URL+"/articles/",
		server.URL+"/batch",
	)
	if err != nil {
		t.Fatalf("resolveGoogleNewsArticleURLWithEndpoints: %v", err)
	}
	if resolved != "https://publisher.example/story" {
		t.Fatalf("resolved = %q", resolved)
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

func TestStaleHeartbeatIsInternalOnly(t *testing.T) {
	ctx := context.Background()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(rssFixture("Quiet item", "https://example.com/quiet", "quiet-guid")))
	}))
	defer server.Close()

	cfg := testConfig(t)
	configureSource(t, cfg, Source{
		Key:       "quiet",
		Label:     "Quiet",
		Kind:      sqlite.SourceKindRSS,
		URL:       server.URL,
		Section:   "technology",
		Threshold: sqlite.ThresholdMedium,
		Enabled:   true,
	})
	if _, err := RunBriefTask(ctx, cfg, BriefTaskRequest{Action: BriefActionRun}); err != nil {
		t.Fatalf("first RunBriefTask: %v", err)
	}
	store, err := sqlite.New(ctx, sqlite.Config{DatabasePath: cfg.DatabasePath})
	if err != nil {
		t.Fatalf("sqlite.New: %v", err)
	}
	if err := store.SetRuntimeConfig(ctx, "last_check", time.Now().UTC().Add(-5*time.Hour).Format(time.RFC3339Nano)); err != nil {
		t.Fatalf("SetRuntimeConfig: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("close store: %v", err)
	}

	second, err := RunBriefTask(ctx, cfg, BriefTaskRequest{Action: BriefActionRun})
	if err != nil {
		t.Fatalf("second RunBriefTask: %v", err)
	}
	if second.HealthFootnote != "" || second.Summary != "NO_REPLY" {
		t.Fatalf("second = %+v", second)
	}
	if len(second.HealthDelta.NewWarnings) == 0 || !strings.Contains(second.HealthDelta.NewWarnings[0], "last_check") {
		t.Fatalf("health delta = %+v", second.HealthDelta)
	}
}

func TestRecurringFailureCreatesHealthWarning(t *testing.T) {
	ctx := context.Background()
	cfg := testConfig(t)
	configureSource(t, cfg, Source{
		Key:       "broken",
		Label:     "Broken",
		Kind:      sqlite.SourceKindRSS,
		URL:       "https://127.0.0.1:1/no-feed.xml",
		Section:   "technology",
		Threshold: sqlite.ThresholdMedium,
		Enabled:   true,
	})
	var result BriefTaskResult
	for i := 0; i < 3; i++ {
		next, err := RunBriefTask(ctx, cfg, BriefTaskRequest{Action: BriefActionRun})
		if err != nil {
			t.Fatalf("RunBriefTask %d: %v", i+1, err)
		}
		result = next
	}
	if !strings.Contains(strings.Join(result.HealthDelta.NewWarnings, "\n"), "3+ consecutive") {
		t.Fatalf("health delta = %+v", result.HealthDelta)
	}
}

func TestRecurringFailureResolvesForRemovedSource(t *testing.T) {
	ctx := context.Background()
	cfg := testConfig(t)
	configureSource(t, cfg, Source{
		Key:       "broken",
		Label:     "Broken",
		Kind:      sqlite.SourceKindRSS,
		URL:       "https://127.0.0.1:1/no-feed.xml",
		Section:   "technology",
		Threshold: sqlite.ThresholdMedium,
		Enabled:   true,
	})
	for i := 0; i < 3; i++ {
		if _, err := RunBriefTask(ctx, cfg, BriefTaskRequest{Action: BriefActionRun}); err != nil {
			t.Fatalf("RunBriefTask %d: %v", i+1, err)
		}
	}

	quiet := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(rssFixture("Quiet", "https://example.com/quiet", "quiet-guid")))
	}))
	defer quiet.Close()
	configureSource(t, cfg, Source{
		Key:       "quiet",
		Label:     "Quiet",
		Kind:      sqlite.SourceKindRSS,
		URL:       quiet.URL,
		Section:   "technology",
		Threshold: sqlite.ThresholdMedium,
		Enabled:   true,
	})

	result, err := RunBriefTask(ctx, cfg, BriefTaskRequest{Action: BriefActionRun})
	if err != nil {
		t.Fatalf("RunBriefTask after removing source: %v", err)
	}
	if !strings.Contains(strings.Join(result.HealthDelta.ResolvedWarnings, "\n"), "3+ consecutive") {
		t.Fatalf("health delta = %+v", result.HealthDelta)
	}
	if len(result.HealthDelta.NewWarnings) != 0 {
		t.Fatalf("health delta = %+v", result.HealthDelta)
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
	configureSources(t, cfg, []Source{source})
}

func configureSources(t *testing.T, cfg runclient.Config, sources []Source) {
	t.Helper()
	result, err := RunConfigTask(context.Background(), cfg, ConfigTaskRequest{
		Action:  ConfigActionReplaceSources,
		Sources: sources,
	})
	if err != nil {
		t.Fatalf("RunConfigTask: %v", err)
	}
	if result.Rejected {
		t.Fatalf("config rejected: %s", result.RejectionReason)
	}
}

func configureOutletPolicies(t *testing.T, cfg runclient.Config, outlets []OutletPolicy) {
	t.Helper()
	result, err := RunConfigTask(context.Background(), cfg, ConfigTaskRequest{
		Action:  ConfigActionReplaceOutletPolicies,
		Outlets: outlets,
	})
	if err != nil {
		t.Fatalf("RunConfigTask: %v", err)
	}
	if result.Rejected {
		t.Fatalf("outlet config rejected: %s", result.RejectionReason)
	}
}

func rssFixture(title, link, guid string) string {
	return `<?xml version="1.0"?>
<rss version="2.0"><channel>
<item><title>` + title + `</title><link>` + link + `</link><guid>` + guid + `</guid><pubDate>Thu, 23 Apr 2026 00:00:00 GMT</pubDate></item>
</channel></rss>`
}
