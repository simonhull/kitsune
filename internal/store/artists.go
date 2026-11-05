package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/simonhull/kitsune/internal/store/sqlcdb"
)

// ArtistSummary is an artist as a list shows it.
type ArtistSummary struct {
	ID         string // The artist's ULID.
	Name       string
	AlbumCount int // The albums the artist owns.
}

// ArtistDetail is an artist with its albums, newest first. Albums are the ones
// the artist owns: as an album artist, or as the first artist of an album that
// names none. AppearsOn are the others it has a track on. A joined name is one
// artist, so a person's page leaves out an album shared under such a name.
type ArtistDetail struct {
	ArtistSummary

	Albums, AppearsOn []AlbumSummary
}

// Artists returns every artist who owns an album with a live track, whole,
// ordered by sort name ignoring case. Various Artists is not one of them.
func (s *Store) Artists(ctx context.Context) ([]ArtistSummary, error) {
	rows, err := s.rq.ListOwningArtists(ctx)
	if err != nil {
		return nil, fmt.Errorf("artists: %w", err)
	}
	out := make([]ArtistSummary, 0, len(rows))
	for i := range rows {
		r := &rows[i]
		if !isVarious(r.MatchKey, r.Mbid) {
			out = append(out, ArtistSummary{ID: r.Ulid, Name: r.Name, AlbumCount: int(r.AlbumCount)})
		}
	}
	return out, nil
}

// Artist returns the artist with the public ID id. It returns an error wrapping
// [ErrNotFound] for an unknown ID, an artist with no live track, or Various Artists.
func (s *Store) Artist(ctx context.Context, id string) (ArtistDetail, error) {
	row, err := s.rq.GetArtist(ctx, id)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && isVarious(row.MatchKey, row.Mbid)) {
		return ArtistDetail{}, fmt.Errorf("artist %s: %w", id, ErrNotFound)
	}
	if err != nil {
		return ArtistDetail{}, fmt.Errorf("artist %s: %w", id, err)
	}
	albums, err := s.rq.ListArtistAlbums(ctx, row.ID)
	if err != nil {
		return ArtistDetail{}, fmt.Errorf("artist %s: albums: %w", id, err)
	}
	d := ArtistDetail{ID: row.Ulid, Name: row.Name}
	for i := range albums {
		a := &albums[i]
		summary := summarize(&sqlcdb.ListAlbumsByNameRow{
			ID: a.ID, Ulid: a.Ulid, Name: a.Name, SortName: a.SortName, Artist: a.Artist, Year: a.Year,
			Compilation: a.Compilation, AddedAt: a.AddedAt, ArtworkHash: a.ArtworkHash, ArtworkWidth: a.ArtworkWidth,
			ArtworkHeight: a.ArtworkHeight, ArtworkAccent: a.ArtworkAccent, ArtworkGround: a.ArtworkGround,
			TrackCount: a.TrackCount,
		})
		if a.Owned != 0 {
			d.Albums = append(d.Albums, summary)
		} else {
			d.AppearsOn = append(d.AppearsOn, summary)
		}
	}
	d.AlbumCount = len(d.Albums)
	return d, nil
}
