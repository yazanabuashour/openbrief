package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/yazanabuashour/openbrief/internal/domain"
	_ "modernc.org/sqlite"
)

const (
	SourceKindRSS           = domain.SourceKindRSS
	SourceKindAtom          = domain.SourceKindAtom
	SourceKindGitHubRelease = domain.SourceKindGitHubRelease

	ThresholdAlways = domain.ThresholdAlways
	ThresholdMedium = domain.ThresholdMedium
	ThresholdHigh   = domain.ThresholdHigh
	ThresholdAudit  = domain.ThresholdAudit

	URLCanonicalizationNone               = domain.URLCanonicalizationNone
	URLCanonicalizationFeedBurnerRedirect = domain.URLCanonicalizationFeedBurnerRedirect
	URLCanonicalizationGoogleNewsArticle  = domain.URLCanonicalizationGoogleNewsArticle

	OutletExtractionNone        = domain.OutletExtractionNone
	OutletExtractionTitleSuffix = domain.OutletExtractionTitleSuffix
	OutletExtractionURLHost     = domain.OutletExtractionURLHost
	OutletExtractionRSSSource   = domain.OutletExtractionRSSSource

	RuntimeConfigConfigurationVersion = "configuration_version"
	RuntimeConfigMaxDeliveryItems     = "max_delivery_items"
	ConfigurationVersionV2            = "v2"
	DefaultMaxDeliveryItems           = 7
	MaxDeliveryItemsUpperBound        = 25
)

type Source = domain.Source
type OutletPolicy = domain.OutletPolicy

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
