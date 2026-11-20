package server

import (
	"encoding/hex"
	"errors"
	"net/http"

	"github.com/simonhull/kitsune/api"
	"github.com/simonhull/kitsune/internal/store"
)

// getArtists lists the artists who own an album, whole: a library has few enough.
func (s *Server) getArtists(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r.Context()) // Always there, behind authenticate.
	artists, err := s.library.Artists(r.Context(), user.ID)
	if err != nil {
		s.internalError(w, r, "list artists", err)
		return
	}
	body := api.ArtistList{Artists: make([]api.ArtistEntry, len(artists))}
	for i := range artists {
		a := &artists[i]
		body.Artists[i] = api.ArtistEntry{
			ID: a.ID, Name: a.Name, AlbumCount: a.AlbumCount, AddedAt: a.AddedAt, Plays: a.Plays,
			Thumb: toAPIPicture(a.Thumb),
		}
	}
	writeBody(w, http.StatusOK, api.JSONMediaType, &body)
}

// getArtist returns one artist with its albums.
func (s *Server) getArtist(w http.ResponseWriter, r *http.Request) {
	d, err := s.library.Artist(r.Context(), r.PathValue("id"))
	if errors.Is(err, store.ErrNotFound) {
		writeProblem(w, r, http.StatusNotFound, "no such artist")
		return
	}
	if err != nil {
		s.internalError(w, r, "get artist", err)
		return
	}
	body := api.ArtistDetail{
		ID: d.ID, Name: d.Name, AlbumCount: d.AlbumCount, SongCount: d.SongCount,
		Albums: s.toAPIAlbumEntries(d.Albums), AppearsOn: s.toAPIAlbumEntries(d.AppearsOn),
		MBID: d.MBID, Images: toAPIArtistImages(&d.Images), Bio: toAPIBio(d.Bio),
	}
	writeBody(w, http.StatusOK, api.JSONMediaType, &body)
}

func toAPIArtistLinks(links []store.ArtistLink) []api.ArtistLink {
	out := make([]api.ArtistLink, len(links))
	for i := range links {
		out[i] = api.ArtistLink{ID: links[i].ID, Name: links[i].Name}
	}
	return out
}

func (s *Server) toAPIAlbumEntries(albums []store.AlbumSummary) []api.AlbumEntry {
	out := make([]api.AlbumEntry, len(albums))
	for i := range albums {
		out[i] = s.toAPIAlbumEntry(&albums[i])
	}
	return out
}

// toAPIPicture is p on the wire, or nil.
func toAPIPicture(p *store.Picture) *api.Picture {
	if p == nil {
		return nil
	}
	return &api.Picture{
		Hash: hex.EncodeToString(p.Hash[:]), Accent: p.Accent, Ground: p.Ground,
		Width: p.Width, Height: p.Height, Alpha: p.Alpha,
	}
}

// toAPIExtraImage is img on the wire, or nil.
func toAPIExtraImage(img *store.ExtraImage) *api.ExtraImage {
	if img == nil {
		return nil
	}
	out := &api.ExtraImage{
		Hash: hex.EncodeToString(img.Hash[:]), Accent: img.Accent, Ground: img.Ground,
		Width: img.Width, Height: img.Height, Alpha: img.Alpha, Source: img.Source,
	}
	if c := img.Credit; c != nil {
		out.Credit = &api.Credit{Author: c.Author, License: c.License, LicenseURL: c.LicenseURL, PageURL: c.PageURL}
	}
	return out
}

func toAPIArtistImages(imgs *store.ArtistImages) api.ArtistImages {
	return api.ArtistImages{
		Thumb: toAPIExtraImage(imgs.Thumb), Background: toAPIExtraImage(imgs.Background),
		Logo: toAPIExtraImage(imgs.Logo), Banner: toAPIExtraImage(imgs.Banner),
	}
}

// toAPIBio is b on the wire, or nil.
func toAPIBio(b *store.Bio) *api.ArtistBio {
	if b == nil {
		return nil
	}
	return &api.ArtistBio{
		Summary: b.Summary, Body: b.Body, Source: b.Source, URL: b.URL,
		License: b.License, LicenseURL: b.LicenseURL, Lang: b.Lang,
	}
}

// toAPIAlbumExtras is an album's extras on the wire; hasCover says it has a
// cover of its own.
func toAPIAlbumExtras(ex *store.AlbumExtras, hasCover bool) api.AlbumExtras {
	out := api.AlbumExtras{Discs: make([]api.DiscArt, 0, len(ex.Discs)), Back: toAPIExtraImage(ex.Back)}
	for i := range ex.Discs {
		out.Discs = append(out.Discs, api.DiscArt{Disc: ex.Discs[i].Disc, Image: *toAPIExtraImage(&ex.Discs[i].ExtraImage)})
	}
	if hasCover {
		out.CoverSource = api.CoverFromFiles
	}
	return out
}
