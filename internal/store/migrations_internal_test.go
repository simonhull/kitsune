package store

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"strings"
	"testing"
	"testing/fstest"
)

// wantManifest is the manifest files should have: one line per migration.
func wantManifest(t *testing.T, files fs.FS) string {
	t.Helper()
	steps, err := parseMigrations(files)
	if err != nil {
		t.Fatal(err)
	}
	var b strings.Builder
	for i := range steps {
		s := &steps[i]
		label := strings.TrimSuffix(s.name[len("0000_"):], ".sql")
		fmt.Fprintf(&b, "%04d %s %x\n", s.version, label, sha256.Sum256([]byte(s.sql)))
	}
	return b.String()
}

// manifestMismatch returns the difference between files and their manifest.
func manifestMismatch(t *testing.T, files fs.FS) string {
	t.Helper()
	got, err := fs.ReadFile(files, path.Join(migrationsDir, manifestName))
	if err != nil {
		t.Fatal(err)
	}
	if want := wantManifest(t, files); string(got) != want {
		return fmt.Sprintf("manifest.txt:\n%swant:\n%s", got, want)
	}
	return ""
}

func TestManifestMatchesMigrations(t *testing.T) {
	t.Parallel()
	if diff := manifestMismatch(t, migrationFiles); diff != "" {
		t.Errorf("shipped migrations never change; a new one adds a line.\n%s", diff)
	}
}

// migrationsFS copies the shipped migrations into a filesystem a test can edit.
func migrationsFS(t *testing.T) fstest.MapFS {
	t.Helper()
	entries, err := fs.ReadDir(migrationFiles, migrationsDir)
	if err != nil {
		t.Fatal(err)
	}
	files := fstest.MapFS{}
	for _, e := range entries {
		p := path.Join(migrationsDir, e.Name())
		body, readErr := fs.ReadFile(migrationFiles, p)
		if readErr != nil {
			t.Fatal(readErr)
		}
		files[p] = &fstest.MapFile{Data: body}
	}
	return files
}

func TestEditedMigrationFailsTheManifest(t *testing.T) {
	t.Parallel()
	files := migrationsFS(t)
	f := files[path.Join(migrationsDir, "0001_baseline.sql")]
	f.Data = append(f.Data, []byte("\nCREATE TABLE sneaked_in (x INTEGER) STRICT;\n")...)
	if manifestMismatch(t, files) == "" {
		t.Error("an edited 0001 passed the manifest")
	}
}

func TestParseMigrationsIsStrict(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		files string // Space-separated.
		want  error
	}{
		{"gap", "0001_a.sql 0003_c.sql", errMigrationGap},
		{"repeat from two branches", "0001_a.sql 0002_b.sql 0002_c.sql", errMigrationGap},
		{"not from one", "0002_b.sql", errMigrationGap},
		{"short number", "1_a.sql", errMigrationName},
		{"long number", "00001_a.sql", errMigrationName},
		{"no name", "0001.sql", errMigrationName},
		{"uppercase", "0001_A.sql", errMigrationName},
		{"not sql", "0001_a.sql.orig", errMigrationName},
		{"stray file", "0001_a.sql notes.md", errMigrationName},
		{"manifest only is fine", "", nil},
		{"contiguous", "0001_a.sql 0002_b.sql", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			files := fstest.MapFS{path.Join(migrationsDir, manifestName): {}}
			for f := range strings.FieldsSeq(tt.files) {
				files[path.Join(migrationsDir, f)] = &fstest.MapFile{Data: []byte("SELECT 1;")}
			}
			if _, err := parseMigrations(files); !errors.Is(err, tt.want) {
				t.Errorf("err = %v, want %v", err, tt.want)
			}
		})
	}
}
