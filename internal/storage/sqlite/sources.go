package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/yazanabuashour/openbrief/internal/domain"
)

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
	normalized, err := domain.NormalizeSources(sources)
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
	normalized, err := domain.NormalizeSource(source)
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

func NormalizeSource(source Source) (Source, error) {
	return domain.NormalizeSource(source)
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
