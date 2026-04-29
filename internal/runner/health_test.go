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

func TestTransientFetchFailureIsSourceScoped(t *testing.T) {
	ctx := context.Background()
	healthy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(rssFixture("Healthy item", "https://example.com/healthy", "healthy-guid")))
	}))
	defer healthy.Close()

	cfg := testConfig(t)
	configureSources(t, cfg, []Source{
		{
			Key:       "broken",
			Label:     "Broken",
			Kind:      sqlite.SourceKindRSS,
			URL:       "https://127.0.0.1:1/no-feed.xml",
			Section:   "technology",
			Threshold: sqlite.ThresholdMedium,
			Enabled:   true,
		},
		{
			Key:       "healthy",
			Label:     "Healthy",
			Kind:      sqlite.SourceKindRSS,
			URL:       healthy.URL,
			Section:   "technology",
			Threshold: sqlite.ThresholdMedium,
			Enabled:   true,
		},
	})

	result, err := RunBriefTask(ctx, cfg, BriefTaskRequest{Action: BriefActionRun, DryRun: true})
	if err != nil {
		t.Fatalf("RunBriefTask: %v", err)
	}
	if len(result.Candidates) != 1 || result.Candidates[0].Title != "Healthy item" {
		t.Fatalf("candidates = %+v", result.Candidates)
	}
	if len(result.FetchStatus) != 2 {
		t.Fatalf("fetch status = %+v", result.FetchStatus)
	}
	if result.FetchStatus[0].SourceKey != "broken" || result.FetchStatus[0].Status != "error" {
		t.Fatalf("fetch status = %+v", result.FetchStatus)
	}
	if result.FetchStatus[1].SourceKey != "healthy" || result.FetchStatus[1].Status != "ok" {
		t.Fatalf("fetch status = %+v", result.FetchStatus)
	}
	if len(result.HealthDelta.NewWarnings) != 1 || !strings.Contains(result.HealthDelta.NewWarnings[0], "broken") {
		t.Fatalf("health delta = %+v", result.HealthDelta)
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
