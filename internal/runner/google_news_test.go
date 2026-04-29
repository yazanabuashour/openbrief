package runner

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/yazanabuashour/openbrief/internal/storage/sqlite"
)

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

func TestGoogleNewsFeedCanonicalizationParallelPreservesOrder(t *testing.T) {
	ctx := context.Background()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/feed":
			_, _ = w.Write([]byte(rssFixtureItems(
				rssFixtureItem{title: "First", link: "https://news.google.com/rss/articles/article-one?hl=en-US", guid: "https://news.google.com/rss/articles/article-one?hl=en-US"},
				rssFixtureItem{title: "Direct", link: "https://direct.example/story", guid: "direct-guid"},
				rssFixtureItem{title: "Second", link: "https://news.google.com/rss/articles/article-two?hl=en-US", guid: "https://news.google.com/rss/articles/article-two?hl=en-US"},
			)))
		case "/articles/article-one", "/articles/article-two":
			_, _ = w.Write([]byte(`<c-wiz data-n-a-sg="signature" data-n-a-ts="1775407930"></c-wiz>`))
		case "/batch":
			_ = r.ParseForm()
			body := r.Form.Get("f.req")
			switch {
			case strings.Contains(body, "article-one"):
				time.Sleep(25 * time.Millisecond)
				_, _ = w.Write(googleNewsBatchResponse(t, "https://publisher.example/one"))
			case strings.Contains(body, "article-two"):
				_, _ = w.Write(googleNewsBatchResponse(t, "https://publisher.example/two"))
			default:
				t.Fatalf("batch body missing article id: %s", body)
			}
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	fetcher := newFetcher()
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
	if len(output.Unresolved) != 0 {
		t.Fatalf("unresolved = %+v", output.Unresolved)
	}
	if len(output.Items) != 3 {
		t.Fatalf("items = %+v", output.Items)
	}
	gotURLs := []string{output.Items[0].URL, output.Items[1].URL, output.Items[2].URL}
	wantURLs := []string{"https://publisher.example/one", "https://direct.example/story", "https://publisher.example/two"}
	for i := range wantURLs {
		if gotURLs[i] != wantURLs[i] {
			t.Fatalf("urls = %+v", gotURLs)
		}
	}
	if output.Items[0].Identity != "https://publisher.example/one" || output.Items[2].Identity != "https://publisher.example/two" {
		t.Fatalf("items = %+v", output.Items)
	}
}

func TestGoogleNewsFeedCanonicalizationDoesNotMemoizeFailure(t *testing.T) {
	ctx := context.Background()
	var articleRequests atomic.Int32
	articleURL := "https://news.google.com/rss/articles/example-id?hl=en-US"
	publisherURL := "https://publisher.example/example"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/feed":
			_, _ = w.Write([]byte(rssFixtureItems(
				rssFixtureItem{title: "First", link: articleURL, guid: articleURL},
			)))
		case "/articles/example-id":
			if articleRequests.Add(1) == 1 {
				http.Error(w, "temporary failure", http.StatusBadGateway)
				return
			}
			_, _ = w.Write([]byte(`<c-wiz data-n-a-sg="signature" data-n-a-ts="1775407930"></c-wiz>`))
		case "/batch":
			_, _ = w.Write(googleNewsBatchResponse(t, publisherURL))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	fetcher := newFetcher()
	fetcher.googleNewsArticleBaseURL = server.URL + "/articles/"
	fetcher.googleNewsBatchEndpoint = server.URL + "/batch"
	source := Source{
		Key:                 "google",
		Label:               "Google",
		Kind:                sqlite.SourceKindRSS,
		URL:                 server.URL + "/feed",
		Section:             "technology",
		Threshold:           sqlite.ThresholdMedium,
		Enabled:             true,
		URLCanonicalization: sqlite.URLCanonicalizationGoogleNewsArticle,
	}

	first, err := fetcher.FetchDetailed(ctx, source)
	if err != nil {
		t.Fatalf("first FetchDetailed: %v", err)
	}
	if len(first.Unresolved) != 1 {
		t.Fatalf("first unresolved = %+v", first.Unresolved)
	}

	second, err := fetcher.FetchDetailed(ctx, source)
	if err != nil {
		t.Fatalf("second FetchDetailed: %v", err)
	}
	if articleRequests.Load() != 2 {
		t.Fatalf("article requests = %d", articleRequests.Load())
	}
	if len(second.Unresolved) != 0 {
		t.Fatalf("second unresolved = %+v", second.Unresolved)
	}
	if len(second.Items) != 1 || second.Items[0].URL != publisherURL {
		t.Fatalf("second items = %+v", second.Items)
	}
}

func TestGoogleNewsFeedCanonicalizationBacksOffAfterRateLimit(t *testing.T) {
	ctx := context.Background()
	withGoogleNewsBackoffTestSetting(t, 40*time.Millisecond)
	var mu sync.Mutex
	var articleStarts []time.Time
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/rate-feed":
			_, _ = w.Write([]byte(rssFixtureItems(
				rssFixtureItem{title: "Rate limited", link: "https://news.google.com/rss/articles/rate-limited?hl=en-US", guid: "rate-limited"},
			)))
		case r.URL.Path == "/follow-feed":
			_, _ = w.Write([]byte(rssFixtureItems(
				rssFixtureItem{title: "Follow one", link: "https://news.google.com/rss/articles/follow-one?hl=en-US", guid: "follow-one"},
				rssFixtureItem{title: "Follow two", link: "https://news.google.com/rss/articles/follow-two?hl=en-US", guid: "follow-two"},
			)))
		case strings.HasPrefix(r.URL.Path, "/articles/"):
			mu.Lock()
			articleStarts = append(articleStarts, time.Now())
			mu.Unlock()
			if r.URL.Path == "/articles/rate-limited" {
				http.Error(w, "rate limited", http.StatusTooManyRequests)
				return
			}
			_, _ = w.Write([]byte(`<c-wiz data-n-a-sg="signature" data-n-a-ts="1775407930"></c-wiz>`))
		case r.URL.Path == "/batch":
			_ = r.ParseForm()
			id := firstRequestedArticleIDFromValues(r.Form.Get("f.req"), "follow-one", "follow-two")
			if id == "" {
				t.Fatalf("batch body missing article id: %s", r.Form.Get("f.req"))
			}
			_, _ = w.Write(googleNewsBatchResponse(t, "https://publisher.example/"+id))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	fetcher := newFetcher()
	fetcher.canonicalizationConcurrency = 1
	fetcher.googleNewsArticleBaseURL = server.URL + "/articles/"
	fetcher.googleNewsBatchEndpoint = server.URL + "/batch"
	source := Source{
		Key:                 "google",
		Label:               "Google",
		Kind:                sqlite.SourceKindRSS,
		Section:             "technology",
		Threshold:           sqlite.ThresholdMedium,
		Enabled:             true,
		URLCanonicalization: sqlite.URLCanonicalizationGoogleNewsArticle,
	}
	rateLimitedSource := source
	rateLimitedSource.URL = server.URL + "/rate-feed"
	first, err := fetcher.FetchDetailed(ctx, rateLimitedSource)
	if err != nil {
		t.Fatalf("first FetchDetailed: %v", err)
	}
	if len(first.Unresolved) != 1 {
		t.Fatalf("first unresolved = %+v", first.Unresolved)
	}
	followSource := source
	followSource.URL = server.URL + "/follow-feed"
	output, err := fetcher.FetchDetailed(ctx, followSource)
	if err != nil {
		t.Fatalf("second FetchDetailed: %v", err)
	}
	if len(output.Unresolved) != 0 {
		t.Fatalf("second unresolved = %+v", output.Unresolved)
	}
	if len(output.Items) != 2 {
		t.Fatalf("items = %+v", output.Items)
	}
	mu.Lock()
	starts := append([]time.Time(nil), articleStarts...)
	mu.Unlock()
	if len(starts) != 3 {
		t.Fatalf("article starts = %d", len(starts))
	}
	if gap := starts[1].Sub(starts[0]); gap < 30*time.Millisecond {
		t.Fatalf("first follow-up started after %s, want backoff", gap)
	}
	if output.Items[0].URL != "https://publisher.example/follow-one" || output.Items[1].URL != "https://publisher.example/follow-two" {
		t.Fatalf("items = %+v", output.Items)
	}
}

func TestRunBriefGoogleNewsRateLimitDoesNotBlockHealthySource(t *testing.T) {
	ctx := context.Background()
	withGoogleNewsBackoffTestSetting(t, 10*time.Millisecond)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/google-feed":
			_, _ = w.Write([]byte(rssFixture("Rate limited Google", "https://news.google.com/rss/articles/rate-limited?hl=en-US", "rate-limited")))
		case "/healthy-feed":
			_, _ = w.Write([]byte(rssFixture("Healthy story", "https://publisher.example/healthy", "healthy-guid")))
		case "/articles/rate-limited":
			http.Error(w, "rate limited", http.StatusTooManyRequests)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	withCanonicalizationTestSettings(t, server.URL+"/articles/", server.URL+"/batch", 5, 2, 2, time.Second)

	cfg := testConfig(t)
	configureSources(t, cfg, []Source{
		{
			Key:                 "google",
			Label:               "Google",
			Kind:                sqlite.SourceKindRSS,
			URL:                 server.URL + "/google-feed",
			Section:             "technology",
			Threshold:           sqlite.ThresholdMedium,
			Enabled:             true,
			URLCanonicalization: sqlite.URLCanonicalizationGoogleNewsArticle,
		},
		{
			Key:       "healthy",
			Label:     "Healthy",
			Kind:      sqlite.SourceKindRSS,
			URL:       server.URL + "/healthy-feed",
			Section:   "technology",
			Threshold: sqlite.ThresholdMedium,
			Enabled:   true,
		},
	})

	result, err := RunBriefTask(ctx, cfg, BriefTaskRequest{Action: BriefActionRun, DryRun: true})
	if err != nil {
		t.Fatalf("RunBriefTask: %v", err)
	}
	var hasHealthy bool
	for _, candidate := range result.Candidates {
		if candidate.SourceKey == "healthy" && candidate.URL == "https://publisher.example/healthy" {
			hasHealthy = true
		}
	}
	if !hasHealthy {
		t.Fatalf("candidates = %+v", result.Candidates)
	}
	if len(result.SuppressedUnresolved) != 1 || !strings.HasPrefix(result.SuppressedUnresolved[0].Reason, "HTTP 429") {
		t.Fatalf("suppressed unresolved = %+v", result.SuppressedUnresolved)
	}
	if len(result.FetchStatus) != 2 {
		t.Fatalf("fetch status = %+v", result.FetchStatus)
	}
	for _, status := range result.FetchStatus {
		if status.Status != "ok" {
			t.Fatalf("fetch status = %+v", result.FetchStatus)
		}
	}
}
