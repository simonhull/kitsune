package store_test

import (
	"errors"
	"fmt"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/simonhull/kitsune/internal/rating"
	"github.com/simonhull/kitsune/internal/store"
)

// trashLib is a library of two albums: album 1 with three songs, the second of
// which is trashed, and album 2 by its own artist with one song, which is
// trashed too, so the album and its artist leave every list with it. Every
// song is played and rated, so Home and the ratings would show each one.
type trashLib struct {
	s       *store.Store
	user    int64
	live    []store.TrackWrite // Album 1's first and third songs.
	trashed []store.TrackWrite // Album 1's second song and album 2's only one.
}

// soloArtist is the trashed album's artist.
func soloArtist() store.ArtistWrite {
	return store.ArtistWrite{ULID: artistULID("solo"), MatchKey: "solo", Name: "Solo", SortName: "Solo"}
}

// firstOwner is the artist of album 1's first song, which stays live.
func firstOwner() string { return artistULID("artist-1001") }

func newTrashLib(t *testing.T) *trashLib {
	t.Helper()
	s, clock := openBrowse(t)
	u, err := s.CreateUser(t.Context(), store.NewUser{Name: simon, PasswordHash: "h", Role: store.RoleAdmin})
	if err != nil {
		t.Fatal(err)
	}
	one := []store.TrackWrite{albumTrack(1, 1, "Album 1"), albumTrack(1, 2, "Album 1"), albumTrack(1, 3, "Album 1")}
	for i := range one {
		one[i].TrackNo = i + 1
		one[i].Title = fmt.Sprintf("Hymn %d", i+1)
		one[i].Tags = []store.Tag{{Key: genreKey, Value: "Rock"}}
	}
	two := albumTrack(2, 1, "Album 2")
	two.Title = "Hymn Alone"
	two.Artists = []store.ArtistWrite{soloArtist()}
	two.Tags = []store.Tag{{Key: genreKey, Value: "Blues"}}
	writeAll(t, s, slices.Concat(one, []store.TrackWrite{two})...)
	l := &trashLib{s: s, user: u.ID, live: []store.TrackWrite{one[0], one[2]}, trashed: []store.TrackWrite{one[1], two}}
	every := slices.Concat(l.live, l.trashed)
	for i := range every {
		w := &every[i]
		play := &store.NewPlay{UserID: u.ID, TrackID: w.ULID, PlayedAt: clock.Now(), Source: "web", ClientID: strconv.Itoa(i)}
		if err = s.RecordPlay(t.Context(), play); err != nil {
			t.Fatal(err)
		}
		if err = s.Rate(t.Context(), u.ID, w.ULID, rating.Max); err != nil {
			t.Fatal(err)
		}
	}
	trashBy(t, s, u.ID, clock.Now(), l.ids(l.trashed)...)
	if err = s.RefreshNames(t.Context()); err != nil {
		t.Fatal(err)
	}
	return l
}

// trashBy marks the tracks ids trashed by the user by, as the scanner does once
// their files are in the Trash.
func trashBy(t *testing.T, s *store.Store, by int64, at time.Time, ids ...string) []store.TrashTrack {
	t.Helper()
	got, err := s.TrashableTracks(t.Context(), ids)
	if err != nil {
		t.Fatal(err)
	}
	marks := make([]store.Trashing, len(got))
	for i := range got {
		marks[i] = store.Trashing{ID: got[i].ID, TrashPath: ".kitsune-trash/" + got[i].ULID + "/" + got[i].RelPath}
	}
	if err = s.MarkTrashed(t.Context(), marks, by, at); err != nil {
		t.Fatal(err)
	}
	return got
}

// ids returns the public IDs of the tracks.
func (l *trashLib) ids(tws []store.TrackWrite) []string {
	out := make([]string, len(tws))
	for i := range tws {
		out[i] = tws[i].ULID
	}
	return out
}

// must returns what a read gave, once t is given: it fails t on err, and
// otherwise returns v. must(read(...))(t) takes a read's two results whole.
func must[V any](v V, err error) func(t *testing.T) V {
	return func(t *testing.T) V {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
}

// gone fails t unless err is ErrNotFound: what the read asked for is no more.
func gone(t *testing.T, err error) []string {
	t.Helper()
	if !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
	return nil
}

// liveRead is one read that means "live": what it shows, by public ID, and one
// live ID it must show, so that a read showing nothing can't pass.
type liveRead struct {
	name string
	want string // Empty for a read of something that is gone.
	read func(t *testing.T, l *trashLib) []string
}

// TestTrashedTrackIsInNoRead holds every read of the library to one rule: it
// fails if a trashed song, its album with nothing else live, or its artist with
// nothing else, is among what the read shows.
func TestTrashedTrackIsInNoRead(t *testing.T) {
	t.Parallel()
	l := newTrashLib(t)
	hidden := append(l.ids(l.trashed), albumID(2), soloArtist().ULID)
	for _, tc := range liveReads(l.live[0].ULID) {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := tc.read(t, l)
			for _, id := range hidden {
				if slices.Contains(got, id) {
					t.Errorf("shows %s, which is trashed", id)
				}
			}
			if tc.want != "" && !slices.Contains(got, tc.want) {
				t.Errorf("shows %q, want %s among them", got, tc.want)
			}
		})
	}
}

// liveReads are the reads of the library, live the ID of a song each shows.
func liveReads(live string) []liveRead {
	return []liveRead{
		{"index", live, readIndex(&store.IndexOptions{})},
		{"index with every field", live, readIndex(&store.IndexOptions{
			Details: true, AlbumArtists: true, Plays: true, Shuffle: true, Measured: true, TagKeys: []string{genreKey},
		})},
		{"index of the albums an event names", live, readIndex(&store.IndexOptions{Albums: []string{albumID(1), albumID(2)}, Plays: true})},
		{"an artist's index", live, readIndex(&store.IndexOptions{Artist: firstOwner(), Details: true})},
		{"a trashed artist's index", "", readTrashedArtistIndex},
		{"albums by added", albumID(1), readAlbums(store.SortAdded)},
		{"albums by name", albumID(1), readAlbums(store.SortName)},
		{"an album", live, readAlbum},
		{"a trashed album", "", func(t *testing.T, l *trashLib) []string {
			t.Helper()
			_, err := l.s.Album(t.Context(), albumID(2))
			return gone(t, err)
		}},
		{"artists", firstOwner(), readArtists},
		{"an artist", albumID(1), readArtist},
		{"a trashed artist", "", func(t *testing.T, l *trashLib) []string {
			t.Helper()
			_, err := l.s.Artist(t.Context(), soloArtist().ULID)
			return gone(t, err)
		}},
		{"search", live, readSearch("hymn")},
		{"search by the trashed album and artist", "", readSearch("solo album 2")},
		{"stream", live, readStreams},
		{"playback's live tracks", live, readPlayback},
		{"shuffle's catalog", albumID(1), readShuffle},
		{"home", albumID(1), readHome},
		{"an album's edit files", live, readEditFiles},
		{"genres", albumID(1), readGenres},
	}
}

func readIndex(opts *store.IndexOptions) func(t *testing.T, l *trashLib) []string {
	return func(t *testing.T, l *trashLib) []string {
		t.Helper()
		return indexIDs(must(l.s.LibraryIndex(t.Context(), l.user, opts))(t))
	}
}

func readTrashedArtistIndex(t *testing.T, l *trashLib) []string {
	t.Helper()
	_, err := l.s.LibraryIndex(t.Context(), l.user, &store.IndexOptions{Artist: soloArtist().ULID})
	return gone(t, err)
}

func indexIDs(ix store.LibraryIndex) []string {
	var out []string
	for _, ar := range ix.Artists {
		out = append(out, ar.ID)
		for i := range ar.Albums {
			al := &ar.Albums[i]
			out = append(out, al.ID)
			for j := range al.Tracks {
				out = append(out, al.Tracks[j].ID)
			}
		}
	}
	return out
}

func summaries(as []store.AlbumSummary) []string {
	out := make([]string, len(as))
	for i := range as {
		out[i] = as[i].ID
	}
	return out
}

func readAlbums(sort store.AlbumSort) func(t *testing.T, l *trashLib) []string {
	return func(t *testing.T, l *trashLib) []string {
		t.Helper()
		return summaries(must(l.s.Albums(t.Context(), sort, 50, ""))(t).Albums)
	}
}

func readAlbum(t *testing.T, l *trashLib) []string {
	t.Helper()
	d := must(l.s.Album(t.Context(), albumID(1)))(t)
	if d.TrackCount != len(l.live) || len(d.Discs) != 1 || d.Discs[0].Songs != len(l.live) {
		t.Errorf("album counts %d songs, its disc %+v; want %d", d.TrackCount, d.Discs, len(l.live))
	}
	out := make([]string, 0, 1+len(d.Tracks))
	out = append(out, d.ID)
	for i := range d.Tracks {
		out = append(out, d.Tracks[i].ID)
	}
	return out
}

func readArtists(t *testing.T, l *trashLib) []string {
	t.Helper()
	as := must(l.s.Artists(t.Context(), l.user))(t)
	out := make([]string, len(as))
	for i := range as {
		out[i] = as[i].ID
	}
	return out
}

func readArtist(t *testing.T, l *trashLib) []string {
	t.Helper()
	d := must(l.s.Artist(t.Context(), firstOwner()))(t)
	if d.SongCount != 1 { // Each of album 1's songs has an artist of its own.
		t.Errorf("artist counts %d songs, want 1", d.SongCount)
	}
	return slices.Concat(summaries(d.Albums), summaries(d.AppearsOn))
}

func readSearch(q string) func(t *testing.T, l *trashLib) []string {
	return func(t *testing.T, l *trashLib) []string {
		t.Helper()
		r := must(l.s.Search(t.Context(), store.ParseSearch(q)))(t)
		out := summaries(r.Albums)
		for _, a := range r.Artists {
			out = append(out, a.ID)
		}
		for i := range r.Tracks {
			out = append(out, r.Tracks[i].ID)
		}
		return out
	}
}

func readStreams(t *testing.T, l *trashLib) []string {
	t.Helper()
	var out []string
	for _, id := range slices.Concat(l.ids(l.live), l.ids(l.trashed)) {
		_, err := l.s.TrackFile(t.Context(), id)
		switch {
		case err == nil:
			out = append(out, id)
		case !errors.Is(err, store.ErrNotFound):
			t.Fatal(err)
		}
	}
	return out
}

func readPlayback(t *testing.T, l *trashLib) []string {
	t.Helper()
	m := must(l.s.LiveTracks(t.Context(), slices.Concat(l.ids(l.live), l.ids(l.trashed))))(t)
	out := make([]string, 0, 2*len(m))
	for track, album := range m {
		out = append(out, track, album)
	}
	return out
}

func readShuffle(t *testing.T, l *trashLib) []string {
	t.Helper()
	c := must(l.s.ShuffleCatalog(t.Context(), l.user))(t)
	if len(c.Songs) != len(l.live) {
		t.Errorf("shuffle holds %d songs, want %d", len(c.Songs), len(l.live))
	}
	out := make([]string, len(c.Songs))
	for i := range c.Songs {
		out[i] = c.Songs[i].AlbumULID
	}
	return out
}

func readHome(t *testing.T, l *trashLib) []string {
	t.Helper()
	h := must(l.s.History(t.Context(), l.user, time.UnixMilli(0)))(t)
	out := summaries(h.Recent)
	for i := range h.OnRepeat {
		out = append(out, h.OnRepeat[i].Album.ID)
	}
	return out
}

func readEditFiles(t *testing.T, l *trashLib) []string {
	t.Helper()
	files, _, err := l.s.AlbumEditFiles(t.Context(), albumID(1))
	if err != nil {
		t.Fatal(err)
	}
	out := make([]string, len(files))
	for i := range files {
		out[i] = files[i].ULID
	}
	return out
}

func readGenres(t *testing.T, l *trashLib) []string {
	t.Helper()
	counts := must(l.s.AlbumTagCounts(t.Context(), []string{genreKey}))(t)
	out := make([]string, len(counts))
	for i := range counts {
		out[i] = counts[i].AlbumID
		if counts[i].Tracks != len(l.live) {
			t.Errorf("genre counts %d songs, want %d", counts[i].Tracks, len(l.live))
		}
	}
	return out
}

// The counts a library shows leave a trashed song out too, and no background
// pass waits on one.
func TestTrashedTrackIsInNoCount(t *testing.T) {
	t.Parallel()
	l := newTrashLib(t)
	facts := must(l.s.Facts(t.Context()))(t)
	if facts.Tracks != int64(len(l.live)) || facts.Albums != 1 {
		t.Errorf("facts = %d songs on %d albums, want %d on 1", facts.Tracks, facts.Albums, len(l.live))
	}
	if got := must(l.s.RootStats(t.Context()))(t)[testRoot].Tracks; got != int64(len(l.live)) {
		t.Errorf("root holds %d songs, want %d", got, len(l.live))
	}
	if waiting := must(l.s.CountUndigested(t.Context()))(t); waiting != 0 {
		t.Errorf("%d songs wait for a fingerprint, want none: the trashed ones never do", waiting)
	}
}

// The scanner never sees a trashed track: not at its path, and not as a
// missing track a moved file might be.
func TestScannerNeverSeesATrashedTrack(t *testing.T) {
	t.Parallel()
	l := newTrashLib(t)
	states := fileStates(t, l.s)
	if len(states) != len(l.live) {
		t.Errorf("file states = %v, want only the %d live songs", states, len(l.live))
	}
	for i := range l.trashed {
		w := &l.trashed[i]
		if _, ok := states[w.RelPath]; ok {
			t.Errorf("file states hold %s, which is trashed", w.RelPath)
		}
		if missing := must(l.s.MissingByDigest(t.Context(), *w.Digest))(t); len(missing) != 0 {
			t.Errorf("MissingByDigest(%s) = %v, want none", w.Title, missing)
		}
	}
	if must(l.s.AnyMissingWithDigest(t.Context()))(t) {
		t.Error("AnyMissingWithDigest = true, want false: a trashed song is no move to look for")
	}
}

// A grace delete leaves a trashed track for the Trash's own.
func TestDeleteMissingBeforeKeepsTheTrash(t *testing.T) {
	t.Parallel()
	l := newTrashLib(t)
	if n := must(l.s.DeleteMissingBefore(t.Context(), time.UnixMilli(1<<62)))(t); n != 0 {
		t.Errorf("DeleteMissingBefore = %d, want 0", n)
	}
	if _, err := l.s.TrashedTracks(t.Context(), l.ids(l.trashed)); err != nil {
		t.Errorf("the trashed songs went: %v", err)
	}
}

// A root that holds nothing but trashed songs is not a known root, so taking it
// out of the configuration finds nothing to mark.
func TestKnownRootsLeaveTheTrashOut(t *testing.T) {
	t.Parallel()
	s, clock := openBrowse(t)
	w := albumTrack(1, 1, "Album 1")
	w.Root = "/elsewhere"
	writeAll(t, s, w)
	trashBy(t, s, 0, clock.Now(), w.ULID)
	if roots := must(s.KnownRoots(t.Context()))(t); len(roots) != 0 {
		t.Errorf("KnownRoots = %q, want none", roots)
	}
}

// Trashing keeps the track's row and ID, moves its path into the Trash, and
// notes its album changed.
func TestMarkTrashed(t *testing.T) {
	t.Parallel()
	s, clock := openBrowse(t)
	w := albumTrack(1, 1, "Album 1")
	writeAll(t, s, w)
	s.TakeChangedAlbums()
	got := trashBy(t, s, 0, clock.Now(), w.ULID)
	if changed := s.TakeChangedAlbums(); !slices.Equal(changed, []string{albumID(1)}) {
		t.Errorf("changed after trash = %q, want the album", changed)
	}
	tr := must(s.TrashedTracks(t.Context(), []string{w.ULID}))(t)[0]
	want := ".kitsune-trash/" + w.ULID + "/" + w.RelPath
	if tr.RelPath != want || tr.RestorePath != w.RelPath || tr.ID != got[0].ID {
		t.Errorf("trashed = %+v, want it at %s, back to %s", tr, want, w.RelPath)
	}
	if _, err := s.TrashableTracks(t.Context(), []string{w.ULID}); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("trashing it again = %v, want ErrNotFound", err)
	}
}

// Restoring puts the track back live at its old path, and notes its album.
func TestUnmarkTrashed(t *testing.T) {
	t.Parallel()
	s, clock := openBrowse(t)
	w := albumTrack(1, 1, "Album 1")
	writeAll(t, s, w)
	id := trashBy(t, s, 0, clock.Now(), w.ULID)[0].ID
	s.TakeChangedAlbums()
	if err := s.UnmarkTrashed(t.Context(), []int64{id}); err != nil {
		t.Fatal(err)
	}
	if changed := s.TakeChangedAlbums(); !slices.Equal(changed, []string{albumID(1)}) {
		t.Errorf("changed after restore = %q, want the album", changed)
	}
	if f := must(s.TrackFile(t.Context(), w.ULID))(t); f.RelPath != w.RelPath {
		t.Errorf("restored file = %+v, want it live at %s", f, w.RelPath)
	}
	if err := s.UnmarkTrashed(t.Context(), []int64{id}); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("restoring it again = %v, want ErrNotFound", err)
	}
}

// A restore is refused where any track has the old path now.
func TestPathTaken(t *testing.T) {
	t.Parallel()
	s, clock := openBrowse(t)
	w := albumTrack(1, 1, "Album 1")
	writeAll(t, s, w)
	trashBy(t, s, 0, clock.Now(), w.ULID)
	if must(s.PathTaken(t.Context(), w.Root, w.RelPath))(t) {
		t.Error("PathTaken after trash = true, want free")
	}
	again := albumTrack(1, 2, "Album 1")
	again.RelPath = w.RelPath // A new file where the trashed one was.
	writeAll(t, s, again)
	if !must(s.PathTaken(t.Context(), w.Root, w.RelPath))(t) {
		t.Error("PathTaken with a new song there = false, want taken")
	}
}

// Emptying deletes the trashed tracks with their users' data, and nothing live.
func TestDeleteTrashed(t *testing.T) {
	t.Parallel()
	l := newTrashLib(t)
	trashed := must(l.s.TrashedTracks(t.Context(), l.ids(l.trashed)))(t)
	live := must(l.s.TrashableTracks(t.Context(), l.ids(l.live)))(t)
	if n := must(l.s.DeleteTrashed(t.Context(), []int64{trashed[0].ID, trashed[1].ID, live[0].ID}))(t); n != 2 {
		t.Fatalf("DeleteTrashed = %d, want the 2 trashed", n)
	}
	if _, err := l.s.TrashableTracks(t.Context(), l.ids(l.live)); err != nil {
		t.Errorf("a live song went: %v", err)
	}
	if list := must(l.s.Trash(t.Context()))(t); list.Count != 0 {
		t.Errorf("Trash = %+v, want empty", list)
	}
	if pruned := must(l.s.PruneOrphans(t.Context()))(t); pruned.Albums != 1 {
		t.Errorf("PruneOrphans = %+v, want the emptied album gone", pruned)
	}
}

// The Trash lists its songs by album, the album with the latest trashed first,
// each with who trashed it and its size.
func TestTrashLists(t *testing.T) {
	t.Parallel()
	s, clock := openBrowse(t)
	u := must(s.CreateUser(t.Context(), store.NewUser{Name: simon, PasswordHash: "h", Role: store.RoleAdmin}))(t)
	older, newer := albumTrack(1, 1, "Album 1"), albumTrack(2, 1, "Album 2")
	writeAll(t, s, older, newer)
	trashBy(t, s, u.ID, clock.Now(), older.ULID)
	trashBy(t, s, 0, clock.Now().Add(time.Hour), newer.ULID)
	list := must(s.Trash(t.Context()))(t)
	if list.Count != 2 || list.Bytes != older.Size+newer.Size || len(list.Albums) != 2 {
		t.Fatalf("Trash = %+v, want 2 songs on 2 albums", list)
	}
	if list.Albums[0].ID != albumID(2) || list.Albums[1].ID != albumID(1) {
		t.Errorf("albums = %s, %s; want the newest first", list.Albums[0].ID, list.Albums[1].ID)
	}
	if by := list.Albums[1].Tracks[0].TrashedBy; by != simon {
		t.Errorf("trashed by %q, want %s", by, simon)
	}
	if by := list.Albums[0].Tracks[0].TrashedBy; by != "" {
		t.Errorf("trashed by %q, want nobody", by)
	}
}

// An import never matches a trashed song, by its old path or by its tags, so a
// re-import brings nothing onto it and it stays trashed.
func TestImportLeavesATrashedSongTrashed(t *testing.T) {
	t.Parallel()
	l := newTrashLib(t)
	keys := store.MBIDKeys{Recording: []string{genreKey}} // Any key the songs carry stands in for an MBID's.
	got := must(l.s.ImportTargets(t.Context(), keys))(t)
	ids := make([]string, 0, len(got.Tracks))
	for i := range got.Tracks {
		ids = append(ids, got.Tracks[i].ULID)
		if len(got.Tracks[i].RecordingMBIDs) == 0 {
			t.Errorf("%s has no IDs read for it", got.Tracks[i].ULID)
		}
	}
	if want := l.ids(l.live); !slices.Equal(ids, want) {
		t.Errorf("import targets = %q, want only the live %q", ids, want)
	}
	if _, err := l.s.TrashedTracks(t.Context(), l.ids(l.trashed)); err != nil {
		t.Errorf("not trashed any more: %v", err)
	}
}
