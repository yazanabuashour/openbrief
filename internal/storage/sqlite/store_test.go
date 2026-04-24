package sqlite

import (
	"context"
	"path/filepath"
	"testing"
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
	if runtimeConfig["configuration_version"] != "v1" {
		t.Fatalf("configuration_version = %q, want v1", runtimeConfig["configuration_version"])
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
