package store

import "testing"

func TestLibrarySchema(t *testing.T) {
	t.Parallel()
	s := openTest(t)
	for _, name := range []string{"artists", "albums", "tracks", "track_tags", "track_artists", "scan_issues", "tracks_fts"} {
		q := "SELECT count(*) FROM sqlite_schema WHERE name = '" + name + "'"
		if got := rawInt(t, s.r, q); got != 1 {
			t.Errorf("table %s: count = %d, want 1", name, got)
		}
	}
	var table, onDelete string
	err := s.r.QueryRowContext(t.Context(),
		`SELECT "table", on_delete FROM pragma_foreign_key_list('track_tags')`).Scan(&table, &onDelete)
	if err != nil {
		t.Fatal(err)
	}
	if table != "tracks" || onDelete != "CASCADE" {
		t.Errorf("track_tags FK = %s %s, want tracks CASCADE", table, onDelete)
	}
}

func TestFTSRemovesDiacritics(t *testing.T) {
	t.Parallel()
	s := openTest(t)
	// Deliberate setup: the FTS table has no public write path until WriteBatch exists.
	_, err := s.w.ExecContext(t.Context(),
		"INSERT INTO tracks_fts (rowid, title, artists, album, album_artist) VALUES (1, 'x', ?, ?, '')",
		foldSearch("Zayde Wølf"), foldSearch("Beyoncé"))
	if err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{"wolf", "beyonce"} {
		var n int64
		if err = s.r.QueryRowContext(t.Context(), "SELECT count(*) FROM tracks_fts WHERE tracks_fts MATCH ?", q).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n != 1 {
			t.Errorf("MATCH %q found %d rows, want 1", q, n)
		}
	}
}
