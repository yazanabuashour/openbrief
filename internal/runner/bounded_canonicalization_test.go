package runner

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/yazanabuashour/openbrief/internal/storage/sqlite"
)

func TestBoundedFeedCanonicalizationTimeoutFallsBackToOriginalItem(t *testing.T) {
	ctx := context.Background()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/feed":
			_, _ = w.Write([]byte(rssFixture("Slow story", "https://news.google.com/rss/articles/slow-story?hl=en-US", "slow-guid")))
		case "/articles/slow-story":
			time.Sleep(200 * time.Millisecond)
			_, _ = w.Write([]byte(`<c-wiz data-n-a-sg="signature" data-n-a-ts="1775407930"></c-wiz>`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	withCanonicalizationTestSettings(t, server.URL+"/articles/", server.URL+"/batch", 5, 1, 4, 20*time.Millisecond)

	cfg := testConfig(t)
	configureSource(t, cfg, Source{
		Key:                 "slow-google",
		Label:               "Slow Google",
		Kind:                sqlite.SourceKindRSS,
		URL:                 server.URL + "/feed",
		Section:             "technology",
		Threshold:           sqlite.ThresholdMedium,
		Enabled:             true,
		URLCanonicalization: sqlite.URLCanonicalizationGoogleNewsArticle,
	})

	start := time.Now()
	result, err := RunBriefTask(ctx, cfg, BriefTaskRequest{Action: BriefActionRun, DryRun: true})
	if err != nil {
		t.Fatalf("RunBriefTask: %v", err)
	}
	if elapsed := time.Since(start); elapsed > 500*time.Millisecond {
		t.Fatalf("RunBriefTask took %s", elapsed)
	}
	if len(result.Candidates) != 1 || result.Candidates[0].URL != "https://news.google.com/rss/articles/slow-story?hl=en-US" {
		t.Fatalf("candidates = %+v", result.Candidates)
	}
	if len(result.SuppressedUnresolved) != 1 || result.SuppressedUnresolved[0].Reason != "url canonicalization timed out" {
		t.Fatalf("suppressed unresolved = %+v", result.SuppressedUnresolved)
	}
	if len(result.FetchStatus) != 1 || result.FetchStatus[0].SuppressedUnresolved != 1 || result.FetchStatus[0].Status != "ok" || result.FetchStatus[0].Items != 1 {
		t.Fatalf("fetch status = %+v", result.FetchStatus)
	}
}

func TestCanonicalizationFallbackStateMatchesLaterCanonicalIdentity(t *testing.T) {
	ctx := context.Background()
	publisher := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}))
	defer publisher.Close()

	var slow atomic.Bool
	slow.Store(true)
	var feed *httptest.Server
	feed = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/feed":
			link := feed.URL + "/redirect"
			_, _ = w.Write([]byte(rssFixture("Same story", link, link)))
		case "/redirect":
			if slow.Load() {
				time.Sleep(200 * time.Millisecond)
			}
			http.Redirect(w, r, publisher.URL+"/story", http.StatusFound)
		default:
			http.NotFound(w, r)
		}
	}))
	defer feed.Close()
	withCanonicalizationTestSettings(t, "", "", 5, 1, 4, 20*time.Millisecond)

	cfg := testConfig(t)
	configureSource(t, cfg, Source{
		Key:                 "redirecting-feed",
		Label:               "Redirecting Feed",
		Kind:                sqlite.SourceKindRSS,
		URL:                 feed.URL + "/feed",
		Section:             "technology",
		Threshold:           sqlite.ThresholdMedium,
		Enabled:             true,
		URLCanonicalization: sqlite.URLCanonicalizationFeedBurnerRedirect,
	})

	first, err := RunBriefTask(ctx, cfg, BriefTaskRequest{Action: BriefActionRun})
	if err != nil {
		t.Fatalf("first RunBriefTask: %v", err)
	}
	if len(first.Candidates) != 1 || len(first.SuppressedUnresolved) != 1 {
		t.Fatalf("first result = %+v", first)
	}
	originalIdentity := feed.URL + "/redirect"
	state := sourceState(t, cfg, "redirecting-feed")
	if state.LatestIdentity != originalIdentity || state.LatestFeedIdentity != originalIdentity {
		t.Fatalf("source state = %+v", state)
	}

	slow.Store(false)
	second, err := RunBriefTask(ctx, cfg, BriefTaskRequest{Action: BriefActionRun})
	if err != nil {
		t.Fatalf("second RunBriefTask: %v", err)
	}
	if len(second.Candidates) != 0 || len(second.SuppressedUnresolved) != 0 {
		t.Fatalf("second result = %+v", second)
	}
	state = sourceState(t, cfg, "redirecting-feed")
	if state.LatestIdentity != publisher.URL+"/story" || state.LatestFeedIdentity != originalIdentity {
		t.Fatalf("source state = %+v", state)
	}
}

func TestBoundedFeedCanonicalizationSourceLimitSuppressesExcessItems(t *testing.T) {
	ctx := context.Background()
	var articleRequests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/feed":
			var items []rssFixtureItem
			for i := 1; i <= 7; i++ {
				id := fmt.Sprintf("article-%d", i)
				link := "https://news.google.com/rss/articles/" + id + "?hl=en-US"
				items = append(items, rssFixtureItem{title: fmt.Sprintf("Story %d", i), link: link, guid: link})
			}
			_, _ = w.Write([]byte(rssFixtureItems(items...)))
		case strings.HasPrefix(r.URL.Path, "/articles/article-"):
			articleRequests.Add(1)
			_, _ = w.Write([]byte(`<c-wiz data-n-a-sg="signature" data-n-a-ts="1775407930"></c-wiz>`))
		case r.URL.Path == "/batch":
			_ = r.ParseForm()
			id := firstRequestedArticleID(r.Form.Get("f.req"))
			if id == "" {
				t.Fatalf("batch body missing article id: %s", r.Form.Get("f.req"))
			}
			_, _ = w.Write(googleNewsBatchResponse(t, "https://publisher.example/"+id))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	withCanonicalizationTestSettings(t, server.URL+"/articles/", server.URL+"/batch", 5, 8, 4, time.Second)

	cfg := testConfig(t)
	configureSource(t, cfg, Source{
		Key:                 "limited-google",
		Label:               "Limited Google",
		Kind:                sqlite.SourceKindRSS,
		URL:                 server.URL + "/feed",
		Section:             "technology",
		Threshold:           sqlite.ThresholdMedium,
		Enabled:             true,
		URLCanonicalization: sqlite.URLCanonicalizationGoogleNewsArticle,
	})

	result, err := RunBriefTask(ctx, cfg, BriefTaskRequest{Action: BriefActionRun, DryRun: true})
	if err != nil {
		t.Fatalf("RunBriefTask: %v", err)
	}
	if articleRequests.Load() != 5 {
		t.Fatalf("article requests = %d", articleRequests.Load())
	}
	if len(result.SuppressedUnresolved) != 2 {
		t.Fatalf("suppressed unresolved = %+v", result.SuppressedUnresolved)
	}
	for _, item := range result.SuppressedUnresolved {
		if item.Reason != canonicalizationSkippedReason {
			t.Fatalf("suppressed unresolved = %+v", result.SuppressedUnresolved)
		}
	}
	if len(result.FetchStatus) != 1 || result.FetchStatus[0].Items != 5 || result.FetchStatus[0].SuppressedUnresolved != 2 {
		t.Fatalf("fetch status = %+v", result.FetchStatus)
	}
}

func TestBoundedFeedCanonicalizationLimitKeepsProcessedItemsBeforeState(t *testing.T) {
	ctx := context.Background()
	var articleRequests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/feed":
			var items []rssFixtureItem
			for i := 1; i <= 6; i++ {
				id := fmt.Sprintf("article-%d", i)
				link := "https://news.google.com/rss/articles/" + id + "?hl=en-US"
				items = append(items, rssFixtureItem{title: fmt.Sprintf("Story %d", i), link: link, guid: link})
			}
			_, _ = w.Write([]byte(rssFixtureItems(items...)))
		case strings.HasPrefix(r.URL.Path, "/articles/article-"):
			articleRequests.Add(1)
			_, _ = w.Write([]byte(`<c-wiz data-n-a-sg="signature" data-n-a-ts="1775407930"></c-wiz>`))
		case r.URL.Path == "/batch":
			_ = r.ParseForm()
			id := firstRequestedArticleID(r.Form.Get("f.req"))
			if id == "" {
				t.Fatalf("batch body missing article id: %s", r.Form.Get("f.req"))
			}
			_, _ = w.Write(googleNewsBatchResponse(t, "https://publisher.example/"+id))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	withCanonicalizationTestSettings(t, server.URL+"/articles/", server.URL+"/batch", 5, 8, 4, time.Second)

	cfg := testConfig(t)
	configureSource(t, cfg, Source{
		Key:                 "limited-google",
		Label:               "Limited Google",
		Kind:                sqlite.SourceKindRSS,
		URL:                 server.URL + "/feed",
		Section:             "technology",
		Threshold:           sqlite.ThresholdMedium,
		Enabled:             true,
		URLCanonicalization: sqlite.URLCanonicalizationGoogleNewsArticle,
	})
	seedSourceState(t, cfg, sqlite.SourceState{
		SourceKey:         "limited-google",
		LatestIdentity:    "https://publisher.example/article-6",
		LatestTitle:       "Story 6",
		LatestURL:         "https://publisher.example/article-6",
		LatestPublishedAt: "Thu, 23 Apr 2026 00:00:00 GMT",
	})

	result, err := RunBriefTask(ctx, cfg, BriefTaskRequest{Action: BriefActionRun})
	if err != nil {
		t.Fatalf("RunBriefTask: %v", err)
	}
	if articleRequests.Load() != 5 {
		t.Fatalf("article requests = %d", articleRequests.Load())
	}
	if len(result.Candidates) != 5 {
		t.Fatalf("candidates = %+v", result.Candidates)
	}
	if len(result.FetchStatus) != 1 || result.FetchStatus[0].Items != 5 || result.FetchStatus[0].SuppressedUnresolved != 1 {
		t.Fatalf("fetch status = %+v", result.FetchStatus)
	}
	state := sourceState(t, cfg, "limited-google")
	if state.LatestIdentity != "https://publisher.example/article-1" {
		t.Fatalf("source state = %+v", state)
	}
}

func TestManyBoundedFeedCanonicalizationSourcesCompleteWithinBoundedWindow(t *testing.T) {
	ctx := context.Background()
	var activeArticles atomic.Int32
	var maxActiveArticles atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/feed/"):
			sourceKey := strings.TrimPrefix(r.URL.Path, "/feed/")
			link := "https://news.google.com/rss/articles/" + sourceKey + "?hl=en-US"
			_, _ = w.Write([]byte(rssFixture("Slow "+sourceKey, link, link)))
		case strings.HasPrefix(r.URL.Path, "/articles/source-"):
			next := activeArticles.Add(1)
			updateMaxAtomic(&maxActiveArticles, next)
			time.Sleep(200 * time.Millisecond)
			activeArticles.Add(-1)
			_, _ = w.Write([]byte(`<c-wiz data-n-a-sg="signature" data-n-a-ts="1775407930"></c-wiz>`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	withCanonicalizationTestSettings(t, server.URL+"/articles/", server.URL+"/batch", 5, 1, 4, 40*time.Millisecond)

	cfg := testConfig(t)
	var sources []Source
	for i := 0; i < 8; i++ {
		key := fmt.Sprintf("source-%02d", i)
		sources = append(sources, Source{
			Key:                 key,
			Label:               "Source " + key,
			Kind:                sqlite.SourceKindRSS,
			URL:                 server.URL + "/feed/" + key,
			Section:             "technology",
			Threshold:           sqlite.ThresholdMedium,
			Enabled:             true,
			URLCanonicalization: sqlite.URLCanonicalizationGoogleNewsArticle,
		})
	}
	configureSources(t, cfg, sources)

	start := time.Now()
	result, err := RunBriefTask(ctx, cfg, BriefTaskRequest{Action: BriefActionRun, DryRun: true})
	if err != nil {
		t.Fatalf("RunBriefTask: %v", err)
	}
	if elapsed := time.Since(start); elapsed > 500*time.Millisecond {
		t.Fatalf("RunBriefTask took %s", elapsed)
	}
	if maxActiveArticles.Load() < 2 {
		t.Fatalf("max active article requests = %d", maxActiveArticles.Load())
	}
	if len(result.SuppressedUnresolved) != len(sources) {
		t.Fatalf("suppressed unresolved = %+v", result.SuppressedUnresolved)
	}
	if len(result.FetchStatus) != len(sources) {
		t.Fatalf("fetch status = %+v", result.FetchStatus)
	}
	for _, status := range result.FetchStatus {
		if status.SuppressedUnresolved != 1 || status.Status != "ok" {
			t.Fatalf("fetch status = %+v", result.FetchStatus)
		}
	}
}
