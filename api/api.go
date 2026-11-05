// Package api holds Kitsune's native API contract: the OpenAPI document
// (openapi.yaml, embedded as Spec) and the Go types for its schemas, shared by
// the server and by clients.
package api

import (
	_ "embed"
	"time"
)

// Spec is the OpenAPI 3.1 document that defines the API.
//
//go:embed openapi.yaml
var Spec []byte

// Media types used by the API.
const (
	JSONMediaType    = "application/json"
	ProblemMediaType = "application/problem+json"
)

// Problem is an RFC 9457 problem, the body of every error response.
type Problem struct {
	Type      string `json:"type"`
	Title     string `json:"title"`
	Status    int    `json:"status"`
	Detail    string `json:"detail,omitzero"`
	RequestID string `json:"requestId,omitzero"`
}

// LoginRequest is the body of POST /auth/login.
type LoginRequest struct {
	Name     string `json:"name"`
	Password string `json:"password"`
	// Cookie asks for the session in a cookie instead of in the response body.
	Cookie bool `json:"cookie,omitzero"`
}

// User is an account as the API shows it. It has no password-hash field.
type User struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	Role      string    `json:"role"`
	CreatedAt time.Time `json:"createdAt"`
}

// Session is the response to a successful login.
type Session struct {
	// Token is present only when the login did not ask for a cookie.
	Token     string    `json:"token,omitzero"`
	ExpiresAt time.Time `json:"expiresAt"`
	User      User      `json:"user"`
}

// Artwork is an album's cover. Hash names it for GET /artwork/{hash}; Accent
// and Ground are colors as "#rrggbb".
type Artwork struct {
	Hash   string `json:"hash"`
	Accent string `json:"accent"`
	Ground string `json:"ground"`
	// Width and Height are those of the source image.
	Width  int `json:"width"`
	Height int `json:"height"`
}

// AlbumEntry is an album as a list shows it.
type AlbumEntry struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	Artist      string   `json:"artist"`
	Year        int      `json:"year,omitzero"`
	TrackCount  int      `json:"trackCount"`
	Compilation bool     `json:"compilation"`
	Artwork     *Artwork `json:"artwork,omitzero"`
}

// AlbumPage is the response to GET /albums.
type AlbumPage struct {
	Albums []AlbumEntry `json:"albums"`
	// NextCursor is absent on the last page.
	NextCursor string `json:"nextCursor,omitzero"`
}

// ReplayGain holds loudness values; an absent one is unknown.
type ReplayGain struct {
	TrackGain float64 `json:"trackGain,omitzero"`
	AlbumGain float64 `json:"albumGain,omitzero"`
	TrackPeak float64 `json:"trackPeak,omitzero"`
	AlbumPeak float64 `json:"albumPeak,omitzero"`
}

// Track is a track as an album shows it.
type Track struct {
	ID         string      `json:"id"`
	Title      string      `json:"title"`
	Artists    []string    `json:"artists"`
	DiscNo     int         `json:"discNo,omitzero"`
	TrackNo    int         `json:"trackNo,omitzero"`
	DurationMs int64       `json:"durationMs"`
	Format     string      `json:"format"`
	SampleRate int         `json:"sampleRate,omitzero"`
	BitDepth   int         `json:"bitDepth,omitzero"`
	ReplayGain *ReplayGain `json:"replayGain,omitzero"`
}

// AlbumDetail is the response to GET /albums/{id}: an [AlbumEntry]'s fields,
// repeated because the schema is flat, and the album's tracks. Artists are who
// Artist links to; with none, Artist is plain text.
type AlbumDetail struct {
	ID          string       `json:"id"`
	Title       string       `json:"title"`
	Artist      string       `json:"artist"`
	Artists     []ArtistLink `json:"artists"`
	Year        int          `json:"year,omitzero"`
	TrackCount  int          `json:"trackCount"`
	Compilation bool         `json:"compilation"`
	Artwork     *Artwork     `json:"artwork,omitzero"`
	Tracks      []Track      `json:"tracks"`
	DurationMs  int64        `json:"durationMs"`
	Formats     []string     `json:"formats"`
	AddedAt     time.Time    `json:"addedAt"`
}

// ArtistEntry is an artist as a list shows it. AlbumCount counts the albums it owns.
type ArtistEntry struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	AlbumCount int    `json:"albumCount"`
}

// ArtistList is the response to GET /artists.
type ArtistList struct {
	Artists []ArtistEntry `json:"artists"`
}

// ArtistDetail is the response to GET /artists/{id}. Albums are those it owns,
// and AppearsOn the others it has a track on, both newest first.
type ArtistDetail struct {
	ID         string       `json:"id"`
	Name       string       `json:"name"`
	AlbumCount int          `json:"albumCount"`
	Albums     []AlbumEntry `json:"albums"`
	AppearsOn  []AlbumEntry `json:"appearsOn"`
}

// ArtistLink is just enough of an artist to link to it.
type ArtistLink struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// TrackHit is a track as a search result shows it. Artwork is its album's.
type TrackHit struct {
	ID         string   `json:"id"`
	Title      string   `json:"title"`
	Artists    []string `json:"artists"`
	AlbumID    string   `json:"albumId"`
	Album      string   `json:"album"`
	DurationMs int64    `json:"durationMs"`
	Artwork    *Artwork `json:"artwork,omitzero"`
}

// SearchResults is the response to GET /search.
type SearchResults struct {
	Artists []ArtistLink `json:"artists"`
	Albums  []AlbumEntry `json:"albums"`
	Tracks  []TrackHit   `json:"tracks"`
}

// LibraryFacts is the response to GET /library.
type LibraryFacts struct {
	Albums     int64 `json:"albums"`
	Tracks     int64 `json:"tracks"`
	DurationMs int64 `json:"durationMs"`
	// Roots is the number of configured library roots.
	Roots int `json:"roots"`
	// Issues counts the files the scanner could not read.
	Issues int64 `json:"issues"`
}
