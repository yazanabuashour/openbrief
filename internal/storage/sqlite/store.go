package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
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

	URLCanonicalizationNone               = "none"
	URLCanonicalizationFeedBurnerRedirect = "feedburner_redirect"
	URLCanonicalizationGoogleNewsArticle  = "google_news_article_url"

	OutletExtractionNone        = "none"
	OutletExtractionTitleSuffix = "title_suffix"
	OutletExtractionURLHost     = "url_host"
	OutletExtractionRSSSource   = "rss_source"

	RuntimeConfigConfigurationVersion = "configuration_version"
	RuntimeConfigMaxDeliveryItems     = "max_delivery_items"
	ConfigurationVersionV2            = "v2"
	DefaultMaxDeliveryItems           = 7
	MaxDeliveryItemsUpperBound        = 25
)

var (
	sourceKeyPattern  = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]*$`)
	gitHubRepoPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9-]*/[A-Za-z0-9._-]+$`)
)

const evalAllowFileURLsEnv = "OPENBRIEF_EVAL_ALLOW_FILE_URLS"

type Config struct {
	DatabasePath string
}

type Store struct {
	db  *sql.DB
	now func() time.Time
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
