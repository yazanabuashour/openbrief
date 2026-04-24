package runner

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/yazanabuashour/openbrief/internal/runclient"
	"github.com/yazanabuashour/openbrief/internal/storage/sqlite"
)

const (
	BriefActionRun            = "run_brief"
	BriefActionRecordDelivery = "record_delivery"
	BriefActionValidate       = "validate"
)

var deliveryBulletPattern = regexp.MustCompile(`(?m)^-\s+\[([^\]]+)\]\(<([^>]+)>\)\s*$`)

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
	store := rt.Store()
	paths := rt.Paths()
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

	recent, err := store.RecentSentItems(ctx, time.Now().UTC().Add(-24*time.Hour))
	if err != nil {
		return BriefTaskResult{}, err
	}
	recentLookup := recentSentLookup(recent)
	fetcher := newFetcher()
	var mustInclude []BriefItem
	var candidates []BriefItem
	var suppressed []SuppressedItem
	var statuses []FetchStatus
	currentWarnings := map[string]string{}

	for _, source := range sources {
		state, err := store.SourceState(ctx, source.Key)
		if err != nil {
			return BriefTaskResult{}, err
		}
		items, fetchErr := fetcher.Fetch(ctx, source)
		status := FetchStatus{SourceKey: source.Key, Status: "ok", Items: len(items)}
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

		newItems := selectNewItems(items, state)
		status.NewItems = len(newItems)
		for _, item := range newItems {
			briefItem := itemToBriefItem(source, item)
			if isRecentlySent(briefItem, recentLookup) {
				suppressed = append(suppressed, SuppressedItem{
					SourceKey: source.Key,
					Title:     briefItem.Title,
					URL:       briefItem.URL,
					Reason:    "recently_sent",
				})
				continue
			}
			if source.Threshold == sqlite.ThresholdAlways || source.Kind == sqlite.SourceKindGitHubRelease {
				mustInclude = append(mustInclude, briefItem)
			} else if source.Threshold != sqlite.ThresholdAudit {
				candidates = append(candidates, briefItem)
			}
		}
		if !request.DryRun {
			if len(items) > 0 {
				top := items[0]
				if err := store.UpsertSourceState(ctx, sqlite.SourceState{
					SourceKey:         source.Key,
					LatestIdentity:    top.Identity,
					LatestTitle:       top.Title,
					LatestURL:         top.URL,
					LatestPublishedAt: top.PublishedAt,
					CheckedAt:         time.Now().UTC(),
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

	healthDelta, err := store.HealthDelta(ctx, currentWarnings, !request.DryRun)
	if err != nil {
		return BriefTaskResult{}, err
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
		Paths:          paths,
		RunID:          runID,
		MustInclude:    mustInclude,
		Candidates:     candidates,
		RecentSent:     convertSentItems(recent),
		Suppressed:     suppressed,
		FetchStatus:    statuses,
		HealthFootnote: healthFootnote,
		HealthDelta:    healthDelta,
		Summary:        summary,
	}, nil
}

func recordDelivery(ctx context.Context, rt *runclient.Runtime, request BriefTaskRequest) (BriefTaskResult, error) {
	paths := rt.Paths()
	if strings.TrimSpace(request.RunID) == "" {
		return rejectedBrief(paths, "run_id is required"), nil
	}
	if strings.TrimSpace(request.Message) == "" {
		return rejectedBrief(paths, "message is required"), nil
	}
	items := parseDeliveryMessage(request.Message)
	stored, err := rt.Store().InsertDelivery(ctx, request.RunID, request.Message, items)
	if err != nil {
		return BriefTaskResult{}, err
	}
	return BriefTaskResult{
		Paths:     paths,
		RunID:     request.RunID,
		SentItems: convertSentItems(stored),
		Summary:   fmt.Sprintf("recorded delivery with %d sent items", len(stored)),
	}, nil
}

func selectNewItems(items []fetchedItem, state *sqlite.SourceState) []fetchedItem {
	if len(items) == 0 {
		return nil
	}
	if state == nil || state.LatestIdentity == "" {
		return items[:1]
	}
	for i, item := range items {
		if item.Identity == state.LatestIdentity {
			return items[:i]
		}
	}
	return items[:1]
}

func itemToBriefItem(source Source, item fetchedItem) BriefItem {
	return BriefItem{
		SourceKey:   source.Key,
		SourceLabel: source.Label,
		Kind:        source.Kind,
		Section:     source.Section,
		Threshold:   source.Threshold,
		Title:       item.Title,
		URL:         item.URL,
		PublishedAt: item.PublishedAt,
	}
}

type recentLookup struct {
	urls      map[string]struct{}
	titleKeys map[string]struct{}
}

func recentSentLookup(items []sqlite.SentItem) recentLookup {
	lookup := recentLookup{urls: map[string]struct{}{}, titleKeys: map[string]struct{}{}}
	for _, item := range items {
		if item.URL != "" {
			lookup.urls[item.URL] = struct{}{}
		}
		if key := sqlite.NormalizeTitleKey(item.Title); key != "" {
			lookup.titleKeys[key] = struct{}{}
		}
	}
	return lookup
}

func isRecentlySent(item BriefItem, lookup recentLookup) bool {
	if item.URL != "" {
		if _, ok := lookup.urls[item.URL]; ok {
			return true
		}
	}
	_, ok := lookup.titleKeys[sqlite.NormalizeTitleKey(item.Title)]
	return ok
}

func parseDeliveryMessage(message string) []sqlite.SentItem {
	matches := deliveryBulletPattern.FindAllStringSubmatch(message, -1)
	items := make([]sqlite.SentItem, 0, len(matches))
	for _, match := range matches {
		title := strings.TrimSpace(match[1])
		url := strings.TrimSpace(match[2])
		if title == "" || url == "" {
			continue
		}
		items = append(items, sqlite.SentItem{Title: title, URL: url})
	}
	return items
}

func convertSentItems(items []sqlite.SentItem) []SentItem {
	out := make([]SentItem, 0, len(items))
	for _, item := range items {
		out = append(out, SentItem{Title: item.Title, URL: item.URL, SentAt: item.SentAt})
	}
	return out
}

func buildHealthFootnote(delta sqlite.HealthDelta) string {
	var parts []string
	if len(delta.NewWarnings) > 0 {
		parts = append(parts, "NEW: "+warningSources(delta.NewWarnings))
	}
	if len(delta.ResolvedWarnings) > 0 {
		parts = append(parts, "RESOLVED: "+warningSources(delta.ResolvedWarnings))
	}
	if len(parts) == 0 {
		return ""
	}
	return "Feed health changes - " + strings.Join(parts, " | ") + "."
}

func warningSources(warnings []string) string {
	var names []string
	for _, warning := range warnings {
		start := strings.Index(warning, "`")
		end := -1
		if start >= 0 {
			end = strings.Index(warning[start+1:], "`")
		}
		if start >= 0 && end >= 0 {
			names = append(names, "`"+warning[start+1:start+1+end]+"`")
		}
	}
	if len(names) == 0 {
		return strings.Join(warnings, ", ")
	}
	sort.Strings(names)
	return strings.Join(names, ", ")
}

func briefSummary(mustInclude []BriefItem, candidates []BriefItem, healthFootnote string) string {
	total := len(mustInclude) + len(candidates)
	if total == 0 && healthFootnote == "" {
		return "NO_REPLY"
	}
	return fmt.Sprintf("must_include=%d candidates=%d health_footnote=%t", len(mustInclude), len(candidates), healthFootnote != "")
}

func sortBriefItems(items []BriefItem) {
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].SourceKey != items[j].SourceKey {
			return items[i].SourceKey < items[j].SourceKey
		}
		return items[i].Title < items[j].Title
	})
}

func rejectedBrief(paths Paths, reason string) BriefTaskResult {
	return BriefTaskResult{
		Rejected:        true,
		RejectionReason: reason,
		Paths:           paths,
		Summary:         reason,
	}
}
