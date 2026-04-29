package runner

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/yazanabuashour/openbrief/internal/storage/sqlite"
)

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

func TestFeedBurnerRedirectCanonicalizationPreservesOrder(t *testing.T) {
	ctx := context.Background()
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/feed":
			_, _ = w.Write([]byte(rssFixtureItems(
				rssFixtureItem{title: "First redirect", link: server.URL + "/slow-redirect", guid: server.URL + "/slow-redirect"},
				rssFixtureItem{title: "Second redirect", link: server.URL + "/fast-redirect", guid: server.URL + "/fast-redirect"},
				rssFixtureItem{title: "Direct story", link: server.URL + "/direct", guid: "direct-guid"},
			)))
		case "/slow-redirect":
			time.Sleep(25 * time.Millisecond)
			http.Redirect(w, r, server.URL+"/first-final", http.StatusFound)
		case "/fast-redirect":
			http.Redirect(w, r, server.URL+"/second-final", http.StatusFound)
		case "/first-final", "/second-final", "/direct":
			_, _ = w.Write([]byte("ok"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	fetcher := newFetcher()
	fetcher.canonicalizationConcurrency = 2
	output, err := fetcher.FetchDetailed(ctx, Source{
		Key:                 "feed",
		Label:               "Feed",
		Kind:                sqlite.SourceKindRSS,
		URL:                 server.URL + "/feed",
		Section:             "technology",
		Threshold:           sqlite.ThresholdMedium,
		Enabled:             true,
		URLCanonicalization: sqlite.URLCanonicalizationFeedBurnerRedirect,
	})
	if err != nil {
		t.Fatalf("FetchDetailed: %v", err)
	}
	if len(output.Unresolved) != 0 {
		t.Fatalf("unresolved = %+v", output.Unresolved)
	}
	gotURLs := []string{output.Items[0].URL, output.Items[1].URL, output.Items[2].URL}
	wantURLs := []string{server.URL + "/first-final", server.URL + "/second-final", server.URL + "/direct"}
	for i := range wantURLs {
		if gotURLs[i] != wantURLs[i] {
			t.Fatalf("urls = %+v", gotURLs)
		}
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

func TestURLHostOutletExtractionSuppressesCanonicalizationFailureFallback(t *testing.T) {
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
			_, _ = w.Write([]byte(rssFixture("Redirected publisher story", feed.URL+"/redirect", feed.URL+"/redirect")))
		case "/redirect":
			time.Sleep(200 * time.Millisecond)
			http.Redirect(w, r, publisherURL+"/story", http.StatusFound)
		default:
			http.NotFound(w, r)
		}
	}))
	defer feed.Close()
	withCanonicalizationTestSettings(t, "", "", 5, 1, 4, 20*time.Millisecond)

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

	result, err := RunBriefTask(ctx, cfg, BriefTaskRequest{Action: BriefActionRun, DryRun: true})
	if err != nil {
		t.Fatalf("RunBriefTask: %v", err)
	}
	if len(result.Candidates) != 0 || len(result.SuppressedUnresolved) != 1 || len(result.SuppressedPolicy) != 0 {
		t.Fatalf("result = %+v", result)
	}
	if len(result.FetchStatus) != 1 || result.FetchStatus[0].Items != 0 || result.FetchStatus[0].SuppressedUnresolved != 1 {
		t.Fatalf("fetch status = %+v", result.FetchStatus)
	}
}

func TestParseFeedRejectsNonFeedXML(t *testing.T) {
	_, err := parseFeed([]byte(`<?xml version="1.0"?><error><message>bad</message></error>`))
	if err == nil {
		t.Fatal("parseFeed accepted non-feed XML")
	}
}
