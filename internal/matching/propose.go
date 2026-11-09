package matching

import (
	"slices"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/simonhull/kitsune/internal/library"
)

// Values holds proposed values by field, as an edit takes them.
type Values map[library.Field][]string

// Proposal is what a release would write. A field the release has nothing for
// is left out: matching proposes values, never taking one away.
type Proposal struct {
	// Album goes to every track: the album's name, artists, dates, labels,
	// catalog numbers, barcode and genres.
	Album Values
	// AlbumIDs go to every track too, and are always written: the release,
	// its group and its artists.
	AlbumIDs Values
	// Tracks are the release's, medium by medium.
	Tracks []ProposedTrack
}

// ProposedTrack is what one release track would write to the file paired with it.
type ProposedTrack struct {
	Medium   Medium // Without its tracks.
	Track    ReleaseTrack
	Proposed Values // Title, artists, numbers, the disc's name and ISRCs.
	IDs      Values // Recording, track and artists: always written.
}

// mostGenres is how many of a release's genres are proposed.
const mostGenres = 3

// propose returns what the release r would write.
func propose(r *Release) Proposal {
	p := Proposal{Album: Values{}, AlbumIDs: Values{}}
	put(p.Album, library.FieldAlbum, r.Title)
	putAll(p.Album, library.FieldAlbumArtists, names(r.Artists))
	put(p.Album, library.FieldDate, r.Date)
	put(p.Album, library.FieldOriginalDate, r.Original)
	var labels, catalogs []string
	for _, l := range r.Labels {
		labels = appendNew(labels, l.Name)
		catalogs = appendNew(catalogs, l.CatalogNumber)
	}
	putAll(p.Album, library.FieldLabel, labels)
	putAll(p.Album, library.FieldCatalogNumber, catalogs)
	put(p.Album, library.FieldBarcode, r.Barcode)
	genres := make([]string, 0, mostGenres)
	for _, g := range r.Genres[:min(len(r.Genres), mostGenres)] {
		genres = appendNew(genres, sentence(g))
	}
	putAll(p.Album, library.FieldGenres, genres)
	put(p.AlbumIDs, library.FieldMusicBrainzAlbumID, r.ID)
	put(p.AlbumIDs, library.FieldMusicBrainzReleaseGroupID, r.GroupID)
	putAll(p.AlbumIDs, library.FieldMusicBrainzAlbumArtistID, ids(r.Artists))
	for _, f := range r.tracks() {
		t := ProposedTrack{Medium: *f.medium, Track: *f.track, Proposed: Values{}, IDs: Values{}}
		t.Medium.Tracks = nil
		put(t.Proposed, library.FieldTitle, f.track.Title)
		putAll(t.Proposed, library.FieldArtists, names(f.track.Artists))
		putNumber(t.Proposed, library.FieldTrack, f.track.Position)
		putNumber(t.Proposed, library.FieldTrackTotal, f.medium.TrackCount)
		putNumber(t.Proposed, library.FieldDisc, f.medium.Position)
		putNumber(t.Proposed, library.FieldDiscTotal, len(r.Media))
		put(t.Proposed, library.FieldDiscSubtitle, f.medium.Title)
		putAll(t.Proposed, library.FieldISRC, f.track.ISRCs)
		put(t.IDs, library.FieldMusicBrainzRecordingID, f.track.RecordingID)
		put(t.IDs, library.FieldMusicBrainzTrackID, f.track.ID)
		putAll(t.IDs, library.FieldMusicBrainzArtistID, ids(f.track.Artists))
		p.Tracks = append(p.Tracks, t)
	}
	return p
}

// put proposes v for field, unless it is empty.
func put(values Values, field library.Field, v string) {
	if v = strings.TrimSpace(v); v != "" {
		values[field] = []string{v}
	}
}

// putAll proposes vs for field, unless there are none.
func putAll(values Values, field library.Field, vs []string) {
	var kept []string
	for _, v := range vs {
		kept = appendNew(kept, v)
	}
	if len(kept) > 0 {
		values[field] = kept
	}
}

// putNumber proposes n for field, unless it is unknown.
func putNumber(values Values, field library.Field, n int) {
	if n > 0 {
		values[field] = []string{strconv.Itoa(n)}
	}
}

// appendNew appends v, trimmed, unless it is empty or already there.
func appendNew(list []string, v string) []string {
	if v = strings.TrimSpace(v); v == "" || slices.Contains(list, v) {
		return list
	}
	return append(list, v)
}

// sentence returns a genre as a tag holds one: MusicBrainz's are lower-case,
// and "dream pop" reads as "Dream pop".
func sentence(genre string) string {
	r, size := utf8.DecodeRuneInString(genre)
	if size == 0 {
		return genre
	}
	return string(unicode.ToUpper(r)) + genre[size:]
}
