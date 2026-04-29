package runner

import (
	"sort"
	"strings"
	"time"

	"github.com/yazanabuashour/openbrief/internal/domain"
	"github.com/yazanabuashour/openbrief/internal/storage/sqlite"
)

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
		if item.Source.Threshold != domain.ThresholdAudit {
			candidates = append(candidates, item)
		}
	}
	deduped, suppressed := collapseSameRunDuplicates(candidates)
	sortBriefItems(mustInclude)
	sortCollectedItems(deduped)
	return mustInclude, deduped, suppressed
}

func isAlwaysReportSource(source Source) bool {
	return source.AlwaysReport || source.Threshold == domain.ThresholdAlways || source.Kind == domain.SourceKindGitHubRelease
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
