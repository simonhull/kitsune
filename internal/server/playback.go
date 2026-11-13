package server

import (
	"encoding/json/v2"
	"errors"
	"fmt"
	"mime"
	"net/http"

	"github.com/simonhull/kitsune/api"
	"github.com/simonhull/kitsune/internal/playback"
	"github.com/simonhull/kitsune/internal/shuffler"
	"github.com/simonhull/kitsune/internal/store"
)

// maxPlaybackBody is the most a saved record may take: what a browser's
// keepalive request may carry.
const maxPlaybackBody = 64 << 10

// getPlayback answers where the caller left off, resolved against the library
// now. Nothing remembered, or nothing of it left, is a 404.
func (s *Server) getPlayback(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r.Context()) // Always there, behind authenticate.
	stored, err := s.library.Playback(r.Context(), user.ID)
	if errors.Is(err, store.ErrNotFound) {
		writeProblem(w, r, http.StatusNotFound, "nothing remembered")
		return
	}
	if err != nil {
		if r.Context().Err() == nil {
			s.internalError(w, r, "read playback", err)
		}
		return
	}
	rec, err := playback.Decode(&stored)
	if err != nil {
		s.log.WarnContext(r.Context(), "stored playback fails its rules",
			"requestId", requestID(r.Context()), "err", err)
		writeProblem(w, r, http.StatusNotFound, "nothing remembered")
		return
	}
	out, err := s.resolvePlayback(r, user.ID, &rec)
	if errors.Is(err, store.ErrNotFound) {
		writeProblem(w, r, http.StatusNotFound, "nothing remembered is left")
		return
	}
	if err != nil {
		if r.Context().Err() == nil {
			s.internalError(w, r, "resolve playback", err)
		}
		return
	}
	out.UpdatedAt = stored.UpdatedAt.UnixMilli()
	w.Header().Set("Cache-Control", "no-store")
	writeBody(w, http.StatusOK, api.JSONMediaType, &out)
}

// resolvePlayback returns rec as the library has it now, or an error wrapping
// [store.ErrNotFound] when none of it from the current song on is left.
func (s *Server) resolvePlayback(r *http.Request, userID int64, rec *playback.Record) (api.Playback, error) {
	live, err := s.library.LiveTracks(r.Context(), rec.IDs())
	if err != nil {
		return api.Playback{}, err //nolint:wrapcheck // The store's error names what failed.
	}
	resolved, ok := playback.Resolve(rec, live)
	if !ok {
		return api.Playback{}, store.ErrNotFound
	}
	out := toAPIPlayback(rec, &resolved)
	if rec.Continues != "" {
		switch _, err = s.library.AlbumRow(r.Context(), rec.Continues); {
		case err == nil:
			out.Continues = rec.Continues
		case !errors.Is(err, store.ErrNotFound):
			return api.Playback{}, err //nolint:wrapcheck // As above.
		}
	}
	if rec.Shuffle != nil {
		out.Shuffle, err = s.playbackShuffle(r, userID, rec.Shuffle)
		switch {
		case errors.Is(err, errSessionGone):
			out.Kind = string(playback.Songs)
		case err != nil:
			return api.Playback{}, err
		}
	}
	return out, nil
}

// errSessionGone means a remembered shuffle can't go on: the caller no longer
// has its session, or its scope no longer reads.
var errSessionGone = errors.New("shuffle session gone")

// playbackShuffle returns the remembered shuffle with what it draws from and
// how, or an error wrapping [errSessionGone] when it can't go on.
func (s *Server) playbackShuffle(r *http.Request, userID int64, at *playback.ShuffleAt) (*api.PlaybackShuffle, error) {
	sess, err := s.library.ShuffleSessionOf(r.Context(), userID, at.ID)
	if errors.Is(err, store.ErrNotFound) {
		return nil, fmt.Errorf("%w: %w", errSessionGone, err)
	}
	if err != nil {
		return nil, err //nolint:wrapcheck // The store's error names what failed.
	}
	scope, err := shuffler.ParseScope(sess.Scope)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", errSessionGone, err)
	}
	out := &api.PlaybackShuffle{
		ID: at.ID, After: at.After, Mode: sess.Mode,
		Scope: api.ShuffleScope{Kind: string(scope.Kind), ID: scope.ID, Tracks: scope.Tracks},
	}
	return out, nil
}

// toAPIPlayback returns what rec, resolved, says of itself: all but its
// continues and its session, which need the store. A shuffle whose order is
// gone has nothing to go back to, and comes back off.
func toAPIPlayback(rec *playback.Record, resolved *playback.Resolved) api.Playback {
	out := api.Playback{
		Kind: string(rec.Kind), Tracks: toAPIPlaybackSongs(resolved.Tracks), Later: toAPIPlaybackSongs(resolved.Later),
		Index: resolved.Index, PositionMs: resolved.PositionMs, Ordered: rec.Ordered, Moved: resolved.Moved,
		Repeat: string(rec.Repeat), ClientID: rec.ClientID, Device: string(rec.Device),
	}
	if rec.Shuffled != nil && len(resolved.Order) > 0 {
		out.Shuffled = &api.PlaybackShuffled{Seed: rec.Shuffled.Seed, Order: toAPIPlaybackSongs(resolved.Order)}
	}
	return out
}

func toAPIPlaybackSongs(songs []playback.Song) []api.PlaybackSong {
	if songs == nil {
		return nil
	}
	out := make([]api.PlaybackSong, len(songs))
	for i, song := range songs {
		out[i] = api.PlaybackSong{ID: song.ID, AlbumID: song.AlbumID}
	}
	return out
}

// putPlayback replaces where the caller left off. The last write wins, and no
// event goes out.
func (s *Server) putPlayback(w http.ResponseWriter, r *http.Request) {
	rec, err := readPlayback(w, r)
	if err != nil {
		writeProblem(w, r, http.StatusBadRequest, "request body must be a JSON playback record of at most 1000 songs")
		return
	}
	stored, err := playback.Encode(&rec)
	if err != nil {
		s.internalError(w, r, "encode playback", err)
		return
	}
	user, _ := currentUser(r.Context()) // Always there, behind authenticate.
	if err = s.library.SetPlayback(r.Context(), user.ID, &stored); err != nil {
		if r.Context().Err() == nil {
			s.internalError(w, r, "set playback", err)
		}
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// errNotJSON means a request body isn't JSON.
var errNotJSON = errors.New("not application/json")

// readPlayback reads a record of at most [maxPlaybackBody] bytes, checked.
func readPlayback(w http.ResponseWriter, r *http.Request) (playback.Record, error) {
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != api.JSONMediaType {
		return playback.Record{}, errNotJSON
	}
	var req api.PlaybackSave
	err = json.UnmarshalRead(http.MaxBytesReader(w, r.Body, maxPlaybackBody), &req, json.RejectUnknownMembers(true))
	if err != nil {
		return playback.Record{}, err //nolint:wrapcheck // The caller only tells the client the body is bad.
	}
	rec := playback.Record{
		Kind: playback.Kind(req.Kind), Tracks: req.Tracks, Later: req.Later, Continues: req.Continues,
		Index: req.Index, PositionMs: req.PositionMs, Ordered: req.Ordered, Repeat: playback.Repeat(req.Repeat),
		ClientID: req.ClientID, Device: playback.Device(req.Device),
	}
	if rec.Repeat == "" {
		rec.Repeat = playback.RepeatOff
	}
	if req.Shuffle != nil {
		rec.Shuffle = &playback.ShuffleAt{ID: req.Shuffle.ID, After: req.Shuffle.After}
	}
	if req.Shuffled != nil {
		rec.Shuffled = &playback.Shuffled{Seed: req.Shuffled.Seed, Order: req.Shuffled.Order}
	}
	err = playback.Check(&rec)
	return rec, err //nolint:wrapcheck // As above.
}
