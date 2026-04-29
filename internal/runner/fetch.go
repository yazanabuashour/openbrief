package runner

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/yazanabuashour/openbrief/internal/storage/sqlite"
)

var (
	canonicalizationMaxAttemptsPerSource = 5
	canonicalizationConcurrency          = 8
	canonicalizationTimeout              = 3 * time.Second
	googleNewsArticleBaseURL             = "https://news.google.com/articles/"
	googleNewsBatchEndpoint              = "https://news.google.com/_/DotsSplashUi/data/batchexecute"
	googleNewsBackoffDuration            = 250 * time.Millisecond
)

type fetchedItem struct {
	Title        string
	URL          string
	PublishedAt  string
	Identity     string
	FeedIdentity string
	Outlet       string
	RSSSource    string
}

type unresolvedItem struct {
	Title  string
	URL    string
	Reason string
}

type fetchOutput struct {
	Items      []fetchedItem
	Unresolved []unresolvedItem
	Truncated  bool
}

type fetcher struct {
	client                               *http.Client
	canonicalizationMaxAttemptsPerSource int
	canonicalizationConcurrency          int
	canonicalizationTimeout              time.Duration
	googleNewsArticleBaseURL             string
	googleNewsBatchEndpoint              string
	googleNewsResolutionMemo             *googleNewsResolutionMemo
	googleNewsBackoff                    *googleNewsBackoff
}

func newFetcher() fetcher {
	return fetcher{
		client:                               &http.Client{Timeout: 20 * time.Second},
		canonicalizationMaxAttemptsPerSource: canonicalizationMaxAttemptsPerSource,
		canonicalizationConcurrency:          canonicalizationConcurrency,
		canonicalizationTimeout:              canonicalizationTimeout,
		googleNewsArticleBaseURL:             googleNewsArticleBaseURL,
		googleNewsBatchEndpoint:              googleNewsBatchEndpoint,
		googleNewsResolutionMemo:             newGoogleNewsResolutionMemo(),
		googleNewsBackoff:                    newGoogleNewsBackoff(),
	}
}

func (f fetcher) Fetch(ctx context.Context, source Source) ([]fetchedItem, error) {
	output, err := f.FetchDetailed(ctx, source)
	return output.Items, err
}

func (f fetcher) FetchDetailed(ctx context.Context, source Source) (fetchOutput, error) {
	switch source.Kind {
	case sqlite.SourceKindRSS, sqlite.SourceKindAtom:
		return f.fetchFeed(ctx, source)
	case sqlite.SourceKindGitHubRelease:
		items, err := f.fetchGitHubReleases(ctx, source)
		return fetchOutput{Items: items}, err
	default:
		return fetchOutput{}, fmt.Errorf("unsupported source kind %q", source.Kind)
	}
}

func (f fetcher) fetchFeed(ctx context.Context, source Source) (fetchOutput, error) {
	body, err := f.get(ctx, source.URL)
	if err != nil {
		return fetchOutput{}, err
	}
	items, err := parseFeed(body)
	if err != nil {
		return fetchOutput{}, err
	}
	processed, unresolved, truncated, err := f.processFeedItems(ctx, source, items)
	return fetchOutput{Items: processed, Unresolved: unresolved, Truncated: truncated}, err
}

func (f fetcher) processFeedItems(ctx context.Context, source Source, items []fetchedItem) ([]fetchedItem, []unresolvedItem, bool, error) {
	strategy, err := f.urlCanonicalizationStrategy(source.URLCanonicalization)
	if err != nil {
		return nil, nil, false, err
	}
	if strategy.networkBacked {
		return f.processBoundedCanonicalizedFeedItems(ctx, source, items, strategy)
	}
	return processLocalFeedItems(source, items), nil, false, nil
}
