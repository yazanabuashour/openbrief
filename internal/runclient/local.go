package runclient

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/yazanabuashour/openbrief/internal/storage/sqlite"
)

const (
	EnvDatabasePath = "OPENBRIEF_DATABASE_PATH"
	defaultAppDir   = "openbrief"
	defaultDBFile   = "openbrief.sqlite"
)

type Config struct {
	DatabasePath string
}

type Paths struct {
	DataDir      string `json:"data_dir"`
	DatabasePath string `json:"database_path"`
}

type Runtime struct {
	paths Paths
	store *sqlite.Store
}

func ResolvePaths(cfg Config) (Paths, error) {
	databasePath, err := resolveDatabasePath(cfg)
	if err != nil {
		return Paths{}, err
	}
	databasePath = filepath.Clean(databasePath)
	return Paths{
		DataDir:      filepath.Dir(databasePath),
		DatabasePath: databasePath,
	}, nil
}

func Open(ctx context.Context, cfg Config) (*Runtime, error) {
	paths, err := ResolvePaths(cfg)
	if err != nil {
		return nil, err
	}
	store, err := sqlite.New(ctx, sqlite.Config{DatabasePath: paths.DatabasePath})
	if err != nil {
		return nil, err
	}
	return &Runtime{paths: paths, store: store}, nil
}

func (r *Runtime) Close() error {
	if r == nil || r.store == nil {
		return nil
	}
	return r.store.Close()
}

func (r *Runtime) Paths() Paths {
	if r == nil {
		return Paths{}
	}
	return r.paths
}

func (r *Runtime) Store() *sqlite.Store {
	if r == nil {
		return nil
	}
	return r.store
}

func resolveDatabasePath(cfg Config) (string, error) {
	switch {
	case strings.TrimSpace(cfg.DatabasePath) != "":
		return cfg.DatabasePath, nil
	case strings.TrimSpace(os.Getenv(EnvDatabasePath)) != "":
		return os.Getenv(EnvDatabasePath), nil
	default:
		dataDir, err := defaultDataDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(dataDir, defaultDBFile), nil
	}
}

func defaultDataDir() (string, error) {
	xdgDataHome := strings.TrimSpace(os.Getenv("XDG_DATA_HOME"))
	if xdgDataHome != "" && filepath.IsAbs(xdgDataHome) {
		return filepath.Join(xdgDataHome, defaultAppDir), nil
	}
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve user home: %w", err)
	}
	return filepath.Join(homeDir, ".local", "share", defaultAppDir), nil
}
