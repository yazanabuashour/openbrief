package runner

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"
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
	outletPolicies, err := store.ListOutletPolicies(ctx)
	if err != nil {
		return BriefTaskResult{}, err
	}
	recentLookup := recentSentLookup(recent)
	fetcher := newFetcher()
	var collected []collectedItem
	var suppressedRecent []SuppressedRecentItem
	var suppressedPolicy []SuppressedPolicyItem
	var suppressedUnresolved []SuppressedUnresolvedItem
	var suppressed []SuppressedItem
	var statuses []FetchStatus
	currentWarnings := map[string]string{}
	fetchResults := fetchSources(ctx, fetcher, sources)

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
					CheckedAt:          time.Now().UTC(),
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
	addStaleHeartbeatWarning(runtimeConfig, currentWarnings)
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
		if err := store.SetRuntimeConfig(ctx, "last_check", time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
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

func fetchSources(ctx context.Context, fetcher fetcher, sources []Source) []sourceFetchResult {
	results := make([]sourceFetchResult, len(sources))
	concurrency := sourceFetchConcurrency
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

func selectNewItems(items []fetchedItem, state *sqlite.SourceState, truncated bool) []fetchedItem {
	if len(items) == 0 {
		return nil
	}
	if state == nil || state.LatestIdentity == "" {
		return items[:1]
	}
	for i, item := range items {
		if itemMatchesSourceState(item, state) {
			return items[:i]
		}
	}
	if truncated {
		return items
	}
	return items[:1]
}

func itemMatchesSourceState(item fetchedItem, state *sqlite.SourceState) bool {
	if state == nil {
		return false
	}
	itemIdentities := []string{item.Identity, item.feedIdentity()}
	stateIdentities := []string{state.LatestIdentity, state.LatestFeedIdentity}
	for _, itemIdentity := range itemIdentities {
		if strings.TrimSpace(itemIdentity) == "" {
			continue
		}
		for _, stateIdentity := range stateIdentities {
			if itemIdentity == stateIdentity {
				return true
			}
		}
	}
	return false
}

func itemToBriefItem(source Source, item fetchedItem) BriefItem {
	return BriefItem{
		SourceKey:    source.Key,
		SourceLabel:  source.Label,
		Kind:         source.Kind,
		Section:      source.Section,
		Threshold:    source.Threshold,
		PriorityRank: source.PriorityRank,
		AlwaysReport: source.AlwaysReport,
		Title:        item.Title,
		URL:          item.URL,
		PublishedAt:  item.PublishedAt,
		Outlet:       item.Outlet,
	}
}

type recentLookup struct {
	items []recentLookupItem
	urls  map[string]recentLookupItem
}

type recentLookupItem struct {
	title    string
	url      string
	sentAt   time.Time
	titleKey string
	topicKey string
}

func recentSentLookup(items []sqlite.SentItem) recentLookup {
	lookup := recentLookup{urls: map[string]recentLookupItem{}}
	for _, item := range items {
		next := recentLookupItem{
			title:    item.Title,
			url:      item.URL,
			sentAt:   item.SentAt,
			titleKey: sqlite.NormalizeTitleKey(item.Title),
			topicKey: buildTopicKey(item.Title),
		}
		lookup.items = append(lookup.items, next)
		if item.URL != "" {
			lookup.urls[item.URL] = next
		}
	}
	return lookup
}

func recentlySentMatch(item BriefItem, lookup recentLookup) (recentLookupItem, bool) {
	if item.URL != "" {
		if prior, ok := lookup.urls[item.URL]; ok {
			return prior, true
		}
	}
	normalized := topicIdentity{
		titleKey: sqlite.NormalizeTitleKey(item.Title),
		topicKey: buildTopicKey(item.Title),
	}
	for _, prior := range lookup.items {
		if titlesMatchAsDuplicate(normalized, topicIdentity{titleKey: prior.titleKey, topicKey: prior.topicKey}) {
			return prior, true
		}
	}
	return recentLookupItem{}, false
}

type collectedItem struct {
	Source    Source
	Item      fetchedItem
	BriefItem BriefItem
}

func classifyAndDedupe(items []collectedItem) ([]BriefItem, []collectedItem, []SuppressedItem) {
	var mustInclude []BriefItem
	var candidates []collectedItem
	for _, item := range items {
		if isAlwaysReportSource(item.Source) {
			mustInclude = append(mustInclude, item.BriefItem)
			continue
		}
		if item.Source.Threshold != sqlite.ThresholdAudit {
			candidates = append(candidates, item)
		}
	}
	deduped, suppressed := collapseSameRunDuplicates(candidates)
	sortBriefItems(mustInclude)
	sortCollectedItems(deduped)
	return mustInclude, deduped, suppressed
}

func isAlwaysReportSource(source Source) bool {
	return source.AlwaysReport || source.Threshold == sqlite.ThresholdAlways || source.Kind == sqlite.SourceKindGitHubRelease
}

func collapseSameRunDuplicates(items []collectedItem) ([]collectedItem, []SuppressedItem) {
	var representatives []collectedItem
	var suppressed []SuppressedItem
	for _, item := range items {
		existingIndex := -1
		for i, existing := range representatives {
			if effectiveDedupGroup(item.Source) != effectiveDedupGroup(existing.Source) {
				continue
			}
			if titlesMatchAsDuplicate(topicIdentityForTitle(item.BriefItem.Title), topicIdentityForTitle(existing.BriefItem.Title)) {
				existingIndex = i
				break
			}
		}
		if existingIndex < 0 {
			representatives = append(representatives, item)
			continue
		}
		existing := representatives[existingIndex]
		if compareCollectedPreference(item, existing) < 0 {
			suppressed = append(suppressed, suppressedFromBriefItem(existing.BriefItem, "same_run_duplicate"))
			representatives[existingIndex] = item
			continue
		}
		suppressed = append(suppressed, suppressedFromBriefItem(item.BriefItem, "same_run_duplicate"))
	}
	return representatives, suppressed
}

func effectiveDedupGroup(source Source) string {
	if source.DedupGroup != "" {
		return source.DedupGroup
	}
	return source.Key
}

func suppressRecentCandidates(items []collectedItem, lookup recentLookup) ([]BriefItem, []SuppressedRecentItem, []SuppressedItem) {
	var candidates []BriefItem
	var suppressedRecent []SuppressedRecentItem
	var compatibility []SuppressedItem
	for _, item := range items {
		if prior, ok := recentlySentMatch(item.BriefItem, lookup); ok {
			suppressedRecent = append(suppressedRecent, SuppressedRecentItem{
				SourceKey:         item.Source.Key,
				Title:             item.BriefItem.Title,
				URL:               item.BriefItem.URL,
				MatchedPriorTitle: prior.title,
				PriorSentAt:       prior.sentAt.UTC().Format(time.RFC3339Nano),
			})
			compatibility = append(compatibility, suppressedFromBriefItem(item.BriefItem, "recently_sent"))
			continue
		}
		candidates = append(candidates, item.BriefItem)
	}
	sortBriefItems(candidates)
	return candidates, suppressedRecent, compatibility
}

func suppressedFromBriefItem(item BriefItem, reason string) SuppressedItem {
	return SuppressedItem{SourceKey: item.SourceKey, Title: item.Title, URL: item.URL, Reason: reason}
}

func applyOutletPolicies(source Source, items []fetchedItem, policies []sqlite.OutletPolicy) ([]fetchedItem, []SuppressedPolicyItem) {
	if len(items) == 0 || len(policies) == 0 {
		return items, nil
	}
	kept := make([]fetchedItem, 0, len(items))
	var audit []SuppressedPolicyItem
	for _, item := range items {
		policy, outlet, ok := matchingOutletPolicy(item, policies)
		if !ok {
			kept = append(kept, item)
			continue
		}
		audit = append(audit, SuppressedPolicyItem{
			SourceKey: source.Key,
			Title:     item.Title,
			URL:       item.URL,
			Outlet:    outlet,
			Policy:    policy.Policy,
		})
		if policy.Policy == "block" {
			continue
		}
		kept = append(kept, item)
	}
	return kept, audit
}

func matchingOutletPolicy(item fetchedItem, policies []sqlite.OutletPolicy) (sqlite.OutletPolicy, string, bool) {
	outlet := strings.TrimSpace(item.Outlet)
	if outlet == "" {
		return sqlite.OutletPolicy{}, "", false
	}
	normalizedOutlet := normalizeOutletName(outlet)
	for _, policy := range policies {
		if !policy.Enabled {
			continue
		}
		for _, name := range append([]string{policy.Name}, policy.Aliases...) {
			if normalizeOutletName(name) == normalizedOutlet {
				return policy, outlet, true
			}
		}
	}
	return sqlite.OutletPolicy{}, "", false
}

func normalizeOutletName(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = regexp.MustCompile(`\.(com|co|net|org|io|ai|news|uk)\b`).ReplaceAllString(value, " ")
	value = regexp.MustCompile(`[^a-z0-9]+`).ReplaceAllString(value, " ")
	return strings.Join(strings.Fields(value), " ")
}

type topicIdentity struct {
	titleKey string
	topicKey string
}

func topicIdentityForTitle(title string) topicIdentity {
	return topicIdentity{
		titleKey: sqlite.NormalizeTitleKey(title),
		topicKey: buildTopicKey(title),
	}
}

var topicStopWords = map[string]struct{}{
	"a": {}, "an": {}, "and": {}, "as": {}, "at": {}, "by": {}, "for": {}, "from": {},
	"in": {}, "into": {}, "is": {}, "of": {}, "on": {}, "or": {}, "the": {}, "to": {},
	"with": {}, "says": {}, "will": {}, "live": {}, "update": {}, "updates": {}, "report": {},
	"reportedly": {}, "analysis": {}, "watch": {}, "first": {}, "photos": {},
}

func buildTopicKey(title string) string {
	normalized := sqlite.NormalizeTitleKey(title)
	for _, prefix := range []string{"live updates ", "first thing ", "photos ", "analysis ", "report ", "watch "} {
		normalized = strings.TrimPrefix(normalized, prefix)
	}
	for _, marker := range []string{" as ", " after ", " amid ", " over ", " following ", " because "} {
		if idx := strings.Index(normalized, marker); idx >= 0 {
			normalized = strings.TrimSpace(normalized[:idx])
			break
		}
	}
	rawTokens := strings.Fields(normalized)
	var tokens []string
	seen := map[string]struct{}{}
	for i := 0; i < len(rawTokens); i++ {
		token := normalizeTopicToken(rawTokens[i])
		if token == "playstation" && i+1 < len(rawTokens) && rawTokens[i+1] == "5" {
			token = "ps5"
			i++
		}
		if _, skip := topicStopWords[token]; skip || token == "" {
			continue
		}
		if _, ok := seen[token]; ok {
			continue
		}
		seen[token] = struct{}{}
		tokens = append(tokens, token)
		if len(tokens) == 6 {
			break
		}
	}
	return strings.Join(tokens, " ")
}

func normalizeTopicToken(token string) string {
	switch token {
	case "prices", "price", "raise", "raises", "raising", "raised", "hike", "hikes", "hiked":
		return "price"
	default:
		return token
	}
}

func titlesMatchAsDuplicate(a topicIdentity, b topicIdentity) bool {
	if a.titleKey == "" || b.titleKey == "" {
		return false
	}
	if a.titleKey == b.titleKey {
		return true
	}
	aTokens := strings.Fields(a.topicKey)
	bTokens := strings.Fields(b.topicKey)
	if len(aTokens) == 0 || len(bTokens) == 0 {
		return false
	}
	bSet := map[string]struct{}{}
	for _, token := range bTokens {
		bSet[token] = struct{}{}
	}
	overlap := 0
	for _, token := range aTokens {
		if _, ok := bSet[token]; ok {
			overlap++
		}
	}
	ratio := float64(overlap) / float64(max(1, len(bTokens)))
	return overlap >= 4 && ratio >= 0.7
}

func compareCollectedPreference(a collectedItem, b collectedItem) int {
	if a.Source.PriorityRank != b.Source.PriorityRank {
		return a.Source.PriorityRank - b.Source.PriorityRank
	}
	aDate := parseItemTime(a.BriefItem.PublishedAt)
	bDate := parseItemTime(b.BriefItem.PublishedAt)
	if !aDate.Equal(bDate) {
		if aDate.After(bDate) {
			return -1
		}
		return 1
	}
	if a.Source.Key != b.Source.Key {
		return strings.Compare(a.Source.Key, b.Source.Key)
	}
	return strings.Compare(a.BriefItem.Title, b.BriefItem.Title)
}

func parseItemTime(value string) time.Time {
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, time.RFC1123Z, time.RFC1123} {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed
		}
	}
	return time.Time{}
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
	newSources := warningSources(delta.NewWarnings)
	resolvedSources := warningSources(delta.ResolvedWarnings)
	newSet := stringSet(newSources)
	resolvedSet := stringSet(resolvedSources)
	var newOnly []string
	var resolvedOnly []string
	var flapped []string
	for _, source := range newSources {
		if _, ok := resolvedSet[source]; ok {
			flapped = append(flapped, source)
			continue
		}
		newOnly = append(newOnly, source)
	}
	for _, source := range resolvedSources {
		if _, ok := newSet[source]; !ok {
			resolvedOnly = append(resolvedOnly, source)
		}
	}
	var parts []string
	if len(newOnly) > 0 {
		parts = append(parts, "NEW: "+joinWarningSources(newOnly))
	}
	if len(resolvedOnly) > 0 {
		parts = append(parts, "RESOLVED: "+joinWarningSources(resolvedOnly))
	}
	if len(flapped) > 0 {
		parts = append(parts, "FLAPPED (new+resolved this run): "+joinWarningSources(flapped))
	}
	if len(parts) == 0 {
		return ""
	}
	return "Feed health changes - " + strings.Join(parts, " | ") + "."
}

func warningSources(warnings []string) []string {
	seen := map[string]struct{}{}
	for _, warning := range warnings {
		name, ok := feedWarningSource(warning)
		if !ok {
			continue
		}
		seen[name] = struct{}{}
	}
	names := make([]string, 0, len(seen))
	for name := range seen {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func feedWarningSource(warning string) (string, bool) {
	if !strings.HasPrefix(warning, "Feed `") {
		return "", false
	}
	rest := strings.TrimPrefix(warning, "Feed `")
	end := strings.Index(rest, "`")
	if end < 0 {
		return "", false
	}
	return rest[:end], true
}

func stringSet(values []string) map[string]struct{} {
	out := map[string]struct{}{}
	for _, value := range values {
		out[value] = struct{}{}
	}
	return out
}

func joinWarningSources(names []string) string {
	out := make([]string, 0, len(names))
	for _, name := range names {
		out = append(out, "`"+name+"`")
	}
	return strings.Join(out, ", ")
}

func addStaleHeartbeatWarning(runtimeConfig map[string]string, currentWarnings map[string]string) {
	lastCheck := strings.TrimSpace(runtimeConfig["last_check"])
	if lastCheck == "" {
		return
	}
	checkedAt, err := time.Parse(time.RFC3339Nano, lastCheck)
	if err != nil {
		return
	}
	diff := time.Since(checkedAt)
	if diff <= 4*time.Hour {
		return
	}
	hours := int(diff.Hours())
	minutes := int(diff.Minutes()) % 60
	currentWarnings["runtime:last_check"] = fmt.Sprintf("`last_check` was %dh %dm ago - heartbeat may be stalled", hours, minutes)
}

func enabledSourceKeys(sources []Source) map[string]struct{} {
	keys := make(map[string]struct{}, len(sources))
	for _, source := range sources {
		keys[source.Key] = struct{}{}
	}
	return keys
}

func addRecurringFailureWarnings(logs []sqlite.FetchLog, activeSources map[string]struct{}, currentWarnings map[string]string) {
	bySource := map[string][]sqlite.FetchLog{}
	for _, log := range logs {
		if _, ok := activeSources[log.SourceKey]; !ok {
			continue
		}
		bySource[log.SourceKey] = append(bySource[log.SourceKey], log)
	}
	for source, sourceLogs := range bySource {
		if len(sourceLogs) < 3 {
			continue
		}
		recent := sourceLogs[len(sourceLogs)-3:]
		allFailed := true
		for _, log := range recent {
			if log.Status != "error" {
				allFailed = false
				break
			}
		}
		if !allFailed {
			continue
		}
		lastErr := recent[len(recent)-1].Error
		if lastErr == "" {
			lastErr = "unknown error"
		}
		currentWarnings["feed-recurring:"+source] = fmt.Sprintf("Feed `%s` has failed 3+ consecutive runs (last error: %s)", source, lastErr)
	}
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
		if items[i].PriorityRank != items[j].PriorityRank {
			return items[i].PriorityRank < items[j].PriorityRank
		}
		if items[i].SourceKey != items[j].SourceKey {
			return items[i].SourceKey < items[j].SourceKey
		}
		return items[i].Title < items[j].Title
	})
}

func sortCollectedItems(items []collectedItem) {
	sort.SliceStable(items, func(i, j int) bool {
		return compareCollectedPreference(items[i], items[j]) < 0
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
