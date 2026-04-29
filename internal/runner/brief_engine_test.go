package runner

import (
	"context"
	"testing"
	"time"

	"github.com/yazanabuashour/openbrief/internal/domain"
	"github.com/yazanabuashour/openbrief/internal/storage/sqlite"
)

func TestRunBriefWithDepsPersistsThroughInjectedSeams(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 4, 29, 12, 0, 0, 0, time.UTC)
	store := newFakeBriefStore([]Source{{
		Key:       "example",
		Label:     "Example",
		Kind:      domain.SourceKindRSS,
		URL:       "https://example.com/feed.xml",
		Section:   "technology",
		Threshold: domain.ThresholdMedium,
		Enabled:   true,
	}})
	fetcher := fakeBriefFetcher{outputs: map[string]fetchOutput{
		"example": {
			Items: []fetchedItem{{
				Title:       "Injected story",
				URL:         "https://example.com/story",
				Identity:    "guid-1",
				PublishedAt: now.Format(time.RFC3339),
			}},
		},
	}}

	result, err := runBriefWithDeps(ctx, briefRunDeps{
		paths:            Paths{DatabasePath: "openbrief.sqlite"},
		store:            store,
		fetcher:          fetcher,
		now:              func() time.Time { return now },
		fetchConcurrency: 1,
	}, BriefTaskRequest{Action: BriefActionRun})
	if err != nil {
		t.Fatalf("runBriefWithDeps: %v", err)
	}
	if result.RunID != "run-1" || len(result.Candidates) != 1 || result.Candidates[0].Title != "Injected story" {
		t.Fatalf("result = %+v", result)
	}
	if !store.started || !store.finished {
		t.Fatalf("run persistence not recorded: started=%v finished=%v", store.started, store.finished)
	}
	if got := store.recentSince; !got.Equal(now.Add(-24 * time.Hour)) {
		t.Fatalf("recent since = %s", got)
	}
	if len(store.sourceStateWrites) != 1 || store.sourceStateWrites[0].LatestIdentity != "guid-1" || !store.sourceStateWrites[0].CheckedAt.Equal(now) {
		t.Fatalf("source state writes = %+v", store.sourceStateWrites)
	}
	if len(store.fetchLogs) != 1 || store.fetchLogs[0].RunID != "run-1" || store.fetchLogs[0].NewItemCount != 1 {
		t.Fatalf("fetch logs = %+v", store.fetchLogs)
	}
	if store.runtimeConfigWrites["last_check"] != now.Format(time.RFC3339Nano) {
		t.Fatalf("runtime config writes = %+v", store.runtimeConfigWrites)
	}
	if !store.healthMutate {
		t.Fatal("health delta was not asked to mutate on non-dry run")
	}
}

func TestRunBriefWithDepsDryRunDoesNotPersist(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 4, 29, 12, 0, 0, 0, time.UTC)
	store := newFakeBriefStore([]Source{{
		Key:       "example",
		Label:     "Example",
		Kind:      domain.SourceKindRSS,
		URL:       "https://example.com/feed.xml",
		Section:   "technology",
		Threshold: domain.ThresholdMedium,
		Enabled:   true,
	}})
	fetcher := fakeBriefFetcher{outputs: map[string]fetchOutput{
		"example": {
			Items: []fetchedItem{{
				Title:    "Dry-run story",
				URL:      "https://example.com/dry-run",
				Identity: "guid-1",
			}},
		},
	}}

	result, err := runBriefWithDeps(ctx, briefRunDeps{
		paths:            Paths{DatabasePath: "openbrief.sqlite"},
		store:            store,
		fetcher:          fetcher,
		now:              func() time.Time { return now },
		fetchConcurrency: 1,
	}, BriefTaskRequest{Action: BriefActionRun, DryRun: true})
	if err != nil {
		t.Fatalf("runBriefWithDeps: %v", err)
	}
	if result.RunID != "dry-run" || len(result.Candidates) != 1 {
		t.Fatalf("result = %+v", result)
	}
	if store.started || store.finished || len(store.sourceStateWrites) != 0 || len(store.fetchLogs) != 0 || len(store.runtimeConfigWrites) != 0 {
		t.Fatalf("dry run persisted state: started=%v finished=%v source=%+v fetch=%+v runtime=%+v", store.started, store.finished, store.sourceStateWrites, store.fetchLogs, store.runtimeConfigWrites)
	}
	if store.healthMutate {
		t.Fatal("health delta mutated on dry run")
	}
}

func TestRunBriefWithDepsFallsBackForInvalidStoredMaxDeliveryItems(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 4, 29, 12, 0, 0, 0, time.UTC)
	store := newFakeBriefStore([]Source{{
		Key:       "example",
		Label:     "Example",
		Kind:      domain.SourceKindRSS,
		URL:       "https://example.com/feed.xml",
		Section:   "technology",
		Threshold: domain.ThresholdMedium,
		Enabled:   true,
	}})
	store.runtimeConfig[sqlite.RuntimeConfigMaxDeliveryItems] = "not-a-number"
	fetcher := fakeBriefFetcher{outputs: map[string]fetchOutput{
		"example": {Items: []fetchedItem{{Title: "Fallback story", URL: "https://example.com/fallback", Identity: "guid-1"}}},
	}}

	result, err := runBriefWithDeps(ctx, briefRunDeps{
		paths:            Paths{DatabasePath: "openbrief.sqlite"},
		store:            store,
		fetcher:          fetcher,
		now:              func() time.Time { return now },
		fetchConcurrency: 1,
	}, BriefTaskRequest{Action: BriefActionRun, DryRun: true})
	if err != nil {
		t.Fatalf("runBriefWithDeps: %v", err)
	}
	if result.MaxDeliveryItems != sqlite.DefaultMaxDeliveryItems {
		t.Fatalf("MaxDeliveryItems = %d, want %d", result.MaxDeliveryItems, sqlite.DefaultMaxDeliveryItems)
	}
	if len(result.HealthDelta.NewWarnings) == 0 || store.healthCurrent["runtime:"+sqlite.RuntimeConfigMaxDeliveryItems] == "" {
		t.Fatalf("HealthDelta = %+v current=%+v", result.HealthDelta, store.healthCurrent)
	}
}

type fakeBriefStore struct {
	sources             []Source
	runtimeConfig       map[string]string
	runtimeConfigWrites map[string]string
	sourceStates        map[string]*sqlite.SourceState
	sourceStateWrites   []sqlite.SourceState
	fetchLogs           []sqlite.FetchLog
	recentSince         time.Time
	healthCurrent       map[string]string
	healthMutate        bool
	started             bool
	finished            bool
}

func newFakeBriefStore(sources []Source) *fakeBriefStore {
	return &fakeBriefStore{
		sources: sources,
		runtimeConfig: map[string]string{
			sqlite.RuntimeConfigConfigurationVersion: sqlite.ConfigurationVersionV2,
			sqlite.RuntimeConfigMaxDeliveryItems:     "7",
		},
		runtimeConfigWrites: map[string]string{},
		sourceStates:        map[string]*sqlite.SourceState{},
	}
}

func (s *fakeBriefStore) ListSources(context.Context, bool) ([]Source, error) {
	return s.sources, nil
}

func (s *fakeBriefStore) StartRun(context.Context, bool) (string, error) {
	s.started = true
	return "run-1", nil
}

func (s *fakeBriefStore) RecentSentItems(_ context.Context, since time.Time) ([]sqlite.SentItem, error) {
	s.recentSince = since
	return nil, nil
}

func (s *fakeBriefStore) ListOutletPolicies(context.Context) ([]OutletPolicy, error) {
	return nil, nil
}

func (s *fakeBriefStore) SourceState(_ context.Context, sourceKey string) (*sqlite.SourceState, error) {
	return s.sourceStates[sourceKey], nil
}

func (s *fakeBriefStore) UpsertSourceState(_ context.Context, state sqlite.SourceState) error {
	s.sourceStateWrites = append(s.sourceStateWrites, state)
	s.sourceStates[state.SourceKey] = &state
	return nil
}

func (s *fakeBriefStore) InsertFetchLog(_ context.Context, log sqlite.FetchLog) error {
	s.fetchLogs = append(s.fetchLogs, log)
	return nil
}

func (s *fakeBriefStore) RuntimeConfig(context.Context) (map[string]string, error) {
	out := map[string]string{}
	for key, value := range s.runtimeConfig {
		out[key] = value
	}
	return out, nil
}

func (s *fakeBriefStore) RecentFetchLogs(context.Context, int) ([]sqlite.FetchLog, error) {
	return s.fetchLogs, nil
}

func (s *fakeBriefStore) HealthDelta(_ context.Context, current map[string]string, mutate bool) (sqlite.HealthDelta, error) {
	s.healthCurrent = map[string]string{}
	var delta sqlite.HealthDelta
	for key, value := range current {
		s.healthCurrent[key] = value
		delta.NewWarnings = append(delta.NewWarnings, value)
	}
	s.healthMutate = mutate
	return delta, nil
}

func (s *fakeBriefStore) SetRuntimeConfig(_ context.Context, key string, value string) error {
	s.runtimeConfigWrites[key] = value
	s.runtimeConfig[key] = value
	return nil
}

func (s *fakeBriefStore) FinishRun(context.Context, string, string, string) error {
	s.finished = true
	return nil
}

type fakeBriefFetcher struct {
	outputs map[string]fetchOutput
	errs    map[string]error
}

func (f fakeBriefFetcher) FetchDetailed(_ context.Context, source Source) (fetchOutput, error) {
	return f.outputs[source.Key], f.errs[source.Key]
}
