package sqlite

import (
	"context"
	"sort"
	"time"
)

type FetchLog struct {
	RunID        string
	SourceKey    string
	Status       string
	Error        string
	ItemCount    int
	NewItemCount int
	CreatedAt    time.Time
}

type HealthDelta struct {
	NewWarnings      []string `json:"new_warnings,omitempty"`
	ResolvedWarnings []string `json:"resolved_warnings,omitempty"`
}

func (s *Store) StartRun(ctx context.Context, dryRun bool) (string, error) {
	id, err := newRunID()
	if err != nil {
		return "", err
	}
	_, err = s.db.ExecContext(ctx, `
INSERT INTO brief_run (id, started_at, dry_run, status, summary)
VALUES (?, ?, ?, 'running', '')`, id, s.now().Format(time.RFC3339Nano), boolInt(dryRun))
	return id, err
}

func (s *Store) FinishRun(ctx context.Context, id string, status string, summary string) error {
	_, err := s.db.ExecContext(ctx, `
UPDATE brief_run SET finished_at = ?, status = ?, summary = ? WHERE id = ?`,
		s.now().Format(time.RFC3339Nano), status, summary, id)
	return err
}

func (s *Store) InsertFetchLog(ctx context.Context, log FetchLog) error {
	createdAt := log.CreatedAt
	if createdAt.IsZero() {
		createdAt = s.now()
	}
	_, err := s.db.ExecContext(ctx, `
INSERT INTO fetch_log (run_id, source_key, status, error, item_count, new_item_count, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?)`,
		log.RunID, log.SourceKey, log.Status, log.Error, log.ItemCount, log.NewItemCount, createdAt.UTC().Format(time.RFC3339Nano))
	return err
}

func (s *Store) RecentFetchLogs(ctx context.Context, limit int) ([]FetchLog, error) {
	if limit <= 0 {
		limit = 200
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT run_id, source_key, status, error, item_count, new_item_count, created_at
FROM fetch_log
ORDER BY id DESC
LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = rows.Close()
	}()
	var logs []FetchLog
	for rows.Next() {
		var log FetchLog
		var createdAt string
		if err := rows.Scan(&log.RunID, &log.SourceKey, &log.Status, &log.Error, &log.ItemCount, &log.NewItemCount, &createdAt); err != nil {
			return nil, err
		}
		log.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
		logs = append(logs, log)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for i, j := 0, len(logs)-1; i < j; i, j = i+1, j-1 {
		logs[i], logs[j] = logs[j], logs[i]
	}
	return logs, nil
}

func (s *Store) HealthDelta(ctx context.Context, current map[string]string, mutate bool) (HealthDelta, error) {
	prior, err := s.activeWarnings(ctx)
	if err != nil {
		return HealthDelta{}, err
	}
	var delta HealthDelta
	for key, message := range current {
		if _, ok := prior[key]; !ok {
			delta.NewWarnings = append(delta.NewWarnings, message)
		}
	}
	for key, message := range prior {
		if _, ok := current[key]; !ok {
			delta.ResolvedWarnings = append(delta.ResolvedWarnings, message)
		}
	}
	sort.Strings(delta.NewWarnings)
	sort.Strings(delta.ResolvedWarnings)
	if !mutate {
		return delta, nil
	}
	now := s.now().Format(time.RFC3339Nano)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return HealthDelta{}, err
	}
	for key, message := range current {
		if _, err := tx.ExecContext(ctx, `
INSERT INTO health_warning (key_name, message, active, first_seen_at, last_seen_at)
VALUES (?, ?, 1, ?, ?)
ON CONFLICT(key_name) DO UPDATE SET
	message = excluded.message,
	active = 1,
	last_seen_at = excluded.last_seen_at,
	resolved_at = NULL`, key, message, now, now); err != nil {
			_ = tx.Rollback()
			return HealthDelta{}, err
		}
	}
	for key := range prior {
		if _, ok := current[key]; ok {
			continue
		}
		if _, err := tx.ExecContext(ctx, `
UPDATE health_warning SET active = 0, resolved_at = ?, last_seen_at = ? WHERE key_name = ?`, now, now, key); err != nil {
			_ = tx.Rollback()
			return HealthDelta{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return HealthDelta{}, err
	}
	return delta, nil
}

func (s *Store) activeWarnings(ctx context.Context) (map[string]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT key_name, message FROM health_warning WHERE active = 1`)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = rows.Close()
	}()
	warnings := map[string]string{}
	for rows.Next() {
		var key, message string
		if err := rows.Scan(&key, &message); err != nil {
			return nil, err
		}
		warnings[key] = message
	}
	return warnings, rows.Err()
}
