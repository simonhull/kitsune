package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/simonhull/kitsune/internal/store/sqlcdb"
)

// LastfmState says whether Last.fm still takes a connected account.
type LastfmState uint8

// The states of a connected account.
const (
	// LastfmOK means Last.fm takes the account's session.
	LastfmOK LastfmState = iota + 1
	// LastfmReconnect means Last.fm rejected the session: nothing is sent
	// until the person connects again, and the outbox waits.
	LastfmReconnect
)

// String is the state as stored: "ok" or "reconnect".
func (s LastfmState) String() string {
	switch s {
	case LastfmOK:
		return "ok"
	case LastfmReconnect:
		return "reconnect"
	}
	return fmt.Sprintf("LastfmState(%d)", uint8(s))
}

// parseLastfmState is the inverse of [LastfmState.String].
func parseLastfmState(s string) LastfmState {
	if s == LastfmReconnect.String() {
		return LastfmReconnect
	}
	return LastfmOK
}

// OutboxKind is what an outbox item asks Last.fm to do.
type OutboxKind uint8

// The kinds of outbox item.
const (
	OutboxScrobble OutboxKind = iota + 1
	OutboxLove
	OutboxUnlove
)

// String is the kind as stored.
func (k OutboxKind) String() string {
	switch k {
	case OutboxScrobble:
		return "scrobble"
	case OutboxLove:
		return "love"
	case OutboxUnlove:
		return "unlove"
	}
	return fmt.Sprintf("OutboxKind(%d)", uint8(k))
}

// parseOutboxKind is the inverse of [OutboxKind.String].
func parseOutboxKind(s string) OutboxKind {
	switch s {
	case OutboxLove.String():
		return OutboxLove
	case OutboxUnlove.String():
		return OutboxUnlove
	}
	return OutboxScrobble
}

// LastfmSwitches are what a person lets Kitsune send to Last.fm. All three
// start on.
type LastfmSwitches struct {
	Scrobble   bool // Plays that count.
	NowPlaying bool // The song as it starts.
	Loves      bool // Hearts on tracks, as loves.
}

// LastfmAccount is a person's connected Last.fm account and what waits for it.
type LastfmAccount struct {
	Name string
	// SessionKey signs calls for the account. It is as sensitive as a
	// password and never leaves the server.
	SessionKey  string
	Switches    LastfmSwitches
	State       LastfmState
	ConnectedAt time.Time
	LastSentAt  time.Time // Zero before the first send Last.fm took.
	Waiting     int       // Scrobbles in the outbox.
	Attempts    int       // The most failed sends of anything in the outbox.
	NextAt      time.Time // When the outbox next sends; zero when it is empty.
}

// LastfmTrack is what Last.fm is told about a track.
type LastfmTrack struct {
	Artists     []string // The track's artists, in tag order.
	Title       string
	Album       string
	AlbumArtist string // The album's display artist.
	MBID        string // The MusicBrainz track ID, or "".
	Duration    time.Duration
	Number      int // 0 when unknown.
}

// LastfmItem is one thing waiting in the outbox.
type LastfmItem struct {
	ID       int64
	Kind     OutboxKind
	At       time.Time // When the play started, or the heart changed.
	Attempts int
	Track    LastfmTrack
}

// LastfmAccount returns the user's connected account. It returns an error
// wrapping [ErrNotFound] when the user has not connected one.
func (s *Store) LastfmAccount(ctx context.Context, userID int64) (LastfmAccount, error) {
	row, err := s.rq.GetLastfmAccount(ctx, userID)
	if errors.Is(err, sql.ErrNoRows) {
		return LastfmAccount{}, fmt.Errorf("last.fm account: %w", ErrNotFound)
	}
	if err != nil {
		return LastfmAccount{}, fmt.Errorf("last.fm account: %w", err)
	}
	acct := LastfmAccount{
		Name:        row.Name,
		SessionKey:  row.SessionKey,
		Switches:    LastfmSwitches{Scrobble: row.Scrobble == 1, NowPlaying: row.NowPlaying == 1, Loves: row.Loves == 1},
		State:       parseLastfmState(row.State),
		ConnectedAt: fromMS(row.ConnectedAt),
		Waiting:     int(row.Waiting),
		Attempts:    int(row.Attempts),
	}
	if row.LastSentAt != nil {
		acct.LastSentAt = fromMS(*row.LastSentAt)
	}
	if row.NextAt != 0 {
		acct.NextAt = fromMS(row.NextAt)
	}
	return acct, nil
}

// ConnectLastfm stores the user's Last.fm session, in place of any they had.
// A new account starts with every switch on; a reconnected one keeps its
// switches, and whatever waited for it is due at once.
func (s *Store) ConnectLastfm(ctx context.Context, userID int64, name, sessionKey string) error {
	now := ms(s.now())
	err := s.inTx(ctx, func(q *sqlcdb.Queries, _ *sql.Tx) error {
		err := q.UpsertLastfmAccount(ctx, sqlcdb.UpsertLastfmAccountParams{
			UserID: userID, Name: name, SessionKey: sessionKey, ConnectedAt: now,
		})
		if err != nil {
			return err //nolint:wrapcheck // Wrapped once, below.
		}
		return q.ResetLastfmOutbox(ctx, sqlcdb.ResetLastfmOutboxParams{NextAt: now, UserID: userID})
	})
	if err != nil {
		return fmt.Errorf("connect last.fm: %w", err)
	}
	return nil
}

// SetLastfmSwitches sets what the user lets Kitsune send. Turning a switch
// off sends nothing new of its kind; what already waits still goes. It
// returns an error wrapping [ErrNotFound] when the user has no account.
func (s *Store) SetLastfmSwitches(ctx context.Context, userID int64, sw LastfmSwitches) error {
	n, err := s.wq.UpdateLastfmSwitches(ctx, sqlcdb.UpdateLastfmSwitchesParams{
		Scrobble: flag(sw.Scrobble), NowPlaying: flag(sw.NowPlaying), Loves: flag(sw.Loves), UserID: userID,
	})
	switch {
	case err != nil:
		return fmt.Errorf("set last.fm switches: %w", err)
	case n == 0:
		return fmt.Errorf("set last.fm switches: %w", ErrNotFound)
	}
	return nil
}

// SetLastfmReconnect marks the user's account as needing them to connect
// again. Its outbox stays.
func (s *Store) SetLastfmReconnect(ctx context.Context, userID int64) error {
	err := s.wq.SetLastfmState(ctx, sqlcdb.SetLastfmStateParams{State: LastfmReconnect.String(), UserID: userID})
	if err != nil {
		return fmt.Errorf("set last.fm state: %w", err)
	}
	return nil
}

// DisconnectLastfm forgets the user's account and everything waiting for it.
// It does nothing for a user with no account.
func (s *Store) DisconnectLastfm(ctx context.Context, userID int64) error {
	if err := s.wq.DeleteLastfmAccount(ctx, userID); err != nil {
		return fmt.Errorf("disconnect last.fm: %w", err)
	}
	return nil
}

// enqueueLastfm adds an item to the user's outbox, in the caller's
// transaction, when the user has an account with that kind switched on.
func enqueueLastfm(ctx context.Context, q *sqlcdb.Queries, userID, trackRow int64, kind OutboxKind, at, now time.Time) error {
	err := q.EnqueueLastfm(ctx, sqlcdb.EnqueueLastfmParams{
		Kind: kind.String(), TrackID: trackRow, At: ms(at), Now: ms(now), UserID: userID,
	})
	if err != nil {
		return fmt.Errorf("enqueue last.fm %s: %w", kind, err)
	}
	return nil
}

// LastfmDueUsers returns the users whose accounts Last.fm takes and who have
// something due at now.
func (s *Store) LastfmDueUsers(ctx context.Context, now time.Time) ([]int64, error) {
	ids, err := s.rq.ListLastfmDueUsers(ctx, ms(now))
	if err != nil {
		return nil, fmt.Errorf("last.fm due users: %w", err)
	}
	return ids, nil
}

// LastfmDue returns up to limit of the user's items of kinds due at now,
// oldest first. mbidKeys are the raw tag keys that hold a track's MusicBrainz
// ID.
func (s *Store) LastfmDue(ctx context.Context, userID int64, now time.Time, limit int, mbidKeys []string, kinds ...OutboxKind) ([]LastfmItem, error) {
	names := make([]string, len(kinds))
	for i, k := range kinds {
		names[i] = k.String()
	}
	rows, err := s.rq.ListLastfmDue(ctx, sqlcdb.ListLastfmDueParams{
		MbidKeys: mbidKeys, UserID: userID, NextAt: ms(now), Kinds: names, Limit: int64(limit),
	})
	if err != nil {
		return nil, fmt.Errorf("last.fm due: %w", err)
	}
	items := make([]LastfmItem, len(rows))
	trackRows := make([]int64, len(rows))
	for i := range rows {
		r := &rows[i]
		trackRows[i] = r.TrackRow
		items[i] = LastfmItem{
			ID: r.ID, Kind: parseOutboxKind(r.Kind), At: fromMS(r.At), Attempts: int(r.Attempts),
			Track: lastfmTrack(r.Title, r.Album, r.AlbumArtist, r.Mbid, r.DurationMs, r.TrackNo),
		}
	}
	artists, err := s.trackArtistNames(ctx, trackRows)
	if err != nil {
		return nil, err
	}
	for i := range items {
		items[i].Track.Artists = artists[trackRows[i]]
	}
	return items, nil
}

// LastfmTrackInfo returns what Last.fm is told about the track with the ULID
// id. It returns an error wrapping [ErrNotFound] for a track that isn't in
// the library.
func (s *Store) LastfmTrackInfo(ctx context.Context, id string, mbidKeys []string) (LastfmTrack, error) {
	row, err := s.rq.GetLastfmTrack(ctx, sqlcdb.GetLastfmTrackParams{MbidKeys: mbidKeys, Ulid: id})
	if errors.Is(err, sql.ErrNoRows) {
		return LastfmTrack{}, fmt.Errorf("last.fm track %s: %w", id, ErrNotFound)
	}
	if err != nil {
		return LastfmTrack{}, fmt.Errorf("last.fm track %s: %w", id, err)
	}
	t := lastfmTrack(row.Title, row.Album, row.AlbumArtist, row.Mbid, row.DurationMs, row.TrackNo)
	artists, err := s.trackArtistNames(ctx, []int64{row.TrackRow})
	if err != nil {
		return LastfmTrack{}, err
	}
	t.Artists = artists[row.TrackRow]
	return t, nil
}

// lastfmTrack builds a track from its columns.
func lastfmTrack(title, album, albumArtist, mbid string, durationMS int64, number *int64) LastfmTrack {
	t := LastfmTrack{
		Title: title, Album: album, AlbumArtist: albumArtist, MBID: mbid,
		Duration: time.Duration(durationMS) * time.Millisecond,
	}
	if number != nil {
		t.Number = int(*number)
	}
	return t
}

// trackArtistNames returns the artists of each track row, in tag order.
func (s *Store) trackArtistNames(ctx context.Context, trackRows []int64) (map[int64][]string, error) {
	out := make(map[int64][]string, len(trackRows))
	if len(trackRows) == 0 {
		return out, nil
	}
	rows, err := s.rq.ListTrackArtistNames(ctx, trackRows)
	if err != nil {
		return nil, fmt.Errorf("track artists: %w", err)
	}
	for _, r := range rows {
		out[r.TrackID] = append(out[r.TrackID], r.Name)
	}
	return out, nil
}

// LastfmSent removes the items with ids from the user's outbox, once Last.fm
// has them, and records at as the account's latest send. ignored items are
// removed without counting as a send.
func (s *Store) LastfmSent(ctx context.Context, userID int64, ids []int64, at time.Time) error {
	err := s.inTx(ctx, func(q *sqlcdb.Queries, _ *sql.Tx) error {
		if err := deleteLastfmItems(ctx, q, userID, ids); err != nil {
			return err
		}
		return q.MarkLastfmSent(ctx, sqlcdb.MarkLastfmSentParams{LastSentAt: new(ms(at)), UserID: userID})
	})
	if err != nil {
		return fmt.Errorf("last.fm sent: %w", err)
	}
	return nil
}

// DropLastfm removes the items with ids from the user's outbox without
// sending them: Last.fm refused them, and would again.
func (s *Store) DropLastfm(ctx context.Context, userID int64, ids []int64) error {
	err := s.inTx(ctx, func(q *sqlcdb.Queries, _ *sql.Tx) error {
		return deleteLastfmItems(ctx, q, userID, ids)
	})
	if err != nil {
		return fmt.Errorf("drop last.fm items: %w", err)
	}
	return nil
}

// deleteLastfmItems deletes the user's items with ids.
func deleteLastfmItems(ctx context.Context, q *sqlcdb.Queries, userID int64, ids []int64) error {
	for _, id := range ids {
		if err := q.DeleteLastfmItem(ctx, sqlcdb.DeleteLastfmItemParams{ID: id, UserID: userID}); err != nil {
			return err //nolint:wrapcheck // Wrapped once, by the caller.
		}
	}
	return nil
}

// DeferLastfm holds everything in the user's outbox until until, counting a
// failed attempt on each item. Holding all of it keeps the order.
func (s *Store) DeferLastfm(ctx context.Context, userID int64, until time.Time) error {
	if err := s.wq.DeferLastfm(ctx, sqlcdb.DeferLastfmParams{NextAt: ms(until), UserID: userID}); err != nil {
		return fmt.Errorf("defer last.fm: %w", err)
	}
	return nil
}

// DropOldLastfmScrobbles removes the user's scrobbles of plays that started
// before before, which Last.fm no longer takes, and returns how many.
func (s *Store) DropOldLastfmScrobbles(ctx context.Context, userID int64, before time.Time) (int64, error) {
	n, err := s.wq.DropOldLastfmScrobbles(ctx, sqlcdb.DropOldLastfmScrobblesParams{UserID: userID, At: ms(before)})
	if err != nil {
		return 0, fmt.Errorf("drop old last.fm scrobbles: %w", err)
	}
	return n, nil
}
