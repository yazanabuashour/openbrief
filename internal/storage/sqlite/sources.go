package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"
)

type Source struct {
	Key                 string `json:"key"`
	Label               string `json:"label"`
	Kind                string `json:"kind"`
	URL                 string `json:"url,omitempty"`
	Repo                string `json:"repo,omitempty"`
	Section             string `json:"section"`
	Threshold           string `json:"threshold"`
	Enabled             bool   `json:"enabled"`
	URLCanonicalization string `json:"url_canonicalization,omitempty"`
	OutletExtraction    string `json:"outlet_extraction,omitempty"`
	DedupGroup          string `json:"dedup_group,omitempty"`
	PriorityRank        int    `json:"priority_rank,omitempty"`
	AlwaysReport        bool   `json:"always_report,omitempty"`
}

func (s *Store) ListSources(ctx context.Context, enabledOnly bool) ([]Source, error) {
	query := `SELECT key, label, kind, url, repo, section, threshold, enabled, url_canonicalization, outlet_extraction, dedup_group, priority_rank, always_report FROM brief_source`
	if enabledOnly {
		query += ` WHERE enabled = 1`
	}
	query += ` ORDER BY key`
	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = rows.Close()
	}()
	var sources []Source
	for rows.Next() {
		var source Source
		var enabled int
		var alwaysReport int
		if err := rows.Scan(
			&source.Key,
			&source.Label,
			&source.Kind,
			&source.URL,
			&source.Repo,
			&source.Section,
			&source.Threshold,
			&enabled,
			&source.URLCanonicalization,
			&source.OutletExtraction,
			&source.DedupGroup,
			&source.PriorityRank,
			&alwaysReport,
		); err != nil {
			return nil, err
		}
		source.Enabled = enabled == 1
		source.AlwaysReport = alwaysReport == 1
		sources = append(sources, source)
	}
	return sources, rows.Err()
}

func (s *Store) ReplaceSources(ctx context.Context, sources []Source) ([]Source, error) {
	normalized, err := normalizeSources(sources)
	if err != nil {
		return nil, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM brief_source`); err != nil {
		_ = tx.Rollback()
		return nil, err
	}
	for _, source := range normalized {
		if err := upsertSourceTx(ctx, tx, source, s.now()); err != nil {
			_ = tx.Rollback()
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return s.ListSources(ctx, false)
}

func (s *Store) UpsertSource(ctx context.Context, source Source) (Source, error) {
	normalized, err := NormalizeSource(source)
	if err != nil {
		return Source{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Source{}, err
	}
	if err := upsertSourceTx(ctx, tx, normalized, s.now()); err != nil {
		_ = tx.Rollback()
		return Source{}, err
	}
	if err := tx.Commit(); err != nil {
		return Source{}, err
	}
	return normalized, nil
}

func (s *Store) DeleteSource(ctx context.Context, key string) error {
	key = strings.TrimSpace(key)
	if key == "" {
		return errors.New("source key is required")
	}
	_, err := s.db.ExecContext(ctx, `DELETE FROM brief_source WHERE key = ?`, key)
	return err
}

func upsertSourceTx(ctx context.Context, tx *sql.Tx, source Source, now time.Time) error {
	ts := now.Format(time.RFC3339Nano)
	_, err := tx.ExecContext(ctx, `
INSERT INTO brief_source (key, label, kind, url, repo, section, threshold, enabled, url_canonicalization, outlet_extraction, dedup_group, priority_rank, always_report, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(key) DO UPDATE SET
	label = excluded.label,
	kind = excluded.kind,
	url = excluded.url,
	repo = excluded.repo,
	section = excluded.section,
	threshold = excluded.threshold,
	enabled = excluded.enabled,
	url_canonicalization = excluded.url_canonicalization,
	outlet_extraction = excluded.outlet_extraction,
	dedup_group = excluded.dedup_group,
	priority_rank = excluded.priority_rank,
	always_report = excluded.always_report,
	updated_at = excluded.updated_at`,
		source.Key,
		source.Label,
		source.Kind,
		source.URL,
		source.Repo,
		source.Section,
		source.Threshold,
		boolInt(source.Enabled),
		source.URLCanonicalization,
		source.OutletExtraction,
		source.DedupGroup,
		source.PriorityRank,
		boolInt(source.AlwaysReport),
		ts,
		ts,
	)
	return err
}

func normalizeSources(sources []Source) ([]Source, error) {
	seen := map[string]struct{}{}
	normalized := make([]Source, 0, len(sources))
	for _, source := range sources {
		next, err := NormalizeSource(source)
		if err != nil {
			return nil, err
		}
		if _, ok := seen[next.Key]; ok {
			return nil, fmt.Errorf("duplicate source key %q", next.Key)
		}
		seen[next.Key] = struct{}{}
		normalized = append(normalized, next)
	}
	return normalized, nil
}

func NormalizeSource(source Source) (Source, error) {
	source.Key = strings.TrimSpace(strings.ToLower(source.Key))
	source.Label = strings.TrimSpace(source.Label)
	source.Kind = strings.TrimSpace(strings.ToLower(source.Kind))
	source.URL = strings.TrimSpace(source.URL)
	source.Repo = strings.TrimSpace(source.Repo)
	source.Section = strings.TrimSpace(strings.ToLower(source.Section))
	source.Threshold = strings.TrimSpace(strings.ToLower(source.Threshold))
	source.URLCanonicalization = strings.TrimSpace(strings.ToLower(source.URLCanonicalization))
	source.OutletExtraction = strings.TrimSpace(strings.ToLower(source.OutletExtraction))
	source.DedupGroup = strings.TrimSpace(strings.ToLower(source.DedupGroup))
	if source.Key == "" || !sourceKeyPattern.MatchString(source.Key) {
		return Source{}, errors.New("source key must be lowercase letters, numbers, dot, underscore, or hyphen")
	}
	if source.Label == "" {
		return Source{}, fmt.Errorf("source %q label is required", source.Key)
	}
	if source.Section == "" {
		return Source{}, fmt.Errorf("source %q section is required", source.Key)
	}
	if source.Threshold == "" {
		source.Threshold = ThresholdMedium
	}
	if source.URLCanonicalization == "" {
		source.URLCanonicalization = URLCanonicalizationNone
	}
	if source.OutletExtraction == "" {
		source.OutletExtraction = OutletExtractionNone
	}
	switch source.Threshold {
	case ThresholdAlways, ThresholdMedium, ThresholdHigh, ThresholdAudit:
	default:
		return Source{}, fmt.Errorf("source %q threshold must be always, medium, high, or audit", source.Key)
	}
	switch source.URLCanonicalization {
	case URLCanonicalizationNone, URLCanonicalizationFeedBurnerRedirect, URLCanonicalizationGoogleNewsArticle:
	default:
		return Source{}, fmt.Errorf("source %q url_canonicalization must be none, feedburner_redirect, or google_news_article_url", source.Key)
	}
	switch source.OutletExtraction {
	case OutletExtractionNone, OutletExtractionTitleSuffix, OutletExtractionURLHost, OutletExtractionRSSSource:
	default:
		return Source{}, fmt.Errorf("source %q outlet_extraction must be none, title_suffix, url_host, or rss_source", source.Key)
	}
	switch source.Kind {
	case SourceKindRSS, SourceKindAtom:
		if err := validateFetchURL(source.URL); err != nil {
			return Source{}, fmt.Errorf("source %q url: %w", source.Key, err)
		}
	case SourceKindGitHubRelease:
		if source.Repo == "" && source.URL == "" {
			return Source{}, fmt.Errorf("source %q repo or url is required for github_release", source.Key)
		}
		if source.Repo != "" && !gitHubRepoPattern.MatchString(source.Repo) {
			return Source{}, fmt.Errorf("source %q repo must be owner/name", source.Key)
		}
		if source.URL != "" {
			if err := validateFetchURL(source.URL); err != nil {
				return Source{}, fmt.Errorf("source %q url: %w", source.Key, err)
			}
		}
	default:
		return Source{}, fmt.Errorf("source %q kind must be rss, atom, or github_release", source.Key)
	}
	return source, nil
}

func validateFetchURL(value string) error {
	if strings.TrimSpace(value) == "" {
		return errors.New("is required")
	}
	parsed, err := url.Parse(value)
	if err != nil {
		return err
	}
	if parsed.Scheme == "file" && allowFileURLsForEval() {
		if parsed.Path == "" {
			return errors.New("file URL must include a path")
		}
		return nil
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return errors.New("must be http or https")
	}
	if parsed.Host == "" {
		return errors.New("must include a host")
	}
	return nil
}

func allowFileURLsForEval() bool {
	return os.Getenv(evalAllowFileURLsEnv) == "1"
}

func (s *Store) UpsertSourceState(ctx context.Context, state SourceState) error {
	if strings.TrimSpace(state.SourceKey) == "" || strings.TrimSpace(state.LatestIdentity) == "" {
		return errors.New("source state key and latest identity are required")
	}
	checkedAt := state.CheckedAt
	if checkedAt.IsZero() {
		checkedAt = s.now()
	}
	_, err := s.db.ExecContext(ctx, `
INSERT INTO source_state (source_key, latest_identity, latest_feed_identity, latest_title, latest_url, latest_published_at, checked_at)
VALUES (?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(source_key) DO UPDATE SET
	latest_identity = excluded.latest_identity,
	latest_feed_identity = excluded.latest_feed_identity,
	latest_title = excluded.latest_title,
	latest_url = excluded.latest_url,
	latest_published_at = excluded.latest_published_at,
	checked_at = excluded.checked_at`,
		state.SourceKey, state.LatestIdentity, state.LatestFeedIdentity, state.LatestTitle, state.LatestURL, state.LatestPublishedAt, checkedAt.UTC().Format(time.RFC3339Nano))
	return err
}
