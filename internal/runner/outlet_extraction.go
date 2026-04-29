package runner

import (
	"net/url"
	"strings"

	"github.com/yazanabuashour/openbrief/internal/domain"
)

func extractOutlet(source Source, item fetchedItem) string {
	switch source.OutletExtraction {
	case domain.OutletExtractionTitleSuffix:
		parts := strings.Split(item.Title, " - ")
		if len(parts) < 2 {
			return ""
		}
		return strings.TrimSpace(parts[len(parts)-1])
	case domain.OutletExtractionURLHost:
		parsed, err := url.Parse(item.URL)
		if err != nil {
			return ""
		}
		return strings.TrimPrefix(strings.ToLower(parsed.Hostname()), "www.")
	case domain.OutletExtractionRSSSource:
		return cleanText(item.RSSSource)
	default:
		return ""
	}
}
