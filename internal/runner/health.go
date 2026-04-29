package runner

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/yazanabuashour/openbrief/internal/storage/sqlite"
)

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

func addStaleHeartbeatWarning(runtimeConfig map[string]string, currentWarnings map[string]string, now time.Time) {
	lastCheck := strings.TrimSpace(runtimeConfig["last_check"])
	if lastCheck == "" {
		return
	}
	checkedAt, err := time.Parse(time.RFC3339Nano, lastCheck)
	if err != nil {
		return
	}
	diff := now.Sub(checkedAt)
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
