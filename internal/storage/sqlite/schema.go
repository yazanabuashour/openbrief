package sqlite

import (
	"context"
	"fmt"
	"time"
)

func (s *Store) configure(ctx context.Context) error {
	for _, stmt := range []string{
		"PRAGMA foreign_keys = ON",
		"PRAGMA busy_timeout = 5000",
		"PRAGMA journal_mode = WAL",
		"PRAGMA synchronous = NORMAL",
	} {
		if _, err := s.db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("configure sqlite: %w", err)
		}
	}
	return nil
}

func (s *Store) initSchema(ctx context.Context) error {
	schema := []string{
		`CREATE TABLE IF NOT EXISTS runtime_config (
			key_name TEXT PRIMARY KEY,
			value_text TEXT NOT NULL,
			updated_at TEXT NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS brief_source (
			key TEXT PRIMARY KEY,
			label TEXT NOT NULL,
			kind TEXT NOT NULL,
			url TEXT NOT NULL DEFAULT '',
			repo TEXT NOT NULL DEFAULT '',
			section TEXT NOT NULL,
			threshold TEXT NOT NULL,
			enabled INTEGER NOT NULL,
			url_canonicalization TEXT NOT NULL DEFAULT 'none',
			outlet_extraction TEXT NOT NULL DEFAULT 'none',
			dedup_group TEXT NOT NULL DEFAULT '',
			priority_rank INTEGER NOT NULL DEFAULT 0,
			always_report INTEGER NOT NULL DEFAULT 0,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS outlet_policy (
			name TEXT PRIMARY KEY,
			aliases_json TEXT NOT NULL,
			policy TEXT NOT NULL,
			note TEXT NOT NULL,
			enabled INTEGER NOT NULL,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS source_state (
			source_key TEXT PRIMARY KEY REFERENCES brief_source(key) ON DELETE CASCADE,
			latest_identity TEXT NOT NULL,
			latest_feed_identity TEXT NOT NULL DEFAULT '',
			latest_title TEXT NOT NULL,
			latest_url TEXT NOT NULL,
			latest_published_at TEXT NOT NULL,
			checked_at TEXT NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS brief_run (
			id TEXT PRIMARY KEY,
			started_at TEXT NOT NULL,
			finished_at TEXT,
			dry_run INTEGER NOT NULL,
			status TEXT NOT NULL,
			summary TEXT NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS fetch_log (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			run_id TEXT NOT NULL,
			source_key TEXT NOT NULL,
			status TEXT NOT NULL,
			error TEXT NOT NULL,
			item_count INTEGER NOT NULL,
			new_item_count INTEGER NOT NULL,
			created_at TEXT NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS health_warning (
			key_name TEXT PRIMARY KEY,
			message TEXT NOT NULL,
			active INTEGER NOT NULL,
			first_seen_at TEXT NOT NULL,
			last_seen_at TEXT NOT NULL,
			resolved_at TEXT
		);`,
		`CREATE TABLE IF NOT EXISTS delivery (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			run_id TEXT NOT NULL,
			message TEXT NOT NULL,
			delivered_at TEXT NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS sent_item (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			delivery_id INTEGER NOT NULL REFERENCES delivery(id) ON DELETE CASCADE,
			run_id TEXT NOT NULL,
			title TEXT NOT NULL,
			url TEXT NOT NULL,
			title_key TEXT NOT NULL,
			sent_at TEXT NOT NULL
		);`,
		`CREATE INDEX IF NOT EXISTS idx_sent_item_sent_at ON sent_item(sent_at DESC);`,
		`CREATE INDEX IF NOT EXISTS idx_fetch_log_run_id ON fetch_log(run_id);`,
	}
	for _, stmt := range schema {
		if _, err := s.db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("initialize sqlite schema: %w", err)
		}
	}
	now := s.now().Format(time.RFC3339Nano)
	if _, err := s.db.ExecContext(ctx, `
INSERT INTO runtime_config (key_name, value_text, updated_at)
VALUES (?, ?, ?)
ON CONFLICT(key_name) DO NOTHING`, RuntimeConfigConfigurationVersion, ConfigurationVersionV2, now); err != nil {
		return fmt.Errorf("initialize runtime config: %w", err)
	}
	if err := s.migrateSchema(ctx); err != nil {
		return err
	}
	return nil
}

func (s *Store) migrateSchema(ctx context.Context) error {
	for _, column := range []struct {
		name string
		def  string
	}{
		{name: "url_canonicalization", def: "TEXT NOT NULL DEFAULT 'none'"},
		{name: "outlet_extraction", def: "TEXT NOT NULL DEFAULT 'none'"},
		{name: "dedup_group", def: "TEXT NOT NULL DEFAULT ''"},
		{name: "priority_rank", def: "INTEGER NOT NULL DEFAULT 0"},
		{name: "always_report", def: "INTEGER NOT NULL DEFAULT 0"},
	} {
		if err := s.ensureColumn(ctx, "brief_source", column.name, column.def); err != nil {
			return err
		}
	}
	if err := s.ensureColumn(ctx, "source_state", "latest_feed_identity", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}
	now := s.now().Format(time.RFC3339Nano)
	if _, err := s.db.ExecContext(ctx, `
INSERT INTO runtime_config (key_name, value_text, updated_at)
VALUES (?, ?, ?)
ON CONFLICT(key_name) DO UPDATE SET
	value_text = excluded.value_text,
	updated_at = excluded.updated_at`, RuntimeConfigConfigurationVersion, ConfigurationVersionV2, now); err != nil {
		return fmt.Errorf("migrate runtime config: %w", err)
	}
	if _, err := s.db.ExecContext(ctx, `
INSERT INTO runtime_config (key_name, value_text, updated_at)
VALUES (?, ?, ?)
ON CONFLICT(key_name) DO NOTHING`, RuntimeConfigMaxDeliveryItems, fmt.Sprintf("%d", DefaultMaxDeliveryItems), now); err != nil {
		return fmt.Errorf("seed brief runtime config: %w", err)
	}
	return nil
}

func (s *Store) ensureColumn(ctx context.Context, table string, name string, definition string) error {
	rows, err := s.db.QueryContext(ctx, `PRAGMA table_info(`+table+`)`)
	if err != nil {
		return fmt.Errorf("inspect %s columns: %w", table, err)
	}
	defer func() {
		_ = rows.Close()
	}()
	for rows.Next() {
		var cid int
		var columnName, columnType string
		var notNull int
		var defaultValue any
		var pk int
		if err := rows.Scan(&cid, &columnName, &columnType, &notNull, &defaultValue, &pk); err != nil {
			return err
		}
		if columnName == name {
			return rows.Err()
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if _, err := s.db.ExecContext(ctx, fmt.Sprintf(`ALTER TABLE %s ADD COLUMN %s %s`, table, name, definition)); err != nil {
		return fmt.Errorf("add %s.%s: %w", table, name, err)
	}
	return nil
}
