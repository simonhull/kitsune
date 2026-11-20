package library

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/simonhull/kitsune/internal/store"
)

// maxNfoBytes caps an artist.nfo a scan reads.
const maxNfoBytes = 1 << 20

// Why an artist.nfo gives no bio.
var (
	errNfoUnreadable = errors.New("not an artist.nfo Kitsune can read")
	errNfoNoBio      = errors.New("holds no biography")
	errNfoOtherMBID  = errors.New("names another artist's MusicBrainz ID")
)

// nfoArtist is what Kitsune reads of Kodi's artist.nfo: the biography, and
// the MusicBrainz ID that says whose it is.
type nfoArtist struct {
	Biography, MBID string
}

// parseNfo reads the bio of an artist.nfo for the artist with MusicBrainz ID
// mbid ("" when unknown). Nothing is inferred: the body is the biography as
// written, its lines as paragraphs, and the summary is its first paragraph. An
// nfo naming another MusicBrainz ID is another artist's, and gives nothing.
func parseNfo(data []byte, mbid string) (*store.NfoBio, error) {
	a, err := readNfo(data)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", errNfoUnreadable, err)
	}
	if mbid != "" && a.MBID != "" && !strings.EqualFold(a.MBID, mbid) {
		return nil, errNfoOtherMBID
	}
	paras := paragraphs(a.Biography)
	if len(paras) == 0 {
		return nil, errNfoNoBio
	}
	return &store.NfoBio{Summary: paras[0], Body: strings.Join(paras, "\n\n")}, nil
}

// errNotArtist means an nfo's root element is not <artist>.
var errNotArtist = errors.New("its root is not <artist>")

// rootChild is the depth of an element directly under the root: Kitsune
// reads only those.
const rootChild = 2

// readNfo reads the text of <biography> and <musicBrainzArtistID> directly
// under the <artist> root, matching names ignoring case, as Kodi does. The
// decoder is lenient: Kodi's own files, and those of tools that write them,
// aren't always well formed.
func readNfo(data []byte) (nfoArtist, error) {
	d := xml.NewDecoder(bytes.NewReader(data))
	d.Strict = false
	d.Entity = xml.HTMLEntity
	var r nfoReader
	for {
		tok, err := d.Token()
		switch {
		case errors.Is(err, io.EOF):
			return r.artist, nil
		case err != nil:
			return r.artist, err //nolint:wrapcheck // parseNfo wraps it with what failed.
		}
		if terr := r.take(tok); terr != nil {
			return r.artist, terr
		}
	}
}

// nfoReader follows an nfo's elements, keeping the text it wants.
type nfoReader struct {
	artist nfoArtist
	path   []string // Lower-case names of the open elements.
}

// take follows one token.
func (r *nfoReader) take(tok xml.Token) error {
	switch t := tok.(type) {
	case xml.StartElement:
		name := strings.ToLower(t.Name.Local)
		if len(r.path) == 0 && name != "artist" {
			return errNotArtist
		}
		r.path = append(r.path, name)
	case xml.EndElement:
		if len(r.path) > 0 {
			r.path = r.path[:len(r.path)-1]
		}
	case xml.CharData:
		if len(r.path) == rootChild {
			r.text(r.path[rootChild-1], string(t))
		}
	}
	return nil
}

// text keeps the text of the root's child element.
func (r *nfoReader) text(element, text string) {
	switch element {
	case "biography":
		r.artist.Biography += text
	case "musicbrainzartistid":
		r.artist.MBID += strings.TrimSpace(text)
	}
}

// paragraphs splits text into its non-empty lines, each trimmed and made
// valid UTF-8.
func paragraphs(text string) []string {
	var out []string
	for line := range strings.Lines(strings.ReplaceAll(text, "\r", "\n")) {
		if p := strings.TrimSpace(strings.ToValidUTF8(line, "�")); p != "" {
			out = append(out, p)
		}
	}
	return out
}
