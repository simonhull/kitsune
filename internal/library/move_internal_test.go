package library

import (
	"context"
	"errors"
	"path/filepath"
	"slices"
	"testing"

	"github.com/simonhull/audiometa"

	"github.com/simonhull/kitsune/internal/store"
)

const (
	renamed    = "Renamed"
	firstAlbum = "First"
)

// errOf drops the album an edit landed on.
func errOf(_ string, err error) error { return err }

// renameAlbum renames every track's album.
func renameAlbum() *AlbumEdit {
	return &AlbumEdit{All: Edit{Fields: map[Field][]string{FieldAlbum: {renamed}}}}
}

// failWrite makes the nth write fail with the error it returns.
func (l *editLib) failWrite(n int) error {
	boom := errors.New("disk full")
	writes := 0
	l.scanner.write = func(ctx context.Context, path string, edit func(*audiometa.File) error) error {
		writes++
		if writes == n {
			return boom
		}
		return audiometa.WriteContext(ctx, path, edit)
	}
	return boom
}

func TestRenamingAnAlbumKeepsItsRow(t *testing.T) {
	t.Parallel()
	l := newEditLib(t)
	if _, err := l.scanner.EditAlbum(t.Context(), l.album.ID, l.album.ETag, renameAlbum()); err != nil {
		t.Fatal(err)
	}
	after := l.albumDetail()
	if after.ID != l.album.ID || after.Title != renamed || !after.AddedAt.Equal(l.album.AddedAt) {
		t.Errorf("album = %s %q added %v, want %s renamed, added %v", after.ID, after.Title, after.AddedAt, l.album.ID, l.album.AddedAt)
	}
	if got := l.scan(Full); got.Added != 0 {
		t.Errorf("a full scan added %d tracks", got.Added)
	}
	if again := l.albumDetail(); again.ID != l.album.ID {
		t.Errorf("after a full scan the album is %s, want %s", again.ID, l.album.ID)
	}
}

func TestARenameThatWritesNothingIsUndone(t *testing.T) {
	t.Parallel()
	l := newEditLib(t)
	boom := l.failWrite(1)
	if _, err := l.scanner.EditAlbum(t.Context(), l.album.ID, l.album.ETag, renameAlbum()); !errors.Is(err, boom) {
		t.Fatalf("EditAlbum = %v, want the write's failure", err)
	}
	l.scan(Full)
	if after := l.albumDetail(); after.ID != l.album.ID || after.Title != l.album.Title {
		t.Errorf("album = %s %q, want %s %q as before", after.ID, after.Title, l.album.ID, l.album.Title)
	}
}

func TestARetryFinishesARename(t *testing.T) {
	t.Parallel()
	l := newEditLib(t)
	boom := l.failWrite(2)
	if _, err := l.scanner.EditAlbum(t.Context(), l.album.ID, l.album.ETag, renameAlbum()); !errors.Is(err, boom) {
		t.Fatalf("EditAlbum = %v, want the write's failure", err)
	}
	half := l.albumDetail()
	if half.ID != l.album.ID || len(half.Tracks) != len(l.album.Tracks) {
		t.Fatalf("after a stop the album is %s with %d tracks, want %s with all %d", half.ID, len(half.Tracks), l.album.ID, len(l.album.Tracks))
	}
	l.scanner.write = func(ctx context.Context, path string, edit func(*audiometa.File) error) error {
		return audiometa.WriteContext(ctx, path, edit)
	}
	if _, err := l.scanner.EditAlbum(t.Context(), half.ID, half.ETag, renameAlbum()); err != nil {
		t.Fatal(err)
	}
	l.scan(Full)
	after := l.albumDetail()
	if after.ID != l.album.ID || after.Title != renamed {
		t.Errorf("album = %s %q, want %s renamed", after.ID, after.Title, l.album.ID)
	}
	ids := func(d *store.AlbumDetail) []string {
		out := make([]string, len(d.Tracks))
		for i := range d.Tracks {
			out[i] = d.Tracks[i].ID
		}
		slices.Sort(out)
		return out
	}
	if !slices.Equal(ids(&after), ids(&l.album)) {
		t.Errorf("tracks %q, want %q", ids(&after), ids(&l.album))
	}
}

// marker is a person whose plays and marks a move must keep.
type marker struct {
	l      *testLib
	userID int64
}

func newMarker(l *testLib) *marker {
	l.t.Helper()
	u, err := l.st.CreateUser(l.t.Context(), store.NewUser{Name: "simon", PasswordHash: "h", Role: store.RoleUser})
	if err != nil {
		l.t.Fatal(err)
	}
	return &marker{l: l, userID: u.ID}
}

// mark plays, hearts and rates the track id: a heart kept from before stars
// were the only like follows a track as its rating does.
func (m *marker) mark(id string) {
	m.l.t.Helper()
	ctx := m.l.t.Context()
	err := m.l.st.RecordPlay(ctx, &store.NewPlay{UserID: m.userID, TrackID: id, PlayedAt: m.l.clock.Now(), Source: playSrc, ClientID: id})
	if err == nil {
		err = m.l.st.Heart(ctx, m.userID, store.KindTrack, id)
	}
	if err == nil {
		err = m.l.st.Rate(ctx, m.userID, id, 8)
	}
	if err != nil {
		m.l.t.Fatal(err)
	}
}

// wantMarked checks the track id, on album, kept what [marker.mark] gave it.
func (m *marker) wantMarked(album, id string) {
	m.l.t.Helper()
	ctx := m.l.t.Context()
	plays, err := m.l.st.RecentPlays(ctx, m.userID, mostRead)
	if err != nil {
		m.l.t.Fatal(err)
	}
	if !slices.ContainsFunc(plays, func(p store.Play) bool { return p.TrackID == id }) {
		m.l.t.Errorf("plays = %+v, want one of %s", plays, id)
	}
	ratings, err := m.l.st.AlbumRatings(ctx, m.userID, album)
	if err != nil {
		m.l.t.Fatal(err)
	}
	if got := ratings[id]; got != 8 {
		m.l.t.Errorf("rating of %s on %s = %v, want 4 stars", id, album, got)
	}
	hearts, err := m.l.st.HeartsSummary(ctx, m.userID)
	if err != nil {
		m.l.t.Fatal(err)
	}
	if hearts.ByStars[3] != 1 {
		m.l.t.Errorf("hearts = %+v, want the one 4★ song still hearted", hearts)
	}
}

func TestMovingATrackKeepsItsIDAndMarks(t *testing.T) {
	t.Parallel()
	l := newEditLib(t)
	m := newMarker(l.testLib)
	moved := l.album.Tracks[0].ID
	m.mark(moved)
	edit := AlbumEdit{Tracks: map[string]Edit{moved: {Fields: map[Field][]string{FieldAlbum: {renamed}}}}}
	landed, err := l.scanner.EditAlbum(t.Context(), l.album.ID, l.album.ETag, &edit)
	if err != nil {
		t.Fatal(err)
	}
	if landed != l.album.ID {
		t.Errorf("landed on %s, want %s: a track is still there", landed, l.album.ID)
	}
	to, err := l.st.TrackAlbum(t.Context(), moved)
	if err != nil {
		t.Fatal(err)
	}
	album, err := l.st.Album(t.Context(), to)
	if err != nil {
		t.Fatal(err)
	}
	if to == l.album.ID || album.Title != renamed || len(album.Tracks) != 1 {
		t.Errorf("the track is on %s %q with %d tracks, want a new album %q of 1", to, album.Title, len(album.Tracks), renamed)
	}
	m.wantMarked(to, moved)
	if left, _ := l.st.Album(t.Context(), l.album.ID); len(left.Tracks) != 1 {
		t.Errorf("the old album has %d tracks, want 1", len(left.Tracks))
	}
}

// pairLib is a library of two albums in one folder.
type pairLib struct {
	*editLib

	first, second store.AlbumDetail // Of a.flac and of bystander.flac.
}

func newPairLib(t *testing.T) *pairLib {
	t.Helper()
	l := &pairLib{editLib: &editLib{testLib: newTestLib(t, 1)}}
	l.root = l.roots[0]
	l.copyTo(l.root, aFLAC, sampleFLAC)
	l.copyTo(l.root, bystander, sampleFLAC)
	seed(t, filepath.Join(l.root, aFLAC), map[string][]string{keyAlbum: {firstAlbum}})
	seed(t, filepath.Join(l.root, bystander), map[string][]string{keyAlbum: {"Second"}})
	l.opts.TagWrites = true
	l.scanner = NewScanner(l.st, l.opts)
	l.scan(Quick)
	page, err := l.st.Albums(t.Context(), store.SortName, mostRead, "")
	if err != nil || len(page.Albums) != 2 {
		t.Fatalf("albums = %+v, %v; want 2", page.Albums, err)
	}
	if l.first, err = l.st.Album(t.Context(), page.Albums[0].ID); err != nil {
		t.Fatal(err)
	}
	if l.second, err = l.st.Album(t.Context(), page.Albums[1].ID); err != nil {
		t.Fatal(err)
	}
	return l
}

func TestMovingEveryTrackMergesTheAlbum(t *testing.T) {
	t.Parallel()
	pl := newPairLib(t)
	l, first, second := pl.editLib, pl.first, pl.second
	m := newMarker(l.testLib)
	track := second.Tracks[0].ID
	m.mark(track)
	if err := l.st.Heart(t.Context(), m.userID, store.KindAlbum, second.ID); err != nil {
		t.Fatal(err)
	}
	edit := AlbumEdit{All: Edit{Fields: map[Field][]string{FieldAlbum: {firstAlbum}}}}
	landed, err := l.scanner.EditAlbum(t.Context(), second.ID, second.ETag, &edit)
	if err != nil {
		t.Fatal(err)
	}
	if landed != first.ID {
		t.Errorf("landed on %s, want the album merged into, %s", landed, first.ID)
	}
	if _, err = l.st.Album(t.Context(), second.ID); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("the merged album answers %v, want ErrNotFound", err)
	}
	merged, err := l.st.Album(t.Context(), first.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(merged.Tracks) != 2 || !merged.AddedAt.Equal(first.AddedAt) {
		t.Errorf("merged album has %d tracks, added %v; want 2, added %v", len(merged.Tracks), merged.AddedAt, first.AddedAt)
	}
	m.wantMarked(first.ID, track)
	if hearts, _ := l.st.HeartsSummary(t.Context(), m.userID); hearts.Albums != 1 {
		t.Errorf("album hearts = %d, want the merged album's moved, 1", hearts.Albums)
	}
}

func TestAnAlbumsYearIsItsFirstTracks(t *testing.T) {
	t.Parallel()
	l := newEditLib(t) // a.flac sorts before bystander.flac.
	second := l.album.Tracks[slices.IndexFunc(l.album.Tracks, func(tr store.Track) bool {
		f, err := l.st.TrackFile(t.Context(), tr.ID)
		return err == nil && f.RelPath == bystander
	})].ID
	edit := AlbumEdit{Tracks: map[string]Edit{second: {Fields: map[Field][]string{FieldDate: {"1999"}}}}}
	if _, err := l.scanner.EditAlbum(t.Context(), l.album.ID, l.album.ETag, &edit); err != nil {
		t.Fatal(err)
	}
	if after := l.albumDetail(); after.Year != l.album.Year {
		t.Errorf("album year = %d after editing its second track, want the first's %d", after.Year, l.album.Year)
	}
	after := l.albumDetail()
	if _, err := l.scanner.EditAlbum(t.Context(), after.ID, after.ETag, &AlbumEdit{All: *yearEdit()}); err != nil {
		t.Fatal(err)
	}
	if got := l.albumDetail(); got.Year != newYear {
		t.Errorf("album year = %d, want %d", got.Year, newYear)
	}
}

func TestATrackMovedOffTakesItsCover(t *testing.T) {
	t.Parallel()
	l := newArtLib(t)
	l.opts.TagWrites = true
	l.scanner = NewScanner(l.st, l.opts)
	root := l.roots[0]
	l.copyTo(root, "a.flac", sampleFLAC)
	l.copyTo(root, "b.flac", sampleFLAC)
	sameAlbum(t, root, "a.flac", "b.flac")
	art, otherArt := testPNG(t, red()), testPNG(t, blue())
	numbered(t, root, "a.flac", 1, 1, art)
	numbered(t, root, "b.flac", 1, 2, otherArt)
	l.scan(Full)
	page, err := l.st.Albums(t.Context(), store.SortAdded, mostRead, "")
	if err != nil || len(page.Albums) != 1 {
		t.Fatalf("albums = %+v, %v", page.Albums, err)
	}
	before, err := l.st.Album(t.Context(), page.Albums[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	moved := before.Tracks[1].ID
	edit := AlbumEdit{Tracks: map[string]Edit{moved: {Fields: map[Field][]string{FieldAlbum: {renamed}}}}}
	if _, err = l.scanner.EditAlbum(t.Context(), before.ID, before.ETag, &edit); err != nil {
		t.Fatal(err)
	}
	wantHash(t, l.albumOf("b.flac").hash, otherArt)
	wantHash(t, l.albumOf("a.flac").hash, art)
}

// byArtist makes every track of l's album by name alone, and returns the album.
func (l *editLib) byArtist(name string) store.AlbumDetail {
	l.t.Helper()
	album := l.albumDetail()
	edit := AlbumEdit{All: Edit{Fields: map[Field][]string{FieldArtists: {name}, FieldAlbumArtists: nil}}}
	if _, err := l.scanner.EditAlbum(l.t.Context(), album.ID, album.ETag, &edit); err != nil {
		l.t.Fatal(err)
	}
	return l.albumDetail()
}

// owners returns the artists who own an album, by name.
func (l *testLib) owners() map[string]string {
	l.t.Helper()
	artists, err := l.st.Artists(l.t.Context(), 0)
	if err != nil {
		l.t.Fatal(err)
	}
	out := make(map[string]string, len(artists))
	for _, a := range artists {
		out[a.Name] = a.ID
	}
	return out
}

func TestRenamingAnArtistEverywhereKeepsItsRow(t *testing.T) {
	t.Parallel()
	l := newEditLib(t)
	album := l.byArtist("Mara Vey")
	before := l.owners()["Mara Vey"]
	edit := AlbumEdit{All: Edit{Fields: map[Field][]string{FieldArtists: {"Mara Vay"}}}}
	if _, err := l.scanner.EditAlbum(t.Context(), album.ID, album.ETag, &edit); err != nil {
		t.Fatal(err)
	}
	if after := l.owners(); len(after) != 1 || after["Mara Vay"] != before {
		t.Errorf("owners = %v, want Mara Vay alone as %s", after, before)
	}
}

func TestRenamingAnArtistOnSomeTracksRelinksThem(t *testing.T) {
	t.Parallel()
	l := newEditLib(t)
	album := l.byArtist("Mara Vey")
	before := l.owners()["Mara Vey"]
	moved := album.Tracks[0].ID
	edit := AlbumEdit{Tracks: map[string]Edit{moved: {Fields: map[Field][]string{FieldArtists: {"Mara Vay"}}}}}
	if _, err := l.scanner.EditAlbum(t.Context(), album.ID, album.ETag, &edit); err != nil {
		t.Fatal(err)
	}
	after := l.owners()
	if after["Mara Vey"] != before || after["Mara Vay"] == "" || after["Mara Vay"] == before {
		t.Errorf("owners = %v, want Mara Vey kept as %s and Mara Vay new", after, before)
	}
}

func TestACaseOnlyRenameUpdatesTheRow(t *testing.T) {
	t.Parallel()
	l := newEditLib(t)
	album := l.byArtist("mara vey")
	before := l.owners()["mara vey"]
	edit := AlbumEdit{All: Edit{Fields: map[Field][]string{FieldArtists: {"Mara Vey"}}}}
	if _, err := l.scanner.EditAlbum(t.Context(), album.ID, album.ETag, &edit); err != nil {
		t.Fatal(err)
	}
	if after := l.owners(); len(after) != 1 || after["Mara Vey"] != before {
		t.Errorf("owners = %v, want Mara Vey alone as %s", after, before)
	}
}
