package store

import (
	"errors"
	"fmt"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/simonhull/kitsune/internal/rating"
)

// importKeys are the tag keys the import tests keep MusicBrainz IDs under.
var importKeys = MBIDKeys{ReleaseTrack: []string{"MUSICBRAINZ_RELEASETRACKID"}, Recording: []string{mbidKey}} //nolint:gochecknoglobals // Read-only fixture shared by the tests below.

// The sources the import tests' plays come from.
const (
	srcDated   = "navidrome"
	srcUndated = "navidrome-undated"
)

func at(sec int64) time.Time { return time.Unix(sec, 0).UTC() }

// sampleImport is an import for user 1 with a play of each source, a heart of
// each kind and two ratings.
func sampleImport() *Import {
	return &Import{
		UserID: 1,
		Plays: []NewPlay{
			{TrackID: trackULID(1), PlayedAt: at(100), Source: srcDated, ClientID: "navidrome:s:1"},
			{TrackID: trackULID(1), PlayedAt: at(99), Source: srcUndated, ClientID: "navidrome:u:m1:1"},
			{TrackID: trackULID(2), PlayedAt: at(50), Source: srcDated, ClientID: "navidrome:d:m2"},
		},
		TrackHearts:  []ImportedHeart{{ID: trackULID(1), At: at(10)}},
		AlbumHearts:  []ImportedHeart{{ID: seedAlbum, At: at(11)}},
		ArtistHearts: []ImportedHeart{{ID: seedArtist, At: at(12)}},
		Ratings: []ImportedRating{
			{TrackID: trackULID(1), Rating: 8, At: at(13)},
			{TrackID: trackULID(2), Rating: 10, At: at(14)},
		},
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
		"track heart " + trackULID(1) + " 10000",
		"album heart " + seedAlbum + " 11000",
		"artist heart " + seedArtist + " 12000",
		"rating " + trackULID(1) + " 8 13000",
		"rating " + trackULID(2) + " 10 14000",
	}
	if !slices.Equal(rows, want) {
		t.Errorf("rows =\n%v\nwant\n%v", rows, want)
	}
	wantFirst := ImportCounts{
		Plays:       map[string]ImportCount{srcDated: {Added: 2}, srcUndated: {Added: 1}},
		TrackHearts: ImportCount{Added: 1}, AlbumHearts: ImportCount{Added: 1}, ArtistHearts: ImportCount{Added: 1},
		Ratings: ImportCount{Added: 2},
	}
	if !reflect.DeepEqual(first, wantFirst) {
		t.Errorf("first run = %+v, want %+v", first, wantFirst)
	}
	wantSecond := ImportCounts{
		Plays:       map[string]ImportCount{srcDated: {There: 2}, srcUndated: {There: 1}},
		TrackHearts: ImportCount{There: 1}, AlbumHearts: ImportCount{There: 1}, ArtistHearts: ImportCount{There: 1},
		Ratings: ImportCount{There: 2},
	}
	if !reflect.DeepEqual(second, wantSecond) {
		t.Errorf("second run = %+v, want %+v", second, wantSecond)
	}
	if again := importedRows(t, s); !slices.Equal(again, rows) {
		t.Errorf("a second run changed rows:\n%v", again)
	}
}

func TestApplyImportKeepsTheirRatingAndTheFirstHeart(t *testing.T) {
	t.Parallel()
	s, now := markedStore(t)
	*now = at(5) // Before the import's hearts: they stay as they are.
	mustHeart(t, s, 1, KindTrack, trackULID(1))
	*now = at(20) // After them: the import's earlier time wins.
	mustHeart(t, s, 1, KindAlbum, seedAlbum)
	mustRate(t, s, 1, trackULID(1), 3)

	c := mustApply(t, s, sampleImport())

	rows := importedRows(t, s)
	for _, want := range []string{
		"track heart " + trackULID(1) + " 5000",
		"album heart " + seedAlbum + " 11000",
		"rating " + trackULID(1) + " 3 20000",
		"rating " + trackULID(2) + " 10 14000",
	} {
		if !slices.Contains(rows, want) {
			t.Errorf("rows lack %q:\n%v", want, rows)
		}
	}
	if c.TrackHearts != (ImportCount{There: 1}) || c.AlbumHearts != (ImportCount{There: 1}) || c.Ratings != (ImportCount{Added: 1, Kept: 1}) {
		t.Errorf("counts = %+v", c)
	}
}

func TestApplyImportBringsBackAHeartTakenOff(t *testing.T) {
	t.Parallel()
	s, _ := markedStore(t)
	mustHeart(t, s, 1, KindTrack, trackULID(1))
	if err := s.Unheart(t.Context(), 1, KindTrack, trackULID(1)); err != nil {
		t.Fatal(err)
	}
	if c := mustApply(t, s, sampleImport()); c.TrackHearts != (ImportCount{Added: 1}) {
		t.Errorf("track hearts = %+v, want one added", c.TrackHearts)
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
	mustHeart(t, s, 1, KindTrack, trackULID(3))
	if got := outbox(t, s); len(got) != 1 {
		t.Errorf("lastfm_outbox after a heart = %v, want one love", got)
	}
}

func TestApplyImportIsAllOrNothing(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		spoil   func(*Import)
		wantErr error
	}{
		{"a track gone", func(imp *Import) { imp.Ratings[1].TrackID = trackULID(99) }, ErrNotFound},
		{"an album gone", func(imp *Import) { imp.AlbumHearts[0].ID = fmt.Sprintf("B%025d", 99) }, ErrNotFound},
		{"no rating", func(imp *Import) { imp.Ratings[1].Rating = rating.None }, rating.ErrBad},
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
