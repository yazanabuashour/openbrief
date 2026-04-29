package runner

import (
	"time"

	"github.com/yazanabuashour/openbrief/internal/storage/sqlite"
)

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
