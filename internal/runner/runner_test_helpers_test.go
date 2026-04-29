package runner

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/yazanabuashour/openbrief/internal/runclient"
	"github.com/yazanabuashour/openbrief/internal/storage/sqlite"
)

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

func configureBriefOptions(t *testing.T, cfg runclient.Config, maxDeliveryItems int) {
	t.Helper()
	result, err := RunConfigTask(context.Background(), cfg, ConfigTaskRequest{
		Action:           ConfigActionSetBriefOptions,
		MaxDeliveryItems: maxDeliveryItems,
	})
	if err != nil {
		t.Fatalf("RunConfigTask: %v", err)
	}
	if result.Rejected {
		t.Fatalf("brief options rejected: %s", result.RejectionReason)
	}
}

func seedSourceState(t *testing.T, cfg runclient.Config, state sqlite.SourceState) {
	t.Helper()
	rt, err := runclient.Open(context.Background(), cfg)
	if err != nil {
		t.Fatalf("open runtime: %v", err)
	}
	defer func() {
		_ = rt.Close()
	}()
	if err := rt.Store().UpsertSourceState(context.Background(), state); err != nil {
		t.Fatalf("upsert source state: %v", err)
	}
}

func sourceState(t *testing.T, cfg runclient.Config, sourceKey string) *sqlite.SourceState {
	t.Helper()
	rt, err := runclient.Open(context.Background(), cfg)
	if err != nil {
		t.Fatalf("open runtime: %v", err)
	}
	defer func() {
		_ = rt.Close()
	}()
	state, err := rt.Store().SourceState(context.Background(), sourceKey)
	if err != nil {
		t.Fatalf("source state: %v", err)
	}
	if state == nil {
		t.Fatalf("source state %q is nil", sourceKey)
	}
	return state
}

func rssFixture(title, link, guid string) string {
	return rssFixtureItems(rssFixtureItem{title: title, link: link, guid: guid})
}

type rssFixtureItem struct {
	title  string
	link   string
	guid   string
	source string
}

func rssFixtureItems(items ...rssFixtureItem) string {
	var builder strings.Builder
	builder.WriteString(`<?xml version="1.0"?>
<rss version="2.0"><channel>
`)
	for _, item := range items {
		builder.WriteString(`<item><title>`)
		builder.WriteString(item.title)
		builder.WriteString(`</title><link>`)
		builder.WriteString(item.link)
		builder.WriteString(`</link><guid>`)
		builder.WriteString(item.guid)
		builder.WriteString(`</guid><pubDate>Thu, 23 Apr 2026 00:00:00 GMT</pubDate>`)
		if item.source != "" {
			builder.WriteString(`<source>`)
			builder.WriteString(item.source)
			builder.WriteString(`</source>`)
		}
		builder.WriteString(`</item>
`)
	}
	builder.WriteString(`</channel></rss>`)
	return builder.String()
}

func googleNewsBatchResponse(t *testing.T, publisherURL string) []byte {
	t.Helper()
	decoded, err := json.Marshal([]any{nil, publisherURL})
	if err != nil {
		t.Fatalf("marshal decoded batch response: %v", err)
	}
	rows, err := json.Marshal([]any{[]any{"wrb.fr", "Fbv4je", string(decoded)}})
	if err != nil {
		t.Fatalf("marshal batch response rows: %v", err)
	}
	return []byte(")]}'\n\n" + string(rows))
}

func firstRequestedArticleID(value string) string {
	for i := 1; i <= 7; i++ {
		id := fmt.Sprintf("article-%d", i)
		if strings.Contains(value, id) {
			return id
		}
	}
	return ""
}

func firstRequestedArticleIDFromValues(value string, ids ...string) string {
	for _, id := range ids {
		if strings.Contains(value, id) {
			return id
		}
	}
	return ""
}

func updateMaxAtomic(maxValue *atomic.Int32, value int32) {
	for {
		current := maxValue.Load()
		if value <= current || maxValue.CompareAndSwap(current, value) {
			return
		}
	}
}

func withGoogleNewsBackoffTestSetting(t *testing.T, duration time.Duration) {
	t.Helper()
	original := googleNewsBackoffDuration
	googleNewsBackoffDuration = duration
	t.Cleanup(func() {
		googleNewsBackoffDuration = original
	})
}

func withCanonicalizationTestSettings(t *testing.T, articleBaseURL string, batchEndpoint string, maxAttempts int, itemConcurrency int, sourceConcurrency int, timeout time.Duration) {
	t.Helper()
	originalMaxAttempts := canonicalizationMaxAttemptsPerSource
	originalItemConcurrency := canonicalizationConcurrency
	originalTimeout := canonicalizationTimeout
	originalArticleBaseURL := googleNewsArticleBaseURL
	originalBatchEndpoint := googleNewsBatchEndpoint
	originalSourceConcurrency := sourceFetchConcurrency

	canonicalizationMaxAttemptsPerSource = maxAttempts
	canonicalizationConcurrency = itemConcurrency
	canonicalizationTimeout = timeout
	googleNewsArticleBaseURL = articleBaseURL
	googleNewsBatchEndpoint = batchEndpoint
	sourceFetchConcurrency = sourceConcurrency

	t.Cleanup(func() {
		canonicalizationMaxAttemptsPerSource = originalMaxAttempts
		canonicalizationConcurrency = originalItemConcurrency
		canonicalizationTimeout = originalTimeout
		googleNewsArticleBaseURL = originalArticleBaseURL
		googleNewsBatchEndpoint = originalBatchEndpoint
		sourceFetchConcurrency = originalSourceConcurrency
	})
}
