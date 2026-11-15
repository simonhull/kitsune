package store

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/binary"
	"encoding/json/v2"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/simonhull/kitsune/internal/rating"
	"github.com/simonhull/kitsune/internal/smartlist"
	"github.com/simonhull/kitsune/internal/store/sqlcdb"
)

// MBIDKeys are the raw tag keys that hold a track's MusicBrainz IDs, as the
// store keeps them. Package library says which they are.
type MBIDKeys struct {
	ReleaseTrack []string // The track on its release.
	Recording    []string // The recording, on any release.
}

// ImportTrack is a track as an import resolves another server's songs against
// it. A track missing from disk is one too: it keeps its ratings.
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
// every track and every artist but Various Artists, which nobody rates.
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
	UserID  int64
	Plays   []NewPlay
	Ratings []ImportedRating
	// HeartRatings are ratings the importer gave songs for a heart. They are
	// written as Ratings are, after them, and counted apart.
	HeartRatings []ImportedRating
	// Playlists are written after the ratings, each whole or, when the person
	// has one with its Key already, not at all.
	Playlists []ImportedPlaylist
}

// ImportedPlaylist is a playlist from another server.
type ImportedPlaylist struct {
	ID   string // The ULID it takes if it is new.
	Key  string // Where it came from, such as navidrome:<its ID>: a re-run finds it by this.
	Name string // It must pass [PlaylistName].
	// CreatedAt is when the other server made it.
	CreatedAt time.Time
	// Tracks are its songs by ULID, in order, a song as often as it is in it:
	// at most [MostPlaylistEntries].
	Tracks []string
	// Rules make it a smart playlist, which holds no Tracks; nil for a
	// regular one.
	Rules *smartlist.Rules
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
	Ratings      ImportCount
	HeartRatings ImportCount
	// Playlists counts playlists made now and those already here, which are
	// left as they are; PlaylistSongs counts their entries the same way.
	Playlists, PlaylistSongs ImportCount
}

// ApplyImport writes imp in one transaction, so a failure leaves the person's
// rows as they were. It queues nothing for Last.fm: the other server sent its
// plays and loves there already.
//
// Plays already there, by client ID, are left alone. A rating already there
// is kept, since it is the newer opinion.
//
// A playlist the person has with the same key already is left as it is, a
// rename and every change since included.
//
// It returns an error wrapping [ErrNotFound] if an item's ULID is no longer in
// the library, [rating.ErrBad] for a rating that isn't one,
// [ErrBadPlaylistName] for a playlist's name that isn't one,
// [ErrPlaylistFull] for one with too many songs and [smartlist.ErrBad] for a
// smart one that names songs.
func (s *Store) ApplyImport(ctx context.Context, imp *Import) (ImportCounts, error) {
	if err := checkImport(imp); err != nil {
		return ImportCounts{}, fmt.Errorf("apply import: %w", err)
	}
	var c ImportCounts
	err := s.inPreparedTx(ctx, func(q *sqlcdb.Queries, _ *sql.Tx) error {
		c = ImportCounts{Plays: map[string]ImportCount{}} // The writer may retry a transaction.
		w := importWriter{q: q, user: imp.UserID, now: ms(s.now())}
		if err := w.plays(ctx, imp.Plays, c.Plays); err != nil {
			return err
		}
		if err := w.ratings(ctx, imp.Ratings, &c.Ratings); err != nil {
			return err
		}
		if err := w.ratings(ctx, imp.HeartRatings, &c.HeartRatings); err != nil {
			return err
		}
		return w.playlists(ctx, imp.Playlists, &c)
	})
	if err != nil {
		return ImportCounts{}, fmt.Errorf("apply import: %w", err)
	}
	return c, nil
}

// checkImport refuses an import whose ratings or playlists aren't ones.
func checkImport(imp *Import) error {
	for _, r := range slices.Concat(imp.Ratings, imp.HeartRatings) {
		if !r.Rating.Valid() {
			return fmt.Errorf("track %s: %w", r.TrackID, rating.ErrBad)
		}
	}
	for i := range imp.Playlists {
		p := &imp.Playlists[i]
		if name, err := PlaylistName(p.Name); err != nil || name != p.Name {
			return fmt.Errorf("playlist %s: %w", p.Key, ErrBadPlaylistName)
		}
		if len(p.Tracks) > MostPlaylistEntries {
			return fmt.Errorf("playlist %s: %w", p.Key, ErrPlaylistFull)
		}
		if p.Rules != nil && len(p.Tracks) > 0 {
			return fmt.Errorf("playlist %s: a smart playlist holds no songs: %w", p.Key, smartlist.ErrBad)
		}
	}
	return nil
}

// importWriter writes one person's import inside a transaction.
type importWriter struct {
	q    *sqlcdb.Queries
	user int64
	now  int64 // Unix ms: what a new playlist was last updated at.
}

// playlists makes each playlist the person has none with its key of, with its
// songs spaced as the store spaces them.
func (w importWriter) playlists(ctx context.Context, playlists []ImportedPlaylist, c *ImportCounts) error {
	for i := range playlists {
		p := &playlists[i]
		key := p.Key
		_, err := w.q.PlaylistByImportKey(ctx, sqlcdb.PlaylistByImportKeyParams{UserID: w.user, ImportKey: &key})
		if err == nil {
			c.Playlists.There++
			c.PlaylistSongs.There += len(p.Tracks)
			continue
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("playlist %s: %w", p.Key, err)
		}
		if err = w.playlist(ctx, p); err != nil {
			return fmt.Errorf("playlist %s: %w", p.Key, err)
		}
		c.Playlists.Added++
		c.PlaylistSongs.Added += len(p.Tracks)
	}
	return nil
}

// playlist makes one new playlist with its songs, or a smart one with its
// rules and a seed of its own.
func (w importWriter) playlist(ctx context.Context, p *ImportedPlaylist) error {
	key := p.Key
	params := sqlcdb.InsertPlaylistParams{
		Ulid: p.ID, UserID: w.user, Name: p.Name, Source: PlaylistImported, ImportKey: &key,
		CreatedAt: ms(p.CreatedAt), UpdatedAt: w.now,
	}
	if p.Rules != nil {
		encoded, err := p.Rules.MarshalJSON()
		if err != nil {
			return err //nolint:wrapcheck // It says "encode rules".
		}
		rules := string(encoded)
		seed, err := importSeed()
		if err != nil {
			return err
		}
		params.Rules, params.Seed = &rules, &seed
	}
	row, err := w.q.InsertPlaylist(ctx, params)
	if err != nil {
		return fmt.Errorf("insert: %w", err)
	}
	if len(p.Tracks) == 0 {
		return nil
	}
	pairs := make([][2]int64, len(p.Tracks))
	for i, id := range p.Tracks {
		rowID, rowErr := rowIDOf(ctx, w.q, KindTrack, id)
		if rowErr != nil {
			return fmt.Errorf("track %s: %w", id, rowErr)
		}
		pairs[i] = [2]int64{int64(i+1) * positionStep, rowID}
	}
	encoded, err := json.Marshal(pairs)
	if err != nil {
		return fmt.Errorf("encode entries: %w", err)
	}
	_, err = w.q.InsertPlaylistEntries(ctx, sqlcdb.InsertPlaylistEntriesParams{PlaylistID: row, AddedAt: ms(p.CreatedAt), Entries: string(encoded)})
	if err != nil {
		return fmt.Errorf("insert entries: %w", err)
	}
	return nil
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
		halves, err := w.q.GetTrackRating(ctx, sqlcdb.GetTrackRatingParams{UserID: w.user, TrackID: rowID})
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

// mostImportSeed bounds an imported smart playlist's seed below 2⁵³, which a
// client holds exactly.
const mostImportSeed = 1 << 53

// importSeed draws the seed an imported smart playlist's random choices follow.
func importSeed() (int64, error) {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return 0, fmt.Errorf("seed: %w", err)
	}
	return int64(binary.LittleEndian.Uint64(b[:]) % mostImportSeed), nil
}
