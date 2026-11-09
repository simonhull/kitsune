package matching

import (
	"strings"
	"unicode"
)

// Query is what a search asks a source for. Empty fields aren't asked.
type Query struct {
	Artist, Album string
	// Catalog is a catalog number, and Barcode a barcode: what's on the spine
	// or the back.
	Catalog, Barcode string
	// loose asks for any of the words of the album and artist, not the phrases.
	loose bool
}

// Empty reports whether q asks for nothing.
func (q *Query) Empty() bool {
	return strings.TrimSpace(q.Artist+q.Album+q.Catalog+q.Barcode) == ""
}

// minBarcode is the fewest digits a barcode has: EAN-8.
const minBarcode = 8

// TermsQuery returns the query for what a person typed: an album, an artist,
// and a catalog number or barcode in one box, which is a barcode when it is
// all digits and long enough.
func TermsQuery(artist, album, code string) Query {
	code = strings.TrimSpace(code)
	q := Query{Artist: strings.TrimSpace(artist), Album: strings.TrimSpace(album)}
	if len(code) >= minBarcode && strings.IndexFunc(code, func(r rune) bool { return r < '0' || r > '9' }) < 0 {
		q.Barcode = code
	} else {
		q.Catalog = code
	}
	return q
}

// loosened returns q asking for words, not phrases.
func (q *Query) loosened() Query {
	loose := *q
	loose.loose = true
	return loose
}

// lucene returns q in MusicBrainz's search syntax: each field it has, as a
// phrase, all of them required.
func (q *Query) lucene() string {
	var parts []string
	for _, f := range []struct{ field, value string }{
		{"release", q.Album}, {"artist", q.Artist}, {"catno", q.Catalog}, {"barcode", q.Barcode},
	} {
		v := strings.TrimSpace(f.value)
		switch {
		case v == "":
		case q.loose && (f.field == "release" || f.field == "artist"):
			parts = append(parts, f.field+":("+words(v)+")")
		default:
			parts = append(parts, f.field+":"+phrase(v))
		}
	}
	return strings.Join(parts, " AND ")
}

// luceneSpecial are the characters Lucene's syntax gives a meaning.
const luceneSpecial = `+-&|!(){}[]^"~*?:\/`

// words returns v's words for Lucene, each escaped, any of which may match.
func words(v string) string {
	var b strings.Builder
	for _, r := range v {
		switch {
		case unicode.IsControl(r):
			b.WriteByte(' ')
		case strings.ContainsRune(luceneSpecial, r):
			b.WriteByte('\\')
			b.WriteRune(r)
		default:
			b.WriteRune(r)
		}
	}
	return strings.Join(strings.Fields(b.String()), " ")
}

// phrase quotes v as a Lucene phrase.
func phrase(v string) string {
	v = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, v)
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(v) + `"`
}
