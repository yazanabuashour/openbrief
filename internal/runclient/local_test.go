package runclient

import (
	"path/filepath"
	"testing"
)

func TestResolvePathsExplicitDatabaseWins(t *testing.T) {
	t.Setenv(EnvDatabasePath, filepath.Join(t.TempDir(), "env.sqlite"))
	explicit := filepath.Join(t.TempDir(), "explicit", "openbrief.sqlite")

	paths, err := ResolvePaths(Config{DatabasePath: explicit})
	if err != nil {
		t.Fatalf("ResolvePaths: %v", err)
	}
	if paths.DatabasePath != explicit {
		t.Fatalf("DatabasePath = %q, want %q", paths.DatabasePath, explicit)
	}
	if paths.DataDir != filepath.Dir(explicit) {
		t.Fatalf("DataDir = %q, want %q", paths.DataDir, filepath.Dir(explicit))
	}
}

func TestResolvePathsUsesDatabaseEnv(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "env", "openbrief.sqlite")
	t.Setenv(EnvDatabasePath, dbPath)

	paths, err := ResolvePaths(Config{})
	if err != nil {
		t.Fatalf("ResolvePaths: %v", err)
	}
	if paths.DatabasePath != dbPath {
		t.Fatalf("DatabasePath = %q, want %q", paths.DatabasePath, dbPath)
	}
}

func TestResolvePathsUsesXDGDefault(t *testing.T) {
	xdg := filepath.Join(t.TempDir(), "xdg")
	t.Setenv(EnvDatabasePath, "")
	t.Setenv("XDG_DATA_HOME", xdg)

	paths, err := ResolvePaths(Config{})
	if err != nil {
		t.Fatalf("ResolvePaths: %v", err)
	}
	want := filepath.Join(xdg, "openbrief", "openbrief.sqlite")
	if paths.DatabasePath != want {
		t.Fatalf("DatabasePath = %q, want %q", paths.DatabasePath, want)
	}
}

func TestResolvePathsIgnoresRetiredDataDirEnv(t *testing.T) {
	xdg := filepath.Join(t.TempDir(), "xdg")
	t.Setenv(EnvDatabasePath, "")
	t.Setenv("OPENBRIEF_DATA_DIR", filepath.Join(t.TempDir(), "retired-data-dir"))
	t.Setenv("XDG_DATA_HOME", xdg)

	paths, err := ResolvePaths(Config{})
	if err != nil {
		t.Fatalf("ResolvePaths: %v", err)
	}
	want := filepath.Join(xdg, "openbrief", "openbrief.sqlite")
	if paths.DatabasePath != want {
		t.Fatalf("DatabasePath = %q, want %q", paths.DatabasePath, want)
	}
	if paths.DataDir != filepath.Dir(want) {
		t.Fatalf("DataDir = %q, want %q", paths.DataDir, filepath.Dir(want))
	}
}
