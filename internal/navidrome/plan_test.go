package navidrome_test

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/simonhull/kitsune/internal/navidrome"
	"github.com/simonhull/kitsune/internal/rating"
	"github.com/simonhull/kitsune/internal/store"
	"github.com/simonhull/kitsune/internal/store/navidromedb"
)

const (
	ndUser  = "nd-u1"
	ndName  = "Simon"
	account = int64(7)
	root    = "/music"
	albumA  = "B000000000000000000000000A"
	albumB  = "B000000000000000000000000B"

	kName    = "simon"
	kimName  = "kim"
	pathOne  = "one.flac"
	pathGone = "gone.flac"
	pathY    = "y.flac"
	recMBID  = "rec-1"
	mbShared = "mb-3"
	pathDup  = "dup.flac"
)

var now = time.Date(2026, 11, 12, 0, 0, 0, 0, time.UTC) //nolint:gochecknoglobals // Read-only fixture.

func ts(day, sec int) time.Time { return time.Date(2026, 1, day, 0, 0, sec, 0, time.UTC) }

func track(n byte) string { return "T00000000000000000000000" + string([]byte{'0', n}) }

// world is a Navidrome source and a Kitsune library to plan between.
type world struct {
	songs   []navidromedb.Song
	notes   []navidromedb.Annotation
	scrobs  []navidromedb.Scrobble
	artists []navidromedb.Artist
	tracks  []store.ImportTrack
	kArts   []store.ImportArtist
	opts    navidrome.Options
}

func song(id, path string) navidromedb.Song {
	return navidromedb.Song{ID: id, LibraryID: 1, Path: path, AlbumID: "nd-album"}
}

func ktrack(n byte, path, album string) store.ImportTrack {
	return store.ImportTrack{ULID: track(n), Root: root, RelPath: path, Album: album}
}

func (w *world) plan(t *testing.T) (navidrome.Plan, error) {
	t.Helper()
	src := navidrome.Source{
		Libraries: []navidromedb.Library{{ID: 1, Path: root}},
		Users:     []navidromedb.User{{ID: ndUser, Name: ndName}},
		Songs:     w.songs,
		Marks:     map[string]navidrome.Marks{ndUser: {Annotations: w.notes, Scrobbles: w.scrobs}},
		Artists:   w.artists,
	}
	targets := store.ImportTargets{Tracks: w.tracks, Artists: w.kArts}
	opts := w.opts
	if opts.Accounts == nil {
		opts.Accounts = []navidrome.Account{{ID: account, Name: kName}}
	}
	opts.Now = now
	p, err := navidrome.New(&src, &targets, &opts)
	if err != nil {
		return p, fmt.Errorf("plan: %w", err)
	}
	return p, nil
}

func (w *world) person(t *testing.T) navidrome.Person {
	t.Helper()
	p, err := w.plan(t)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.People) != 1 {
		t.Fatalf("people = %+v, want one", p.People)
	}
	return p.People[0]
}

// starred is a song annotation with a star and nothing else.
func starred(id string) navidromedb.Annotation {
	return navidromedb.Annotation{ItemID: id, ItemType: navidromedb.ItemSong, Starred: true, StarredAt: ts(1, 0)}
}

func TestSongsMatchByPathThenMBID(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		songs     []navidromedb.Song
		tracks    []store.ImportTrack
		opts      navidrome.Options
		wantTrack string // Empty: unmatched.
		wantPath  bool
	}{
		{
			name:  "path",
			songs: []navidromedb.Song{song("s1", "A/one.flac")},
			tracks: []store.ImportTrack{
				ktrack('1', "A/one.flac", albumA),
				{ULID: track('2'), Root: "/elsewhere", RelPath: "A/one.flac", Album: albumA},
			},
			wantTrack: track('1'), wantPath: true,
		},
		{
			name:      "path under a root -root names",
			songs:     []navidromedb.Song{song("s1", "A/one.flac")},
			tracks:    []store.ImportTrack{{ULID: track('2'), Root: "/srv/music", RelPath: "A/one.flac", Album: albumA}},
			opts:      navidrome.Options{Roots: map[string]string{root: "/srv/music"}},
			wantTrack: track('2'), wantPath: true,
		},
		{
			name:      "path -map names",
			songs:     []navidromedb.Song{song("s1", "Old/one.flac")},
			tracks:    []store.ImportTrack{ktrack('3', "New/one.flac", albumA)},
			opts:      navidrome.Options{Paths: map[string]string{"Old/one.flac": "New/one.flac"}},
			wantTrack: track('3'), wantPath: true,
		},
		{
			name:  "release-track MBID, in any case, before recording",
			songs: []navidromedb.Song{{ID: "s1", LibraryID: 1, Path: pathGone, ReleaseTrackMBID: "RT-1", RecordingMBID: recMBID}},
			tracks: []store.ImportTrack{
				{ULID: track('4'), Root: root, RelPath: "x.flac", Album: albumA, ReleaseTrackMBIDs: []string{"rt-1"}},
				{ULID: track('5'), Root: root, RelPath: pathY, Album: albumA, RecordingMBIDs: []string{recMBID}},
			},
			wantTrack: track('4'),
		},
		{
			name:      "recording MBID",
			songs:     []navidromedb.Song{{ID: "s1", LibraryID: 1, Path: pathGone, RecordingMBID: recMBID}},
			tracks:    []store.ImportTrack{{ULID: track('5'), Root: root, RelPath: pathY, Album: albumA, RecordingMBIDs: []string{recMBID, "REC-1"}}},
			wantTrack: track('5'),
		},
		{
			name: "an MBID two Navidrome songs share",
			songs: []navidromedb.Song{
				{ID: "s1", LibraryID: 1, Path: pathGone, RecordingMBID: recMBID},
				{ID: "s2", LibraryID: 1, Path: "also-gone.flac", RecordingMBID: recMBID},
			},
			tracks: []store.ImportTrack{{ULID: track('5'), Root: root, RelPath: pathY, Album: albumA, RecordingMBIDs: []string{recMBID}}},
		},
		{
			name:  "an MBID two Kitsune tracks share",
			songs: []navidromedb.Song{{ID: "s1", LibraryID: 1, Path: pathGone, RecordingMBID: recMBID}},
			tracks: []store.ImportTrack{
				{ULID: track('5'), Root: root, RelPath: pathY, Album: albumA, RecordingMBIDs: []string{recMBID}},
				{ULID: track('6'), Root: root, RelPath: "z.flac", Album: albumA, RecordingMBIDs: []string{recMBID}},
			},
		},
		{
			name:   "nothing",
			songs:  []navidromedb.Song{song("s1", pathGone)},
			tracks: []store.ImportTrack{ktrack('1', "A/one.flac", albumA)},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			opts := tt.opts
			opts.HeartStars = heartStars
			w := world{songs: tt.songs, tracks: tt.tracks, opts: opts, notes: []navidromedb.Annotation{starred("s1")}}
			p := w.person(t)
			r := p.Report.UnratedHearts
			var got string
			if len(p.Import.HeartRatings) > 0 {
				got = p.Import.HeartRatings[0].TrackID
			}
			if got != tt.wantTrack || (r.ByPath == 1) != tt.wantPath || (r.ByMBID == 1) != (tt.wantTrack != "" && !tt.wantPath) {
				t.Errorf("heart on %q, tally %+v; want %q, by path %v", got, r, tt.wantTrack, tt.wantPath)
			}
			if tt.wantTrack == "" && !slices.Equal(p.Report.Unmatched, []string{tt.songs[0].Path}) {
				t.Errorf("unmatched = %v, want the song's path", p.Report.Unmatched)
			}
		})
	}
}

// playsOf lists a person's plays as "track client source time".
func playsOf(p *navidrome.Person) []string {
	out := make([]string, len(p.Import.Plays))
	for i, pl := range p.Import.Plays {
		out[i] = strings.Join([]string{pl.TrackID, pl.ClientID, pl.Source, pl.PlayedAt.Format(time.RFC3339Nano)}, " ")
	}
	return out
}

func TestPlaysAreStampedAsLateAsTheyCanBe(t *testing.T) {
	t.Parallel()
	songs := []navidromedb.Song{song("s1", pathOne), song("s2", "two.flac"), song("s3", "three.flac")}
	tracks := []store.ImportTrack{ktrack('1', pathOne, albumA), ktrack('2', "two.flac", albumA), ktrack('3', "three.flac", albumA)}
	first := ts(10, 0) // The user's first scrobble.
	tests := []struct {
		name   string
		notes  []navidromedb.Annotation
		scrobs []navidromedb.Scrobble
		want   []string
	}{
		{
			name: "scrobbles, and a surplus before the first",
			notes: []navidromedb.Annotation{
				{ItemID: "s1", ItemType: navidromedb.ItemSong, PlayCount: 4, PlayDate: ts(12, 0)},
			},
			scrobs: []navidromedb.Scrobble{
				{ID: 31, SongID: "s1", PlayedAt: ts(12, 0)},
				{ID: 30, SongID: "s1", PlayedAt: ts(11, 0)},
				{ID: 20, SongID: "s2", PlayedAt: first}, // Another song's: it sets the first.
			},
			want: []string{
				track('1') + " navidrome:s:31 navidrome 2026-01-12T00:00:00Z",
				track('1') + " navidrome:s:30 navidrome 2026-01-11T00:00:00Z",
				track('1') + " navidrome:u:s1:1 navidrome-undated 2026-01-09T23:59:59.999Z",
				track('1') + " navidrome:u:s1:2 navidrome-undated 2026-01-09T23:59:59.999Z",
				// A scrobble of a song with no annotation still counts.
				track('2') + " navidrome:s:20 navidrome 2026-01-10T00:00:00Z",
			},
		},
		{
			name: "no scrobbles: the last play at play_date, the rest there too",
			notes: []navidromedb.Annotation{
				{ItemID: "s3", ItemType: navidromedb.ItemSong, PlayCount: 3, PlayDate: ts(5, 7)},
				{ItemID: "s2", ItemType: navidromedb.ItemSong, PlayCount: 1, PlayDate: ts(4, 0)},
				{ItemID: "s1", ItemType: navidromedb.ItemSong, Starred: true, StarredAt: ts(1, 0)}, // No plays.
			},
			want: []string{
				track('2') + " navidrome:d:s2 navidrome 2026-01-04T00:00:00Z",
				track('3') + " navidrome:d:s3 navidrome 2026-01-05T00:00:07Z",
				track('3') + " navidrome:u:s3:1 navidrome-undated 2026-01-05T00:00:07Z",
				track('3') + " navidrome:u:s3:2 navidrome-undated 2026-01-05T00:00:07Z",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			w := world{songs: songs, tracks: tracks, notes: tt.notes, scrobs: tt.scrobs}
			p := w.person(t)
			if got := playsOf(&p); !slices.Equal(got, tt.want) {
				t.Errorf("plays =\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(tt.want, "\n"))
			}
			for _, pl := range p.Import.Plays {
				if pl.UserID != account {
					t.Errorf("play for user %d, want %d", pl.UserID, account)
				}
			}
			checkLatest(t, &p, tt.notes)
		})
	}
}

// checkLatest checks that each played song's latest play is its play_date,
// which is what recency reads. Song sN is track N.
func checkLatest(t *testing.T, p *navidrome.Person, notes []navidromedb.Annotation) {
	t.Helper()
	latest := map[string]time.Time{}
	for _, pl := range p.Import.Plays {
		if pl.PlayedAt.After(latest[pl.TrackID]) {
			latest[pl.TrackID] = pl.PlayedAt
		}
	}
	for i := range notes {
		if a := &notes[i]; a.PlayCount > 0 && !latest[track(a.ItemID[1])].Equal(a.PlayDate) {
			t.Errorf("%s: latest play %v, want play_date %v", a.ItemID, latest[track(a.ItemID[1])], a.PlayDate)
		}
	}
}

func TestPlayCountsAddUp(t *testing.T) {
	t.Parallel()
	w := world{
		songs:  []navidromedb.Song{song("s1", pathOne), song("s2", pathGone)},
		tracks: []store.ImportTrack{ktrack('1', pathOne, albumA)},
		notes: []navidromedb.Annotation{
			{ItemID: "s1", ItemType: navidromedb.ItemSong, PlayCount: 3, PlayDate: ts(3, 0)},
			{ItemID: "s2", ItemType: navidromedb.ItemSong, PlayCount: 2, PlayDate: ts(2, 0)},
		},
	}
	r := w.person(t).Report
	if r.Plays != (navidrome.Tally{Source: 5, ByPath: 3, Unmatched: 2}) ||
		r.Dated != (navidrome.Tally{Source: 2, ByPath: 1, Unmatched: 1}) ||
		r.Undated != (navidrome.Tally{Source: 3, ByPath: 2, Unmatched: 1}) {
		t.Errorf("plays %+v, dated %+v, undated %+v", r.Plays, r.Dated, r.Undated)
	}
}

func TestRatingsBecomeHalves(t *testing.T) {
	t.Parallel()
	w := world{
		songs:  []navidromedb.Song{song("s1", pathOne), song("s2", "two.flac"), song("s3", pathDup)},
		tracks: []store.ImportTrack{ktrack('1', pathOne, albumA), ktrack('2', "two.flac", albumA)},
		opts:   navidrome.Options{Paths: map[string]string{pathDup: "two.flac"}},
		notes: []navidromedb.Annotation{
			{ItemID: "s1", ItemType: navidromedb.ItemSong, Rating: 4, RatedAt: ts(1, 0)},
			{ItemID: "s2", ItemType: navidromedb.ItemSong, Rating: 1},                    // No rated_at.
			{ItemID: "s3", ItemType: navidromedb.ItemSong, Rating: 5, RatedAt: ts(2, 0)}, // Same track: the dated one is later.
		},
	}
	p := w.person(t)
	want := []store.ImportedRating{
		{TrackID: track('1'), Rating: rating.Rating(8), At: ts(1, 0)},
		{TrackID: track('2'), Rating: rating.Rating(10), At: ts(2, 0)},
	}
	if !slices.Equal(p.Import.Ratings, want) {
		t.Errorf("ratings = %+v, want %+v", p.Import.Ratings, want)
	}
	if p.Report.Ratings != (navidrome.Tally{Source: 3, ByPath: 3}) {
		t.Errorf("tally = %+v", p.Report.Ratings)
	}
}

// heartStars is what the tests rate a starred song with no rating, 4★.
const heartStars rating.Rating = 8

func TestUnratedHeartsBecomeHeartStarsAtTheEarliestStar(t *testing.T) {
	t.Parallel()
	w := world{
		songs:  []navidromedb.Song{song("s1", pathOne), song("s2", pathDup), song("s3", "two.flac"), song("s4", "three.flac")},
		tracks: []store.ImportTrack{ktrack('1', pathOne, albumA), ktrack('2', "two.flac", albumA), ktrack('3', "three.flac", albumA)},
		opts:   navidrome.Options{Paths: map[string]string{pathDup: pathOne}, HeartStars: heartStars},
		notes: []navidromedb.Annotation{
			{ItemID: "s1", ItemType: navidromedb.ItemSong, Starred: true, StarredAt: ts(5, 0)},
			{ItemID: "s2", ItemType: navidromedb.ItemSong, Starred: true, StarredAt: ts(3, 0)},
			{ItemID: "s3", ItemType: navidromedb.ItemSong, Starred: true}, // No starred_at.
			{ItemID: "s4", ItemType: navidromedb.ItemSong, Starred: true, StarredAt: ts(4, 0), Rating: 2, RatedAt: ts(6, 0)},
		},
	}
	p := w.person(t)
	want := []store.ImportedRating{{TrackID: track('1'), Rating: heartStars, At: ts(3, 0)}, {TrackID: track('2'), Rating: heartStars, At: now}}
	if !slices.Equal(p.Import.HeartRatings, want) || p.Report.UnratedHearts != (navidrome.Tally{Source: 3, ByPath: 3}) {
		t.Errorf("heart ratings %+v, tally %+v; want %+v", p.Import.HeartRatings, p.Report.UnratedHearts, want)
	}
	if rated := []store.ImportedRating{{TrackID: track('3'), Rating: 4, At: ts(6, 0)}}; !slices.Equal(p.Import.Ratings, rated) || p.Report.RatedHearts != 1 {
		t.Errorf("ratings %+v, rated hearts %d; want %+v, 1: a starred, rated song keeps just its rating", p.Import.Ratings, p.Report.RatedHearts, rated)
	}

	w.opts.HeartStars = rating.None
	if p = w.person(t); len(p.Import.HeartRatings) != 0 || p.Report.UnratedHearts.Source != 3 {
		t.Errorf("with no heart stars: heart ratings %+v, tally %+v; want none, still counted", p.Import.HeartRatings, p.Report.UnratedHearts)
	}
}

func TestAHeartNeverOutranksARating(t *testing.T) {
	t.Parallel()
	w := world{
		songs:  []navidromedb.Song{song("s1", pathOne), song("s2", pathDup)},
		tracks: []store.ImportTrack{ktrack('1', pathOne, albumA)},
		opts:   navidrome.Options{Paths: map[string]string{pathDup: pathOne}, HeartStars: heartStars},
		notes: []navidromedb.Annotation{
			{ItemID: "s1", ItemType: navidromedb.ItemSong, Starred: true, StarredAt: ts(5, 0)},
			{ItemID: "s2", ItemType: navidromedb.ItemSong, Rating: 1, RatedAt: ts(1, 0)},
		},
	}
	p := w.person(t)
	if len(p.Import.HeartRatings) != 0 || len(p.Import.Ratings) != 1 || p.Import.Ratings[0].Rating != 2 {
		t.Errorf("ratings %+v, heart ratings %+v; want the one rating alone", p.Import.Ratings, p.Import.HeartRatings)
	}
}

func TestAlbumHeartsAreCountedThroughTheirSongs(t *testing.T) {
	t.Parallel()
	songs := []navidromedb.Song{
		{ID: "s1", LibraryID: 1, Path: "a/1.flac", AlbumID: "split"},
		{ID: "s2", LibraryID: 1, Path: "a/2.flac", AlbumID: "split"},
		{ID: "s3", LibraryID: 1, Path: "b/1.flac", AlbumID: "whole"},
		{ID: "s4", LibraryID: 1, Path: "c/1.flac", AlbumID: "lost"},
	}
	tracks := []store.ImportTrack{ktrack('1', "a/1.flac", albumA), ktrack('2', "a/2.flac", albumB), ktrack('3', "b/1.flac", albumA)}
	album := func(id string, at time.Time) navidromedb.Annotation {
		return navidromedb.Annotation{ItemID: id, ItemType: navidromedb.ItemAlbum, Starred: true, StarredAt: at}
	}
	w := world{
		songs: songs, tracks: tracks,
		notes: []navidromedb.Annotation{
			album("lost", ts(1, 0)), album("split", ts(3, 0)), album("whole", ts(2, 0)),
			{ItemID: "rated", ItemType: navidromedb.ItemAlbum, Rating: 3},
		},
	}
	p := w.person(t)
	if len(p.Import.Ratings)+len(p.Import.HeartRatings) != 0 {
		t.Errorf("import = %+v, want nothing: album marks aren't imported", p.Import)
	}
	r := p.Report
	if r.AlbumHearts != (navidrome.Tally{Source: 3, BySong: 2, Unmatched: 1}) || r.SplitAlbums != 1 || r.AlbumRatings != 1 {
		t.Errorf("report: hearts %+v, split %d, album ratings %d", r.AlbumHearts, r.SplitAlbums, r.AlbumRatings)
	}
}

func TestArtistHeartsAreCounted(t *testing.T) {
	t.Parallel()
	artists := []store.ImportArtist{
		{ULID: "A1", MatchKey: "the band", MBID: "mb-1"},
		{ULID: "A2", MatchKey: "other", MBID: "MB-2"},
		{ULID: "A3", MatchKey: "twin", MBID: mbShared},
		{ULID: "A4", MatchKey: "twin too", MBID: mbShared},
	}
	tests := []struct {
		name    string
		artist  navidromedb.Artist
		note    navidromedb.Annotation
		tally   navidrome.Tally
		ratings int
	}{
		{
			name: "by name, folded", artist: navidromedb.Artist{ID: "x", Name: "  The  BAND ", MBID: "mb-9"},
			note: navidromedb.Annotation{Starred: true, StarredAt: ts(1, 0)}, tally: navidrome.Tally{Source: 1, ByName: 1},
		},
		{
			name: "by MBID, a rating", artist: navidromedb.Artist{ID: "x", Name: "Renamed", MBID: "mb-2"},
			note: navidromedb.Annotation{Rating: 5, RatedAt: ts(2, 0)}, tally: navidrome.Tally{Source: 1, ByMBID: 1}, ratings: 1,
		},
		{
			name: "an MBID two artists share", artist: navidromedb.Artist{ID: "x", Name: "Nobody", MBID: mbShared},
			note:  navidromedb.Annotation{Starred: true},
			tally: navidrome.Tally{Source: 1, Unmatched: 1},
		},
		{
			// ImportTargets leaves Various Artists out, so nothing can find it.
			name: "Various Artists", artist: navidromedb.Artist{ID: "x", Name: "Various Artists", MBID: "89ad4ac3-39f7-470e-963a-56509c546377"},
			note:  navidromedb.Annotation{Starred: true},
			tally: navidrome.Tally{Source: 1, Unmatched: 1},
		},
		{
			name: "neither starred nor rated", artist: navidromedb.Artist{ID: "x", Name: "The Band"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			note := tt.note
			note.ItemID, note.ItemType = "x", navidromedb.ItemArtist
			w := world{kArts: artists, artists: []navidromedb.Artist{tt.artist}, notes: []navidromedb.Annotation{note}}
			p := w.person(t)
			if p.Report.ArtistHearts != tt.tally || p.Report.ArtistRatings != tt.ratings || len(p.Import.Ratings) != 0 {
				t.Errorf("tally %+v, artist ratings %d, import %+v; want %+v, %d, nothing", p.Report.ArtistHearts, p.Report.ArtistRatings, p.Import, tt.tally, tt.ratings)
			}
		})
	}
}

func TestUsers(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name         string
		accounts     []navidrome.Account
		users        map[string]string
		wantTo       []string
		wantUnmapped []string
		wantErr      error
	}{
		{name: "by name, any ASCII case", accounts: []navidrome.Account{{ID: 1, Name: "SIMON"}}, wantTo: []string{"SIMON"}},
		{name: "no account", accounts: []navidrome.Account{{ID: 1, Name: kimName}}, wantUnmapped: []string{ndName}},
		{name: "-user", accounts: []navidrome.Account{{ID: 1, Name: kimName}}, users: map[string]string{ndName: "Kim"}, wantTo: []string{kimName}},
		{name: "-user to nobody", accounts: []navidrome.Account{{ID: 1, Name: kimName}}, users: map[string]string{ndName: "lee"}, wantErr: navidrome.ErrNoAccount},
		{name: "-user from nobody", accounts: []navidrome.Account{{ID: 1, Name: kName}}, users: map[string]string{"Lee": kName}, wantErr: navidrome.ErrNoUser},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			w := world{opts: navidrome.Options{Accounts: tt.accounts, Users: tt.users}}
			p, err := w.plan(t)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
			var to []string
			for _, person := range p.People {
				to = append(to, person.To)
			}
			if !slices.Equal(to, tt.wantTo) || !slices.Equal(p.Unmapped, tt.wantUnmapped) {
				t.Errorf("to %v, unmapped %v; want %v, %v", to, p.Unmapped, tt.wantTo, tt.wantUnmapped)
			}
		})
	}
}

func TestOptionsThatNameNothingAreRefused(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		opts navidrome.Options
		want error
	}{
		{name: "-map from no song", opts: navidrome.Options{Paths: map[string]string{"nope.flac": pathOne}}, want: navidrome.ErrMapFrom},
		{name: "-map to no track", opts: navidrome.Options{Paths: map[string]string{pathOne: "nope.flac"}}, want: navidrome.ErrMapTo},
		{name: "-root from no library", opts: navidrome.Options{Roots: map[string]string{"/nope": root}}, want: navidrome.ErrRoot},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			w := world{songs: []navidromedb.Song{song("s1", pathOne)}, tracks: []store.ImportTrack{ktrack('1', pathOne, albumA)}, opts: tt.opts}
			if _, err := w.plan(t); !errors.Is(err, tt.want) {
				t.Errorf("err = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestGate(t *testing.T) {
	t.Parallel()
	songs := []navidromedb.Song{song("s1", "1.flac"), song("s2", "2.flac"), song("s3", "3.flac"), song("s4", "4.flac")}
	notes := []navidromedb.Annotation{starred("s1"), starred("s2"), starred("s3"), starred("s4")}
	tests := []struct {
		name    string
		tracks  []store.ImportTrack
		wantErr bool
	}{
		{name: "half", tracks: []store.ImportTrack{ktrack('1', "1.flac", albumA), ktrack('2', "2.flac", albumA)}},
		{
			name: "under half, MBIDs aside", wantErr: true,
			tracks: []store.ImportTrack{ktrack('1', "1.flac", albumA)},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			w := world{songs: songs, notes: notes, tracks: tt.tracks}
			p, err := w.plan(t)
			if err != nil {
				t.Fatal(err)
			}
			if err = p.Gate.Check(); (err != nil) != tt.wantErr || (err != nil && !errors.Is(err, navidrome.ErrGate)) {
				t.Errorf("Check = %v (gate %+v)", err, p.Gate)
			}
		})
	}
}
