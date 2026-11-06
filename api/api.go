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

// EditProblem is the [Problem] of a tag edit that was refused or stopped. Each
// track is atomic and an album is not: the tracks in Changed hold the edit and
// the ones in Remaining don't. The first of Remaining is the one that failed.
type EditProblem struct {
	Problem `json:",inline"`

	Changed   []string `json:"changed"`
	Remaining []string `json:"remaining"`
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

// Me is the response to GET /me: the [User], and what the user may do.
type Me struct {
	User `json:",inline"`

	// CanEditTags is true for an administrator on a server with tag writes on.
	CanEditTags bool `json:"canEditTags"`
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
	// AddedAt is when the album first appeared in the library.
	AddedAt time.Time `json:"addedAt"`
	// SortName is the title as the album sorts: its sort tag, else the title.
	SortName string `json:"sortName"`
	// SortArtist is who the album sorts under. Only GET /albums sends it.
	SortArtist string `json:"sortArtist,omitzero"`
	// DisplayGenres is what genres the album shows and filters by: split, cleaned
	// and spelled one way across the library. The most used on it come first.
	DisplayGenres []string `json:"displayGenres"`
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
	TrackTotal int         `json:"trackTotal,omitzero"`
	DiscTotal  int         `json:"discTotal,omitzero"`
	Year       int         `json:"year,omitzero"`
	Genres     []string    `json:"genres"`
	// Etag changes when the file does. It is not the stream's ETag.
	Etag string `json:"etag"`
}

// TrackDetail is the response to GET /tracks/{id}: a [Track], and its
// album with the album's new If-Match value.
type TrackDetail struct {
	Track `json:",inline"`

	AlbumID   string `json:"albumId"`
	AlbumEtag string `json:"albumEtag"`
}

// TagEdit is a change to a file's tags. Set holds new values by field name; a
// field it doesn't name stays as it is.
type TagEdit struct {
	Set map[string][]string `json:"set,omitzero"`
}

// TrackTagEdit is a [TagEdit] of one track.
type TrackTagEdit struct {
	TagEdit `json:",inline"`

	ID string `json:"id"`
}

// AlbumEdit is the body of POST /albums/{id}/edits. All goes to every track, and
// Tracks to the ones they name, on top of All.
type AlbumEdit struct {
	All    TagEdit        `json:"all,omitzero"`
	Tracks []TrackTagEdit `json:"tracks,omitzero"`
}

// AlbumTags is the response to GET /albums/{id}/tags: what a tag editor shows.
// Etag is the album's If-Match value.
type AlbumTags struct {
	Etag   string      `json:"etag"`
	Tracks []TrackTags `json:"tracks"`
}

// TrackTags is one track of an [AlbumTags]. Fields holds the values of each field
// the file has, and Tags everything else. File is the file's base name.
type TrackTags struct {
	ID     string              `json:"id"`
	File   string              `json:"file"`
	Fields map[string][]string `json:"fields"`
	Tags   []RawTag            `json:"tags"`
}

// RawTag is a tag no field covers, under its format's own key.
type RawTag struct {
	Key    string   `json:"key"`
	Values []string `json:"values"`
}

// AlbumDetail is the response to GET /albums/{id}: an [AlbumEntry]'s fields,
// repeated because the schema is flat, and the album's tracks. Artists are who
// Artist links to; with none, Artist is plain text.
type AlbumDetail struct {
	ID          string       `json:"id"`
	Title       string       `json:"title"`
	SortName    string       `json:"sortName"`
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
	// Genres is every genre of the tracks, each once, in track order.
	Genres []string `json:"genres"`
	// DisplayGenres is as on [AlbumEntry]; Genres stays raw for the tag editor.
	DisplayGenres []string `json:"displayGenres"`
	// Etag is the If-Match value for POST /albums/{id}/edits.
	Etag string `json:"etag"`
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
	// IssueCounts breaks the issues down by kind, and counts track warnings.
	IssueCounts IssueCounts `json:"issueCounts"`
	// Scan says where scanning stands.
	Scan ScanStatus `json:"scan"`
}

// IssueCounts counts what a scan had trouble with. It is all a non-administrator
// sees of it.
type IssueCounts struct {
	Unreadable  int64 `json:"unreadable"`
	Corrupted   int64 `json:"corrupted"`
	Unsupported int64 `json:"unsupported"`
	// Warnings counts the live tracks read with warnings. They play.
	Warnings int64 `json:"warnings"`
}

// ScanIssues is the response to GET /admin/library/issues.
type ScanIssues struct {
	Issues   []ScanIssue   `json:"issues"`
	Warnings []TrackWarned `json:"warnings"`
}

// ScanIssue is a file the scanner could not read. Path is relative to its
// library root, and Message holds no root.
type ScanIssue struct {
	Path    string `json:"path"`
	Kind    string `json:"kind"`
	Message string `json:"message"`
}

// TrackWarned is a live track read with warnings. Path is relative to its library
// root, and Messages hold no root.
type TrackWarned struct {
	TrackID  string   `json:"trackId"`
	Path     string   `json:"path"`
	Messages []string `json:"messages"`
}

// ScanRequest is the body of POST /admin/library/scan. Mode is quick or full.
type ScanRequest struct {
	Mode string `json:"mode"`
}

// The values of ScanStatus.State.
const (
	ScanIdle    = "idle"
	ScanRunning = "running"
)

// The values of ScanResult.Outcome.
const (
	ScanFinished = "finished"
	ScanFailed   = "failed"
)

// ScanStatus says where scanning stands. Mode, Read, Total and StartedAt describe
// the running scan.
type ScanStatus struct {
	State     string     `json:"state"`
	Mode      string     `json:"mode,omitzero"`
	Read      int        `json:"read"`
	Total     int        `json:"total"` // 0 until the scan has counted its files.
	StartedAt *time.Time `json:"startedAt,omitzero"`
	// Last is the last scan to end since the server started.
	Last *ScanResult `json:"last,omitzero"`
}

// ScanResult says how a scan ended.
type ScanResult struct {
	Mode         string    `json:"mode"`
	Outcome      string    `json:"outcome"`
	StartedAt    time.Time `json:"startedAt"`
	EndedAt      time.Time `json:"endedAt"`
	Seen         int       `json:"seen"`
	Read         int       `json:"read"`
	Added        int       `json:"added"`
	Updated      int       `json:"updated"`
	Moved        int       `json:"moved"`
	Missing      int       `json:"missing"`
	Deleted      int64     `json:"deleted"`
	Issues       int       `json:"issues"`
	SkippedRoots int       `json:"skippedRoots"`
	// Error says why a failed scan failed, for administrators only.
	Error string `json:"error,omitzero"`
}
