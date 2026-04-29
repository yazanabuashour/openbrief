package runner

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/yazanabuashour/openbrief/internal/storage/sqlite"
)

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

func TestRSSSourceOutletPolicyBlocksBeforeNewItemSelection(t *testing.T) {
	ctx := context.Background()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(rssFixtureItems(
			rssFixtureItem{title: "Blocked story", link: "https://example.com/blocked", guid: "blocked", source: "Blocked Outlet"},
			rssFixtureItem{title: "Open story", link: "https://example.com/open", guid: "open", source: "Open Outlet"},
		)))
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
		OutletExtraction: sqlite.OutletExtractionRSSSource,
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
	if len(result.Candidates) != 1 || result.Candidates[0].Title != "Open story" || result.Candidates[0].Outlet != "Open Outlet" {
		t.Fatalf("candidates = %+v", result.Candidates)
	}
	if len(result.SuppressedPolicy) != 1 || result.SuppressedPolicy[0].Outlet != "Blocked Outlet" {
		t.Fatalf("suppressed policy = %+v", result.SuppressedPolicy)
	}
}

func TestRSSSourceOutletPolicyWatchAuditsButKeepsCandidate(t *testing.T) {
	ctx := context.Background()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(rssFixtureItems(
			rssFixtureItem{title: "Watched story", link: "https://example.com/watch", guid: "watch-guid", source: "Watch Outlet"},
		)))
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
		OutletExtraction: sqlite.OutletExtractionRSSSource,
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
	if len(result.Candidates) != 1 || result.Candidates[0].Outlet != "Watch Outlet" {
		t.Fatalf("candidates = %+v", result.Candidates)
	}
	if len(result.SuppressedPolicy) != 1 || result.SuppressedPolicy[0].Policy != "watch" || result.SuppressedPolicy[0].Outlet != "Watch Outlet" {
		t.Fatalf("suppressed policy = %+v", result.SuppressedPolicy)
	}
}

func TestRSSSourceOutletPolicyMissingSourceDoesNotMatchTitleSuffix(t *testing.T) {
	ctx := context.Background()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(rssFixture("Open story - Blocked Outlet", "https://example.com/open", "open-guid")))
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
		OutletExtraction: sqlite.OutletExtractionRSSSource,
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
	if len(result.Candidates) != 1 || result.Candidates[0].Outlet != "" {
		t.Fatalf("candidates = %+v", result.Candidates)
	}
	if len(result.SuppressedPolicy) != 0 {
		t.Fatalf("suppressed policy = %+v", result.SuppressedPolicy)
	}
}
