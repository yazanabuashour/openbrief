package runner

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"
)

const googleNewsArticleResolverKind = "google_news_article_url"

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

type googleNewsBackoff struct {
	mu        sync.Mutex
	nextStart time.Time
	interval  time.Duration
}

func newGoogleNewsBackoff() *googleNewsBackoff {
	return &googleNewsBackoff{interval: googleNewsBackoffDuration}
}

func (b *googleNewsBackoff) wait(ctx context.Context) error {
	if b == nil || b.interval <= 0 {
		return nil
	}

	b.mu.Lock()
	now := time.Now()
	waitUntil := b.nextStart
	if waitUntil.IsZero() || !now.Before(waitUntil) {
		b.mu.Unlock()
		return nil
	}
	b.nextStart = waitUntil.Add(b.interval)
	b.mu.Unlock()

	timer := time.NewTimer(time.Until(waitUntil))
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (b *googleNewsBackoff) record(err error) {
	if b == nil || b.interval <= 0 || !isGoogleNewsBackoffSignal(err) {
		return
	}

	b.mu.Lock()
	defer b.mu.Unlock()
	nextStart := time.Now().Add(b.interval)
	if b.nextStart.Before(nextStart) {
		b.nextStart = nextStart
	}
}

func isGoogleNewsBackoffSignal(err error) bool {
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	if err == nil {
		return false
	}
	return strings.HasPrefix(err.Error(), "HTTP 429")
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
		if err := f.googleNewsBackoff.wait(resolveCtx); err != nil {
			return "", err
		}
		resolved, err := f.resolveGoogleNewsArticleURL(resolveCtx, value)
		f.googleNewsBackoff.record(err)
		return resolved, err
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
