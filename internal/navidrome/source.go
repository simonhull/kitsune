// Package navidrome plans an import from a Navidrome database. It resolves
// Navidrome's songs, albums and artists against Kitsune's library, stamps its
// plays with times, and reports what it found. It writes nothing and speaks no
// SQL: [Load] reads through navidromedb, and the store writes the plan.
package navidrome

import (
	"context"
	"fmt"
	"slices"

	"github.com/simonhull/kitsune/internal/store/navidromedb"
)

// Source is everything an import reads from a Navidrome database.
type Source struct {
	Libraries []navidromedb.Library
	Users     []navidromedb.User
	Songs     []navidromedb.Song
	Marks     map[string]Marks     // By Navidrome user ID.
	Artists   []navidromedb.Artist // The ones somebody starred or rated.
}

// Marks is one Navidrome user's plays and marks.
type Marks struct {
	Annotations []navidromedb.Annotation
	Scrobbles   []navidromedb.Scrobble
	Left        navidromedb.Left
}

// Load reads everything an import needs from db.
func Load(ctx context.Context, db *navidromedb.DB) (Source, error) {
	var (
		src Source
		err error
	)
	if src.Libraries, err = db.Libraries(ctx); err != nil {
		return Source{}, fmt.Errorf("load: %w", err)
	}
	if src.Users, err = db.Users(ctx); err != nil {
		return Source{}, fmt.Errorf("load: %w", err)
	}
	if src.Songs, err = db.Songs(ctx); err != nil {
		return Source{}, fmt.Errorf("load: %w", err)
	}
	src.Marks = make(map[string]Marks, len(src.Users))
	var artistIDs []string
	for _, u := range src.Users {
		m, loadErr := loadMarks(ctx, db, u.ID)
		if loadErr != nil {
			return Source{}, fmt.Errorf("load: %w", loadErr)
		}
		src.Marks[u.ID] = m
		for i := range m.Annotations {
			if a := &m.Annotations[i]; a.ItemType == navidromedb.ItemArtist && marked(a) {
				artistIDs = append(artistIDs, a.ItemID)
			}
		}
	}
	slices.Sort(artistIDs)
	if src.Artists, err = db.Artists(ctx, slices.Compact(artistIDs)); err != nil {
		return Source{}, fmt.Errorf("load: %w", err)
	}
	return src, nil
}

// loadMarks reads one user's plays and marks.
func loadMarks(ctx context.Context, db *navidromedb.DB, userID string) (Marks, error) {
	var (
		m   Marks
		err error
	)
	if m.Annotations, err = db.Annotations(ctx, userID); err != nil {
		return Marks{}, err //nolint:wrapcheck // navidromedb names the table; Load wraps once.
	}
	if m.Scrobbles, err = db.Scrobbles(ctx, userID); err != nil {
		return Marks{}, err //nolint:wrapcheck // As above.
	}
	if m.Left, err = db.Left(ctx, userID); err != nil {
		return Marks{}, err //nolint:wrapcheck // As above.
	}
	return m, nil
}

// marked reports whether an annotation carries a star or a rating.
func marked(a *navidromedb.Annotation) bool { return a.Starred || a.Rating > 0 }
