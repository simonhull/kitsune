package server

import (
	"errors"
	"io"
	"mime"
	"net/http"
	"slices"

	"github.com/simonhull/kitsune/api"
	"github.com/simonhull/kitsune/internal/artwork"
	"github.com/simonhull/kitsune/internal/scrape"
	"github.com/simonhull/kitsune/internal/store"
)

// Choose artwork (plan 072, Phase 4): an administrator's picker of an artist's
// images, its pins and uploads, Look again now and Not this artist.

// maxUpload is the most an uploaded image may weigh: 32 MiB, as a source's.
const maxUpload = 32 << 20

// noArtDetail answers a picker route on a server with no artist pass.
const noArtDetail = "artist art isn't set up on this server"

// uploadTypes are the media types an upload may be: what artwork decodes.
func uploadTypes() []string { return []string{"image/jpeg", "image/png", "image/webp"} }

// pickerKind is the kind a request names, from its path or its query; it
// answers 400 for none it knows, and reports whether it did.
func pickerKind(w http.ResponseWriter, r *http.Request, raw string) (store.ArtistImageKind, bool) {
	kind, ok := store.ParseArtistImageKind(raw)
	if !ok {
		writeProblem(w, r, http.StatusBadRequest, "kind must be thumb, background, logo or banner")
	}
	return kind, ok
}

// getArtistImageCandidates lists what an artist could show of one kind.
func (s *Server) getArtistImageCandidates(w http.ResponseWriter, r *http.Request) {
	if s.scraper == nil {
		writeProblem(w, r, http.StatusServiceUnavailable, noArtDetail)
		return
	}
	kind, ok := pickerKind(w, r, r.URL.Query().Get("kind"))
	if !ok {
		return
	}
	got, err := s.scraper.Candidates(r.Context(), r.PathValue("id"), kind)
	if err != nil {
		s.pickerFailed(w, r, "list artist image candidates", err)
		return
	}
	body := api.ArtistImageCandidates{
		Kind: string(got.Kind), MBID: got.MBID, Pinned: got.Pinned, Held: got.Held,
		Candidates: make([]api.ImageCandidate, len(got.Offers)), Sources: make([]api.CandidateSource, len(got.Sources)),
	}
	for i := range got.Offers {
		body.Candidates[i] = toAPIImageCandidate(&got.Offers[i])
	}
	for i, src := range got.Sources {
		body.Sources[i] = api.CandidateSource{Source: src.Source, State: src.State, Words: src.Words}
	}
	w.Header().Set("Cache-Control", "no-store")
	writeBody(w, http.StatusOK, api.JSONMediaType, &body)
}

func toAPIImageCandidate(o *scrape.Offer) api.ImageCandidate {
	out := api.ImageCandidate{
		ID: o.ID, Source: o.Source, Width: o.Width, Height: o.Height, Lang: o.Lang,
		Credit: toAPICredit(o.Credit), Preview: toAPIPicture(o.Preview), Chosen: o.Chosen, Pinned: o.Pinned,
		PinnedBy: o.PinnedBy,
	}
	if o.Pinned {
		at := o.PinnedAt
		out.PinnedAt = &at
	}
	return out
}

// toAPICredit is c on the wire, or nil.
func toAPICredit(c *store.Credit) *api.Credit {
	if c == nil {
		return nil
	}
	return &api.Credit{Author: c.Author, License: c.License, LicenseURL: c.LicenseURL, PageURL: c.PageURL}
}

// putArtistImagePin pins a candidate.
func (s *Server) putArtistImagePin(w http.ResponseWriter, r *http.Request) {
	if s.scraper == nil {
		writeProblem(w, r, http.StatusServiceUnavailable, noArtDetail)
		return
	}
	kind, ok := pickerKind(w, r, r.PathValue("kind"))
	if !ok {
		return
	}
	var req api.ArtistImagePin
	if err := readJSON(w, r, &req); err != nil || req.Candidate == "" {
		writeProblem(w, r, http.StatusBadRequest, "send JSON naming the candidate to pin")
		return
	}
	user, _ := currentUser(r.Context())
	img, err := s.scraper.Pin(r.Context(), r.PathValue("id"), kind, req.Candidate, user.ID)
	if err != nil {
		s.pickerFailed(w, r, "pin an artist image", err)
		return
	}
	s.pinned(w, r, img)
}

// pinned publishes the artist's change and answers with the image it shows.
func (s *Server) pinned(w http.ResponseWriter, r *http.Request, img *store.ExtraImage) {
	s.publishArtist(r.PathValue("id"))
	writeBody(w, http.StatusOK, api.JSONMediaType, toAPIExtraImage(img))
}

// publishArtist tells everyone the artist id shows other images or another bio.
func (s *Server) publishArtist(id string) {
	NewLibraryEvents(s.events).ArtistsChanged([]string{id})
}

// deleteArtistImagePin unpins a kind: Automatic.
func (s *Server) deleteArtistImagePin(w http.ResponseWriter, r *http.Request) {
	if s.scraper == nil {
		writeProblem(w, r, http.StatusServiceUnavailable, noArtDetail)
		return
	}
	kind, ok := pickerKind(w, r, r.PathValue("kind"))
	if !ok {
		return
	}
	gone, err := s.scraper.Unpin(r.Context(), r.PathValue("id"), kind)
	if err != nil {
		s.pickerFailed(w, r, "unpin an artist image", err)
		return
	}
	if gone {
		s.publishArtist(r.PathValue("id"))
	}
	w.WriteHeader(http.StatusNoContent)
}

// postArtistImageUpload pins an uploaded image, sent as the body itself.
func (s *Server) postArtistImageUpload(w http.ResponseWriter, r *http.Request) {
	if s.scraper == nil {
		writeProblem(w, r, http.StatusServiceUnavailable, noArtDetail)
		return
	}
	kind, ok := pickerKind(w, r, r.PathValue("kind"))
	if !ok {
		return
	}
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || !isUploadType(mediaType) {
		writeProblem(w, r, http.StatusUnsupportedMediaType, "send the image itself, as image/jpeg, image/png or image/webp")
		return
	}
	data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxUpload))
	if _, tooLarge := errors.AsType[*http.MaxBytesError](err); tooLarge {
		writeProblem(w, r, http.StatusRequestEntityTooLarge, "an image may weigh at most 32 MiB")
		return
	}
	if err != nil {
		writeProblem(w, r, http.StatusBadRequest, "the image didn't arrive whole")
		return
	}
	user, _ := currentUser(r.Context())
	img, err := s.scraper.PinUpload(r.Context(), r.PathValue("id"), kind, data, user.ID)
	if err != nil {
		s.pickerFailed(w, r, "pin an uploaded artist image", err)
		return
	}
	s.pinned(w, r, img)
}

func isUploadType(mediaType string) bool { return slices.Contains(uploadTypes(), mediaType) }

// postArtistRefresh asks the sources about an artist again now.
func (s *Server) postArtistRefresh(w http.ResponseWriter, r *http.Request) {
	if s.scraper == nil {
		writeProblem(w, r, http.StatusServiceUnavailable, "the artist pass isn't set up on this server")
		return
	}
	if err := s.scraper.Refresh(r.Context(), r.PathValue("id")); err != nil {
		s.pickerFailed(w, r, "look again at an artist", err)
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

// postArtistReject is Not this artist.
func (s *Server) postArtistReject(w http.ResponseWriter, r *http.Request) {
	if s.scraper == nil {
		writeProblem(w, r, http.StatusServiceUnavailable, "the artist pass isn't set up on this server")
		return
	}
	user, _ := currentUser(r.Context())
	if err := s.scraper.Reject(r.Context(), r.PathValue("id"), user.ID); err != nil {
		s.pickerFailed(w, r, "hold an artist back", err)
		return
	}
	s.publishArtist(r.PathValue("id"))
	w.WriteHeader(http.StatusNoContent)
}

// pickerRefusal is a condition the picker's work refuses, and how it is answered.
type pickerRefusal struct {
	err    error
	status int
	detail string
}

// pickerRefusals are the picker's refusals, in the order they're matched.
func pickerRefusals() []pickerRefusal {
	return []pickerRefusal{
		{store.ErrNotFound, http.StatusNotFound, "no such artist"},
		{scrape.ErrNoCandidate, http.StatusUnprocessableEntity, "no candidate of that kind has that ID now; list them again"},
		{artwork.ErrBadImage, http.StatusUnprocessableEntity, "that isn't an image Kitsune can read"},
		{artwork.ErrTooLarge, http.StatusUnprocessableEntity, "that image has more than 64 megapixels"},
		{scrape.ErrSourceFailed, http.StatusServiceUnavailable, "the source didn't give the image; try again in a while"},
		{scrape.ErrNoArtwork, http.StatusServiceUnavailable, noArtDetail},
		{scrape.ErrLookupsOff, http.StatusConflict, "artist lookups are off: turn them on in Health"},
		{scrape.ErrNoMBID, http.StatusConflict, "this artist has no MusicBrainz ID: match its albums to give it one"},
		{scrape.ErrHeld, http.StatusConflict, "Not this artist holds it back until its MusicBrainz ID changes"},
	}
}

// pickerFailed answers what the picker's work refused, or the server's error.
func (s *Server) pickerFailed(w http.ResponseWriter, r *http.Request, what string, err error) {
	if r.Context().Err() != nil {
		return // The client went: no one hears an answer.
	}
	for _, f := range pickerRefusals() {
		if errors.Is(err, f.err) {
			writeProblem(w, r, f.status, f.detail)
			return
		}
	}
	s.internalError(w, r, what, err)
}
