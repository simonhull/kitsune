package store

import (
	"crypto/sha256"
	"fmt"
	"slices"
	"testing"
	"time"
)

func testArtwork(seed string) *ArtworkWrite {
	return &ArtworkWrite{
		Hash: sha256.Sum256([]byte(seed)), Width: 600, Height: 500,
		Accent: "#c83838", Ground: "#741f1f", Source: ArtworkEmbedded,
	}
}

func albumHash(t *testing.T, s *Store, groupKey string) []byte {
	t.Helper()
	var h []byte
	err := s.r.QueryRowContext(t.Context(), "SELECT artwork_hash FROM albums WHERE group_key = ?", groupKey).Scan(&h)
	if err != nil {
		t.Fatal(err)
	}
	return h
}

func artworkTrack(i int, art *ArtworkWrite) TrackWrite {
	tw := newWrite(i)
	tw.Album.Artwork = art
	return tw
}

func TestArtworkSchema(t *testing.T) {
	t.Parallel()
	s := openTest(t)
	if got := rawInt(t, s.r, "SELECT count(*) FROM sqlite_schema WHERE name IN ('artwork', 'albums_artwork')"); got != 2 {
		t.Errorf("artwork table and albums_artwork index: count = %d, want 2", got)
	}
	_, err := s.w.ExecContext(t.Context(),
		"INSERT INTO artwork (hash, width, height, accent, ground, source, alpha) VALUES (zeroblob(32), 1, 1, '#000000', '#000000', 'web', 0)")
	if err == nil {
		t.Error("an unknown artwork source was accepted")
	}
	_, err = s.w.ExecContext(t.Context(),
		"INSERT INTO artwork (hash, width, height, accent, ground, source, alpha) VALUES (zeroblob(5), 1, 1, '#000000', '#000000', 'folder', 0)")
	if err == nil {
		t.Error("a short artwork hash was accepted")
	}
}

func TestWriteBatchStoresArtwork(t *testing.T) {
	t.Parallel()
	s := openTest(t)
	art := testArtwork("a")
	writeAll(t, s, artworkTrack(0, art))

	var w, h int64
	var accent, ground, source string
	err := s.r.QueryRowContext(t.Context(), "SELECT width, height, accent, ground, source FROM artwork WHERE hash = ?", art.Hash[:]).
		Scan(&w, &h, &accent, &ground, &source)
	if err != nil {
		t.Fatal(err)
	}
	if w != 600 || h != 500 || accent != art.Accent || ground != art.Ground || source != "embedded" {
		t.Errorf("artwork row = %d %d %s %s %s", w, h, accent, ground, source)
	}
	if got := albumHash(t, s, newWrite(0).Album.GroupKey); !slices.Equal(got, art.Hash[:]) {
		t.Errorf("album artwork_hash = %x, want %x", got, art.Hash)
	}
}

func TestArtworkUpsertLastWriteWins(t *testing.T) {
	t.Parallel()
	s := openTest(t)
	art := testArtwork("a")
	writeAll(t, s, artworkTrack(0, art))
	again := *art
	again.Accent, again.Ground, again.Source, again.Width = "#112233", "#010203", ArtworkFolder, 700
	writeAll(t, s, artworkTrack(1, &again))

	if n := count(t, s, "SELECT count(*) FROM artwork"); n != 1 {
		t.Fatalf("artwork rows = %d, want 1", n)
	}
	var accent, source string
	var w int64
	if err := s.r.QueryRowContext(t.Context(), "SELECT accent, source, width FROM artwork").Scan(&accent, &source, &w); err != nil {
		t.Fatal(err)
	}
	if accent != "#112233" || source != "folder" || w != 700 {
		t.Errorf("row = %s %s %d, want the second write's", accent, source, w)
	}
}

func TestNilArtworkKeepsTheCover(t *testing.T) {
	t.Parallel()
	s := openTest(t)
	art := testArtwork("a")
	writeAll(t, s, artworkTrack(0, art))
	// Another track of the same album, without art.
	other := newWrite(1)
	other.Album = newWrite(0).Album
	other.Album.Artwork = nil
	writeAll(t, s, other)
	if got := albumHash(t, s, newWrite(0).Album.GroupKey); !slices.Equal(got, art.Hash[:]) {
		t.Errorf("album artwork_hash = %x after a write without art, want %x kept", got, art.Hash)
	}

	// A different cover replaces it.
	next := testArtwork("b")
	repl := newWrite(2)
	repl.Album = newWrite(0).Album
	repl.Album.Artwork = next
	writeAll(t, s, repl)
	if got := albumHash(t, s, newWrite(0).Album.GroupKey); !slices.Equal(got, next.Hash[:]) {
		t.Errorf("album artwork_hash = %x, want the new cover %x", got, next.Hash)
	}
}

func TestAlbumWithoutArtworkIsNull(t *testing.T) {
	t.Parallel()
	s := openTest(t)
	writeAll(t, s, newWrite(0))
	if got := albumHash(t, s, newWrite(0).Album.GroupKey); got != nil {
		t.Errorf("artwork_hash = %x, want NULL", got)
	}
}

func TestAlbumsShareOneImage(t *testing.T) {
	t.Parallel()
	s := openTest(t)
	art := testArtwork("shared")
	writeAll(t, s, artworkTrack(0, art), artworkTrack(1, art))
	if n := count(t, s, "SELECT count(*) FROM artwork"); n != 1 {
		t.Errorf("artwork rows = %d, want 1", n)
	}
	if n := count(t, s, "SELECT count(*) FROM albums WHERE artwork_hash = ?", art.Hash[:]); n != 2 {
		t.Errorf("albums using the image = %d, want 2", n)
	}
}

func deleteTrack(t *testing.T, s *Store, i int) {
	t.Helper()
	if _, err := s.w.ExecContext(t.Context(), "DELETE FROM tracks WHERE rel_path = ?", newWrite(i).RelPath); err != nil {
		t.Fatal(err)
	}
}

func prune(t *testing.T, s *Store) Pruned {
	t.Helper()
	got, err := s.PruneOrphans(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func TestPruneReturnsUnusedArtwork(t *testing.T) {
	t.Parallel()
	s := openTest(t)
	shared, own := testArtwork("shared"), testArtwork("own")
	// Albums 0 and 1 share an image; album 2 has its own; album 3 has none.
	writeAll(t, s, artworkTrack(0, shared), artworkTrack(1, shared), artworkTrack(2, own), newWrite(3))
	if got := prune(t, s); got.Albums != 0 || len(got.Artwork) != 0 {
		t.Fatalf("PruneOrphans with every album in use = %+v", got)
	}

	// Remove tracks 1 and 2: the shared image is still used by album 0.
	deleteTrack(t, s, 1)
	deleteTrack(t, s, 2)
	got := prune(t, s)
	if got.Albums != 2 || len(got.Artwork) != 1 || got.Artwork[0] != own.Hash {
		t.Errorf("PruneOrphans = %+v, want 2 albums and only the unshared image", got)
	}
	if n := count(t, s, "SELECT count(*) FROM artwork"); n != 1 {
		t.Errorf("artwork rows left = %d, want 1", n)
	}
	// Removing the last user frees the shared image too.
	deleteTrack(t, s, 0)
	if got = prune(t, s); len(got.Artwork) != 1 || got.Artwork[0] != shared.Hash {
		t.Errorf("PruneOrphans = %+v, want the shared image", got)
	}
}

func TestInvalidArtworkWritesNothing(t *testing.T) {
	t.Parallel()
	s := openTest(t)
	bad := testArtwork("x")
	bad.Source = "web"
	err := s.WriteBatch(t.Context(), &Batch{Tracks: []TrackWrite{artworkTrack(0, bad)}})
	if err == nil {
		t.Fatal("WriteBatch accepted an unknown artwork source")
	}
	if n := count(t, s, "SELECT count(*) FROM tracks"); n != 0 {
		t.Errorf("tracks = %d, want 0", n)
	}
}

// useArtwork stores art as an image no album uses, and has the artist of
// newWrite(artist) use it as its thumb and the album of newWrite(album) as its
// back cover, as a scan reading their folders would.
func useArtwork(t *testing.T, s *Store, art *ArtworkWrite, artist, album int) {
	t.Helper()
	for _, q := range []struct {
		query string
		args  []any
	}{
		{"INSERT INTO artwork (hash, width, height, accent, ground, source, alpha) VALUES (?, 1, 1, '#000000', '#000000', 'folder', 0)", []any{art.Hash[:]}},
		{`INSERT INTO artist_images (artist_id, kind, artwork_hash, source, file_root, file_path, file_size, file_mtime, found_at)
			SELECT id, 'thumb', ?, 'folder', '/m', 'artist.jpg', 1, 1, 1 FROM artists WHERE match_key = ?`, []any{art.Hash[:], fmt.Sprintf("artist-%d", artist)}},
		{`INSERT INTO album_images (album_id, kind, disc, artwork_hash, source, file_root, file_path, file_size, file_mtime, found_at)
			SELECT id, 'back', 0, ?, 'folder', '/m', 'back.jpg', 1, 1, 1 FROM albums WHERE group_key = ?`, []any{art.Hash[:], fmt.Sprintf("album-%d", album)}},
	} {
		if _, err := s.w.ExecContext(t.Context(), q.query, q.args...); err != nil {
			t.Fatalf("%s: %v", q.query, err)
		}
	}
}

// An image that only an artist or an album extra uses stays through a prune,
// and goes with the last of them.
func TestPruneKeepsArtistAndAlbumImages(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name          string
		artist, album int // Whose thumb and whose back cover the image is.
		gone          []int
		wantKept      bool
	}{
		{name: "an artist's and an album's", artist: 0, album: 0, wantKept: true},
		{name: "after another album went", artist: 0, album: 0, gone: []int{1}, wantKept: true},
		{name: "the artist's after its album went", artist: 1, album: 0, gone: []int{0}, wantKept: true},
		{name: "the album's after its artist went", artist: 0, album: 1, gone: []int{0}, wantKept: true},
		{name: "after both went", artist: 0, album: 0, gone: []int{0}, wantKept: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			s := openTest(t)
			writeAll(t, s, newWrite(0), newWrite(1))
			art := testArtwork("an artist's own")
			useArtwork(t, s, art, tt.artist, tt.album)
			for _, i := range tt.gone {
				deleteTrack(t, s, i)
			}
			got := prune(t, s)
			kept := count(t, s, "SELECT count(*) FROM artwork WHERE hash = ?", art.Hash[:]) == 1
			if kept != tt.wantKept {
				t.Fatalf("image kept = %v, want %v (pruned %+v)", kept, tt.wantKept, got)
			}
			if returned := slices.Contains(got.Artwork, art.Hash); returned == kept {
				t.Errorf("PruneOrphans returned the image = %v with it kept = %v", returned, kept)
			}
		})
	}
}

// A prune or a delete of missing tracks that deletes nothing, as most scans'
// do, leaves the rows kept for the index and the scan's layout standing; one
// that deletes counts as a write.
func TestPruneOfNothingKeepsTheRows(t *testing.T) {
	t.Parallel()
	s := openTest(t)
	writeAll(t, s, newWrite(0), newWrite(1))
	before := s.library.gen.Load()
	if got := prune(t, s); got.Albums+got.Artists != 0 || len(got.Artwork) != 0 {
		t.Fatalf("PruneOrphans = %+v, want nothing to prune", got)
	}
	if n, err := s.DeleteMissingBefore(t.Context(), time.Now()); err != nil || n != 0 {
		t.Fatalf("DeleteMissingBefore = %d, %v; want nothing deleted", n, err)
	}
	if gen := s.library.gen.Load(); gen != before {
		t.Errorf("a prune and a delete of nothing moved the library writes from %d to %d", before, gen)
	}
	deleteTrack(t, s, 1)
	if got := prune(t, s); got.Albums != 1 {
		t.Fatalf("PruneOrphans = %+v, want the emptied album", got)
	}
	if gen := s.library.gen.Load(); gen == before {
		t.Error("a prune that deleted left the kept rows standing")
	}
}
