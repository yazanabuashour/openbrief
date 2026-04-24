package sqlite

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

const (
	SourceKindRSS           = "rss"
	SourceKindAtom          = "atom"
	SourceKindGitHubRelease = "github_release"

	ThresholdAlways = "always"
	ThresholdMedium = "medium"
	ThresholdHigh   = "high"
	ThresholdAudit  = "audit"
)

var (
	sourceKeyPattern  = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]*$`)
	gitHubRepoPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9-]*/[A-Za-z0-9._-]+$`)
)

type Config struct {
	DatabasePath string
}

type Store struct {
	db  *sql.DB
	now func() time.Time
}

type Source struct {
	Key       string `json:"key"`
	Label     string `json:"label"`
	Kind      string `json:"kind"`
	URL       string `json:"url,omitempty"`
	Repo      string `json:"repo,omitempty"`
	Section   string `json:"section"`
	Threshold string `json:"threshold"`
	Enabled   bool   `json:"enabled"`
}

type OutletPolicy struct {
	Name    string   `json:"name"`
	Aliases []string `json:"aliases,omitempty"`
	Policy  string   `json:"policy"`
	Note    string   `json:"note,omitempty"`
	Enabled bool     `json:"enabled"`
}

type SourceState struct {
	SourceKey         string
	LatestIdentity    string
	LatestTitle       string
	LatestURL         string
	LatestPublishedAt string
	CheckedAt         time.Time
}

type FetchLog struct {
	RunID        string
	SourceKey    string
	Status       string
	Error        string
	ItemCount    int
	NewItemCount int
}

type HealthDelta struct {
	NewWarnings      []string `json:"new_warnings,omitempty"`
	ResolvedWarnings []string `json:"resolved_warnings,omitempty"`
}

type SentItem struct {
	Title  string    `json:"title"`
	URL    string    `json:"url"`
	SentAt time.Time `json:"sent_at"`
}

func New(ctx context.Context, cfg Config) (*Store, error) {
	if strings.TrimSpace(cfg.DatabasePath) == "" {
		return nil, errors.New("database path is required")
	}
	if err := os.MkdirAll(filepath.Dir(cfg.DatabasePath), 0o755); err != nil {
		return nil, fmt.Errorf("create database directory: %w", err)
	}
	db, err := sql.Open("sqlite", cfg.DatabasePath)
	if err != nil {
		return nil, fmt.Errorf("open sqlite database: %w", err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	store := &Store{
		db: db,
		now: func() time.Time {
			return time.Now().UTC()
		},
	}
	if err := store.configure(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := store.initSchema(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	return store, nil
}

func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

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
VALUES ('configuration_version', 'v1', ?)
ON CONFLICT(key_name) DO NOTHING`, now); err != nil {
		return fmt.Errorf("initialize runtime config: %w", err)
	}
	return nil
}

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

func (s *Store) ListSources(ctx context.Context, enabledOnly bool) ([]Source, error) {
	query := `SELECT key, label, kind, url, repo, section, threshold, enabled FROM brief_source`
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
		if err := rows.Scan(&source.Key, &source.Label, &source.Kind, &source.URL, &source.Repo, &source.Section, &source.Threshold, &enabled); err != nil {
			return nil, err
		}
		source.Enabled = enabled == 1
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
INSERT INTO brief_source (key, label, kind, url, repo, section, threshold, enabled, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(key) DO UPDATE SET
	label = excluded.label,
	kind = excluded.kind,
	url = excluded.url,
	repo = excluded.repo,
	section = excluded.section,
	threshold = excluded.threshold,
	enabled = excluded.enabled,
	updated_at = excluded.updated_at`,
		source.Key, source.Label, source.Kind, source.URL, source.Repo, source.Section, source.Threshold, boolInt(source.Enabled), ts, ts)
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
	switch source.Threshold {
	case ThresholdAlways, ThresholdMedium, ThresholdHigh, ThresholdAudit:
	default:
		return Source{}, fmt.Errorf("source %q threshold must be always, medium, high, or audit", source.Key)
	}
	switch source.Kind {
	case SourceKindRSS, SourceKindAtom:
		if err := validateHTTPURL(source.URL); err != nil {
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
			if err := validateHTTPURL(source.URL); err != nil {
				return Source{}, fmt.Errorf("source %q url: %w", source.Key, err)
			}
		}
	default:
		return Source{}, fmt.Errorf("source %q kind must be rss, atom, or github_release", source.Key)
	}
	return source, nil
}

func validateHTTPURL(value string) error {
	if strings.TrimSpace(value) == "" {
		return errors.New("is required")
	}
	parsed, err := url.Parse(value)
	if err != nil {
		return err
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return errors.New("must be http or https")
	}
	if parsed.Host == "" {
		return errors.New("must include a host")
	}
	return nil
}

func (s *Store) ListOutletPolicies(ctx context.Context) ([]OutletPolicy, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT name, aliases_json, policy, note, enabled FROM outlet_policy ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = rows.Close()
	}()
	var policies []OutletPolicy
	for rows.Next() {
		var policy OutletPolicy
		var aliases string
		var enabled int
		if err := rows.Scan(&policy.Name, &aliases, &policy.Policy, &policy.Note, &enabled); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(aliases), &policy.Aliases)
		policy.Enabled = enabled == 1
		policies = append(policies, policy)
	}
	return policies, rows.Err()
}

func (s *Store) ReplaceOutletPolicies(ctx context.Context, policies []OutletPolicy) ([]OutletPolicy, error) {
	normalized, err := normalizeOutletPolicies(policies)
	if err != nil {
		return nil, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM outlet_policy`); err != nil {
		_ = tx.Rollback()
		return nil, err
	}
	now := s.now().Format(time.RFC3339Nano)
	for _, policy := range normalized {
		aliases, err := json.Marshal(policy.Aliases)
		if err != nil {
			_ = tx.Rollback()
			return nil, err
		}
		if _, err := tx.ExecContext(ctx, `
INSERT INTO outlet_policy (name, aliases_json, policy, note, enabled, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?)`,
			policy.Name, string(aliases), policy.Policy, policy.Note, boolInt(policy.Enabled), now, now); err != nil {
			_ = tx.Rollback()
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return s.ListOutletPolicies(ctx)
}

func normalizeOutletPolicies(policies []OutletPolicy) ([]OutletPolicy, error) {
	seen := map[string]struct{}{}
	normalized := make([]OutletPolicy, 0, len(policies))
	for _, policy := range policies {
		policy.Name = strings.TrimSpace(policy.Name)
		policy.Policy = strings.TrimSpace(strings.ToLower(policy.Policy))
		policy.Note = strings.TrimSpace(policy.Note)
		if policy.Name == "" {
			return nil, errors.New("outlet policy name is required")
		}
		if policy.Policy == "" {
			policy.Policy = "allow"
		}
		switch policy.Policy {
		case "allow", "block", "watch":
		default:
			return nil, fmt.Errorf("outlet policy %q policy must be allow, block, or watch", policy.Name)
		}
		for i := range policy.Aliases {
			policy.Aliases[i] = strings.TrimSpace(policy.Aliases[i])
		}
		policy.Aliases = compactStrings(policy.Aliases)
		key := strings.ToLower(policy.Name)
		if _, ok := seen[key]; ok {
			return nil, fmt.Errorf("duplicate outlet policy %q", policy.Name)
		}
		seen[key] = struct{}{}
		normalized = append(normalized, policy)
	}
	return normalized, nil
}

func (s *Store) SourceState(ctx context.Context, sourceKey string) (*SourceState, error) {
	row := s.db.QueryRowContext(ctx, `
SELECT source_key, latest_identity, latest_title, latest_url, latest_published_at, checked_at
FROM source_state WHERE source_key = ?`, sourceKey)
	var state SourceState
	var checkedAt string
	if err := row.Scan(&state.SourceKey, &state.LatestIdentity, &state.LatestTitle, &state.LatestURL, &state.LatestPublishedAt, &checkedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	parsed, _ := time.Parse(time.RFC3339Nano, checkedAt)
	state.CheckedAt = parsed
	return &state, nil
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
INSERT INTO source_state (source_key, latest_identity, latest_title, latest_url, latest_published_at, checked_at)
VALUES (?, ?, ?, ?, ?, ?)
ON CONFLICT(source_key) DO UPDATE SET
	latest_identity = excluded.latest_identity,
	latest_title = excluded.latest_title,
	latest_url = excluded.latest_url,
	latest_published_at = excluded.latest_published_at,
	checked_at = excluded.checked_at`,
		state.SourceKey, state.LatestIdentity, state.LatestTitle, state.LatestURL, state.LatestPublishedAt, checkedAt.UTC().Format(time.RFC3339Nano))
	return err
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
	_, err := s.db.ExecContext(ctx, `
INSERT INTO fetch_log (run_id, source_key, status, error, item_count, new_item_count, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?)`,
		log.RunID, log.SourceKey, log.Status, log.Error, log.ItemCount, log.NewItemCount, s.now().Format(time.RFC3339Nano))
	return err
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

func (s *Store) RecentSentItems(ctx context.Context, since time.Time) ([]SentItem, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT title, url, sent_at FROM sent_item
WHERE sent_at >= ?
ORDER BY sent_at DESC, id DESC`, since.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = rows.Close()
	}()
	var items []SentItem
	for rows.Next() {
		var item SentItem
		var sentAt string
		if err := rows.Scan(&item.Title, &item.URL, &sentAt); err != nil {
			return nil, err
		}
		item.SentAt, _ = time.Parse(time.RFC3339Nano, sentAt)
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Store) InsertDelivery(ctx context.Context, runID string, message string, items []SentItem) ([]SentItem, error) {
	if strings.TrimSpace(runID) == "" {
		return nil, errors.New("run_id is required")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	deliveredAt := s.now().UTC()
	result, err := tx.ExecContext(ctx, `
INSERT INTO delivery (run_id, message, delivered_at)
VALUES (?, ?, ?)`, runID, message, deliveredAt.Format(time.RFC3339Nano))
	if err != nil {
		_ = tx.Rollback()
		return nil, err
	}
	deliveryID, err := result.LastInsertId()
	if err != nil {
		_ = tx.Rollback()
		return nil, err
	}
	for i := range items {
		items[i].SentAt = deliveredAt
		if _, err := tx.ExecContext(ctx, `
INSERT INTO sent_item (delivery_id, run_id, title, url, title_key, sent_at)
VALUES (?, ?, ?, ?, ?, ?)`,
			deliveryID, runID, items[i].Title, items[i].URL, NormalizeTitleKey(items[i].Title), deliveredAt.Format(time.RFC3339Nano)); err != nil {
			_ = tx.Rollback()
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return items, nil
}

func NormalizeTitleKey(text string) string {
	text = strings.ToLower(strings.TrimSpace(text))
	var b strings.Builder
	previousSpace := false
	for _, r := range text {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			previousSpace = false
		default:
			if !previousSpace {
				b.WriteByte(' ')
				previousSpace = true
			}
		}
	}
	return strings.Join(strings.Fields(b.String()), " ")
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func compactStrings(values []string) []string {
	seen := map[string]struct{}{}
	var out []string
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		key := strings.ToLower(value)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, value)
	}
	return out
}

func newRunID() (string, error) {
	var bytes [16]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes[:]), nil
}
