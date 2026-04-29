package runner

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/yazanabuashour/openbrief/internal/runclient"
	"github.com/yazanabuashour/openbrief/internal/storage/sqlite"
)

const (
	BriefActionRun            = "run_brief"
	BriefActionRecordDelivery = "record_delivery"
	BriefActionValidate       = "validate"
)

var sourceFetchConcurrency = 4

type briefStore interface {
	ListSources(context.Context, bool) ([]Source, error)
	StartRun(context.Context, bool) (string, error)
	RecentSentItems(context.Context, time.Time) ([]sqlite.SentItem, error)
	ListOutletPolicies(context.Context) ([]OutletPolicy, error)
	SourceState(context.Context, string) (*sqlite.SourceState, error)
	UpsertSourceState(context.Context, sqlite.SourceState) error
	InsertFetchLog(context.Context, sqlite.FetchLog) error
	RuntimeConfig(context.Context) (map[string]string, error)
	RecentFetchLogs(context.Context, int) ([]sqlite.FetchLog, error)
	HealthDelta(context.Context, map[string]string, bool) (sqlite.HealthDelta, error)
	SetRuntimeConfig(context.Context, string, string) error
	FinishRun(context.Context, string, string, string) error
}

type briefFetcher interface {
	FetchDetailed(context.Context, Source) (fetchOutput, error)
}

type briefRunDeps struct {
	paths            Paths
	store            briefStore
	fetcher          briefFetcher
	now              func() time.Time
	fetchConcurrency int
}

func RunBriefTask(ctx context.Context, cfg runclient.Config, request BriefTaskRequest) (BriefTaskResult, error) {
	rt, err := runclient.Open(ctx, cfg)
	if err != nil {
		return BriefTaskResult{}, err
	}
	defer func() {
		_ = rt.Close()
	}()

	switch request.Action {
	case BriefActionValidate:
		return BriefTaskResult{Paths: rt.Paths(), Summary: "valid"}, nil
	case BriefActionRun:
		return runBrief(ctx, rt, request)
	case BriefActionRecordDelivery:
		return recordDelivery(ctx, rt, request)
	default:
		return rejectedBrief(rt.Paths(), fmt.Sprintf("unsupported brief action %q", request.Action)), nil
	}
}

func runBrief(ctx context.Context, rt *runclient.Runtime, request BriefTaskRequest) (BriefTaskResult, error) {
	return runBriefWithDeps(ctx, briefRunDeps{
		paths:            rt.Paths(),
		store:            rt.Store(),
		fetcher:          newFetcher(),
		now:              func() time.Time { return time.Now().UTC() },
		fetchConcurrency: sourceFetchConcurrency,
	}, request)
}

func runBriefWithDeps(ctx context.Context, deps briefRunDeps, request BriefTaskRequest) (BriefTaskResult, error) {
	if deps.now == nil {
		deps.now = func() time.Time { return time.Now().UTC() }
	}
	if deps.fetcher == nil {
		deps.fetcher = newFetcher()
	}
	paths := deps.paths
	store := deps.store
	sources, err := store.ListSources(ctx, true)
	if err != nil {
		return BriefTaskResult{}, err
	}
	if len(sources) == 0 {
		return rejectedBrief(paths, "no enabled sources configured"), nil
	}

	runID := "dry-run"
	if !request.DryRun {
		runID, err = store.StartRun(ctx, false)
		if err != nil {
			return BriefTaskResult{}, err
		}
	}

	recent, err := store.RecentSentItems(ctx, deps.now().Add(-24*time.Hour))
	if err != nil {
		return BriefTaskResult{}, err
	}
	outletPolicies, err := store.ListOutletPolicies(ctx)
	if err != nil {
		return BriefTaskResult{}, err
	}
	recentLookup := recentSentLookup(recent)
	var collected []collectedItem
	var suppressedRecent []SuppressedRecentItem
	var suppressedPolicy []SuppressedPolicyItem
	var suppressedUnresolved []SuppressedUnresolvedItem
	var suppressed []SuppressedItem
	var statuses []FetchStatus
	currentWarnings := map[string]string{}
	fetchResults := fetchSources(ctx, deps.fetcher, sources, deps.fetchConcurrency)

	for i, source := range sources {
		state, err := store.SourceState(ctx, source.Key)
		if err != nil {
			return BriefTaskResult{}, err
		}
		output := fetchResults[i].output
		fetchErr := fetchResults[i].err
		status := FetchStatus{SourceKey: source.Key, Status: "ok", Items: len(output.Items), SuppressedUnresolved: len(output.Unresolved)}
		for _, item := range output.Unresolved {
			suppressedUnresolved = append(suppressedUnresolved, SuppressedUnresolvedItem{
				SourceKey: source.Key,
				Title:     item.Title,
				URL:       item.URL,
				Reason:    item.Reason,
			})
		}
		if fetchErr != nil {
			status.Status = "error"
			status.Error = fetchErr.Error()
			currentWarnings["feed:"+source.Key] = fmt.Sprintf("Feed `%s` failed this run (%s)", source.Key, fetchErr.Error())
			statuses = append(statuses, status)
			if !request.DryRun {
				_ = store.InsertFetchLog(ctx, sqlite.FetchLog{
					RunID:     runID,
					SourceKey: source.Key,
					Status:    "error",
					Error:     fetchErr.Error(),
				})
			}
			continue
		}

		items, policyItems := applyOutletPolicies(source, output.Items, outletPolicies)
		status.SuppressedPolicy = len(policyItems)
		suppressedPolicy = append(suppressedPolicy, policyItems...)
		newItems := selectNewItems(items, state, output.Truncated)
		status.NewItems = len(newItems)
		for _, item := range newItems {
			collected = append(collected, collectedItem{Source: source, Item: item, BriefItem: itemToBriefItem(source, item)})
		}
		if !request.DryRun {
			if len(items) > 0 {
				top := items[0]
				if err := store.UpsertSourceState(ctx, sqlite.SourceState{
					SourceKey:          source.Key,
					LatestIdentity:     top.Identity,
					LatestFeedIdentity: top.feedIdentity(),
					LatestTitle:        top.Title,
					LatestURL:          top.URL,
					LatestPublishedAt:  top.PublishedAt,
					CheckedAt:          deps.now(),
				}); err != nil {
					return BriefTaskResult{}, err
				}
			}
			_ = store.InsertFetchLog(ctx, sqlite.FetchLog{
				RunID:        runID,
				SourceKey:    source.Key,
				Status:       "ok",
				ItemCount:    status.Items,
				NewItemCount: status.NewItems,
			})
		}
		statuses = append(statuses, status)
	}

	mustInclude, candidateItems, sameRunSuppressed := classifyAndDedupe(collected)
	suppressed = append(suppressed, sameRunSuppressed...)
	candidates, recentSuppressed, compatibilityRecentSuppressed := suppressRecentCandidates(candidateItems, recentLookup)
	suppressedRecent = append(suppressedRecent, recentSuppressed...)
	suppressed = append(suppressed, compatibilityRecentSuppressed...)

	runtimeConfig, err := store.RuntimeConfig(ctx)
	if err != nil {
		return BriefTaskResult{}, err
	}
	options := resolveBriefOptions(runtimeConfig)
	if options.Warning != "" {
		currentWarnings["runtime:"+sqlite.RuntimeConfigMaxDeliveryItems] = options.Warning
	}
	addStaleHeartbeatWarning(runtimeConfig, currentWarnings, deps.now())
	if !request.DryRun {
		fetchLogs, err := store.RecentFetchLogs(ctx, 500)
		if err != nil {
			return BriefTaskResult{}, err
		}
		addRecurringFailureWarnings(fetchLogs, enabledSourceKeys(sources), currentWarnings)
	}
	healthDelta, err := store.HealthDelta(ctx, currentWarnings, !request.DryRun)
	if err != nil {
		return BriefTaskResult{}, err
	}
	if !request.DryRun {
		if err := store.SetRuntimeConfig(ctx, "last_check", deps.now().Format(time.RFC3339Nano)); err != nil {
			return BriefTaskResult{}, err
		}
	}
	healthFootnote := buildHealthFootnote(healthDelta)
	sortBriefItems(mustInclude)
	sortBriefItems(candidates)
	sort.Slice(statuses, func(i, j int) bool { return statuses[i].SourceKey < statuses[j].SourceKey })

	summary := briefSummary(mustInclude, candidates, healthFootnote)
	if !request.DryRun {
		status := "ok"
		if err := store.FinishRun(ctx, runID, status, summary); err != nil {
			return BriefTaskResult{}, err
		}
	}
	return BriefTaskResult{
		Paths:                paths,
		RunID:                runID,
		MustInclude:          mustInclude,
		Candidates:           candidates,
		RecentSent:           convertSentItems(recent),
		Suppressed:           suppressed,
		SuppressedRecent:     suppressedRecent,
		SuppressedPolicy:     suppressedPolicy,
		SuppressedUnresolved: suppressedUnresolved,
		FetchStatus:          statuses,
		HealthFootnote:       healthFootnote,
		HealthDelta:          healthDelta,
		MaxDeliveryItems:     options.MaxDeliveryItems,
		Summary:              summary,
	}, nil
}

type sourceFetchResult struct {
	output fetchOutput
	err    error
}

func fetchSources(ctx context.Context, fetcher briefFetcher, sources []Source, concurrency int) []sourceFetchResult {
	results := make([]sourceFetchResult, len(sources))
	if concurrency <= 0 {
		concurrency = 1
	}
	sem := make(chan struct{}, concurrency)
	var wg sync.WaitGroup
	for i, source := range sources {
		wg.Add(1)
		go func(index int, src Source) {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-ctx.Done():
				results[index].err = ctx.Err()
				return
			}
			output, err := fetcher.FetchDetailed(ctx, src)
			results[index] = sourceFetchResult{output: output, err: err}
		}(i, source)
	}
	wg.Wait()
	return results
}
