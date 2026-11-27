package api

import "time"

// ArtistImagePin is the body of PUT /admin/artists/{id}/images/{kind}: the
// candidate to pin, by the ID GET /admin/artists/{id}/images gave it.
type ArtistImagePin struct {
	Candidate string `json:"candidate"`
}

// ArtistImageCandidates is the response to GET /admin/artists/{id}/images:
// Choose artwork's candidates for one kind.
type ArtistImageCandidates struct {
	Kind       string            `json:"kind"`
	MBID       string            `json:"mbid,omitzero"`
	Pinned     bool              `json:"pinned"`
	Held       bool              `json:"held"`
	Candidates []ImageCandidate  `json:"candidates"`
	Sources    []CandidateSource `json:"sources"`
}

// ImageCandidate is one image an artist could show.
type ImageCandidate struct {
	ID       string     `json:"id"`
	Source   string     `json:"source"`
	Width    int        `json:"width"`
	Height   int        `json:"height"`
	Lang     string     `json:"lang,omitzero"`
	Credit   *Credit    `json:"credit,omitzero"`
	Preview  *Picture   `json:"preview,omitzero"`
	Chosen   bool       `json:"chosen"`
	Pinned   bool       `json:"pinned"`
	PinnedBy string     `json:"pinnedBy,omitzero"`
	PinnedAt *time.Time `json:"pinnedAt,omitzero"`
}

// CandidateSource is how one source stood for the candidates, with why it
// failed in words.
type CandidateSource struct {
	Source string `json:"source"`
	State  string `json:"state"`
	Words  string `json:"words,omitzero"`
}
