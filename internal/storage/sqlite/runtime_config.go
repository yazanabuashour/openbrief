package sqlite

import (
	"context"
	"errors"
	"strings"
	"time"
)

func (s *Store) RuntimeConfig(ctx context.Context) (map[string]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT key_name, value_text FROM runtime_config ORDER BY key_name`)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = rows.Close()
	}()
	values := map[string]string{}
	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			return nil, err
		}
		values[key] = value
	}
	return values, rows.Err()
}

func (s *Store) SetRuntimeConfig(ctx context.Context, key string, value string) error {
	key = strings.TrimSpace(key)
	if key == "" {
		return errors.New("runtime config key is required")
	}
	_, err := s.db.ExecContext(ctx, `
INSERT INTO runtime_config (key_name, value_text, updated_at)
VALUES (?, ?, ?)
ON CONFLICT(key_name) DO UPDATE SET
	value_text = excluded.value_text,
	updated_at = excluded.updated_at`, key, value, s.now().Format(time.RFC3339Nano))
	return err
}
