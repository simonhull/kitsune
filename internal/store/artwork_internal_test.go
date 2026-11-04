package store

import (
	"crypto/sha256"
	"slices"
	"testing"
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
		"INSERT INTO artwork (hash, width, height, accent, ground, source) VALUES (zeroblob(32), 1, 1, '#000000', '#000000', 'web')")
	if err == nil {
		t.Error("an unknown artwork source was accepted")
	}
	_, err = s.w.ExecContext(t.Context(),
		"INSERT INTO artwork (hash, width, height, accent, ground, source) VALUES (zeroblob(5), 1, 1, '#000000', '#000000', 'folder')")
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
