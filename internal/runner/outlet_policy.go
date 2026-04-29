package runner

import (
	"regexp"
	"strings"

	"github.com/yazanabuashour/openbrief/internal/storage/sqlite"
)

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
