package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/simonhull/kitsune/internal/rating"
	"github.com/simonhull/kitsune/internal/store/sqlcdb"
)

// MBIDKeys are the raw tag keys that hold a track's MusicBrainz IDs, as the
// store keeps them. Package library says which they are.
type MBIDKeys struct {
	ReleaseTrack []string // The track on its release.
	Recording    []string // The recording, on any release.
}

// ImportTrack is a track as an import resolves another server's songs against
// it. A track missing from disk is one too: it keeps its marks.
type ImportTrack struct {
	ULID          string
	Root, RelPath string
	Album         string // The album's ULID.
	// ReleaseTrackMBIDs and RecordingMBIDs are the IDs its tags hold, as tagged.
	ReleaseTrackMBIDs, RecordingMBIDs []string
}

// ImportArtist is an artist as an import resolves another server's artists
// against it.
type ImportArtist struct {
	ULID     string
	MatchKey string
	MBID     string // Empty when unknown.
}

// ImportTargets is what an import resolves another server's items against:
// every track and every artist but Various Artists, which nobody hearts.
type ImportTargets struct {
	Tracks  []ImportTrack
	Artists []ImportArtist
}

// ImportTargets reads every track with its MusicBrainz IDs, under keys, and
// every artist, in one read.
func (s *Store) ImportTargets(ctx context.Context, keys MBIDKeys) (ImportTargets, error) {
	tx, err := s.r.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return ImportTargets{}, fmt.Errorf("import targets: begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }() // Nothing was written: there is nothing to undo.
	q := s.rq.WithTx(tx)
	tracks, err := importTracks(ctx, q, keys)
	if err != nil {
		return ImportTargets{}, fmt.Errorf("import targets: %w", err)
	}
	artists, err := importArtists(ctx, q)
	if err != nil {
		return ImportTargets{}, fmt.Errorf("import targets: %w", err)
	}
	return ImportTargets{Tracks: tracks, Artists: artists}, nil
}

// importTracks reads every track with the MusicBrainz IDs its tags hold.
func importTracks(ctx context.Context, q *sqlcdb.Queries, keys MBIDKeys) ([]ImportTrack, error) {
	rows, err := q.ImportTracks(ctx)
	if err != nil {
		return nil, fmt.Errorf("tracks: %w", err)
	}
	tracks := make([]ImportTrack, len(rows))
	at := make(map[string]int, len(rows))
	for i, t := range rows {
		tracks[i] = ImportTrack{ULID: t.Ulid, Root: t.Root, RelPath: t.RelPath, Album: t.Album}
		at[t.Ulid] = i
	}
	tags, err := q.ImportTrackTags(ctx, slices.Concat(keys.ReleaseTrack, keys.Recording))
	if err != nil {
		return nil, fmt.Errorf("tags: %w", err)
	}
	for _, tag := range tags {
		i, ok := at[tag.Ulid]
		if !ok {
			return nil, errNoTrack
		}
		t := &tracks[i]
		if slices.Contains(keys.ReleaseTrack, tag.Key) {
			t.ReleaseTrackMBIDs = append(t.ReleaseTrackMBIDs, tag.Value)
		}
		if slices.Contains(keys.Recording, tag.Key) {
			t.RecordingMBIDs = append(t.RecordingMBIDs, tag.Value)
		}
	}
	return tracks, nil
}

// importArtists reads every artist but Various Artists.
func importArtists(ctx context.Context, q *sqlcdb.Queries) ([]ImportArtist, error) {
	rows, err := q.ImportArtists(ctx)
	if err != nil {
		return nil, fmt.Errorf("artists: %w", err)
	}
	var artists []ImportArtist
	for _, a := range rows {
		if isVarious(a.MatchKey, a.Mbid) {
			continue
		}
		var mbid string
		if a.Mbid != nil {
			mbid = *a.Mbid
		}
		artists = append(artists, ImportArtist{ULID: a.Ulid, MatchKey: a.MatchKey, MBID: mbid})
	}
	return artists, nil
}

// ImportedHeart is a heart from another server, on the item with the ULID ID.
type ImportedHeart struct {
	ID string
	At time.Time
}

// ImportedRating is a rating from another server, of the track with the ULID
// TrackID.
type ImportedRating struct {
	TrackID string
	Rating  rating.Rating
	At      time.Time
}

// Import is one person's resolved import, everything by ULID. Plays carry the
// client IDs and sources the importer chose; their UserID is ignored.
type Import struct {
	UserID       int64
	Plays        []NewPlay
	TrackHearts  []ImportedHeart
	AlbumHearts  []ImportedHeart
	ArtistHearts []ImportedHeart
	Ratings      []ImportedRating
}

// ImportCount says what became of one kind of an import's rows.
type ImportCount struct {
	Added int // Written now.
	There int // Already there, from an earlier run or the person's own.
	Kept  int // Ratings only: the person's own rating, different, left in place.
}

// ImportCounts says what became of each kind of an import's rows. Plays are
// counted by source.
type ImportCounts struct {
	Plays        map[string]ImportCount
	TrackHearts  ImportCount
	AlbumHearts  ImportCount
	ArtistHearts ImportCount
	Ratings      ImportCount
}

// ApplyImport writes imp in one transaction, so a failure leaves the person's
// rows as they were. It queues nothing for Last.fm: the other server sent its
// plays and loves there already.
//
// Plays already there, by client ID, are left alone. A heart already there
// keeps the earlier of its two times, since a heart's time is when it was
// first given. A rating already there is kept, since it is the newer opinion.
//
// It returns an error wrapping [ErrNotFound] if an item's ULID is no longer in
// the library, and [rating.ErrBad] for a rating that isn't one.
func (s *Store) ApplyImport(ctx context.Context, imp *Import) (ImportCounts, error) {
	for _, r := range imp.Ratings {
		if !r.Rating.Valid() {
			return ImportCounts{}, fmt.Errorf("apply import: track %s: %w", r.TrackID, rating.ErrBad)
		}
	}
	var c ImportCounts
	err := s.inPreparedTx(ctx, func(q *sqlcdb.Queries, _ *sql.Tx) error {
		c = ImportCounts{Plays: map[string]ImportCount{}} // The writer may retry a transaction.
		w := importWriter{q: q, user: imp.UserID}
		if err := w.plays(ctx, imp.Plays, c.Plays); err != nil {
			return err
		}
		for _, h := range []struct {
			kind   Kind
			hearts []ImportedHeart
			count  *ImportCount
		}{
			{KindTrack, imp.TrackHearts, &c.TrackHearts},
			{KindAlbum, imp.AlbumHearts, &c.AlbumHearts},
			{KindArtist, imp.ArtistHearts, &c.ArtistHearts},
		} {
			if err := w.hearts(ctx, h.kind, h.hearts, h.count); err != nil {
				return err
			}
		}
		return w.ratings(ctx, imp.Ratings, &c.Ratings)
	})
	if err != nil {
		return ImportCounts{}, fmt.Errorf("apply import: %w", err)
	}
	return c, nil
}

// importWriter writes one person's import inside a transaction.
type importWriter struct {
	q    *sqlcdb.Queries
	user int64
}

// plays inserts plays, counting them by source.
func (w importWriter) plays(ctx context.Context, plays []NewPlay, counts map[string]ImportCount) error {
	for i := range plays {
		p := &plays[i]
		trackID, err := rowIDOf(ctx, w.q, KindTrack, p.TrackID)
		if err != nil {
			return fmt.Errorf("play of track %s: %w", p.TrackID, err)
		}
		n, err := w.q.InsertPlay(ctx, sqlcdb.InsertPlayParams{
			UserID: w.user, TrackID: trackID, PlayedAt: ms(p.PlayedAt), Source: p.Source, ClientID: p.ClientID,
		})
		if err != nil {
			return fmt.Errorf("play of track %s: %w", p.TrackID, err)
		}
		c := counts[p.Source]
		c.count(n)
		counts[p.Source] = c
	}
	return nil
}

// hearts inserts hearts of kind, moving an existing heart's time earlier.
func (w importWriter) hearts(ctx context.Context, kind Kind, hearts []ImportedHeart, c *ImportCount) error {
	for _, h := range hearts {
		rowID, err := rowIDOf(ctx, w.q, kind, h.ID)
		if err != nil {
			return fmt.Errorf("heart on %s %s: %w", kind, h.ID, err)
		}
		n, err := w.heart(ctx, kind, rowID, ms(h.At))
		if err != nil {
			return fmt.Errorf("heart on %s %s: %w", kind, h.ID, err)
		}
		c.count(n)
	}
	return nil
}

// heart inserts one heart of kind on the row rowID, and returns 1 if it was
// new. One already there gets the time at if that is earlier.
func (w importWriter) heart(ctx context.Context, kind Kind, rowID, at int64) (int64, error) {
	n, err := w.insertHeart(ctx, kind, rowID, at)
	if err != nil || n > 0 {
		return n, err
	}
	return 0, w.earlierHeart(ctx, kind, rowID, at)
}

// insertHeart inserts a heart of kind where there is none, and returns 1 if it did.
func (w importWriter) insertHeart(ctx context.Context, kind Kind, rowID, at int64) (int64, error) {
	switch kind {
	case KindTrack:
		return w.q.InsertTrackHeart(ctx, sqlcdb.InsertTrackHeartParams{UserID: w.user, TrackID: rowID, HeartedAt: at}) //nolint:wrapcheck // Wrapped once, by hearts.
	case KindAlbum:
		return w.q.ImportAlbumHeart(ctx, sqlcdb.ImportAlbumHeartParams{UserID: w.user, AlbumID: rowID, HeartedAt: at}) //nolint:wrapcheck // Wrapped once, by hearts.
	case KindArtist:
		return w.q.ImportArtistHeart(ctx, sqlcdb.ImportArtistHeartParams{UserID: w.user, ArtistID: rowID, HeartedAt: at}) //nolint:wrapcheck // Wrapped once, by hearts.
	}
	return 0, errBadKind
}

// earlierHeart moves the time of a heart of kind to at, if that is earlier.
func (w importWriter) earlierHeart(ctx context.Context, kind Kind, rowID, at int64) error {
	switch kind {
	case KindTrack:
		return w.q.EarlierTrackHeart(ctx, sqlcdb.EarlierTrackHeartParams{HeartedAt: at, UserID: w.user, TrackID: rowID}) //nolint:wrapcheck // Wrapped once, by hearts.
	case KindAlbum:
		return w.q.EarlierAlbumHeart(ctx, sqlcdb.EarlierAlbumHeartParams{HeartedAt: at, UserID: w.user, AlbumID: rowID}) //nolint:wrapcheck // Wrapped once, by hearts.
	case KindArtist:
		return w.q.EarlierArtistHeart(ctx, sqlcdb.EarlierArtistHeartParams{HeartedAt: at, UserID: w.user, ArtistID: rowID}) //nolint:wrapcheck // Wrapped once, by hearts.
	}
	return errBadKind
}

// ratings inserts ratings where the track has none, telling one an earlier
// run wrote from the person's own.
func (w importWriter) ratings(ctx context.Context, ratings []ImportedRating, c *ImportCount) error {
	for _, r := range ratings {
		rowID, err := rowIDOf(ctx, w.q, KindTrack, r.TrackID)
		if err != nil {
			return fmt.Errorf("rating of track %s: %w", r.TrackID, err)
		}
		n, err := w.q.ImportTrackRating(ctx, sqlcdb.ImportTrackRatingParams{UserID: w.user, TrackID: rowID, Halves: r.Rating.Halves(), RatedAt: ms(r.At)})
		if err != nil {
			return fmt.Errorf("rating of track %s: %w", r.TrackID, err)
		}
		if n > 0 {
			c.Added++
			continue
		}
		halves, err := w.q.TrackRatingHalves(ctx, sqlcdb.TrackRatingHalvesParams{UserID: w.user, TrackID: rowID})
		if err != nil {
			return fmt.Errorf("rating of track %s: %w", r.TrackID, err)
		}
		if halves == r.Rating.Halves() {
			c.There++
		} else {
			c.Kept++
		}
	}
	return nil
}

// count counts a row an insert wrote (n is 1) or found there (n is 0).
func (c *ImportCount) count(n int64) {
	if n > 0 {
		c.Added++
	} else {
		c.There++
	}
}

// errNoTrack would mean a tag named a track the same read didn't: impossible
// inside one transaction, but checked rather than trusted.
var errNoTrack = errors.New("tag of a track the read did not return")
