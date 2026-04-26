package runner

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/yazanabuashour/openbrief/internal/storage/sqlite"
)

var (
	canonicalizationMaxAttemptsPerSource = 5
	canonicalizationConcurrency          = 8
	canonicalizationTimeout              = 3 * time.Second
	googleNewsArticleBaseURL             = "https://news.google.com/articles/"
	googleNewsBatchEndpoint              = "https://news.google.com/_/DotsSplashUi/data/batchexecute"
)

const canonicalizationSkippedReason = "url canonicalization skipped after 5-item source limit"

const googleNewsArticleResolverKind = "google_news_article_url"

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
	case "", sqlite.URLCanonicalizationNone:
		return urlCanonicalizationStrategy{}, nil
	case sqlite.URLCanonicalizationFeedBurnerRedirect:
		return urlCanonicalizationStrategy{
			networkBacked: true,
			shouldAttempt: func(value string) bool {
				return strings.TrimSpace(value) != ""
			},
			resolve: f.followRedirect,
		}, nil
	case sqlite.URLCanonicalizationGoogleNewsArticle:
		return urlCanonicalizationStrategy{
			networkBacked: true,
			shouldAttempt: isGoogleNewsArticleURL,
			resolve:       f.resolveMemoizedGoogleNewsArticleURL,
		}, nil
	default:
		return urlCanonicalizationStrategy{}, fmt.Errorf("unsupported url canonicalization strategy %q", strategy)
	}
}

type googleNewsResolutionKey struct {
	kind string
	url  string
}

type googleNewsResolutionResult struct {
	url string
	err error
}

type googleNewsResolutionCall struct {
	done   chan struct{}
	result googleNewsResolutionResult
}

type googleNewsResolutionMemo struct {
	mu    sync.Mutex
	calls map[googleNewsResolutionKey]*googleNewsResolutionCall
}

func newGoogleNewsResolutionMemo() *googleNewsResolutionMemo {
	return &googleNewsResolutionMemo{calls: map[googleNewsResolutionKey]*googleNewsResolutionCall{}}
}

func (m *googleNewsResolutionMemo) resolve(ctx context.Context, key googleNewsResolutionKey, resolve func(context.Context) (string, error)) (string, error) {
	if m == nil {
		return resolve(ctx)
	}

	m.mu.Lock()
	if existing, ok := m.calls[key]; ok {
		m.mu.Unlock()
		select {
		case <-existing.done:
			return existing.result.url, existing.result.err
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}

	call := &googleNewsResolutionCall{done: make(chan struct{})}
	m.calls[key] = call
	m.mu.Unlock()

	call.result.url, call.result.err = resolve(ctx)
	if call.result.err != nil {
		m.mu.Lock()
		if m.calls[key] == call {
			delete(m.calls, key)
		}
		m.mu.Unlock()
	}
	close(call.done)
	return call.result.url, call.result.err
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
				if source.OutletExtraction == sqlite.OutletExtractionURLHost {
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

func (f fetcher) fetchGitHubReleases(ctx context.Context, source Source) ([]fetchedItem, error) {
	endpoint := strings.TrimSpace(source.URL)
	if endpoint == "" {
		endpoint = "https://api.github.com/repos/" + source.Repo + "/releases?per_page=10"
	}
	body, err := f.get(ctx, endpoint)
	if err != nil {
		return nil, err
	}
	var releases []struct {
		TagName     string `json:"tag_name"`
		Name        string `json:"name"`
		HTMLURL     string `json:"html_url"`
		PublishedAt string `json:"published_at"`
		Draft       bool   `json:"draft"`
		Prerelease  bool   `json:"prerelease"`
	}
	if err := json.Unmarshal(body, &releases); err != nil {
		return nil, fmt.Errorf("parse GitHub releases JSON: %w", err)
	}
	var items []fetchedItem
	for _, release := range releases {
		if release.Draft || release.Prerelease || strings.TrimSpace(release.TagName) == "" {
			continue
		}
		title := strings.TrimSpace(release.Name)
		if title == "" {
			title = release.TagName
		}
		if source.Repo != "" {
			title = source.Repo + " " + title
		}
		releaseURL := release.HTMLURL
		if releaseURL == "" && source.Repo != "" {
			releaseURL = "https://github.com/" + source.Repo + "/releases/tag/" + release.TagName
		}
		items = append(items, fetchedItem{
			Title:       title,
			URL:         releaseURL,
			PublishedAt: release.PublishedAt,
			Identity:    release.TagName,
		})
	}
	return items, nil
}

func (f fetcher) get(ctx context.Context, endpoint string) ([]byte, error) {
	parsed, err := url.Parse(endpoint)
	if err == nil && parsed.Scheme == "file" {
		if !allowFileURLsForEval() {
			return nil, fmt.Errorf("file URLs are only available for isolated eval fixtures")
		}
		if parsed.Path == "" {
			return nil, fmt.Errorf("file URL must include a path")
		}
		return os.ReadFile(parsed.Path)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "openbrief/0")
	resp, err := f.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = resp.Body.Close()
	}()
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("HTTP %d for %s", resp.StatusCode, endpoint)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	return body, nil
}

func allowFileURLsForEval() bool {
	return os.Getenv("OPENBRIEF_EVAL_ALLOW_FILE_URLS") == "1"
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

var googleNewsArticlePattern = regexp.MustCompile(`^https://news\.google\.com/rss/articles/([^?/#]+)`)

func isGoogleNewsArticleURL(value string) bool {
	return googleNewsArticlePattern.MatchString(strings.TrimSpace(value))
}

func googleNewsArticleID(value string) (string, error) {
	match := googleNewsArticlePattern.FindStringSubmatch(strings.TrimSpace(value))
	if len(match) != 2 {
		return "", fmt.Errorf("not a Google News article URL")
	}
	return match[1], nil
}

func (f fetcher) resolveGoogleNewsArticleURL(ctx context.Context, value string) (string, error) {
	return f.resolveGoogleNewsArticleURLWithEndpoints(ctx, value, f.googleNewsArticleBaseURL, f.googleNewsBatchEndpoint)
}

func (f fetcher) resolveMemoizedGoogleNewsArticleURL(ctx context.Context, value string) (string, error) {
	key := googleNewsResolutionKey{kind: googleNewsArticleResolverKind, url: value}
	return f.googleNewsResolutionMemo.resolve(ctx, key, func(resolveCtx context.Context) (string, error) {
		return f.resolveGoogleNewsArticleURL(resolveCtx, value)
	})
}

func (f fetcher) resolveGoogleNewsArticleURLWithEndpoints(ctx context.Context, value string, articleBase string, batchEndpoint string) (string, error) {
	articleID, err := googleNewsArticleID(value)
	if err != nil {
		return "", err
	}
	body, err := f.get(ctx, articleBase+articleID)
	if err != nil {
		return "", normalizeGoogleResolverError(err)
	}
	params, err := parseGoogleNewsDecodingParams(string(body), articleID)
	if err != nil {
		return "", err
	}
	raw, err := f.postForm(ctx, batchEndpoint, buildGoogleNewsBatchBody(params))
	if err != nil {
		return "", normalizeGoogleResolverError(err)
	}
	resolved, err := parseGoogleNewsBatchResponse(string(raw))
	if err != nil {
		return "", err
	}
	if !isResolvedPublisherURL(resolved) {
		return "", fmt.Errorf("google decode RPC returned non-publisher URL")
	}
	return resolved, nil
}

type googleNewsDecodingParams struct {
	articleID string
	signature string
	timestamp string
}

func parseGoogleNewsDecodingParams(html string, articleID string) (googleNewsDecodingParams, error) {
	signature := firstSubmatch(html, `data-n-a-sg="([^"]+)"`)
	timestamp := firstSubmatch(html, `data-n-a-ts="([^"]+)"`)
	if signature == "" || timestamp == "" {
		return googleNewsDecodingParams{}, fmt.Errorf("missing decode params for Google News article %s", articleID)
	}
	return googleNewsDecodingParams{articleID: articleID, signature: signature, timestamp: timestamp}, nil
}

func buildGoogleNewsBatchBody(params googleNewsDecodingParams) string {
	articlesReq := []any{
		"Fbv4je",
		fmt.Sprintf(`["garturlreq",[["X","X",["X","X"],null,null,1,1,"US:en",null,1,null,null,null,null,null,0,1],"X","X",1,[1,1,1],1,1,null,0,0,null,0],"%s",%s,"%s"]`, params.articleID, params.timestamp, params.signature),
	}
	wrapped, _ := json.Marshal([]any{[]any{articlesReq}})
	return url.Values{"f.req": []string{string(wrapped)}}.Encode()
}

func parseGoogleNewsBatchResponse(raw string) (string, error) {
	parts := strings.Split(raw, "\n\n")
	if len(parts) < 2 {
		return "", fmt.Errorf("google decode RPC returned no publisher URL")
	}
	var rows []any
	if err := json.Unmarshal([]byte(parts[1]), &rows); err != nil {
		return "", fmt.Errorf("parse Google decode RPC response: %w", err)
	}
	for _, rowValue := range rows {
		row, ok := rowValue.([]any)
		if !ok || len(row) < 3 || row[0] != "wrb.fr" || row[1] != "Fbv4je" {
			continue
		}
		encoded, ok := row[2].(string)
		if !ok {
			continue
		}
		var decoded []any
		if err := json.Unmarshal([]byte(encoded), &decoded); err != nil {
			continue
		}
		if len(decoded) > 1 {
			if resolved, ok := decoded[1].(string); ok && strings.TrimSpace(resolved) != "" {
				return strings.TrimSpace(resolved), nil
			}
		}
	}
	return "", fmt.Errorf("google decode RPC returned no publisher URL")
}

func isResolvedPublisherURL(value string) bool {
	parsed, err := url.Parse(value)
	if err != nil {
		return false
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return false
	}
	return !strings.EqualFold(parsed.Hostname(), "news.google.com")
}

func normalizeGoogleResolverError(err error) error {
	if err == nil {
		return nil
	}
	message := err.Error()
	if strings.HasPrefix(message, "HTTP ") {
		parts := strings.Fields(message)
		if len(parts) >= 2 {
			return fmt.Errorf("HTTP %s while fetching Google News article page", parts[1])
		}
	}
	return err
}

func (f fetcher) postForm(ctx context.Context, endpoint string, body string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded;charset=UTF-8")
	req.Header.Set("User-Agent", "openbrief/0")
	resp, err := f.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = resp.Body.Close()
	}()
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("HTTP %d for %s", resp.StatusCode, endpoint)
	}
	return io.ReadAll(resp.Body)
}

func firstSubmatch(text string, pattern string) string {
	matches := regexp.MustCompile(pattern).FindStringSubmatch(text)
	if len(matches) < 2 {
		return ""
	}
	return strings.TrimSpace(matches[1])
}

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

type rssFeed struct {
	XMLName xml.Name `xml:"rss"`
	Channel struct {
		Items []rssItem `xml:"item"`
	} `xml:"channel"`
}

type rssItem struct {
	Title   string `xml:"title"`
	Link    string `xml:"link"`
	GUID    string `xml:"guid"`
	PubDate string `xml:"pubDate"`
	Source  struct {
		Text string `xml:",chardata"`
	} `xml:"source"`
}

type atomFeed struct {
	XMLName xml.Name    `xml:"feed"`
	Entries []atomEntry `xml:"entry"`
}

type atomEntry struct {
	Title   string     `xml:"title"`
	ID      string     `xml:"id"`
	Updated string     `xml:"updated"`
	Links   []atomLink `xml:"link"`
}

type atomLink struct {
	Href string `xml:"href,attr"`
	Rel  string `xml:"rel,attr"`
}

func parseFeed(body []byte) ([]fetchedItem, error) {
	var rss rssFeed
	if err := xml.Unmarshal(body, &rss); err == nil && rss.XMLName.Local == "rss" {
		items := make([]fetchedItem, 0, len(rss.Channel.Items))
		for _, item := range rss.Channel.Items {
			title := cleanText(item.Title)
			link := strings.TrimSpace(item.Link)
			identity := strings.TrimSpace(item.GUID)
			if identity == "" {
				identity = link
			}
			if identity == "" {
				identity = title
			}
			if title != "" {
				items = append(items, fetchedItem{Title: title, URL: link, PublishedAt: strings.TrimSpace(item.PubDate), Identity: identity, FeedIdentity: identity, RSSSource: item.Source.Text})
			}
		}
		return items, nil
	}
	var atom atomFeed
	if err := xml.Unmarshal(body, &atom); err != nil {
		return nil, fmt.Errorf("parse feed XML: %w", err)
	}
	if atom.XMLName.Local != "feed" {
		return nil, fmt.Errorf("parse feed XML: unsupported feed root %q", atom.XMLName.Local)
	}
	items := make([]fetchedItem, 0, len(atom.Entries))
	for _, entry := range atom.Entries {
		title := cleanText(entry.Title)
		link := atomEntryLink(entry)
		identity := strings.TrimSpace(entry.ID)
		if identity == "" {
			identity = link
		}
		if identity == "" {
			identity = title
		}
		if title != "" {
			items = append(items, fetchedItem{Title: title, URL: link, PublishedAt: strings.TrimSpace(entry.Updated), Identity: identity, FeedIdentity: identity})
		}
	}
	return items, nil
}

func atomEntryLink(entry atomEntry) string {
	if len(entry.Links) == 0 {
		return ""
	}
	for _, link := range entry.Links {
		if link.Rel == "" || link.Rel == "alternate" {
			return strings.TrimSpace(link.Href)
		}
	}
	return strings.TrimSpace(entry.Links[0].Href)
}

func cleanText(value string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
}
