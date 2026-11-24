package api

import "time"

// MetadataStatus is the response to GET /admin/metadata, Health's Metadata
// card: the master switch, how each source stands, where the artist pass
// stands, and the library's album artists by what the pass has of them.
type MetadataStatus struct {
	Remote      bool             `json:"remote"`
	Language    string           `json:"language"`
	Pass        MetadataPass     `json:"pass"`
	Sources     []MetadataSource `json:"sources"`
	Artists     MetadataArtists  `json:"artists"`
	NextRefresh *time.Time       `json:"nextRefresh,omitzero"`
}

// MetadataPass is where the artist pass stands.
type MetadataPass struct {
	Running bool        `json:"running"`
	Done    int         `json:"done"`
	Total   int         `json:"total"`
	Artist  *ArtistLink `json:"artist,omitzero"`
}

// MetadataSource is how one source stands. Neither key is ever sent: only
// whether it is set.
type MetadataSource struct {
	Name      string           `json:"name"`
	Enabled   bool             `json:"enabled"`
	State     string           `json:"state"`
	Key       string           `json:"key"`
	ClientKey string           `json:"clientKey"`
	Until     *time.Time       `json:"until,omitzero"`
	Failure   *MetadataFailure `json:"failure,omitzero"`
}

// MetadataFailure is the last thing that went wrong with a source, in words.
type MetadataFailure struct {
	Words  string    `json:"words"`
	Status int       `json:"status,omitzero"`
	At     time.Time `json:"at"`
}

// MetadataArtists counts the library's album artists by what the pass has of
// them.
type MetadataArtists struct {
	AlbumArtists int `json:"albumArtists"`
	WithMBID     int `json:"withMbid"`
	LookedUp     int `json:"lookedUp"`
	WithArt      int `json:"withArt"`
	FoundNothing int `json:"foundNothing"`
	WithoutMBID  int `json:"withoutMbid"`
}
