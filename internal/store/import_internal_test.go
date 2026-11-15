package store

import (
	"errors"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/simonhull/kitsune/internal/rating"
	"github.com/simonhull/kitsune/internal/smartlist"
)

// importKeys are the tag keys the import tests keep MusicBrainz IDs under.
var importKeys = MBIDKeys{ReleaseTrack: []string{"MUSICBRAINZ_RELEASETRACKID"}, Recording: []string{mbidKey}} //nolint:gochecknoglobals // Read-only fixture shared by the tests below.

// The sources the import tests' plays come from.
const (
	srcDated   = "navidrome"
	srcUndated = "navidrome-undated"
)

func at(sec int64) time.Time { return time.Unix(sec, 0).UTC() }

// sampleImport is an import for user 1 with a play of each source, two
// ratings and one rating given for a heart.
func sampleImport() *Import {
	return &Import{
		UserID: 1,
		Plays: []NewPlay{
			{TrackID: trackULID(1), PlayedAt: at(100), Source: srcDated, ClientID: "navidrome:s:1"},
			{TrackID: trackULID(1), PlayedAt: at(99), Source: srcUndated, ClientID: "navidrome:u:m1:1"},
			{TrackID: trackULID(2), PlayedAt: at(50), Source: srcDated, ClientID: "navidrome:d:m2"},
		},
		Ratings: []ImportedRating{
			{TrackID: trackULID(1), Rating: 8, At: at(13)},
			{TrackID: trackULID(2), Rating: 10, At: at(14)},
		},
		HeartRatings: []ImportedRating{{TrackID: trackULID(3), Rating: 8, At: at(10)}},
		Playlists: []ImportedPlaylist{{
			ID: "01PLAYLISTIMPORTED00000001", Key: "navidrome:p1", Name: "Falling Up", CreatedAt: at(5),
			Tracks: []string{trackULID(2), trackULID(1), trackULID(2)},
		}},
	}
}

func mustApply(t *testing.T, s *Store, imp *Import) ImportCounts {
	t.Helper()
	c, err := s.ApplyImport(t.Context(), imp)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

// importedRows lists user 1's plays, hearts and ratings.
func importedRows(t *testing.T, s *Store) []string {
	t.Helper()
	return slices.Concat(
		rawRows(t, s.w, "SELECT 'play', t.ulid, p.played_at, p.source, p.client_id FROM plays p JOIN tracks t ON t.id = p.track_id WHERE p.user_id = 1 ORDER BY p.client_id"),
		rawRows(t, s.w, "SELECT 'track heart', t.ulid, h.hearted_at FROM track_hearts h JOIN tracks t ON t.id = h.track_id WHERE h.user_id = 1 ORDER BY t.ulid"),
		rawRows(t, s.w, "SELECT 'album heart', a.ulid, h.hearted_at FROM album_hearts h JOIN albums a ON a.id = h.album_id WHERE h.user_id = 1"),
		rawRows(t, s.w, "SELECT 'artist heart', a.ulid, h.hearted_at FROM artist_hearts h JOIN artists a ON a.id = h.artist_id WHERE h.user_id = 1"),
		rawRows(t, s.w, "SELECT 'rating', t.ulid, r.halves, r.rated_at FROM track_ratings r JOIN tracks t ON t.id = r.track_id WHERE r.user_id = 1 ORDER BY t.ulid"),
		rawRows(t, s.w, "SELECT 'playlist', ulid, name, source, import_key, created_at FROM playlists WHERE user_id = 1 ORDER BY id"),
		rawRows(t, s.w, `SELECT 'entry', p.import_key, t.ulid FROM playlist_tracks e
			JOIN playlists p ON p.id = e.playlist_id JOIN tracks t ON t.id = e.track_id
			WHERE p.user_id = 1 ORDER BY p.id, e.position`),
	)
}

func TestApplyImportWritesEveryKindOnce(t *testing.T) {
	t.Parallel()
	s, _ := markedStore(t)

	first := mustApply(t, s, sampleImport())
	rows := importedRows(t, s)
	second := mustApply(t, s, sampleImport())

	want := []string{
		"play " + trackULID(2) + " 50000 navidrome navidrome:d:m2",
		"play " + trackULID(1) + " 100000 navidrome navidrome:s:1",
		"play " + trackULID(1) + " 99000 navidrome-undated navidrome:u:m1:1",
		"rating " + trackULID(1) + " 8 13000",
		"rating " + trackULID(2) + " 10 14000",
		"rating " + trackULID(3) + " 8 10000",
		"playlist 01PLAYLISTIMPORTED00000001 Falling Up navidrome navidrome:p1 5000",
		"entry navidrome:p1 " + trackULID(2),
		"entry navidrome:p1 " + trackULID(1),
		"entry navidrome:p1 " + trackULID(2),
	}
	if !slices.Equal(rows, want) {
		t.Errorf("rows =\n%v\nwant\n%v: no hearts, ever", rows, want)
	}
	wantFirst := ImportCounts{
		Plays:   map[string]ImportCount{srcDated: {Added: 2}, srcUndated: {Added: 1}},
		Ratings: ImportCount{Added: 2}, HeartRatings: ImportCount{Added: 1},
		Playlists: ImportCount{Added: 1}, PlaylistSongs: ImportCount{Added: 3},
	}
	if !reflect.DeepEqual(first, wantFirst) {
		t.Errorf("first run = %+v, want %+v", first, wantFirst)
	}
	wantSecond := ImportCounts{
		Plays:   map[string]ImportCount{srcDated: {There: 2}, srcUndated: {There: 1}},
		Ratings: ImportCount{There: 2}, HeartRatings: ImportCount{There: 1},
		Playlists: ImportCount{There: 1}, PlaylistSongs: ImportCount{There: 3},
	}
	if !reflect.DeepEqual(second, wantSecond) {
		t.Errorf("second run = %+v, want %+v", second, wantSecond)
	}
	if again := importedRows(t, s); !slices.Equal(again, rows) {
		t.Errorf("a second run changed rows:\n%v", again)
	}
}

// A playlist imported once and changed here since stays as it is: a re-run
// neither renames it nor adds to it.
func TestApplyImportKeepsAnImportedPlaylistAsItIsNow(t *testing.T) {
	t.Parallel()
	s, _ := markedStore(t)
	mustApply(t, s, sampleImport())
	if err := s.RenamePlaylist(t.Context(), 1, "01PLAYLISTIMPORTED00000001", "Falling Down"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.RemoveFromPlaylist(t.Context(), 1, "01PLAYLISTIMPORTED00000001", []int64{1}); err != nil {
		t.Fatal(err)
	}
	rows := importedRows(t, s)
	again := sampleImport()
	again.Playlists[0].ID = "01PLAYLISTIMPORTED00000002" // A new run makes new IDs.
	c := mustApply(t, s, again)
	if c.Playlists != (ImportCount{There: 1}) {
		t.Errorf("playlists = %+v, want the one already there", c.Playlists)
	}
	got := importedRows(t, s)
	if !slices.Equal(got, rows) || !slices.Contains(got, "playlist 01PLAYLISTIMPORTED00000001 Falling Down navidrome navidrome:p1 5000") {
		t.Errorf("rows after a re-run =\n%v\nwant\n%v", got, rows)
	}
}

// A smart playlist comes over with its rules and a seed, never an entry; a
// re-run adds nothing, and queues nothing for Last.fm.
func TestApplyImportWritesASmartPlaylistOnce(t *testing.T) {
	t.Parallel()
	s, _ := lastfmStore(t) // Scrobbles and loves on.
	rules, err := smartlist.Parse([]byte(`{"match":"all","rules":[{"field":"rating","op":"atLeast","value":8}],"limit":{"count":20,"unit":"songs","by":"random"},"order":"chosen"}`))
	if err != nil {
		t.Fatal(err)
	}
	imp := sampleImport()
	imp.Playlists = append(imp.Playlists, ImportedPlaylist{
		ID: "01PLAYLISTIMPORTED00000009", Key: "navidrome:p9", Name: "Loved", CreatedAt: at(6), Tracks: []string{}, Rules: &rules,
	})
	first := mustApply(t, s, imp)
	const smartRows = `SELECT ulid, rules, seed IS NOT NULL AND seed >= 0 AND seed < 9007199254740992,
		(SELECT count(*) FROM playlist_tracks e WHERE e.playlist_id = p.id) FROM playlists p WHERE rules IS NOT NULL`
	want := []string{`01PLAYLISTIMPORTED00000009 {"match":"all","rules":[{"field":"rating","op":"atLeast","value":8}],` +
		`"limit":{"count":20,"unit":"songs","by":"random"},"order":"chosen"} 1 0`}
	if got := rawRows(t, s.w, smartRows); !slices.Equal(got, want) {
		t.Errorf("smart playlists = %v, want %v", got, want)
	}
	imp.Playlists[1].ID = "01PLAYLISTIMPORTED00000010" // A new run makes new IDs.
	second := mustApply(t, s, imp)
	if first.Playlists != (ImportCount{Added: 2}) || second.Playlists != (ImportCount{There: 2}) {
		t.Errorf("playlists = %+v then %+v, want 2 added, then 2 there", first.Playlists, second.Playlists)
	}
	if got := rawRows(t, s.w, smartRows); !slices.Equal(got, want) {
		t.Errorf("after a re-run, smart playlists = %v, want %v", got, want)
	}
	if got := outbox(t, s); len(got) != 0 {
		t.Errorf("outbox = %v, want nothing queued", got)
	}
}

func TestApplyImportKeepsTheirRating(t *testing.T) {
	t.Parallel()
	s, now := markedStore(t)
	*now = at(20)
	mustRate(t, s, 1, trackULID(1), 3)
	mustRate(t, s, 1, trackULID(3), 2)

	c := mustApply(t, s, sampleImport())

	rows := importedRows(t, s)
	for _, want := range []string{
		"rating " + trackULID(1) + " 3 20000",
		"rating " + trackULID(2) + " 10 14000",
		"rating " + trackULID(3) + " 2 20000",
	} {
		if !slices.Contains(rows, want) {
			t.Errorf("rows lack %q:\n%v", want, rows)
		}
	}
	if c.Ratings != (ImportCount{Added: 1, Kept: 1}) || c.HeartRatings != (ImportCount{Kept: 1}) {
		t.Errorf("counts = %+v", c)
	}
}

func TestApplyImportQueuesNothingForLastfm(t *testing.T) {
	t.Parallel()
	s, _ := lastfmStore(t) // Scrobbles and loves on.
	mustApply(t, s, sampleImport())
	if got := outbox(t, s); len(got) != 0 {
		t.Errorf("lastfm_outbox = %v, want empty", got)
	}
	// What would queue does still queue, so the check above means something.
	mustRate(t, s, 1, trackULID(4), loveLine)
	if got := outbox(t, s); len(got) != 1 {
		t.Errorf("lastfm_outbox after a rating = %v, want one love", got)
	}
}

func TestApplyImportIsAllOrNothing(t *testing.T) {
	t.Parallel()
	everything, parseErr := smartlist.Parse([]byte(`{"match":"all","rules":[]}`))
	if parseErr != nil {
		t.Fatal(parseErr)
	}
	tests := []struct {
		name    string
		spoil   func(*Import)
		wantErr error
	}{
		{"a track gone", func(imp *Import) { imp.Ratings[1].TrackID = trackULID(99) }, ErrNotFound},
		{"a heart's track gone", func(imp *Import) { imp.HeartRatings[0].TrackID = trackULID(98) }, ErrNotFound},
		{"no rating", func(imp *Import) { imp.Ratings[1].Rating = rating.None }, rating.ErrBad},
		{"a playlist's track gone", func(imp *Import) { imp.Playlists[0].Tracks[2] = trackULID(97) }, ErrNotFound},
		{"a playlist with no name", func(imp *Import) { imp.Playlists[0].Name = " " }, ErrBadPlaylistName},
		{"a playlist too long", func(imp *Import) {
			imp.Playlists[0].Tracks = slices.Repeat([]string{trackULID(1)}, MostPlaylistEntries+1)
		}, ErrPlaylistFull},
		{"a smart playlist with songs", func(imp *Import) { imp.Playlists[0].Rules = &everything }, smartlist.ErrBad},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			s, _ := markedStore(t)
			imp := sampleImport()
			tt.spoil(imp)
			if _, err := s.ApplyImport(t.Context(), imp); !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
			if rows := importedRows(t, s); len(rows) != 0 {
				t.Errorf("a failed import left rows: %v", rows)
			}
		})
	}
}

func TestImportTargets(t *testing.T) {
	t.Parallel()
	s := seededStore(t)
	const tags = `INSERT INTO track_tags (track_id, key, ord, value) VALUES
		(1, 'MUSICBRAINZ_RELEASETRACKID', 0, 'rt-1'), (1, 'MUSICBRAINZ_TRACKID', 0, 'rec-1'),
		(2, 'MUSICBRAINZ_TRACKID', 0, 'rec-2a'), (2, 'MUSICBRAINZ_TRACKID', 1, 'rec-2b'), (2, 'OTHER', 0, 'x')`
	const artists = `UPDATE artists SET mbid = 'mb-band' WHERE id = 1;
		INSERT INTO artists (id, ulid, match_key, name, sort_name, mbid) VALUES (2, 'A0000000000000000000000002', 'various artists', 'Various Artists', 'Various Artists', NULL),
		(3, 'A0000000000000000000000003', 'va', 'VA', 'VA', '89ad4ac3-39f7-470e-963a-56509c546377')`
	for _, q := range []string{tags, artists, "UPDATE tracks SET missing_since = 1 WHERE id = 3"} {
		if _, err := s.w.ExecContext(t.Context(), q); err != nil {
			t.Fatal(err)
		}
	}

	got, err := s.ImportTargets(t.Context(), importKeys)
	if err != nil {
		t.Fatal(err)
	}

	if len(got.Tracks) != len(seedWords) {
		t.Fatalf("%d tracks, want every one of %d, missing included", len(got.Tracks), len(seedWords))
	}
	wantFirst := ImportTrack{
		ULID: trackULID(1), Root: "/m", RelPath: "alpha.flac", Album: seedAlbum,
		ReleaseTrackMBIDs: []string{"rt-1"}, RecordingMBIDs: []string{"rec-1"},
	}
	if !reflect.DeepEqual(got.Tracks[0], wantFirst) {
		t.Errorf("first track = %+v, want %+v", got.Tracks[0], wantFirst)
	}
	if !slices.Equal(got.Tracks[1].RecordingMBIDs, []string{"rec-2a", "rec-2b"}) || got.Tracks[1].ReleaseTrackMBIDs != nil {
		t.Errorf("second track = %+v", got.Tracks[1])
	}
	wantArtists := []ImportArtist{{ULID: seedArtist, MatchKey: "band", MBID: "mb-band"}}
	if !slices.Equal(got.Artists, wantArtists) {
		t.Errorf("artists = %+v, want %+v: Various Artists, by key or MBID, is nobody's", got.Artists, wantArtists)
	}
}
