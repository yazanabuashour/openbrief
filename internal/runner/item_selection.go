package runner

import (
	"strings"

	"github.com/yazanabuashour/openbrief/internal/storage/sqlite"
)

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
