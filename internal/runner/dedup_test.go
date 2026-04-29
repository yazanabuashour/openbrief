package runner

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/yazanabuashour/openbrief/internal/storage/sqlite"
)

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

func TestGoogleNewsFeedCanonicalizationMemoizesDuplicateArticleURL(t *testing.T) {
	ctx := context.Background()
	var articleRequests atomic.Int32
	var batchRequests atomic.Int32
	articleURL := "https://news.google.com/rss/articles/example-id?hl=en-US"
	publisherURL := "https://publisher.example/example"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/feed":
			_, _ = w.Write([]byte(rssFixtureItems(
				rssFixtureItem{title: "First", link: articleURL, guid: articleURL},
				rssFixtureItem{title: "Second", link: articleURL, guid: articleURL},
				rssFixtureItem{title: "Third", link: articleURL, guid: articleURL},
			)))
		case "/articles/example-id":
			articleRequests.Add(1)
			time.Sleep(25 * time.Millisecond)
			_, _ = w.Write([]byte(`<c-wiz data-n-a-sg="signature" data-n-a-ts="1775407930"></c-wiz>`))
		case "/batch":
			batchRequests.Add(1)
			_ = r.ParseForm()
			if !strings.Contains(r.Form.Get("f.req"), "example-id") {
				t.Fatalf("batch body missing article id: %s", r.Form.Get("f.req"))
			}
			_, _ = w.Write(googleNewsBatchResponse(t, publisherURL))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	fetcher := newFetcher()
	fetcher.canonicalizationConcurrency = 3
	fetcher.googleNewsArticleBaseURL = server.URL + "/articles/"
	fetcher.googleNewsBatchEndpoint = server.URL + "/batch"
	output, err := fetcher.FetchDetailed(ctx, Source{
		Key:                 "google",
		Label:               "Google",
		Kind:                sqlite.SourceKindRSS,
		URL:                 server.URL + "/feed",
		Section:             "technology",
		Threshold:           sqlite.ThresholdMedium,
		Enabled:             true,
		URLCanonicalization: sqlite.URLCanonicalizationGoogleNewsArticle,
	})
	if err != nil {
		t.Fatalf("FetchDetailed: %v", err)
	}
	if articleRequests.Load() != 1 {
		t.Fatalf("article requests = %d", articleRequests.Load())
	}
	if batchRequests.Load() != 1 {
		t.Fatalf("batch requests = %d", batchRequests.Load())
	}
	if len(output.Unresolved) != 0 {
		t.Fatalf("unresolved = %+v", output.Unresolved)
	}
	if len(output.Items) != 3 {
		t.Fatalf("items = %+v", output.Items)
	}
	for _, item := range output.Items {
		if item.URL != publisherURL || item.Identity != publisherURL {
			t.Fatalf("items = %+v", output.Items)
		}
	}
}
