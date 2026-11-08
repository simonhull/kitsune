package store

import (
	"bytes"
	"crypto/sha256"
	"database/sql"
	"encoding/json/v2"
	"fmt"
	"path/filepath"
	"slices"
	"strconv"
	"testing"
	"time"
)

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

const testRoot = "/music"

// artistULID returns a 26-character ID for an artist's match key.
func artistULID(key string) string { return fmt.Sprintf("%026s", key) }

// newWrite returns a distinct valid write: its own album and artist per index.
// It mirrors library_test.go's helper; the two packages can't share one.
func newWrite(i int) TrackWrite {
	return TrackWrite{
		ULID:      fmt.Sprintf("01J%023d", i),
		Root:      testRoot,
		RelPath:   fmt.Sprintf("a/%d.flac", i),
		Size:      1000 + int64(i),
		MTime:     time.UnixMilli(1_761_000_000_000 + int64(i)).UTC().Add(456789 * time.Nanosecond),
		Digest:    new(sha256.Sum256([]byte(strconv.Itoa(i)))),
		Format:    "flac",
		Codec:     "flac",
		Duration:  200*time.Second + 123*time.Millisecond,
		Title:     fmt.Sprintf("Title %d", i),
		SortTitle: fmt.Sprintf("Title %d", i),
		Album: AlbumWrite{
			GroupKey: fmt.Sprintf("album-%d", i),
			ULID:     fmt.Sprintf("02J%023d", i),
			Artist:   fmt.Sprintf("Artist %d", i),
			Name:     fmt.Sprintf("Album %d", i),
			SortName: fmt.Sprintf("Album %d", i),
		},
		Artists: []ArtistWrite{{
			ULID:     artistULID(fmt.Sprintf("artist-%d", i)),
			MatchKey: fmt.Sprintf("artist-%d", i),
			Name:     fmt.Sprintf("Artist %d", i),
			SortName: fmt.Sprintf("Artist %d", i),
		}},
	}
}

func writeAll(t *testing.T, s *Store, tws ...TrackWrite) {
	t.Helper()
	if err := s.WriteBatch(t.Context(), &Batch{Tracks: tws}); err != nil {
		t.Fatal(err)
	}
}

// count runs a single-integer query on the read pool.
func count(t *testing.T, s *Store, query string, args ...any) int64 {
	t.Helper()
	var n int64
	if err := s.r.QueryRowContext(t.Context(), query, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func ftsHits(t *testing.T, s *Store, match string) int64 {
	t.Helper()
	return count(t, s, "SELECT count(*) FROM tracks_fts WHERE tracks_fts MATCH ?", match)
}

func trackID(t *testing.T, s *Store, relPath string) int64 {
	t.Helper()
	return count(t, s, "SELECT id FROM tracks WHERE rel_path = ?", relPath)
}

// strs runs a single-text-column query on the read pool.
func strs(t *testing.T, s *Store, query string) []string {
	t.Helper()
	rows, err := s.r.QueryContext(t.Context(), query)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	var out []string
	for rows.Next() {
		var v string
		if err = rows.Scan(&v); err != nil {
			t.Fatal(err)
		}
		out = append(out, v)
	}
	if err = rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestWriteBatchInsertsTrack(t *testing.T) {
	t.Parallel()
	t0 := time.UnixMilli(1_762_000_000_000).UTC()
	s, err := Open(t.Context(), filepath.Join(t.TempDir(), "kitsune.db"), Options{Now: func() time.Time { return t0 }})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })

	w := newWrite(0)
	w.Artists = []ArtistWrite{{ULID: artistULID("a"), MatchKey: "a", Name: "A"}, {ULID: artistULID("b"), MatchKey: "b", Name: "B"}}
	w.AlbumArtists = []ArtistWrite{{ULID: artistULID("c"), MatchKey: "c", Name: "C"}}
	w.Tags = []Tag{{"ARTIST", "A"}, {"ARTIST", "B"}, {"TITLE", "T"}}
	writeAll(t, s, w)

	checkTrackRow(t, s, &w, t0)
	tags := strs(t, s, `SELECT key || '/' || ord || '/' || value FROM track_tags ORDER BY key, ord`)
	if want := []string{"ARTIST/0/A", "ARTIST/1/B", "TITLE/0/T"}; !slices.Equal(tags, want) {
		t.Errorf("tags = %v, want %v", tags, want)
	}
	links := strs(t, s, `SELECT ta.role || '/' || ta.ord || '/' || a.match_key
		FROM track_artists ta JOIN artists a ON a.id = ta.artist_id ORDER BY ta.role, ta.ord`)
	if want := []string{"album_artist/0/c", "artist/0/a", "artist/1/b"}; !slices.Equal(links, want) {
		t.Errorf("artist links = %v, want %v", links, want)
	}
	got, err := s.FileStates(t.Context(), testRoot)
	if err != nil {
		t.Fatal(err)
	}
	st := got[w.RelPath]
	if st.TrackID == 0 || !st.MTime.Equal(w.MTime.Truncate(time.Millisecond)) || st.Size != w.Size {
		t.Errorf("FileStates[%s] = %+v", w.RelPath, st)
	}
}

// checkTrackRow checks the tracks row's columns against w, written at added.
func checkTrackRow(t *testing.T, s *Store, w *TrackWrite, added time.Time) {
	t.Helper()
	var (
		ulid, title                 string
		size, mtime, durMS, addedAt int64
		digest                      []byte
	)
	err := s.r.QueryRowContext(t.Context(),
		"SELECT ulid, title, size, mtime, duration_ms, added_at, audio_digest FROM tracks").
		Scan(&ulid, &title, &size, &mtime, &durMS, &addedAt, &digest)
	if err != nil {
		t.Fatal(err)
	}
	if ulid != w.ULID || title != w.Title || size != w.Size || durMS != 200_123 || !bytes.Equal(digest, w.Digest[:]) {
		t.Errorf("columns = %q %q %d %d %x", ulid, title, size, durMS, digest)
	}
	if mtime != 1_761_000_000_000 || addedAt != added.UnixMilli() {
		t.Errorf("mtime, added_at = %d, %d", mtime, addedAt)
	}
	// Zero values are stored as NULL; a missing mark starts out unset.
	if n := count(t, s, `SELECT count(*) FROM tracks WHERE track_no IS NULL AND rg_track_gain IS NULL
		AND missing_since IS NULL AND sample_rate IS NULL AND lossless = 0 AND compilation = 0`); n != 1 {
		t.Error("track_no, rg_track_gain, missing_since or sample_rate is not NULL, or a flag is set")
	}
}

func TestWriteBatchUpdateKeepsIdentity(t *testing.T) {
	t.Parallel()
	now := time.UnixMilli(1_762_000_000_000).UTC()
	s, err := Open(t.Context(), filepath.Join(t.TempDir(), "kitsune.db"), Options{Now: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })

	w := newWrite(0)
	w.Title = "Oldtitle"
	w.Tags = []Tag{{"OLD", "x"}}
	writeAll(t, s, w)
	id := trackID(t, s, w.RelPath)

	now = now.Add(time.Hour)
	u := newWrite(0)
	u.ID = id
	u.ULID = "ignored"
	u.Title = "Newtitle"
	u.Tags = []Tag{{"NEW", "y"}}
	u.Album.GroupKey = "other-album"
	u.Album.ULID = "02Jother-album-00000000000"
	writeAll(t, s, u)

	var ulid string
	var added int64
	if err = s.r.QueryRowContext(t.Context(), "SELECT ulid, added_at FROM tracks WHERE id = ?", id).Scan(&ulid, &added); err != nil {
		t.Fatal(err)
	}
	if ulid != w.ULID || added != 1_762_000_000_000 {
		t.Errorf("ulid, added_at = %q, %d; want the original %q, 1762000000000", ulid, added, w.ULID)
	}
	if n := count(t, s, "SELECT count(*) FROM track_tags WHERE key = 'OLD'"); n != 0 {
		t.Errorf("%d old tags remain", n)
	}
	if n := count(t, s, "SELECT count(*) FROM track_tags WHERE key = 'NEW'"); n != 1 {
		t.Errorf("%d new tags, want 1", n)
	}
	if ftsHits(t, s, "newtitle") != 1 || ftsHits(t, s, "oldtitle") != 0 {
		t.Error("FTS does not reflect the new title only")
	}
	if n := count(t, s, "SELECT count(*) FROM albums WHERE group_key = 'album-0'"); n != 1 {
		t.Errorf("old album rows = %d, want 1 until PruneOrphans", n)
	}
}

func TestWriteBatchArtistUpsert(t *testing.T) {
	t.Parallel()
	s := openTest(t)
	first, second := newWrite(0), newWrite(1)
	first.Artists = []ArtistWrite{{ULID: artistULID("k"), MatchKey: "k", Name: "Old", SortName: "Old", MBID: "m"}}
	second.Artists = []ArtistWrite{{ULID: artistULID("other"), MatchKey: "k", Name: "New", SortName: "New, The"}}
	writeAll(t, s, first, second)

	var n int64
	var id, name, sortName string
	var mbid sql.NullString
	if err := s.r.QueryRowContext(t.Context(), "SELECT count(*), ulid, name, sort_name, mbid FROM artists WHERE match_key = 'k'").
		Scan(&n, &id, &name, &sortName, &mbid); err != nil {
		t.Fatal(err)
	}
	if n != 1 || name != "New" || sortName != "New, The" || mbid.String != "m" {
		t.Errorf("artist = %d rows, %q, %q, %v; want 1, New, \"New, The\", m", n, name, sortName, mbid)
	}
	if id != artistULID("k") {
		t.Errorf("ulid = %q, want the first write's %q", id, artistULID("k"))
	}
	if total := count(t, s, "SELECT count(*) FROM artists"); total != 1 {
		t.Errorf("artists = %d, want 1", total)
	}
}

func TestWriteBatchKeepsASortNameNoTrackRepeats(t *testing.T) {
	t.Parallel()
	s := openTest(t)
	tagged, untagged, fresh := newWrite(0), newWrite(1), newWrite(2)
	tagged.Artists = []ArtistWrite{{ULID: artistULID("k"), MatchKey: "k", Name: "The Band", SortName: "Band, The"}}
	untagged.Artists = []ArtistWrite{{ULID: artistULID("k"), MatchKey: "k", Name: "The Band"}}
	fresh.Artists = []ArtistWrite{{ULID: artistULID("n"), MatchKey: "n", Name: "Newcomer"}}
	writeAll(t, s, tagged, untagged, fresh)
	if got := strs(t, s, "SELECT sort_name FROM artists ORDER BY match_key"); !slices.Equal(got, []string{"Band, The", "Newcomer"}) {
		t.Errorf("sort names = %q, want the tagged one kept and a new artist's name", got)
	}
}

func TestWriteBatchWarningsJSON(t *testing.T) {
	t.Parallel()
	s := openTest(t)
	nilW, withW := newWrite(0), newWrite(1)
	withW.Warnings = []string{`a "b"`}
	writeAll(t, s, nilW, withW)

	var raw string
	if err := s.r.QueryRowContext(t.Context(), "SELECT scan_warnings FROM tracks WHERE rel_path = ?", nilW.RelPath).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	if raw != "[]" {
		t.Errorf("nil warnings stored as %q, want []", raw)
	}
	if err := s.r.QueryRowContext(t.Context(), "SELECT scan_warnings FROM tracks WHERE rel_path = ?", withW.RelPath).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	var got []string
	if err := json.Unmarshal([]byte(raw), &got); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(got, withW.Warnings) {
		t.Errorf("warnings = %q, want %q", got, withW.Warnings)
	}
}

func TestWriteBatchFTSRow(t *testing.T) {
	t.Parallel()
	s := openTest(t)
	w := newWrite(0)
	w.Artists = []ArtistWrite{{ULID: artistULID("zw"), MatchKey: "zw", Name: "Zayde Wølf"}}
	w.AlbumArtists = []ArtistWrite{{ULID: artistULID("bk"), MatchKey: "bk", Name: "Beyoncé"}}
	writeAll(t, s, w)
	for _, q := range []string{"wolf", "beyonce", "title", "album"} {
		if ftsHits(t, s, q) != 1 {
			t.Errorf("MATCH %q found no row after insert", q)
		}
	}

	u := newWrite(0)
	u.ID = trackID(t, s, w.RelPath)
	u.Artists = []ArtistWrite{{ULID: artistULID("other"), MatchKey: "other", Name: "Someone Else"}}
	writeAll(t, s, u)
	if ftsHits(t, s, "wolf") != 0 {
		t.Error("MATCH wolf still finds the track after the artist was renamed")
	}
	if ftsHits(t, s, "someone") != 1 {
		t.Error("MATCH someone finds nothing after the update")
	}
}

func TestMarkMissing(t *testing.T) {
	t.Parallel()
	s := openTest(t)
	w := newWrite(0)
	writeAll(t, s, w)
	id := trackID(t, s, w.RelPath)
	t1 := time.UnixMilli(1_762_000_000_000).UTC()
	for _, at := range []time.Time{t1, t1.Add(time.Hour)} {
		if err := s.MarkMissing(t.Context(), []int64{id}, at); err != nil {
			t.Fatal(err)
		}
	}
	if got := count(t, s, "SELECT missing_since FROM tracks WHERE id = ?", id); got != t1.UnixMilli() {
		t.Errorf("missing_since = %d, want the first mark %d", got, t1.UnixMilli())
	}
	if err := s.MarkMissing(t.Context(), nil, t1); err != nil {
		t.Errorf("empty MarkMissing: %v", err)
	}
	st, err := s.FileStates(t.Context(), testRoot)
	if err != nil || !st[w.RelPath].Missing {
		t.Errorf("FileStates = %+v, %v; want Missing", st, err)
	}
}

func TestDeleteMissingBefore(t *testing.T) {
	t.Parallel()
	s := openTest(t)
	old, recent := newWrite(0), newWrite(1)
	old.Tags = []Tag{{"K", "v"}}
	old.Title = "Oldsong"
	recent.Title = "Recentsong"
	writeAll(t, s, old, recent)
	t0 := time.UnixMilli(1_762_000_000_000).UTC()
	if err := s.MarkMissing(t.Context(), []int64{trackID(t, s, old.RelPath)}, t0); err != nil {
		t.Fatal(err)
	}
	if err := s.MarkMissing(t.Context(), []int64{trackID(t, s, recent.RelPath)}, t0.Add(2*time.Hour)); err != nil {
		t.Fatal(err)
	}

	n, err := s.DeleteMissingBefore(t.Context(), t0.Add(time.Hour))
	if err != nil || n != 1 {
		t.Fatalf("DeleteMissingBefore = %d, %v; want 1, nil", n, err)
	}
	if got := count(t, s, "SELECT count(*) FROM tracks"); got != 1 {
		t.Errorf("tracks = %d, want 1", got)
	}
	for _, table := range []string{"track_tags", "track_artists"} {
		if got := count(t, s, "SELECT count(*) FROM "+table+" WHERE track_id = 1"); got != 0 {
			t.Errorf("%s rows for the deleted track = %d", table, got)
		}
	}
	if ftsHits(t, s, "oldsong") != 0 || ftsHits(t, s, "recentsong") != 1 {
		t.Error("FTS rows do not match the surviving track")
	}
}

const (
	benchTracks   = 500
	benchAlbums   = 50
	benchArtists  = 150
	benchTagsEach = 25
	benchTagKeys  = 12
)

func openBench(b *testing.B) *Store {
	b.Helper()
	s, err := Open(b.Context(), filepath.Join(b.TempDir(), "kitsune.db"), Options{})
	if err != nil {
		b.Fatal(err)
	}
	return s
}

// benchBatch builds benchTracks synthetic tracks: benchAlbums albums, two
// artists and one album artist each, and benchTagsEach tags each.
func benchBatch() *Batch {
	artist := func(i int) ArtistWrite {
		n := i % benchArtists
		return ArtistWrite{ULID: artistULID(fmt.Sprintf("artist-%d", n)), MatchKey: fmt.Sprintf("artist-%d", n), Name: fmt.Sprintf("Artist %d", n), SortName: fmt.Sprintf("Artist %d", n)}
	}
	b := &Batch{Tracks: make([]TrackWrite, benchTracks)}
	for i := range b.Tracks {
		w := newWrite(i)
		album := i % benchAlbums
		w.Album = AlbumWrite{
			GroupKey: fmt.Sprintf("album-%d", album), ULID: fmt.Sprintf("02J%023d", album), Artist: fmt.Sprintf("Artist %d", album), Name: fmt.Sprintf("Album %d", album),
			SortName: fmt.Sprintf("Album %d", album), Year: 2000 + album,
		}
		w.Artists = []ArtistWrite{artist(i), artist(i + 1)}
		w.AlbumArtists = []ArtistWrite{artist(album)}
		w.Tags = make([]Tag, benchTagsEach)
		for k := range w.Tags {
			w.Tags[k] = Tag{Key: fmt.Sprintf("TAG%d", k%benchTagKeys), Value: fmt.Sprintf("value %d of track %d", k, i)}
		}
		b.Tracks[i] = w
	}
	return b
}

func BenchmarkWriteBatch(b *testing.B) {
	batch := benchBatch()
	b.Run("insert", func(b *testing.B) {
		for b.Loop() {
			b.StopTimer()
			s := openBench(b)
			b.StartTimer()
			if err := s.WriteBatch(b.Context(), batch); err != nil {
				b.Fatal(err)
			}
			b.StopTimer()
			if err := s.Close(); err != nil {
				b.Fatal(err)
			}
			b.StartTimer()
		}
	})
	b.Run("update", func(b *testing.B) {
		s := openBench(b)
		b.Cleanup(func() { _ = s.Close() })
		if err := s.WriteBatch(b.Context(), batch); err != nil {
			b.Fatal(err)
		}
		// A fresh database numbers its tracks 1..benchTracks in insert order.
		upd := &Batch{Tracks: slices.Clone(batch.Tracks)}
		for i := range upd.Tracks {
			upd.Tracks[i].ID = int64(i + 1)
		}
		for b.Loop() {
			if err := s.WriteBatch(b.Context(), upd); err != nil {
				b.Fatal(err)
			}
		}
	})
}

func TestAlbumIdentityKeptOnUpsert(t *testing.T) {
	t.Parallel()
	now := time.UnixMilli(1_762_000_000_000).UTC()
	s, err := Open(t.Context(), filepath.Join(t.TempDir(), "kitsune.db"), Options{Now: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })

	first := newWrite(0)
	writeAll(t, s, first)
	now = now.Add(time.Hour)
	second := newWrite(1)
	second.Album.GroupKey = first.Album.GroupKey // The same album, seen again with a new candidate ULID.
	second.Album.Artist = "Renamed"
	writeAll(t, s, second)

	var ulid, artist string
	var added int64
	q := "SELECT ulid, artist, added_at FROM albums WHERE group_key = ?"
	if err = s.r.QueryRowContext(t.Context(), q, first.Album.GroupKey).Scan(&ulid, &artist, &added); err != nil {
		t.Fatal(err)
	}
	if ulid != first.Album.ULID || added != 1_762_000_000_000 {
		t.Errorf("ulid, added_at = %q, %d; want the original %q, 1762000000000", ulid, added, first.Album.ULID)
	}
	if artist != "Renamed" {
		t.Errorf("artist = %q, want the update to apply", artist)
	}
}

func TestAlbumSchemaForBrowse(t *testing.T) {
	t.Parallel()
	s := openTest(t)
	for _, name := range []string{indexAlbumsSort, indexAlbumsAdded, "tracks_live", "tracks_album"} {
		q := "SELECT count(*) FROM sqlite_schema WHERE type = 'index' AND name = '" + name + "'"
		if got := rawInt(t, s.r, q); got != 1 {
			t.Errorf("index %s: count = %d, want 1", name, got)
		}
	}
	// The collation must sit on the column, or keyset seeks fall back to scans.
	var coll string
	q := "SELECT coll FROM pragma_index_xinfo('" + indexAlbumsSort + "') WHERE name = 'sort_name'"
	if err := s.r.QueryRowContext(t.Context(), q).Scan(&coll); err != nil {
		t.Fatal(err)
	}
	if coll != "NOCASE" {
		t.Errorf("albums_sort collation on sort_name = %q, want NOCASE", coll)
	}
}
