package runner

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/yazanabuashour/openbrief/internal/storage/sqlite"
)

func TestSetBriefOptionsStoresMaxDeliveryItems(t *testing.T) {
	cfg := testConfig(t)
	result, err := RunConfigTask(context.Background(), cfg, ConfigTaskRequest{
		Action:           ConfigActionSetBriefOptions,
		MaxDeliveryItems: 5,
	})
	if err != nil {
		t.Fatalf("RunConfigTask: %v", err)
	}
	if result.Rejected {
		t.Fatalf("config rejected: %s", result.RejectionReason)
	}
	if result.RuntimeConfig[sqlite.RuntimeConfigMaxDeliveryItems] != "5" {
		t.Fatalf("runtime config = %+v", result.RuntimeConfig)
	}

	inspect, err := RunConfigTask(context.Background(), cfg, ConfigTaskRequest{Action: ConfigActionInspectConfig})
	if err != nil {
		t.Fatalf("inspect config: %v", err)
	}
	if inspect.RuntimeConfig[sqlite.RuntimeConfigMaxDeliveryItems] != "5" {
		t.Fatalf("inspect runtime config = %+v", inspect.RuntimeConfig)
	}
}

func TestSetBriefOptionsRejectsInvalidMaxDeliveryItems(t *testing.T) {
	for _, value := range []int{0, -1, sqlite.MaxDeliveryItemsUpperBound + 1} {
		result, err := RunConfigTask(context.Background(), testConfig(t), ConfigTaskRequest{
			Action:           ConfigActionSetBriefOptions,
			MaxDeliveryItems: value,
		})
		if err != nil {
			t.Fatalf("RunConfigTask(%d): %v", value, err)
		}
		if !result.Rejected || !strings.Contains(result.RejectionReason, sqlite.RuntimeConfigMaxDeliveryItems) {
			t.Fatalf("result for %d = %+v", value, result)
		}
	}
}

func TestRunBriefIncludesDefaultMaxDeliveryItems(t *testing.T) {
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

	result, err := RunBriefTask(ctx, cfg, BriefTaskRequest{Action: BriefActionRun})
	if err != nil {
		t.Fatalf("RunBriefTask: %v", err)
	}
	if result.MaxDeliveryItems != sqlite.DefaultMaxDeliveryItems {
		t.Fatalf("MaxDeliveryItems = %d, want %d", result.MaxDeliveryItems, sqlite.DefaultMaxDeliveryItems)
	}
}

func TestRunBriefUsesConfiguredMaxDeliveryItems(t *testing.T) {
	ctx := context.Background()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(rssFixture("Configured item", "https://example.com/configured", "guid-1")))
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
	configureBriefOptions(t, cfg, 5)

	result, err := RunBriefTask(ctx, cfg, BriefTaskRequest{Action: BriefActionRun})
	if err != nil {
		t.Fatalf("RunBriefTask: %v", err)
	}
	if result.MaxDeliveryItems != 5 {
		t.Fatalf("MaxDeliveryItems = %d, want 5", result.MaxDeliveryItems)
	}
}

func TestRunBriefFallsBackForInvalidStoredMaxDeliveryItems(t *testing.T) {
	ctx := context.Background()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(rssFixture("Fallback item", "https://example.com/fallback", "guid-1")))
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
	setRuntimeConfig(t, cfg, sqlite.RuntimeConfigMaxDeliveryItems, "not-a-number")

	result, err := RunBriefTask(ctx, cfg, BriefTaskRequest{Action: BriefActionRun, DryRun: true})
	if err != nil {
		t.Fatalf("RunBriefTask: %v", err)
	}
	if result.MaxDeliveryItems != sqlite.DefaultMaxDeliveryItems {
		t.Fatalf("MaxDeliveryItems = %d, want %d", result.MaxDeliveryItems, sqlite.DefaultMaxDeliveryItems)
	}
	if len(result.HealthDelta.NewWarnings) == 0 || !strings.Contains(result.HealthDelta.NewWarnings[0], sqlite.RuntimeConfigMaxDeliveryItems) {
		t.Fatalf("HealthDelta = %+v", result.HealthDelta)
	}
}

func TestResolveBriefOptionsDefaultsWhenMaxDeliveryItemsAbsent(t *testing.T) {
	options := resolveBriefOptions(map[string]string{})
	if options.MaxDeliveryItems != sqlite.DefaultMaxDeliveryItems || options.Warning != "" {
		t.Fatalf("options = %+v", options)
	}
}
