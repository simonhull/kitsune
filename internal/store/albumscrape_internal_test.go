package store

import (
	"database/sql"
	"testing"
	"time"

	"github.com/simonhull/kitsune/internal/store/sqlcdb"
)

// indexCover is the cover the library index shows of newWrite(0)'s album, nil
// for none, read as a page reads it, through the kept rows.
func indexCover(t *testing.T, s *Store) *Cover {
	t.Helper()
	idx, err := s.LibraryIndex(t.Context(), 0, &IndexOptions{})
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range idx.Artists {
		for i := range a.Albums {
			if al := &a.Albums[i]; al.ID == newWrite(0).Album.ULID {
				return al.Cover
			}
		}
	}
	t.Fatal("the album is not in the index")
	return nil
}

// holdCover fails the test unless the index shows want as the album's cover.
func holdCover(t *testing.T, s *Store, want [32]byte, what string) {
	t.Helper()
	if c := indexCover(t, s); c == nil || c.Hash != want {
		t.Fatalf("index cover = %+v, want %s", c, what)
	}
}

func archiveFront(seed string) *RemoteAlbumImage {
	img := testArtwork(seed)
	img.Source = ArtworkRemote
	return &RemoteAlbumImage{Kind: AlbumCover, Source: ImageFromArchive, RemoteID: seed, Image: img}
}

func writeArchive(t *testing.T, s *Store, album int64, images ...RemoteAlbumImage) bool {
	t.Helper()
	changed, err := s.WriteAlbumRemote(t.Context(), &AlbumRemoteWrite{AlbumID: album, Images: images}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	return changed
}

// An archive cover is the album's in the index at once (the kept rows let go),
// the files' cover beats it at once (albums_cover_from_files), and the archive
// never writes over the files'.
func TestArchiveCoverMeetsTheKeptRowsAndTheFiles(t *testing.T) {
	t.Parallel()
	s := openTest(t)
	writeAll(t, s, newWrite(0))
	album := count(t, s, "SELECT id FROM albums WHERE group_key = 'album-0'")
	if indexCover(t, s) != nil {
		t.Fatal("a new album has a cover")
	}

	front := archiveFront("front-1")
	if !writeArchive(t, s, album, *front) {
		t.Error("setting the archive's cover changed nothing")
	}
	holdCover(t, s, front.Image.Hash, "the archive's")
	if writeArchive(t, s, album, *front) {
		t.Error("the same cover again changed something")
	}
	ex, err := s.albumExtras(t.Context(), album)
	if err != nil || ex.Cover == nil {
		t.Fatalf("extras = %+v, %v; want the archive's cover", ex, err)
	}

	tw := newWrite(1) // Another song of the album, with a cover in its file.
	tw.Album = newWrite(0).Album
	tw.Album.Artwork = testArtwork("files")
	writeAll(t, s, tw)
	holdCover(t, s, tw.Album.Artwork.Hash, "the files'")
	if n := count(t, s, "SELECT count(*) FROM album_images WHERE kind = 'cover'"); n != 0 {
		t.Errorf("%d archive covers left beside the files'", n)
	}
	if writeArchive(t, s, album, *archiveFront("front-2")) {
		t.Error("the archive wrote over the files' cover")
	}
	holdCover(t, s, tw.Album.Artwork.Hash, "the files' still")
}

// Taking the archive's cover away leaves the album with none, and a merge
// prefers a cover from the files to one from the archive.
func TestArchiveCoverGoesAndMerges(t *testing.T) {
	t.Parallel()
	s := openTest(t)
	writeAll(t, s, newWrite(0))
	album := count(t, s, "SELECT id FROM albums WHERE group_key = 'album-0'")
	writeArchive(t, s, album, *archiveFront("front-1"))
	if !writeArchive(t, s, album, RemoteAlbumImage{Kind: AlbumCover, Source: ImageFromArchive}) {
		t.Error("taking the cover away changed nothing")
	}
	if indexCover(t, s) != nil || count(t, s, "SELECT count(*) FROM album_images") != 0 {
		t.Error("the archive's cover stayed")
	}
	if _, err := s.PruneOrphans(t.Context()); err != nil {
		t.Fatal(err)
	}

	writeArchive(t, s, album, *archiveFront("front-1"))
	other := newWrite(2)
	other.Album.Artwork = testArtwork("files")
	writeAll(t, s, other)
	from := count(t, s, "SELECT id FROM albums WHERE group_key = 'album-2'")
	err := s.inTx(t.Context(), func(q *sqlcdb.Queries, _ *sql.Tx) error {
		return q.MergeAlbumInto(t.Context(), sqlcdb.MergeAlbumIntoParams{FromID: from, IntoID: album})
	})
	if err != nil {
		t.Fatal(err)
	}
	if n := count(t, s, "SELECT count(*) FROM albums WHERE id = ? AND artwork_hash = ?", album, other.Album.Artwork.Hash[:]); n != 1 {
		t.Error("a merge kept the archive's cover over the files'")
	}
}

// PruneOrphans keeps the archive's images: they are an album's.
func TestPruneKeepsArchiveImages(t *testing.T) {
	t.Parallel()
	s := openTest(t)
	writeAll(t, s, newWrite(0))
	album := count(t, s, "SELECT id FROM albums WHERE group_key = 'album-0'")
	back := archiveFront("back-1")
	back.Kind = AlbumBack
	writeArchive(t, s, album, *archiveFront("front-1"), *back)
	if _, err := s.PruneOrphans(t.Context()); err != nil {
		t.Fatal(err)
	}
	if n := count(t, s, "SELECT count(*) FROM artwork"); n != 2 {
		t.Errorf("artwork rows = %d, want the archive's front and back kept", n)
	}
}
