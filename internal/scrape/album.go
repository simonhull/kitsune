package scrape

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/simonhull/kitsune/internal/artwork"
	"github.com/simonhull/kitsune/internal/matching"
	"github.com/simonhull/kitsune/internal/remote"
	"github.com/simonhull/kitsune/internal/scrape/fanarttv"
	"github.com/simonhull/kitsune/internal/store"
)

// albumFound is what one source answered about one album.
type albumFound struct {
	source  store.ScrapeSource
	mbid    string // What it was asked by.
	asked   bool   // Asked and answered, with something or with nothing.
	status  store.ScrapeStatus
	failure *Failure
}

// albumQueue is every album with something due, the most played first.
func (s *Scraper) albumQueue(cfg *Config, albums []store.ScrapeAlbum) []store.ScrapeAlbum {
	now := s.now()
	var q []store.ScrapeAlbum
	for i := range albums {
		if a := &albums[i]; len(s.dueAlbumSources(cfg, a, now)) > 0 {
			q = append(q, *a)
		}
	}
	slices.SortStableFunc(q, func(a, b store.ScrapeAlbum) int {
		return cmp.Or(cmp.Compare(b.Plays, a.Plays), cmp.Compare(a.ID, b.ID))
	})
	return q
}

// albumWork asks about each album in queue, people's artists first whenever
// they open one, and reports whether it got to the end.
func (s *Scraper) albumWork(ctx context.Context, cfg *Config, queue []store.ScrapeAlbum) bool {
	defer s.progress.Store(&Progress{})
	asked := map[int64]bool{}
	for i := range queue {
		if s.servePeople(ctx, cfg, asked) != nil || s.waitScan(ctx) != nil {
			return false
		}
		a := &queue[i]
		s.progress.Store(&Progress{Running: true, Albums: true, Done: i, Total: len(queue), Album: a.ULID, Name: a.Name})
		s.album(ctx, cfg, a)
	}
	return ctx.Err() == nil
}

// wantsArchive reports whether the Cover Art Archive may give a anything: a
// front while its files have no cover, a back while its folder has none.
func wantsArchive(a *store.ScrapeAlbum) bool {
	back, ok := a.Held[store.AlbumImageKey{AlbumID: a.ID, Kind: store.AlbumBack}]
	return !a.OwnCover || !ok || back.Source != fromFolder
}

// wantsDiscArt reports whether fanart.tv may give a disc art: its folder has none.
func wantsDiscArt(a *store.ScrapeAlbum) bool {
	for k, h := range a.Held {
		if k.Kind == store.AlbumDisc && h.Source == fromFolder {
			return false
		}
	}
	return true
}

// albumSourceDue reports whether src is to be asked about a at now.
func albumSourceDue(src store.ScrapeSource, a *store.ScrapeAlbum, now time.Time) bool {
	return albumWants(src, a) && Due(albumState(a, src), now)
}

// albumState is what src last said of a, or nil.
func albumState(a *store.ScrapeAlbum, src store.ScrapeSource) *store.ScrapeState {
	if st, ok := a.States[src]; ok {
		return &st
	}
	return nil
}

// albumWants reports whether src may give a anything it lacks: a release
// group to ask fanart.tv by, covers, disc art.
func albumWants(src store.ScrapeSource, a *store.ScrapeAlbum) bool {
	switch src {
	case store.ScrapeMusicBrainz:
		return matching.IsMBID(a.MBID) && a.GroupMBID == "" && wantsDiscArt(a)
	case store.ScrapeArchive:
		return matching.IsMBID(a.MBID) && wantsArchive(a)
	case store.ScrapeFanart:
		return matching.IsMBID(a.GroupMBID) && wantsDiscArt(a)
	case store.ScrapeWikimedia, store.ScrapeLastfm: // Artists' sources.
	}
	return false
}

// dueAlbumSources are the sources to ask about a now: on, not refused or
// resting, and due. fanart.tv is due when the release group is still to be
// looked up, since the lookup may give one it has never been asked by.
func (s *Scraper) dueAlbumSources(cfg *Config, a *store.ScrapeAlbum, now time.Time) []store.ScrapeSource {
	var out []store.ScrapeSource
	usable := s.usable(cfg, now, albumSources())
	for _, src := range usable {
		if albumSourceDue(src, a, now) {
			out = append(out, src)
		}
	}
	if slices.Contains(out, store.ScrapeMusicBrainz) && slices.Contains(usable, store.ScrapeFanart) && !slices.Contains(out, store.ScrapeFanart) {
		out = append(out, store.ScrapeFanart)
	}
	return out
}

// album asks every source due about a, chooses, downloads what it chose, and
// stores it all in one write.
func (s *Scraper) album(ctx context.Context, cfg *Config, a *store.ScrapeAlbum) {
	due := s.dueAlbumSources(cfg, a, s.now())
	if len(due) == 0 {
		return
	}
	w := &store.AlbumRemoteWrite{AlbumID: a.ID}
	answers := s.askAlbum(remote.Background(ctx), cfg, a, due, w)
	if ctx.Err() != nil {
		return
	}
	w.Answers = s.albumAnswers(a, answers)
	changed, err := s.o.Store.WriteAlbumRemote(ctx, w, s.now())
	if err != nil {
		if ctx.Err() == nil {
			s.log.ErrorContext(ctx, "album pass: storing an album's art failed", "album", a.ULID, "err", err)
		}
		return
	}
	if changed && s.o.NotifyAlbums != nil {
		s.o.NotifyAlbums([]string{a.ULID})
	}
}

// askAlbum asks each source in due about a, in order, into w.
func (s *Scraper) askAlbum(ctx context.Context, cfg *Config, a *store.ScrapeAlbum, due []store.ScrapeSource, w *store.AlbumRemoteWrite) []*albumFound {
	answers := make([]*albumFound, 0, len(due))
	for _, src := range due {
		switch src {
		case store.ScrapeMusicBrainz:
			answers = append(answers, s.askGroup(ctx, a, w))
		case store.ScrapeArchive:
			answers = append(answers, s.askArchive(ctx, a, w))
		case store.ScrapeFanart:
			if a.GroupMBID != "" { // Else the lookup found none: there is nothing to ask by.
				answers = append(answers, s.askDiscArt(ctx, cfg, a, w))
			}
		case store.ScrapeWikimedia, store.ScrapeLastfm: // Artists' sources.
		}
	}
	return answers
}

// albumAnswers are the states the answers leave: a source refused, resting or
// stopped midway leaves its state as it was.
func (s *Scraper) albumAnswers(a *store.ScrapeAlbum, answers []*albumFound) []store.AlbumAnswer {
	var out []store.AlbumAnswer
	for _, f := range answers {
		if !f.asked && f.status != store.ScrapeFailed {
			continue
		}
		msg := ""
		if f.failure != nil {
			msg = f.failure.Words
		}
		var prev *store.ScrapeState
		if a.StateKey(f.source) == f.mbid {
			prev = albumState(a, f.source)
		}
		out = append(out, store.AlbumAnswer{Source: f.source, MBID: f.mbid, State: NextAsk(f.status, prev, f.mbid, msg, s.now())})
	}
	return out
}

// settleAlbum records how an answer about an album went, as [Scraper.settle]
// does an artist's.
func (s *Scraper) settleAlbum(ctx context.Context, f *albumFound, found bool, err error) {
	now := s.now()
	switch {
	case err == nil:
		f.asked, f.status = true, store.ScrapeNone
		if found {
			f.status = store.ScrapeFound
		}
		s.healthy(f.source)
	case errors.Is(err, remote.ErrNotFound), errors.Is(err, matching.ErrNotFound):
		f.asked, f.status = true, store.ScrapeNone
		s.healthy(f.source)
	case ctx.Err() != nil:
	case errors.Is(err, remote.ErrRefused):
		s.refuse(ctx, f.source, describe(f.source, err, now))
	default:
		f.status, f.failure = store.ScrapeFailed, describe(f.source, err, now)
		if errors.Is(err, remote.ErrUnavailable) || errors.Is(err, matching.ErrUnavailable) {
			s.restSource(ctx, f.source, f.failure, err)
		} else {
			s.noteFailure(f.source, f.failure)
		}
	}
}

// askGroup looks up a's release group from its release, into w and a.
func (s *Scraper) askGroup(ctx context.Context, a *store.ScrapeAlbum, w *store.AlbumRemoteWrite) *albumFound {
	f := &albumFound{source: store.ScrapeMusicBrainz, mbid: a.MBID}
	group, err := s.o.MusicBrainz.ReleaseGroup(ctx, a.MBID)
	if err == nil && group != "" {
		w.GroupMBID, a.GroupMBID = group, group
	}
	s.settleAlbum(ctx, f, group != "", err)
	return f
}

// askArchive asks the Cover Art Archive for a's front, while its files have
// no cover, and its back, while its folder has none, into w.
func (s *Scraper) askArchive(ctx context.Context, a *store.ScrapeAlbum, w *store.AlbumRemoteWrite) *albumFound {
	f := &albumFound{source: store.ScrapeArchive, mbid: a.MBID}
	art, err := s.o.MusicBrainz.CoverArt(ctx, a.MBID)
	if errors.Is(err, remote.ErrNotFound) {
		err, art = nil, matching.CoverArt{} // Nothing there: what it gave before goes.
	}
	if err == nil {
		credit := &store.Credit{PageURL: "https://musicbrainz.org/release/" + a.MBID + "/cover-art"}
		if !a.OwnCover {
			err = s.archiveImage(ctx, a, w, store.AlbumCover, artwork.Cover, art.Front, credit)
		}
		if back, ok := a.Held[store.AlbumImageKey{AlbumID: a.ID, Kind: store.AlbumBack}]; err == nil && (!ok || back.Source != fromFolder) {
			err = s.archiveImage(ctx, a, w, store.AlbumBack, artwork.Back, art.Back, credit)
		}
	}
	s.settleAlbum(ctx, f, art.Front != nil || art.Back != nil, err)
	return f
}

// archiveImage adds to w what img, the archive's image of kind or nil, decides
// for a: set when it is new, taken away when it is gone, nothing when it is
// the one a holds.
func (s *Scraper) archiveImage(ctx context.Context, a *store.ScrapeAlbum, w *store.AlbumRemoteWrite, kind store.AlbumImageKind, ladder artwork.Kind, img *matching.ArtImage, credit *store.Credit) error {
	held, ok := a.Held[store.AlbumImageKey{AlbumID: a.ID, Kind: kind}]
	heldHere := ok && held.Source == store.ImageFromArchive
	switch {
	case img == nil && heldHere:
		w.Images = append(w.Images, store.RemoteAlbumImage{Kind: kind, Source: store.ImageFromArchive})
		return nil
	case img == nil || (heldHere && held.RemoteID == img.ID):
		return nil
	}
	if s.o.Artwork == nil {
		return nil
	}
	body, err := s.o.MusicBrainz.DownloadArt(ctx, img.URL)
	if err != nil {
		return err //nolint:wrapcheck // It names the archive; settleAlbum reads its sentinel.
	}
	put, err := s.o.Artwork.Put(ctx, ladder, body)
	if err != nil {
		return fmt.Errorf("%w: %w", matching.ErrBadArt, err)
	}
	w.Images = append(w.Images, store.RemoteAlbumImage{
		Kind: kind, Source: store.ImageFromArchive, RemoteID: img.ID, Credit: credit, Image: remoteArtwork(put),
	})
	return nil
}

// askDiscArt asks fanart.tv for a's disc art by its release group, into w.
func (s *Scraper) askDiscArt(ctx context.Context, cfg *Config, a *store.ScrapeAlbum, w *store.AlbumRemoteWrite) *albumFound {
	f := &albumFound{source: store.ScrapeFanart, mbid: a.GroupMBID}
	discs, err := s.o.Fanart.Album(ctx, cfg.Keys, a.GroupMBID)
	if errors.Is(err, remote.ErrNotFound) {
		err, discs = nil, nil
	}
	if err == nil {
		err = s.decideDiscs(ctx, cfg, a, w, discs)
	}
	s.settleAlbum(ctx, f, len(discs) > 0, err)
	return f
}

// discTargets are the discs a's art is kept for: one for every disc of an
// album of one, else each disc.
func discTargets(a *store.ScrapeAlbum) []int {
	if len(a.Discs) <= 1 {
		return []int{store.AllDiscs}
	}
	return a.Discs
}

// DiscCandidates are fanart.tv's disc art for disc target of an album: every
// disc's art (target 0) takes the first disc's, or any when none says which.
func DiscCandidates(discs []Candidate, numbers []int, target int) []Candidate {
	var out []Candidate
	for i, c := range discs {
		if (target == store.AllDiscs && numbers[i] <= 1) || (target != store.AllDiscs && numbers[i] == target) {
			out = append(out, c)
		}
	}
	if target == store.AllDiscs && len(out) == 0 {
		return discs
	}
	return out
}

// decideDiscs adds to w what fanart.tv's disc art decides for each of a's
// discs, and takes away fanart.tv's art of discs it no longer has.
func (s *Scraper) decideDiscs(ctx context.Context, cfg *Config, a *store.ScrapeAlbum, w *store.AlbumRemoteWrite, discs []fanarttv.Disc) error {
	cands := make([]Candidate, len(discs))
	numbers := make([]int, len(discs))
	for i, d := range discs {
		cands[i] = Candidate{Source: store.ImageFromFanart, ID: d.ID, URL: d.URL, Lang: d.Lang, Likes: d.Likes, Width: d.Width, Height: d.Height}
		numbers[i] = d.Disc
	}
	targets := discTargets(a)
	for k, h := range a.Held {
		if k.Kind == store.AlbumDisc && h.Source == store.ImageFromFanart && !slices.Contains(targets, k.Disc) {
			w.Images = append(w.Images, store.RemoteAlbumImage{Kind: store.AlbumDisc, Disc: k.Disc, Source: store.ImageFromFanart})
		}
	}
	for _, t := range targets {
		if err := s.decideDisc(ctx, a, w, t, Pick(DiscCandidates(cands, numbers, t), cfg.Language)); err != nil {
			return err
		}
	}
	return nil
}

// decideDisc adds to w what pick, fanart.tv's art of disc t or nil, decides:
// set when it is new, taken away when fanart.tv has none now.
func (s *Scraper) decideDisc(ctx context.Context, a *store.ScrapeAlbum, w *store.AlbumRemoteWrite, t int, pick *Candidate) error {
	held, ok := a.Held[store.AlbumImageKey{AlbumID: a.ID, Kind: store.AlbumDisc, Disc: t}]
	heldHere := ok && held.Source == store.ImageFromFanart
	switch {
	case pick == nil && heldHere:
		w.Images = append(w.Images, store.RemoteAlbumImage{Kind: store.AlbumDisc, Disc: t, Source: store.ImageFromFanart})
		return nil
	case pick == nil || (heldHere && held.RemoteID == pick.ID) || s.o.Artwork == nil:
		return nil
	}
	body, err := s.o.Fanart.Download(ctx, pick.URL)
	if err != nil {
		return err //nolint:wrapcheck // It names fanart.tv.
	}
	put, err := s.o.Artwork.Put(ctx, artwork.Disc, body)
	if err != nil {
		return fmt.Errorf("%w: %w", errBadImage, err)
	}
	w.Images = append(w.Images, store.RemoteAlbumImage{
		Kind: store.AlbumDisc, Disc: t, Source: store.ImageFromFanart, RemoteID: pick.ID, Image: remoteArtwork(put),
	})
	return nil
}

// errBadImage is an image a source sent that doesn't decode.
var errBadImage = errors.New("an image doesn't decode")

// remoteArtwork is a rendered image as a remote one is stored.
func remoteArtwork(img *artwork.Image) *store.ArtworkWrite {
	return &store.ArtworkWrite{
		Hash: img.Hash, Width: img.Width, Height: img.Height, Accent: img.Accent, Ground: img.Ground,
		Source: store.ArtworkRemote, Alpha: img.Alpha,
	}
}

// nextAlbumDue is when the next album falls due, or a day from now.
func (s *Scraper) nextAlbumDue(cfg *Config, albums []store.ScrapeAlbum) time.Time {
	now := s.now()
	next := now.Add(idleWait)
	usable := s.usable(cfg, now, albumSources())
	for i := range albums {
		a := &albums[i]
		for _, src := range usable {
			if st, ok := a.States[src]; ok && st.NextAt.After(now) && st.NextAt.Before(next) {
				next = st.NextAt
			}
		}
	}
	return next
}
