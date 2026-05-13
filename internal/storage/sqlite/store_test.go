package sqlite

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func TestStoreInitializesEmptyConfig(t *testing.T) {
	ctx := context.Background()
	store, err := New(ctx, Config{DatabasePath: filepath.Join(t.TempDir(), "openbrief.sqlite")})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer func() {
		_ = store.Close()
	}()

	sources, err := store.ListSources(ctx, false)
	if err != nil {
		t.Fatalf("ListSources: %v", err)
	}
	if len(sources) != 0 {
		t.Fatalf("sources = %v, want empty", sources)
	}
	runtimeConfig, err := store.RuntimeConfig(ctx)
	if err != nil {
		t.Fatalf("RuntimeConfig: %v", err)
	}
	if runtimeConfig["configuration_version"] != "v2" {
		t.Fatalf("configuration_version = %q, want v2", runtimeConfig["configuration_version"])
	}
	if runtimeConfig[RuntimeConfigMaxDeliveryItems] != "7" {
		t.Fatalf("max_delivery_items = %q, want 7", runtimeConfig[RuntimeConfigMaxDeliveryItems])
	}
}

func TestReplaceSourcesValidatesAndStores(t *testing.T) {
	ctx := context.Background()
	store, err := New(ctx, Config{DatabasePath: filepath.Join(t.TempDir(), "openbrief.sqlite")})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer func() {
		_ = store.Close()
	}()

	sources, err := store.ReplaceSources(ctx, []Source{{
		Key:       "example",
		Label:     "Example",
		Kind:      SourceKindRSS,
		URL:       "https://example.com/feed.xml",
		Section:   "technology",
		Threshold: ThresholdMedium,
		Enabled:   true,
	}})
	if err != nil {
		t.Fatalf("ReplaceSources: %v", err)
	}
	if len(sources) != 1 || sources[0].Key != "example" {
		t.Fatalf("sources = %+v", sources)
	}
	if sources[0].URLCanonicalization != URLCanonicalizationNone || sources[0].OutletExtraction != OutletExtractionNone {
		t.Fatalf("source defaults = %+v", sources[0])
	}
	if _, err := store.ReplaceSources(ctx, []Source{{Key: "Bad/Key"}}); err == nil {
		t.Fatal("ReplaceSources invalid source succeeded")
	}
	if _, err := store.ReplaceSources(ctx, []Source{{
		Key:       "badrepo",
		Label:     "Bad Repo",
		Kind:      SourceKindGitHubRelease,
		Repo:      "owner/name/extra",
		Section:   "releases",
		Threshold: ThresholdAlways,
		Enabled:   true,
	}}); err == nil {
		t.Fatal("ReplaceSources invalid GitHub repo succeeded")
	}
	if _, err := store.ReplaceSources(ctx, []Source{{
		Key:       "file-feed",
		Label:     "File Feed",
		Kind:      SourceKindRSS,
		URL:       "file:///tmp/openbrief-feed.xml",
		Section:   "technology",
		Threshold: ThresholdMedium,
		Enabled:   true,
	}}); err == nil {
		t.Fatal("ReplaceSources file feed succeeded")
	}
	t.Setenv("OPENBRIEF_EVAL_ALLOW_FILE_URLS", "1")
	if _, err := store.ReplaceSources(ctx, []Source{{
		Key:       "file-feed",
		Label:     "File Feed",
		Kind:      SourceKindRSS,
		URL:       "file:///tmp/openbrief-feed.xml",
		Section:   "technology",
		Threshold: ThresholdMedium,
		Enabled:   true,
	}}); err != nil {
		t.Fatalf("ReplaceSources eval file feed: %v", err)
	}
}

func TestOutletPoliciesRoundTrip(t *testing.T) {
	ctx := context.Background()
	store, err := New(ctx, Config{DatabasePath: filepath.Join(t.TempDir(), "openbrief.sqlite")})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer func() {
		_ = store.Close()
	}()

	policies, err := store.ReplaceOutletPolicies(ctx, []OutletPolicy{{
		Name:    "Example Outlet",
		Aliases: []string{"Example", "Example"},
		Policy:  "watch",
		Enabled: true,
	}})
	if err != nil {
		t.Fatalf("ReplaceOutletPolicies: %v", err)
	}
	if len(policies) != 1 || len(policies[0].Aliases) != 1 || policies[0].Policy != "watch" {
		t.Fatalf("policies = %+v", policies)
	}
}

func TestReplaceSourcesStoresGenericProcessingFields(t *testing.T) {
	ctx := context.Background()
	store, err := New(ctx, Config{DatabasePath: filepath.Join(t.TempDir(), "openbrief.sqlite")})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer func() {
		_ = store.Close()
	}()

	sources, err := store.ReplaceSources(ctx, []Source{{
		Key:                 "news",
		Label:               "News",
		Kind:                SourceKindRSS,
		URL:                 "https://example.com/feed.xml",
		Section:             "technology",
		Threshold:           ThresholdMedium,
		Enabled:             true,
		URLCanonicalization: URLCanonicalizationFeedBurnerRedirect,
		OutletExtraction:    OutletExtractionRSSSource,
		DedupGroup:          "front-page",
		PriorityRank:        3,
		AlwaysReport:        true,
	}})
	if err != nil {
		t.Fatalf("ReplaceSources: %v", err)
	}
	got := sources[0]
	if got.URLCanonicalization != URLCanonicalizationFeedBurnerRedirect ||
		got.OutletExtraction != OutletExtractionRSSSource ||
		got.DedupGroup != "front-page" ||
		got.PriorityRank != 3 ||
		!got.AlwaysReport {
		t.Fatalf("source = %+v", got)
	}
}

func TestRecentDeliveriesReturnsLatestTwoWithIDTieBreak(t *testing.T) {
	ctx := context.Background()
	store, err := New(ctx, Config{DatabasePath: filepath.Join(t.TempDir(), "openbrief.sqlite")})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer func() {
		_ = store.Close()
	}()

	older := time.Date(2026, 4, 23, 1, 0, 0, 0, time.UTC)
	tie := time.Date(2026, 4, 23, 2, 0, 0, 0, time.UTC)
	store.now = func() time.Time { return older }
	if _, err := store.InsertDelivery(ctx, "run-old", "old", nil); err != nil {
		t.Fatalf("insert old delivery: %v", err)
	}
	store.now = func() time.Time { return tie }
	if _, err := store.InsertDelivery(ctx, "run-second", "second", nil); err != nil {
		t.Fatalf("insert second delivery: %v", err)
	}
	if _, err := store.InsertDelivery(ctx, "run-third", "third", nil); err != nil {
		t.Fatalf("insert third delivery: %v", err)
	}

	deliveries, err := store.RecentDeliveries(ctx, 2)
	if err != nil {
		t.Fatalf("RecentDeliveries: %v", err)
	}
	if len(deliveries) != 2 {
		t.Fatalf("deliveries = %+v, want 2", deliveries)
	}
	if deliveries[0].RunID != "run-third" || deliveries[0].Message != "third" || !deliveries[0].DeliveredAt.Equal(tie) {
		t.Fatalf("first delivery = %+v", deliveries[0])
	}
	if deliveries[1].RunID != "run-second" || deliveries[1].Message != "second" || !deliveries[1].DeliveredAt.Equal(tie) {
		t.Fatalf("second delivery = %+v", deliveries[1])
	}
}
