package runner

import (
	"net/url"
	"strings"

	"github.com/yazanabuashour/openbrief/internal/storage/sqlite"
)

func extractOutlet(source Source, item fetchedItem) string {
	switch source.OutletExtraction {
	case sqlite.OutletExtractionTitleSuffix:
		parts := strings.Split(item.Title, " - ")
		if len(parts) < 2 {
			return ""
		}
		return strings.TrimSpace(parts[len(parts)-1])
	case sqlite.OutletExtractionURLHost:
		parsed, err := url.Parse(item.URL)
		if err != nil {
			return ""
		}
		return strings.TrimPrefix(strings.ToLower(parsed.Hostname()), "www.")
	case sqlite.OutletExtractionRSSSource:
		return cleanText(item.RSSSource)
	default:
		return ""
	}
}
