package fanarttv

import (
	"bytes"
	"strconv"
)

// artistJSON is an artist as fanart.tv's JSON has it, as much as Kitsune reads.
type artistJSON struct {
	Status        string      `json:"status"` // "error" when it has none.
	Thumbs        []imageJSON `json:"artistthumb"`
	Backgrounds4K []imageJSON `json:"artist4kbackground"`
	Backgrounds   []imageJSON `json:"artistbackground"`
	HDLogos       []imageJSON `json:"hdmusiclogo"`
	Logos         []imageJSON `json:"musiclogo"`
	Banners       []imageJSON `json:"musicbanner"`
}

// imageJSON is one image. fanart.tv has sent its numbers both as JSON numbers
// and as strings.
type imageJSON struct {
	ID    looseString `json:"id"`
	URL   string      `json:"url"`
	Lang  string      `json:"lang"`
	Likes looseInt    `json:"likes"`
}

// looseString reads a JSON string or number as text.
type looseString string

// UnmarshalJSON implements json.Unmarshaler.
func (s *looseString) UnmarshalJSON(b []byte) error {
	*s = looseString(bytes.Trim(b, `"`))
	return nil
}

// looseInt reads a JSON number, or a string holding one; anything else is 0.
type looseInt int

// UnmarshalJSON implements json.Unmarshaler.
func (n *looseInt) UnmarshalJSON(b []byte) error {
	v, err := strconv.Atoi(string(bytes.Trim(b, `"`)))
	if err != nil {
		v = 0
	}
	*n = looseInt(v)
	return nil
}
