package store

import (
	"slices"
	"testing"
	"time"
)

// artistOf returns the row ID of newWrite(i)'s artist.
func artistOf(t *testing.T, s *Store, i int) int64 {
	t.Helper()
	return count(t, s, "SELECT id FROM artists WHERE match_key = ?", newWrite(i).Artists[0].MatchKey)
}

// hello is a bio.
const hello = "Hi."

func folderFile(rel string) FileRef {
	return FileRef{Root: testRoot, RelPath: rel, Size: 10, MTime: time.UnixMilli(1_761_000_000_000)}
}

// writeEverything gives newWrite(0)'s artist a thumb and a bio and its album a
// back cover, from folders, and returns the artist's row and the thumb.
func writeEverything(t *testing.T, s *Store) (int64, *ArtworkWrite) {
	t.Helper()
	writeAll(t, s, newWrite(0))
	artist := artistOf(t, s, 0)
	album := count(t, s, "SELECT id FROM albums WHERE group_key = 'album-0'")
	thumb, back := testArtwork("thumb"), testArtwork("back")
	thumb.Source, back.Source = ArtworkFolder, ArtworkFolder
	s.TakeChangedAlbums()
	changed, err := s.WriteExtras(t.Context(), &ExtrasWrite{
		ArtistImages: []ArtistImageWrite{{ArtistID: artist, Kind: ArtistThumb, Image: thumb, File: folderFile("Artist/artist.jpg")}},
		AlbumImages:  []AlbumImageWrite{{AlbumID: album, AlbumULID: newWrite(0).Album.ULID, Kind: AlbumBack, Image: back, File: folderFile("Artist/Album/back.jpg")}},
		Bios:         []BioWrite{{ArtistID: artist, Bio: &NfoBio{Summary: hello, Body: hello}, File: folderFile("Artist/artist.nfo")}},
	}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(changed, []int64{artist, artist}) {
		t.Errorf("changed = %v, want the artist's image and bio", changed)
	}
	if got := s.TakeChangedAlbums(); !slices.Equal(got, []string{newWrite(0).Album.ULID}) {
		t.Errorf("changed albums = %v, want the album", got)
	}
	return artist, thumb
}

func TestWriteExtrasSets(t *testing.T) {
	t.Parallel()
	s := openTest(t)
	artist, thumb := writeEverything(t, s)
	l, err := s.FolderLayout(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	stored := l.ArtistImages[ArtistImageKey{ArtistID: artist, Kind: ArtistThumb}]
	if want := folderFile("Artist/artist.jpg"); !stored.File.Same(&want) || stored.Hash != thumb.Hash || stored.Pinned {
		t.Errorf("stored thumb = %+v", stored)
	}
	if l.Bios[artist].File == nil || len(l.AlbumImages) != 1 {
		t.Error("the layout lacks the bio's or the back cover's file")
	}
}

func TestWriteExtrasTakesAway(t *testing.T) {
	t.Parallel()
	s := openTest(t)
	artist, _ := writeEverything(t, s)
	changed, err := s.WriteExtras(t.Context(), &ExtrasWrite{
		ArtistImages: []ArtistImageWrite{{ArtistID: artist, Kind: ArtistThumb}, {ArtistID: artist, Kind: ArtistLogo}},
		Bios:         []BioWrite{{ArtistID: artist}},
	}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(changed, []int64{artist, artist}) {
		t.Errorf("changed = %v, want the thumb and the bio taken away, and no logo", changed)
	}
	if n := count(t, s, "SELECT count(*) FROM artist_images") + count(t, s, "SELECT count(*) FROM artist_bios"); n != 0 {
		t.Errorf("%d artist rows left, want none", n)
	}
}

// A folder's write never replaces or takes away an administrator's pin.
func TestWriteExtrasLeavesAPin(t *testing.T) {
	t.Parallel()
	s := openTest(t)
	writeAll(t, s, newWrite(0))
	artist := artistOf(t, s, 0)
	pin := testArtwork("pin")
	useArtwork(t, s, pin, 0, 0)
	if _, err := s.w.ExecContext(t.Context(), "UPDATE artist_images SET source = 'pinned', file_root = NULL, file_path = NULL, file_size = NULL, file_mtime = NULL"); err != nil {
		t.Fatal(err)
	}
	other := testArtwork("folder")
	other.Source = ArtworkFolder
	for _, w := range []ArtistImageWrite{
		{ArtistID: artist, Kind: ArtistThumb, Image: other, File: folderFile("Artist/artist.jpg")},
		{ArtistID: artist, Kind: ArtistThumb},
	} {
		changed, err := s.WriteExtras(t.Context(), &ExtrasWrite{ArtistImages: []ArtistImageWrite{w}}, time.Now())
		if err != nil {
			t.Fatal(err)
		}
		if len(changed) != 0 {
			t.Errorf("a write over a pin changed %v", changed)
		}
	}
	d, err := s.Artist(t.Context(), newWrite(0).Artists[0].ULID)
	if err != nil {
		t.Fatal(err)
	}
	if d.Images.Thumb == nil || d.Images.Thumb.Hash != pin.Hash || d.Images.Thumb.Source != "pinned" {
		t.Errorf("thumb = %+v, want the pin", d.Images.Thumb)
	}
}

// An artist's images and bio go with it, and the prune then frees their image.
func TestArtistDeleteTakesItsArt(t *testing.T) {
	t.Parallel()
	s := openTest(t)
	writeAll(t, s, newWrite(0), newWrite(1))
	art := testArtwork("own")
	useArtwork(t, s, art, 0, 1)
	if _, err := s.WriteExtras(t.Context(), &ExtrasWrite{
		Bios: []BioWrite{{ArtistID: artistOf(t, s, 0), Bio: &NfoBio{Summary: hello, Body: hello}, File: folderFile("a/artist.nfo")}},
	}, time.Now()); err != nil {
		t.Fatal(err)
	}
	deleteTrack(t, s, 0)
	deleteTrack(t, s, 1)
	got := prune(t, s)
	if n := count(t, s, "SELECT count(*) FROM artist_images") + count(t, s, "SELECT count(*) FROM album_images") + count(t, s, "SELECT count(*) FROM artist_bios"); n != 0 {
		t.Errorf("%d rows of art outlived their artist and album", n)
	}
	if !slices.Contains(got.Artwork, art.Hash) {
		t.Errorf("PruneOrphans = %+v, want the image freed", got)
	}
}

// The thumbs of GET /artists ride on the list, one read for all.
func TestArtistsCarryTheirThumbs(t *testing.T) {
	t.Parallel()
	s := openTest(t)
	writeAll(t, s, newWrite(0), newWrite(1))
	art := testArtwork("thumb")
	useArtwork(t, s, art, 1, 1)
	artists, err := s.Artists(t.Context(), 0)
	if err != nil {
		t.Fatal(err)
	}
	for i := range artists {
		a := &artists[i]
		want := a.Name == newWrite(1).Artists[0].Name
		if (a.Thumb != nil) != want || (want && a.Thumb.Hash != art.Hash) {
			t.Errorf("%s's thumb = %+v, want one: %v", a.Name, a.Thumb, want)
		}
	}
}
