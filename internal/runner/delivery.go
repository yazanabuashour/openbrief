package runner

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/yazanabuashour/openbrief/internal/runclient"
	"github.com/yazanabuashour/openbrief/internal/storage/sqlite"
)

var deliveryBulletPattern = regexp.MustCompile(`(?m)^-\s+\[([^\]]+)\]\(<([^>]+)>\)\s*$`)

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
