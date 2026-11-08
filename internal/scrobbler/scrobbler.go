// Package scrobbler sends what people play and love to Last.fm: the outbox the
// store fills, in batches and in order, and now playing as songs start.
//
// One goroutine, [Scrobbler.Run], does all the sending. It wakes when
// something is queued and once a minute, and backs off per person while
// Last.fm is down.
package scrobbler

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync/atomic"
	"time"

	"github.com/simonhull/kitsune/internal/lastfm"
	"github.com/simonhull/kitsune/internal/store"
)

const (
	// tick is how often the sender looks for due items without being poked.
	tick = time.Minute
	// minBackoff and maxBackoff bound the wait after a temporary failure,
	// which doubles with each failure in a row.
	minBackoff = time.Minute
	maxBackoff = 30 * time.Minute
	// maxAge is how old a play Last.fm still takes.
	maxAge = 14 * 24 * time.Hour
	// maxBatches bounds the batches of each kind one wake sends for one
	// person, so a long backlog can't hold the others up for long.
	maxBatches = 20
	// nowPlayingTimeout bounds a now playing call, which nobody waits on.
	nowPlayingTimeout = 10 * time.Second
	// nowPlayingQueue is how many starts may wait to be sent. More are
	// dropped: now playing is best effort.
	nowPlayingQueue = 16
)

// Options configures [New].
type Options struct {
	// Client talks to Last.fm. Nil means Last.fm is not set up: nothing is
	// sent and [Scrobbler.Configured] is false.
	Client *lastfm.Client
	// MBIDKeys are the raw tag keys that hold a track's MusicBrainz ID.
	MBIDKeys []string
	// Logger reports what was sent and what failed. Nil discards.
	Logger *slog.Logger
	// Now returns the current time. Nil means [time.Now].
	Now func() time.Time
}

// Scrobbler sends the outbox to Last.fm. It is safe for concurrent use.
type Scrobbler struct {
	store    *store.Store
	client   atomic.Pointer[lastfm.Client] // Nil while Last.fm is not set up.
	mbidKeys []string
	log      *slog.Logger
	now      func() time.Time

	poke    chan struct{}
	playing chan start
}

// start is a song that started playing.
type start struct {
	userID  int64
	trackID string
}

// New returns a scrobbler for the outbox in st.
func New(st *store.Store, opts Options) *Scrobbler {
	s := &Scrobbler{
		store: st, mbidKeys: opts.MBIDKeys, log: opts.Logger, now: opts.Now,
		poke: make(chan struct{}, 1), playing: make(chan start, nowPlayingQueue),
	}
	if s.log == nil {
		s.log = slog.New(slog.DiscardHandler)
	}
	if s.now == nil {
		s.now = time.Now
	}
	s.client.Store(opts.Client)
	return s
}

// SetClient replaces the Last.fm client, as when an administrator changes the
// key: nil turns Last.fm off. What waited is sent with the new client. Accounts
// connected under another key may be refused, and are then asked to connect
// again.
func (s *Scrobbler) SetClient(c *lastfm.Client) {
	s.client.Store(c)
	s.Poke()
}

// Configured reports whether Last.fm is set up on this server.
func (s *Scrobbler) Configured() bool { return s.current() != nil }

// AuthURL returns the page where a person approves Kitsune, which sends them
// back to callback. It is "" when Last.fm is not set up.
func (s *Scrobbler) AuthURL(callback string) string {
	c := s.current()
	if c == nil {
		return ""
	}
	return c.AuthURL(callback)
}

// errNotConfigured means Last.fm is not set up on this server.
var errNotConfigured = errors.New("last.fm is not set up")

// Session exchanges the token Last.fm sent back for a session.
//
// # Errors
//
// The error wraps one of [lastfm.ErrTemporary] or [lastfm.ErrRejected], or
// says that Last.fm is not set up.
func (s *Scrobbler) Session(ctx context.Context, token string) (lastfm.Session, error) {
	c := s.current()
	if c == nil {
		return lastfm.Session{}, errNotConfigured
	}
	return c.Session(ctx, token) //nolint:wrapcheck // lastfm's errors name the method and never a secret.
}

// Poke wakes the sender: something was queued, or an account came back.
func (s *Scrobbler) Poke() {
	if !s.Configured() {
		return
	}
	select {
	case s.poke <- struct{}{}:
	default: // A wake is already pending; it will see this too.
	}
}

// NowPlaying asks the sender to tell Last.fm that the user started the track
// with the ULID trackID, when their account takes it. It never waits, and a
// start that finds the queue full is dropped.
func (s *Scrobbler) NowPlaying(userID int64, trackID string) {
	if !s.Configured() {
		return
	}
	select {
	case s.playing <- start{userID: userID, trackID: trackID}:
	default:
		s.log.Debug("last.fm now playing dropped: queue full", "userId", userID)
	}
}

// Run sends until ctx ends. While Last.fm is not set up it sends nothing, and
// [Scrobbler.SetClient] wakes it.
func (s *Scrobbler) Run(ctx context.Context) {
	ticker := time.NewTicker(tick)
	defer ticker.Stop()
	s.flush(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.flush(ctx)
		case <-s.poke:
			s.flush(ctx)
		case p := <-s.playing:
			if client := s.current(); client != nil { // Not if Last.fm was turned off since.
				s.sendNowPlaying(ctx, client, p)
			}
		}
	}
}

// current returns the client, or nil while Last.fm is not set up. A nil
// Scrobbler has none.
func (s *Scrobbler) current() *lastfm.Client {
	if s == nil {
		return nil
	}
	return s.client.Load()
}

// flush sends what is due now for every account Last.fm takes. Only Run calls
// it, so no two sends of the same item can overlap.
func (s *Scrobbler) flush(ctx context.Context) {
	client := s.current()
	if client == nil {
		return
	}
	users, err := s.store.LastfmDueUsers(ctx, s.now())
	if err != nil {
		s.warn(ctx, "last.fm: read the outbox", err)
		return
	}
	for _, id := range users {
		if ctx.Err() != nil {
			return
		}
		s.sendUser(ctx, client, id)
	}
}

// sendUser sends one person's due items: scrobbles in batches, then loves one
// at a time. It stops at the first failure that isn't the item's own fault.
func (s *Scrobbler) sendUser(ctx context.Context, client *lastfm.Client, userID int64) {
	acct, err := s.store.LastfmAccount(ctx, userID)
	if err != nil {
		s.warn(ctx, "last.fm: read the account", err, "userId", userID)
		return
	}
	if acct.State != store.LastfmOK {
		return
	}
	now := s.now()
	if n, dropErr := s.store.DropOldLastfmScrobbles(ctx, userID, now.Add(-maxAge)); dropErr != nil {
		s.warn(ctx, "last.fm: drop old scrobbles", dropErr, "userId", userID)
		return
	} else if n > 0 {
		s.log.WarnContext(ctx, "last.fm: dropped scrobbles too old to send", "userId", userID, "count", n)
	}
	sender := &userSender{s: s, client: client, userID: userID, acct: &acct, now: now}
	if sender.drain(ctx, lastfm.MaxScrobbles, sender.scrobble, store.OutboxScrobble) {
		sender.drain(ctx, 1, sender.love, store.OutboxLove, store.OutboxUnlove)
	}
}

// userSender sends one person's items during one wake.
type userSender struct {
	s      *Scrobbler
	client *lastfm.Client
	userID int64
	acct   *store.LastfmAccount
	now    time.Time
}

// sendFunc sends a batch and reports what Last.fm ignored, by index.
type sendFunc func(ctx context.Context, items []store.LastfmItem) ([]lastfm.Ignored, error)

// drain sends batches of up to size due items of kinds through send until
// none are left. It reports whether sending may go on.
func (u *userSender) drain(ctx context.Context, size int, send sendFunc, kinds ...store.OutboxKind) bool {
	for range maxBatches {
		items, err := u.s.store.LastfmDue(ctx, u.userID, u.now, size, u.s.mbidKeys, kinds...)
		if err != nil {
			u.s.warn(ctx, "last.fm: read the outbox", err, "userId", u.userID)
			return false
		}
		items = u.dropUnsendable(ctx, items)
		if len(items) == 0 {
			return true
		}
		ignored, err := send(ctx, items)
		if !u.settle(ctx, items, ignored, err) {
			return false
		}
	}
	return true
}

// dropUnsendable removes, from the outbox and from items, the items Last.fm
// can't take: a track with no artist or no title.
func (u *userSender) dropUnsendable(ctx context.Context, items []store.LastfmItem) []store.LastfmItem {
	keep := items[:0]
	var drop []int64
	for i := range items {
		if artistOf(&items[i].Track) == "" || items[i].Track.Title == "" {
			drop = append(drop, items[i].ID)
			continue
		}
		keep = append(keep, items[i])
	}
	if len(drop) > 0 {
		if err := u.s.store.DropLastfm(ctx, u.userID, drop); err != nil {
			u.s.warn(ctx, "last.fm: drop unsendable items", err, "userId", u.userID)
		} else {
			u.s.log.WarnContext(ctx, "last.fm: dropped items with no artist or title", "userId", u.userID, "count", len(drop))
		}
	}
	return keep
}

// settle records how a batch went and reports whether sending may go on.
func (u *userSender) settle(ctx context.Context, items []store.LastfmItem, ignored []lastfm.Ignored, err error) bool {
	ids := make([]int64, len(items))
	for i := range items {
		ids[i] = items[i].ID
	}
	switch {
	case err == nil:
		return u.sent(ctx, items, ids, ignored)
	case ctx.Err() != nil:
		return false
	case errors.Is(err, lastfm.ErrTemporary):
		wait := backoff(items[0].Attempts)
		u.s.log.WarnContext(ctx, "last.fm unavailable; trying again later", "userId", u.userID, "err", err, "wait", wait.String())
		if deferErr := u.s.store.DeferLastfm(ctx, u.userID, u.now.Add(wait)); deferErr != nil {
			u.s.warn(ctx, "last.fm: defer the outbox", deferErr, "userId", u.userID)
		}
		return false
	case errors.Is(err, lastfm.ErrSession):
		u.s.needsReconnect(ctx, u.userID, err)
		return false
	default: // Rejected: sending it again would fail the same way.
		u.s.log.WarnContext(ctx, "last.fm rejected items; dropped them", "userId", u.userID,
			"kind", items[0].Kind.String(), "count", len(items), "err", err)
		if dropErr := u.s.store.DropLastfm(ctx, u.userID, ids); dropErr != nil {
			u.s.warn(ctx, "last.fm: drop rejected items", dropErr, "userId", u.userID)
			return false
		}
		return true
	}
}

// sent removes a batch Last.fm took, logging the items it ignored.
func (u *userSender) sent(ctx context.Context, items []store.LastfmItem, ids []int64, ignored []lastfm.Ignored) bool {
	for _, ig := range ignored {
		u.s.log.WarnContext(ctx, "last.fm ignored a scrobble", "userId", u.userID,
			"kind", items[ig.Index].Kind.String(), "code", ig.Code, "reason", ig.Message)
	}
	if err := u.s.store.LastfmSent(ctx, u.userID, ids, u.s.now()); err != nil {
		u.s.warn(ctx, "last.fm: record a send", err, "userId", u.userID)
		return false
	}
	return true
}

// needsReconnect marks the user's account after Last.fm rejected its session.
func (s *Scrobbler) needsReconnect(ctx context.Context, userID int64, err error) {
	s.log.WarnContext(ctx, "last.fm rejected the session; the account needs reconnecting", "userId", userID, "err", err)
	if stateErr := s.store.SetLastfmReconnect(ctx, userID); stateErr != nil {
		s.warn(ctx, "last.fm: mark the account", stateErr, "userId", userID)
	}
}

// scrobble sends items as one track.scrobble call.
func (u *userSender) scrobble(ctx context.Context, items []store.LastfmItem) ([]lastfm.Ignored, error) {
	plays := make([]lastfm.Scrobble, len(items))
	for i := range items {
		plays[i] = lastfm.Scrobble{Track: toTrack(&items[i].Track), At: items[i].At}
	}
	return u.client.Scrobble(ctx, u.acct.SessionKey, plays) //nolint:wrapcheck // settle branches on lastfm's kinds.
}

// love sends one love or unlove.
func (u *userSender) love(ctx context.Context, items []store.LastfmItem) ([]lastfm.Ignored, error) {
	it := &items[0]
	send := u.client.Love
	if it.Kind == store.OutboxUnlove {
		send = u.client.Unlove
	}
	return nil, send(ctx, u.acct.SessionKey, artistOf(&it.Track), it.Track.Title)
}

// sendNowPlaying tells Last.fm what the user started, when their account
// takes it. Nothing is retried.
func (s *Scrobbler) sendNowPlaying(ctx context.Context, client *lastfm.Client, p start) {
	acct, err := s.store.LastfmAccount(ctx, p.userID)
	if err != nil || acct.State != store.LastfmOK || !acct.Switches.NowPlaying {
		return // Not connected, or not wanted.
	}
	track, err := s.store.LastfmTrackInfo(ctx, p.trackID, s.mbidKeys)
	if err != nil || artistOf(&track) == "" || track.Title == "" {
		return // A track that went away, or has no name, has nothing to show.
	}
	t := toTrack(&track)
	callCtx, cancel := context.WithTimeout(ctx, nowPlayingTimeout)
	defer cancel()
	err = client.NowPlaying(callCtx, acct.SessionKey, &t)
	switch {
	case err == nil, ctx.Err() != nil:
	case errors.Is(err, lastfm.ErrSession):
		s.needsReconnect(ctx, p.userID, err)
	default:
		s.log.DebugContext(ctx, "last.fm now playing failed", "userId", p.userID, "err", err)
	}
}

// warn logs a failure of Kitsune's own, unless ctx has ended.
func (s *Scrobbler) warn(ctx context.Context, msg string, err error, args ...any) {
	if ctx.Err() != nil {
		return
	}
	s.log.WarnContext(ctx, msg, append(args, "err", err)...)
}

// backoff is the wait after a temporary failure, for an item that had already
// failed attempts times: a minute, doubling, at most half an hour.
func backoff(attempts int) time.Duration {
	const doublings = 5 // 1m << 5 is past the cap, and a larger shift could overflow.
	return min(maxBackoff, minBackoff<<min(max(attempts, 0), doublings))
}

// artistOf is the artist Last.fm is told: the track's artists, or else the
// album's.
func artistOf(t *store.LastfmTrack) string {
	if a := strings.Join(t.Artists, ", "); a != "" {
		return a
	}
	return t.AlbumArtist
}

// toTrack converts a store track into what Last.fm is told.
func toTrack(t *store.LastfmTrack) lastfm.Track {
	return lastfm.Track{
		Artist: artistOf(t), Title: t.Title, Album: t.Album, AlbumArtist: t.AlbumArtist,
		MBID: t.MBID, Duration: t.Duration, Number: t.Number,
	}
}
