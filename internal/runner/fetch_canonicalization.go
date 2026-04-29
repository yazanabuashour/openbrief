package runner

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"

	"github.com/yazanabuashour/openbrief/internal/domain"
)

const canonicalizationSkippedReason = "url canonicalization skipped after 5-item source limit"

type urlCanonicalizationStrategy struct {
	networkBacked bool
	shouldAttempt func(string) bool
	resolve       func(context.Context, string) (string, error)
}

type feedItemResult struct {
	item       fetchedItem
	unresolved unresolvedItem
	ok         bool
}

func (f fetcher) urlCanonicalizationStrategy(strategy string) (urlCanonicalizationStrategy, error) {
	switch strategy {
	case "", domain.URLCanonicalizationNone:
		return urlCanonicalizationStrategy{}, nil
	case domain.URLCanonicalizationFeedBurnerRedirect:
		return urlCanonicalizationStrategy{
			networkBacked: true,
			shouldAttempt: func(value string) bool {
				return strings.TrimSpace(value) != ""
			},
			resolve: f.followRedirect,
		}, nil
	case domain.URLCanonicalizationGoogleNewsArticle:
		return urlCanonicalizationStrategy{
			networkBacked: true,
			shouldAttempt: isGoogleNewsArticleURL,
			resolve:       f.resolveMemoizedGoogleNewsArticleURL,
		}, nil
	default:
		return urlCanonicalizationStrategy{}, fmt.Errorf("unsupported url canonicalization strategy %q", strategy)
	}
}

func processLocalFeedItems(source Source, items []fetchedItem) []fetchedItem {
	processed := make([]fetchedItem, 0, len(items))
	for _, item := range items {
		next := item
		if outlet := extractOutlet(source, next); outlet != "" {
			next.Outlet = outlet
		}
		processed = append(processed, next)
	}
	return processed
}

func (f fetcher) processBoundedCanonicalizedFeedItems(ctx context.Context, source Source, items []fetchedItem, strategy urlCanonicalizationStrategy) ([]fetchedItem, []unresolvedItem, bool, error) {
	results := make([]feedItemResult, len(items))
	attempts := 0
	truncated := false
	concurrency := f.canonicalizationConcurrency
	if concurrency <= 0 {
		concurrency = 1
	}
	sem := make(chan struct{}, concurrency)
	var wg sync.WaitGroup

	for i, item := range items {
		next := item
		if !strategy.shouldAttempt(next.URL) {
			if outlet := extractOutlet(source, next); outlet != "" {
				next.Outlet = outlet
			}
			results[i] = feedItemResult{item: next, ok: true}
			continue
		}

		attempts++
		if f.canonicalizationMaxAttemptsPerSource > 0 && attempts > f.canonicalizationMaxAttemptsPerSource {
			truncated = true
			results[i] = feedItemResult{unresolved: unresolvedItem{
				Title:  next.Title,
				URL:    next.URL,
				Reason: canonicalizationSkippedReason,
			}}
			continue
		}

		wg.Add(1)
		go func(index int, original fetchedItem) {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-ctx.Done():
				results[index] = feedItemResult{unresolved: unresolvedFromCanonicalizationError(original, ctx.Err())}
				return
			}

			itemCtx := ctx
			cancel := func() {}
			if f.canonicalizationTimeout > 0 {
				itemCtx, cancel = context.WithTimeout(ctx, f.canonicalizationTimeout)
			}
			defer cancel()

			canonicalURL, err := strategy.resolve(itemCtx, original.URL)
			if err != nil {
				if source.OutletExtraction == domain.OutletExtractionURLHost {
					results[index] = feedItemResult{unresolved: unresolvedFromCanonicalizationError(original, err)}
					return
				}
				next := original
				next.FeedIdentity = original.feedIdentity()
				if outlet := extractOutlet(source, next); outlet != "" {
					next.Outlet = outlet
				}
				results[index] = feedItemResult{
					item:       next,
					unresolved: unresolvedFromCanonicalizationError(original, err),
					ok:         true,
				}
				return
			}
			next := original
			next.FeedIdentity = original.feedIdentity()
			if canonicalURL != "" {
				next.URL = canonicalURL
				if next.Identity == original.URL {
					next.Identity = canonicalURL
				}
			}
			if outlet := extractOutlet(source, next); outlet != "" {
				next.Outlet = outlet
			}
			results[index] = feedItemResult{item: next, ok: true}
		}(i, next)
	}

	wg.Wait()

	processed := make([]fetchedItem, 0, len(items))
	var unresolved []unresolvedItem
	for _, result := range results {
		if result.ok {
			processed = append(processed, result.item)
		}
		if result.unresolved.Reason != "" {
			unresolved = append(unresolved, result.unresolved)
		}
	}
	return processed, unresolved, truncated, nil
}

func (item fetchedItem) feedIdentity() string {
	if strings.TrimSpace(item.FeedIdentity) != "" {
		return item.FeedIdentity
	}
	return item.Identity
}

func unresolvedFromCanonicalizationError(item fetchedItem, err error) unresolvedItem {
	reason := ""
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		reason = "url canonicalization timed out"
	case errors.Is(err, context.Canceled):
		reason = "url canonicalization canceled"
	case err != nil:
		reason = err.Error()
	default:
		reason = "url canonicalization failed"
	}
	return unresolvedItem{
		Title:  item.Title,
		URL:    item.URL,
		Reason: reason,
	}
}

func (f fetcher) followRedirect(ctx context.Context, value string) (string, error) {
	if value == "" {
		return "", nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, value, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "openbrief/0")
	resp, err := f.client.Do(req)
	if err != nil {
		return "", err
	}
	defer func() {
		_ = resp.Body.Close()
	}()
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("HTTP %d for %s", resp.StatusCode, value)
	}
	return resp.Request.URL.String(), nil
}
