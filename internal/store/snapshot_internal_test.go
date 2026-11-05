package store

import (
	"database/sql"
	"fmt"
	"os"
	"regexp"
	"strings"
	"testing"
)

// snapshotFile is the committed schema, for reviewers, plans and agents.
const snapshotFile = "schema.snapshot"

// quotedCreate matches the quotes ALTER TABLE ... RENAME leaves on a table's
// name. A rebuild that changes nothing must not change the snapshot.
var quotedCreate = regexp.MustCompile(`^CREATE TABLE "([^"]+)"`)

// trailingSpace matches spaces at the end of a line.
var trailingSpace = regexp.MustCompile(`(?m)[ \t]+$`)

// normalizeDDL strips what doesn't change a schema's meaning from a statement.
func normalizeDDL(ddl string) string {
	ddl = quotedCreate.ReplaceAllString(ddl, "CREATE TABLE $1")
	return trailingSpace.ReplaceAllString(ddl, "")
}

// schemaObject is a row of sqlite_schema.
type schemaObject struct {
	kind, name, ddl string
}

// schemaObjects lists everything SQLite doesn't own, in a stable order.
func schemaObjects(t *testing.T, db *sql.DB) []schemaObject {
	t.Helper()
	rows, err := db.QueryContext(t.Context(), `SELECT type, name, sql FROM sqlite_schema
		WHERE name NOT LIKE 'sqlite\_%' ESCAPE '\'
		ORDER BY tbl_name, type = 'index', name`)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	var objects []schemaObject
	for rows.Next() {
		var o schemaObject
		if err = rows.Scan(&o.kind, &o.name, &o.ddl); err != nil {
			t.Fatal(err)
		}
		objects = append(objects, o)
	}
	if err = rows.Err(); err != nil {
		t.Fatal(err)
	}
	return objects
}

// pragmaRows renders a table-valued pragma about name, one line per row.
func pragmaRows(t *testing.T, db *sql.DB, p objectPragma, name string) string {
	t.Helper()
	rows, err := db.QueryContext(t.Context(), "SELECT * FROM "+p.pragma+"(?)", name) //nolint:gosec,unqueryvet // G202: p.pragma is one of pragmasFor's constants. A pragma's columns are the point: the snapshot records them all.
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	columns, err := rows.Columns()
	if err != nil {
		t.Fatal(err)
	}
	values := make([]any, len(columns))
	targets := make([]any, len(columns))
	for i := range values {
		targets[i] = &values[i]
	}
	var b strings.Builder
	for rows.Next() {
		if err = rows.Scan(targets...); err != nil {
			t.Fatal(err)
		}
		b.WriteString("--   " + p.label)
		for i, c := range columns {
			fmt.Fprintf(&b, " %s=%v", c, values[i])
		}
		b.WriteByte('\n')
	}
	if err = rows.Err(); err != nil {
		t.Fatal(err)
	}
	return b.String()
}

// schemaSnapshot renders db's schema: each object's statement, normalized,
// then what the pragmas say about it.
func schemaSnapshot(t *testing.T, db *sql.DB) string {
	t.Helper()
	var b strings.Builder
	for _, o := range schemaObjects(t, db) {
		fmt.Fprintf(&b, "-- %s %s\n%s;\n", o.kind, o.name, normalizeDDL(o.ddl))
		for _, p := range pragmasFor(o.kind) {
			b.WriteString(pragmaRows(t, db, p, o.name))
		}
		b.WriteByte('\n')
	}
	return b.String()
}

// objectPragma is a table-valued pragma and the label its rows get.
type objectPragma struct{ label, pragma string }

// pragmasFor returns the pragmas that describe an object of kind.
func pragmasFor(kind string) []objectPragma {
	switch kind {
	case "table":
		return []objectPragma{{"column", "pragma_table_xinfo"}, {"foreign key", "pragma_foreign_key_list"}}
	case "index":
		return []objectPragma{{"key", "pragma_index_xinfo"}}
	default:
		return nil
	}
}

// builtSnapshot is the snapshot of a database built from empty.
func builtSnapshot(t *testing.T) string {
	t.Helper()
	return schemaSnapshot(t, openTest(t).w)
}

// committedSnapshot reads the snapshot in the repository.
func committedSnapshot(t *testing.T) string {
	t.Helper()
	want, err := os.ReadFile(snapshotFile)
	if err != nil {
		t.Fatal(err)
	}
	return string(want)
}

// firstDiff describes the first line where two snapshots differ.
func firstDiff(got, want string) string {
	g, w := strings.Split(got, "\n"), strings.Split(want, "\n")
	for i := range min(len(g), len(w)) {
		if g[i] != w[i] {
			return fmt.Sprintf("line %d:\n got: %s\nwant: %s", i+1, g[i], w[i])
		}
	}
	return fmt.Sprintf("lengths differ: got %d lines, want %d", len(g), len(w))
}

// TestSchemaSnapshot checks the committed snapshot. mise run generate sets
// UPDATE_SNAPSHOT to rewrite it.
func TestSchemaSnapshot(t *testing.T) {
	t.Parallel()
	got := builtSnapshot(t)
	if os.Getenv("UPDATE_SNAPSHOT") != "" {
		if err := os.WriteFile(snapshotFile, []byte(got), 0o644); err != nil { //nolint:gosec // G306: a committed file, not a secret.
			t.Fatal(err)
		}
		return
	}
	if want := committedSnapshot(t); got != want {
		t.Errorf("%s is out of date; run mise run generate and review the diff.\n%s", snapshotFile, firstDiff(got, want))
	}
}
