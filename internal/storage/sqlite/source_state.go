package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

type SourceState struct {
	SourceKey          string
	LatestIdentity     string
	LatestFeedIdentity string
	LatestTitle        string
	LatestURL          string
	LatestPublishedAt  string
	CheckedAt          time.Time
}

func (s *Store) SourceState(ctx context.Context, sourceKey string) (*SourceState, error) {
	row := s.db.QueryRowContext(ctx, `
SELECT source_key, latest_identity, latest_feed_identity, latest_title, latest_url, latest_published_at, checked_at
FROM source_state WHERE source_key = ?`, sourceKey)
	var state SourceState
	var checkedAt string
	if err := row.Scan(&state.SourceKey, &state.LatestIdentity, &state.LatestFeedIdentity, &state.LatestTitle, &state.LatestURL, &state.LatestPublishedAt, &checkedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	parsed, _ := time.Parse(time.RFC3339Nano, checkedAt)
	state.CheckedAt = parsed
	return &state, nil
}
