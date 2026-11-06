package library

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"slices"
	"strings"

	"github.com/simonhull/audiometa"

	"github.com/simonhull/kitsune/internal/store"
)

// TrackTags is what a tag editor shows of one track, read from its file.
type TrackTags struct {
	ID     string // Track ULID.
	File   string // Base name.
	Fields map[Field][]string
	// ReadOnly lists the fields whose values can be shown but not replaced.
	ReadOnly []Field
	Tags     []RawTag // Everything no field covers, by key.
}

// RawTag is a tag no field covers, under its format's own key.
type RawTag struct {
	Key    string
	Values []string // A long value is cut, and ReadOnly.
	// ReadOnly means no edit may touch it: it holds gain, a fingerprint, a
	// picture, chapters or binary data, a field has a say in it, or the file's
	// format can't be written.
	ReadOnly bool
}

// AlbumTags reads the tags of the album id from its files, and returns them in
// album order with the album's ETag. The store can't answer this: it lacks what
// audiometa keeps out of the raw tags, an MP3's comment and an M4A's numbers, and
// holds what an older audiometa read until a Full scan. It takes no lock: a write
// replaces a file whole.
//
// It returns an error wrapping [store.ErrNotFound] for an unknown ID, and
// [ErrStale] when a file is gone.
func (s *Scanner) AlbumTags(ctx context.Context, id string) ([]TrackTags, string, error) {
	files, etag, err := s.st.AlbumEditFiles(ctx, id)
	if err != nil {
		return nil, "", err //nolint:wrapcheck // The store's error already says what failed.
	}
	out := make([]TrackTags, len(files))
	for i := range files {
		if out[i], err = readTags(ctx, &files[i]); err != nil {
			return nil, "", err
		}
	}
	return out, etag, nil
}

func readTags(ctx context.Context, file *store.EditFile) (TrackTags, error) {
	f, err := audiometa.OpenContext(ctx, filepath.Join(file.Root, file.RelPath))
	if errors.Is(err, fs.ErrNotExist) {
		return TrackTags{}, ErrStale
	}
	if err != nil {
		return TrackTags{}, fmt.Errorf("read tags: %w", err)
	}
	defer func() { _ = f.Close() }() // Only read.
	tags := TrackTags{ID: file.ULID, File: filepath.Base(file.RelPath), Fields: map[Field][]string{}}
	fam, err := familyOf(f)
	switch {
	case errors.Is(err, ErrUnsupportedEdit): // Readable, but no field has keys in it.
		tags.Tags = otherTags(&f.Tags, nil, func(string, []string) bool { return false })
		return tags, nil
	case err != nil:
		return TrackTags{}, err
	}
	covered := make(map[string]bool)
	for i := range fields {
		row := &fields[i]
		if values := row.values(&f.Tags, fam); len(values) > 0 {
			tags.Fields[row.name] = values
			if row.frozen != nil && row.frozen(values) {
				tags.ReadOnly = append(tags.ReadOnly, row.name)
			}
		}
		for _, key := range row.keys(fam) {
			covered[key] = true
		}
	}
	tags.Tags = otherTags(&f.Tags, covered, func(key string, values []string) bool {
		return rawEditable(fam, key, values, true)
	})
	return tags, nil
}

// otherTags lists the raw tags outside covered, by key.
func otherTags(t *audiometa.Tags, covered map[string]bool, editable func(string, []string) bool) []RawTag {
	var out []RawTag
	for key, values := range t.All() {
		cleaned := clean(values)
		if len(cleaned) == 0 || covered[key] {
			continue
		}
		visible, cut := shown(cleaned)
		out = append(out, RawTag{Key: tagKey(key), Values: visible, ReadOnly: cut || !editable(key, values)})
	}
	slices.SortFunc(out, func(a, b RawTag) int { return strings.Compare(a.Key, b.Key) })
	return out
}
