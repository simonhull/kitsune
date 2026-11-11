package store

import (
	"context"
	"database/sql"
	"encoding/json/v2"
	"errors"
	"fmt"
	"time"

	"github.com/simonhull/kitsune/internal/rating"
	"github.com/simonhull/kitsune/internal/store/sqlcdb"
)

// ErrMoved means a shuffle session moved on while it was being read: another
// device took the page.
var ErrMoved = errors.New("shuffle session moved on")

// ShuffleSong is a live song as a person's shuffle sees it. Its IDs are row IDs,
// which never leave the server.
type ShuffleSong struct {
	ID        int64
	AlbumID   int64
	AlbumULID string
	Artists   []int64       // Its own artists, the primary first.
	Rating    rating.Rating // The person's rating, or none.
	Hearted   bool
	Excluded  bool // The person left it out of shuffle.
}

// ShuffleCatalog is every live song as a person's shuffle sees it, and their
// runs, each a list of song row IDs in play order. A run may name a song that
// isn't live.
type ShuffleCatalog struct {
	Songs []ShuffleSong
	Runs  [][]int64
}

// ShuffleCatalog reads every live song with the user's marks and rules on it.
func (s *Store) ShuffleCatalog(ctx context.Context, userID int64) (ShuffleCatalog, error) {
	tracks, err := s.rq.ListShuffleTracks(ctx, userID)
	if err != nil {
		return ShuffleCatalog{}, fmt.Errorf("shuffle songs: %w", err)
	}
	links, err := s.rq.ListShuffleTrackArtists(ctx)
	if err != nil {
		return ShuffleCatalog{}, fmt.Errorf("shuffle artists: %w", err)
	}
	runs, err := s.rq.ListShuffleRuns(ctx, userID)
	if err != nil {
		return ShuffleCatalog{}, fmt.Errorf("shuffle runs: %w", err)
	}
	out := ShuffleCatalog{Songs: shuffleSongs(tracks, links)}
	for i := range runs {
		if i == 0 || runs[i].RunID != runs[i-1].RunID {
			out.Runs = append(out.Runs, nil)
		}
		last := &out.Runs[len(out.Runs)-1]
		*last = append(*last, runs[i].TrackID)
	}
	return out, nil
}

// shuffleSongs joins songs to their artists. Both lists are in track ID order,
// so one walk does it, and every song's artists are cut from one slice.
func shuffleSongs(tracks []sqlcdb.ListShuffleTracksRow, links []sqlcdb.ListShuffleTrackArtistsRow) []ShuffleSong {
	out := make([]ShuffleSong, len(tracks))
	artists := make([]int64, len(links))
	at := 0
	for i := range tracks {
		t := &tracks[i]
		for at < len(links) && links[at].TrackID < t.ID {
			at++
		}
		from := at
		for at < len(links) && links[at].TrackID == t.ID {
			artists[at] = links[at].ArtistID
			at++
		}
		out[i] = ShuffleSong{
			ID: t.ID, AlbumID: t.AlbumID, AlbumULID: t.AlbumUlid, Artists: artists[from:at:at],
			Rating: ratingOf(t.Halves), Hearted: t.Hearted != 0, Excluded: t.Excluded != 0,
		}
	}
	return out
}

// SongListening is what a person's listens and plays say about one song.
type SongListening struct {
	// LastAt is when it last came up, played or skipped.
	LastAt time.Time
	// EarlySkips counts its skips in shuffle, within the early time of the
	// start, since the time asked for.
	EarlySkips int
	// Starts counts its listens in shuffle that were completed or skipped, and
	// Skips those that were skipped.
	Starts, Skips int
	// Listens counts its listens, in shuffle or not, that were completed or
	// skipped, and Heard sums the share of the song each one heard.
	Listens int
	Heard   float64
}

// ShuffleListening counts what the user's listens say of each song they have
// listened to or played, by row ID. A skip counts as early when it came within
// early of the start, and only since since.
func (s *Store) ShuffleListening(ctx context.Context, userID int64, early time.Duration, since time.Time) (map[int64]SongListening, error) {
	rows, err := s.rq.ListShuffleListens(ctx, sqlcdb.ListShuffleListensParams{
		UserID: userID, EarlyMs: early.Milliseconds(), Since: ms(since),
	})
	if err != nil {
		return nil, fmt.Errorf("listening: %w", err)
	}
	plays, err := s.rq.ListShuffleLastPlays(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("last plays: %w", err)
	}
	out := make(map[int64]SongListening, len(rows)+len(plays))
	for i := range rows {
		r := &rows[i]
		out[r.TrackID] = SongListening{
			LastAt: time.UnixMilli(r.LastAt).UTC(), EarlySkips: int(r.EarlySkips),
			Starts: int(r.Starts), Skips: int(r.Skips), Listens: int(r.Listens), Heard: r.Heard,
		}
	}
	for _, p := range plays {
		l := out[p.TrackID]
		if at := time.UnixMilli(p.LastAt).UTC(); at.After(l.LastAt) {
			l.LastAt = at
		}
		out[p.TrackID] = l
	}
	return out, nil
}

// Sitting is one listen a person chose and played through, outside shuffle:
// when, and by whom.
type Sitting struct {
	At     time.Time
	Artist int64 // The song's primary artist's row ID.
}

// ShuffleSittings returns the user's listens outside shuffle that were
// completed, since since, oldest first.
func (s *Store) ShuffleSittings(ctx context.Context, userID int64, since time.Time) ([]Sitting, error) {
	rows, err := s.rq.ListShuffleSittings(ctx, sqlcdb.ListShuffleSittingsParams{UserID: userID, StartedAt: ms(since)})
	if err != nil {
		return nil, fmt.Errorf("sittings: %w", err)
	}
	out := make([]Sitting, len(rows))
	for i := range rows {
		out[i] = Sitting{At: time.UnixMilli(rows[i].StartedAt).UTC(), Artist: rows[i].ArtistID}
	}
	return out, nil
}

// HeartedAlbumRows returns the row IDs of the albums the Library list's Hearted
// filter keeps whole for the user: those they hearted, and those whose artist
// they hearted.
func (s *Store) HeartedAlbumRows(ctx context.Context, userID int64) ([]int64, error) {
	rows, err := s.rq.ListShuffleHeartedAlbums(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("hearted albums: %w", err)
	}
	return rows, nil
}

// AlbumRow returns the row ID of the album with the ULID id, or an error
// wrapping [ErrNotFound].
func (s *Store) AlbumRow(ctx context.Context, id string) (int64, error) {
	row, err := s.rq.AlbumRowID(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, fmt.Errorf("album %s: %w", id, ErrNotFound)
	}
	if err != nil {
		return 0, fmt.Errorf("album %s: %w", id, err)
	}
	return row, nil
}

// ArtistTrackRows returns the row IDs of the songs the artist with the ULID id
// is on, as an artist or an album artist, or an error wrapping [ErrNotFound]
// when there are none.
func (s *Store) ArtistTrackRows(ctx context.Context, id string) ([]int64, error) {
	rows, err := s.rq.ListArtistTrackIDs(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("artist %s: %w", id, err)
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("artist %s: %w", id, ErrNotFound)
	}
	return rows, nil
}

// TrackRows returns the row ID of each of the tracks with the given ULIDs that
// exists, by ULID.
func (s *Store) TrackRows(ctx context.Context, ids []string) (map[string]int64, error) {
	out := make(map[string]int64, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	encoded, err := json.Marshal(ids)
	if err != nil {
		return nil, fmt.Errorf("encode IDs: %w", err)
	}
	rows, err := s.rq.ListTrackRowIDs(ctx, string(encoded))
	if err != nil {
		return nil, fmt.Errorf("track rows: %w", err)
	}
	for _, r := range rows {
		out[r.Ulid] = r.ID
	}
	return out, nil
}

// TrackHits returns what the tracks with the given row IDs show, by row ID. One
// that has gone is left out.
func (s *Store) TrackHits(ctx context.Context, ids []int64) (map[int64]TrackHit, error) {
	return s.trackHitsByRow(ctx, ids)
}

// ArtistLinks returns the artists with the given row IDs, by row ID.
func (s *Store) ArtistLinks(ctx context.Context, ids []int64) (map[int64]ArtistLink, error) {
	out := make(map[int64]ArtistLink, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	encoded, err := json.Marshal(ids)
	if err != nil {
		return nil, fmt.Errorf("encode IDs: %w", err)
	}
	rows, err := s.rq.ListArtistLinksByID(ctx, string(encoded))
	if err != nil {
		return nil, fmt.Errorf("artist links: %w", err)
	}
	for _, r := range rows {
		out[r.ID] = ArtistLink{ID: r.Ulid, Name: r.Name}
	}
	return out, nil
}

// ShuffleSession is one of a person's shuffles, as the store keeps it. Scope and
// State are the shuffle's own, and the store never reads them.
type ShuffleSession struct {
	ID        string // The session's ULID.
	Scope     string
	Mode      string // finite or endless.
	Seed      uint64
	Position  int64 // How many units it has given.
	State     []byte
	CreatedAt time.Time
	UpdatedAt time.Time
}

// keptShuffles is how many shuffle sessions a person keeps: enough to go back to
// one from another device, few enough that nobody's pile grows.
const keptShuffles = 20

// CreateShuffleSession keeps a new shuffle session of the user's, and lets go of
// their oldest beyond the last few.
func (s *Store) CreateShuffleSession(ctx context.Context, userID int64, ss *ShuffleSession) error {
	err := s.inTx(ctx, func(q *sqlcdb.Queries, _ *sql.Tx) error {
		err := q.InsertShuffleSession(ctx, sqlcdb.InsertShuffleSessionParams{
			Ulid: ss.ID, UserID: userID, Scope: ss.Scope, Mode: ss.Mode,
			Position: ss.Position, State: ss.State, CreatedAt: ms(ss.CreatedAt), UpdatedAt: ms(ss.UpdatedAt),
			Seed: int64(ss.Seed), //nolint:gosec // G115: the seed's bits, read back as they were.
		})
		if err != nil {
			return fmt.Errorf("insert: %w", err)
		}
		if err = q.PruneShuffleSessions(ctx, sqlcdb.PruneShuffleSessionsParams{UserID: userID, Keep: keptShuffles}); err != nil {
			return fmt.Errorf("prune: %w", err)
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("create shuffle session: %w", err)
	}
	return nil
}

// ShuffleSessionOf returns the user's shuffle session id, or an error wrapping
// [ErrNotFound] when they have none by that ID.
func (s *Store) ShuffleSessionOf(ctx context.Context, userID int64, id string) (ShuffleSession, error) {
	row, err := s.rq.GetShuffleSession(ctx, sqlcdb.GetShuffleSessionParams{UserID: userID, Ulid: id})
	if errors.Is(err, sql.ErrNoRows) {
		return ShuffleSession{}, fmt.Errorf("shuffle session %s: %w", id, ErrNotFound)
	}
	if err != nil {
		return ShuffleSession{}, fmt.Errorf("shuffle session %s: %w", id, err)
	}
	return ShuffleSession{
		ID: row.Ulid, Scope: row.Scope, Mode: row.Mode,
		Seed:     uint64(row.Seed), //nolint:gosec // G115: the seed's bits, as they were written.
		Position: row.Position, State: row.State,
		CreatedAt: time.UnixMilli(row.CreatedAt).UTC(), UpdatedAt: time.UnixMilli(row.UpdatedAt).UTC(),
	}, nil
}

// AdvanceShuffleSession moves the user's session on to ss's position and state,
// from the position was. It returns an error wrapping [ErrMoved] when the
// session is no longer at was, and one wrapping [ErrNotFound] when it has gone.
func (s *Store) AdvanceShuffleSession(ctx context.Context, userID int64, ss *ShuffleSession, was int64) error {
	n, err := s.wq.UpdateShuffleSession(ctx, sqlcdb.UpdateShuffleSessionParams{
		Position: ss.Position, State: ss.State, UpdatedAt: ms(ss.UpdatedAt),
		UserID: userID, Ulid: ss.ID, Was: was,
	})
	if err != nil {
		return fmt.Errorf("advance shuffle session %s: %w", ss.ID, err)
	}
	if n == 0 {
		if _, err = s.ShuffleSessionOf(ctx, userID, ss.ID); err != nil {
			return err
		}
		return fmt.Errorf("advance shuffle session %s: %w", ss.ID, ErrMoved)
	}
	return nil
}
