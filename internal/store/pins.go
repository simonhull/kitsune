package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/simonhull/kitsune/internal/store/sqlcdb"
)

// ArtworkUpload is an image an administrator uploaded.
const ArtworkUpload ArtworkSource = "upload"

// Where a pinned image came from, as artist_pins names it: the artist's folder,
// a source, or an upload.
const (
	PinFromFolder    = "folder"
	PinFromFanart    = ImageFromFanart
	PinFromWikimedia = ImageFromWikimedia
	PinFromUpload    = "upload"
)

// ParseArtistImageKind returns the kind s names, and whether it names one.
func ParseArtistImageKind(s string) (ArtistImageKind, bool) {
	switch k := ArtistImageKind(s); k {
	case ArtistThumb, ArtistBackground, ArtistLogo, ArtistBanner:
		return k, true
	}
	return "", false
}

// PinWrite is an administrator's choice of one image for one artist and kind.
type PinWrite struct {
	ArtistID int64
	Kind     ArtistImageKind
	Source   string // One of the PinFrom names.
	RemoteID string // The source's own ID for the image; "" for a folder's or an upload.
	Credit   *Credit
	Image    ArtworkWrite // Upserted with the pin.
	By       int64        // The administrator's user ID.
}

// validate checks w's source against what it carries, as artist_pins' CHECK
// does, so a bad write fails before it reaches SQL.
func (w *PinWrite) validate() error {
	fetched := w.Source == PinFromFanart || w.Source == PinFromWikimedia
	switch {
	case !fetched && w.Source != PinFromFolder && w.Source != PinFromUpload:
		return fmt.Errorf("pin from %q: %w", w.Source, ErrInvalidWrite)
	case fetched != (w.RemoteID != ""):
		return fmt.Errorf("pin from %q with remote ID %q: %w", w.Source, w.RemoteID, ErrInvalidWrite)
	case w.Image.Width <= 0 || w.Image.Height <= 0 || w.Image.Accent == "" || w.Image.Ground == "":
		return fmt.Errorf("pin's image: %w", ErrInvalidWrite)
	}
	if _, ok := ParseArtistImageKind(string(w.Kind)); !ok {
		return fmt.Errorf("pin of kind %q: %w", w.Kind, ErrInvalidWrite)
	}
	return nil
}

// PinArtistImage stores w in one transaction: the image, then the pin over
// whatever the artist's folder and the sources give of the kind, which stays
// beneath it. No scan or pass ever writes a pin.
func (s *Store) PinArtistImage(ctx context.Context, w *PinWrite, now time.Time) error {
	if err := w.validate(); err != nil {
		return err
	}
	err := s.inTx(ctx, func(q *sqlcdb.Queries, _ *sql.Tx) error {
		if err := upsertImage(ctx, q, &w.Image); err != nil {
			return err
		}
		return q.UpsertArtistPin(ctx, sqlcdb.UpsertArtistPinParams{ //nolint:wrapcheck // Wrapped below.
			ArtistID: w.ArtistID, Kind: string(w.Kind), ArtworkHash: w.Image.Hash[:], Source: w.Source,
			RemoteID: orNil(w.RemoteID), Credit: creditJSON(w.Credit), PinnedBy: &w.By, PinnedAt: ms(now),
		})
	})
	if err != nil {
		return fmt.Errorf("pin artist %d's %s: %w", w.ArtistID, w.Kind, err)
	}
	return nil
}

// UnpinArtistImage takes away the artist's pin of kind, if it has one, and
// reports whether it had. In the same write it brings forward the next ask of
// every source about the artist, so the pass fills what the pin kept it from.
func (s *Store) UnpinArtistImage(ctx context.Context, artistID int64, kind ArtistImageKind, now time.Time) (bool, error) {
	var gone bool
	err := s.inTx(ctx, func(q *sqlcdb.Queries, _ *sql.Tx) error {
		n, err := q.DeleteArtistPin(ctx, sqlcdb.DeleteArtistPinParams{ArtistID: artistID, Kind: string(kind)})
		if err != nil {
			return fmt.Errorf("delete: %w", err)
		}
		if gone = n > 0; gone {
			return askAgainFor(ctx, q, artistID, now)
		}
		return nil
	})
	if err != nil {
		return false, fmt.Errorf("unpin artist %d's %s: %w", artistID, kind, err)
	}
	return gone, nil
}

// Pin is an artist's pinned image of one kind.
type Pin struct {
	Picture

	Source   string // One of the PinFrom names.
	RemoteID string
	Credit   *Credit
	By       string // The administrator's name; "" once their account is gone.
	At       time.Time
}

// ArtistPin returns the artist's pin of kind, or nil when it has none.
func (s *Store) ArtistPin(ctx context.Context, artistID int64, kind ArtistImageKind) (*Pin, error) {
	r, err := s.rq.GetArtistPin(ctx, sqlcdb.GetArtistPinParams{ArtistID: artistID, Kind: string(kind)})
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil //nolint:nilnil // No pin is no error.
	}
	if err != nil {
		return nil, fmt.Errorf("artist %d's %s pin: %w", artistID, kind, err)
	}
	return &Pin{
		Picture: picture(r.ArtworkHash, r.Width, r.Height, r.Accent, r.Ground, r.Alpha),
		Source:  r.Source, RemoteID: strOrEmpty(r.RemoteID), Credit: credit(r.Credit),
		By: strOrEmpty(r.PinnedBy), At: fromMS(r.PinnedAt),
	}, nil
}

// Beneath is what the artist's folder or a source gives the artist of one
// kind, whether or not a pin lies over it.
type Beneath struct {
	ExtraImage

	RemoteID string // The source's own ID; "" for a folder's.
}

// ArtistImageBeneath returns what the artist's folder or the sources give it of
// kind, beneath any pin, or nil when they give nothing.
func (s *Store) ArtistImageBeneath(ctx context.Context, artistID int64, kind ArtistImageKind) (*Beneath, error) {
	r, err := s.rq.GetArtistImageBeneath(ctx, sqlcdb.GetArtistImageBeneathParams{ArtistID: artistID, Kind: string(kind)})
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil //nolint:nilnil // Nothing beneath is no error.
	}
	if err != nil {
		return nil, fmt.Errorf("artist %d's %s: %w", artistID, kind, err)
	}
	return &Beneath{
		ExtraImage: ExtraImage{
			Picture: picture(r.ArtworkHash, r.Width, r.Height, r.Accent, r.Ground, r.Alpha),
			Source:  r.Source, Credit: credit(r.Credit),
		},
		RemoteID: strOrEmpty(r.RemoteID),
	}, nil
}

// HoldArtist is Not this artist: in one write it takes away every image and
// bio the sources gave the artist, and any pin of one of them, and holds back
// mbid, so the pass asks no source about the artist while its MBID is that
// one. What the artist's folder gave stays, as do pins of an upload or a
// folder's image.
func (s *Store) HoldArtist(ctx context.Context, artistID int64, mbid string, by int64, now time.Time) error {
	if len(mbid) != mbidLen {
		return fmt.Errorf("hold artist %d by MBID %q: %w", artistID, mbid, ErrInvalidWrite)
	}
	err := s.inTx(ctx, func(q *sqlcdb.Queries, _ *sql.Tx) error {
		if _, err := q.DeleteFetchedArtistImages(ctx, artistID); err != nil {
			return fmt.Errorf("images: %w", err)
		}
		if _, err := q.DeleteFetchedArtistPins(ctx, artistID); err != nil {
			return fmt.Errorf("pins: %w", err)
		}
		if _, err := q.DeleteFetchedBio(ctx, artistID); err != nil {
			return fmt.Errorf("bio: %w", err)
		}
		err := q.UpsertArtistHold(ctx, sqlcdb.UpsertArtistHoldParams{ArtistID: artistID, Mbid: mbid, HeldBy: &by, HeldAt: ms(now)})
		if err != nil {
			return fmt.Errorf("hold: %w", err)
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("hold artist %d: %w", artistID, err)
	}
	return nil
}

// mbidLen is the length of a MusicBrainz ID, a UUID as text.
const mbidLen = 36

// ArtworkPreview returns the preview of a source's image, made at or after
// notBefore, or nil when there is none that fresh.
func (s *Store) ArtworkPreview(ctx context.Context, source, remoteID string, notBefore time.Time) (*Picture, error) {
	r, err := s.rq.GetArtworkPreview(ctx, sqlcdb.GetArtworkPreviewParams{Source: source, RemoteID: remoteID, NotBefore: ms(notBefore)})
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil //nolint:nilnil // None fresh is no error.
	}
	if err != nil {
		return nil, fmt.Errorf("preview of %s %s: %w", source, remoteID, err)
	}
	p := picture(r.Hash, r.Width, r.Height, r.Accent, r.Ground, r.Alpha)
	return &p, nil
}

// PutArtworkPreview notes p as the preview of a source's image, made at now.
func (s *Store) PutArtworkPreview(ctx context.Context, source, remoteID string, p *Picture, now time.Time) error {
	err := s.inTx(ctx, func(q *sqlcdb.Queries, _ *sql.Tx) error {
		return q.UpsertArtworkPreview(ctx, sqlcdb.UpsertArtworkPreviewParams{ //nolint:wrapcheck // Wrapped below.
			Source: source, RemoteID: remoteID, Hash: p.Hash[:], Width: int64(p.Width), Height: int64(p.Height),
			Accent: p.Accent, Ground: p.Ground, Alpha: flag(p.Alpha), MadeAt: ms(now),
		})
	})
	if err != nil {
		return fmt.Errorf("note preview of %s %s: %w", source, remoteID, err)
	}
	return nil
}

// SweepPreviews forgets the previews made before before, and returns the
// hashes among them nothing uses any more (no artwork row, no fresher
// preview), whose files the caller removes.
func (s *Store) SweepPreviews(ctx context.Context, before time.Time) ([][32]byte, error) {
	var unused [][32]byte
	err := s.inTx(ctx, func(q *sqlcdb.Queries, _ *sql.Tx) error {
		unused = unused[:0] // The writer may retry: the last try says.
		hashes, err := q.DeleteOldPreviews(ctx, ms(before))
		if err != nil {
			return fmt.Errorf("delete: %w", err)
		}
		for _, h := range hashes {
			used, err := q.HashInUse(ctx, h)
			if err != nil {
				return fmt.Errorf("in use: %w", err)
			}
			if used == 0 {
				unused = append(unused, [32]byte(h))
			}
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("sweep previews: %w", err)
	}
	return unused, nil
}
