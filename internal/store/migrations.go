package store

import (
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"regexp"
	"strconv"
)

// migrationsDir is where the numbered files live, in [migrationFiles].
const migrationsDir = "migrations"

// manifestName is the file listing every migration with its sha256. Nothing
// reads it at run time: a test checks it, and it makes two branches that add a
// migration conflict in git.
const manifestName = "manifest.txt"

//go:embed migrations
var migrationFiles embed.FS

// migrationName is the only shape a migration file may have: NNNN_name.sql.
var migrationName = regexp.MustCompile(`^(\d{4})_[a-z0-9_]+\.sql$`)

var (
	// errMigrationName means a file in migrations/ isn't a migration.
	errMigrationName = errors.New("want NNNN_name.sql")
	// errMigrationGap means the numbers aren't contiguous from 1.
	errMigrationGap = errors.New("numbers must run from 1 without gaps or repeats")
)

// migration is one numbered step of the schema.
type migration struct {
	version int64  // The user_version the step leaves behind.
	name    string // The file name, which errors report.
	sql     string
}

// parseMigrations reads the migrations from files, in order. It accepts
// nothing but the manifest and contiguous NNNN_name.sql files.
func parseMigrations(files fs.FS) ([]migration, error) {
	entries, err := fs.ReadDir(files, migrationsDir)
	if err != nil {
		return nil, fmt.Errorf("read migrations: %w", err)
	}
	steps := make([]migration, 0, len(entries))
	for _, e := range entries {
		if e.Name() == manifestName {
			continue
		}
		step, parseErr := parseMigration(files, e.Name())
		if parseErr != nil {
			return nil, parseErr
		}
		if step.version != int64(len(steps)+1) {
			return nil, fmt.Errorf("migration %s: %w", step.name, errMigrationGap)
		}
		steps = append(steps, step)
	}
	return steps, nil
}

// parseMigration reads the migration called name.
func parseMigration(files fs.FS, name string) (migration, error) {
	match := migrationName.FindStringSubmatch(name)
	if match == nil {
		return migration{}, fmt.Errorf("migration %s: %w", name, errMigrationName)
	}
	version, err := strconv.ParseInt(match[1], 10, 64)
	if err != nil {
		return migration{}, fmt.Errorf("migration %s: %w", name, err)
	}
	body, err := fs.ReadFile(files, path.Join(migrationsDir, name))
	if err != nil {
		return migration{}, fmt.Errorf("migration %s: %w", name, err)
	}
	return migration{version: version, name: name, sql: string(body)}, nil
}
