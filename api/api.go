// Package api holds Kitsune's native API contract: the OpenAPI document
// (openapi.yaml, embedded as Spec) and the Go types for its schemas, shared by
// the server and by clients.
package api

import (
	_ "embed"
	"encoding/json/jsontext"
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
	// Album is the album whose etag was stale, on a 412 from POST /edits.
	Album string `json:"album,omitzero"`
}

// LoginRequest is the body of POST /auth/login.
type LoginRequest struct {
	Name     string `json:"name"`
	Password string `json:"password"`
	// Cookie asks for the session in a cookie instead of in the response body.
	Cookie bool `json:"cookie,omitzero"`
}

// SetupStatus is the response to GET /setup.
type SetupStatus struct {
	// Needed is true while the server has no accounts, so the first is still to be made.
	Needed bool `json:"needed"`
	// CodeRequired is true when making it needs the setup code from the server's
	// log, because the caller is outside the server's network.
	CodeRequired bool `json:"codeRequired"`
}

// SetupRequest is the body of POST /setup.
type SetupRequest struct {
	Name     string `json:"name"`
	Password string `json:"password"`
	// Code is the setup code from the server's log, needed only from outside its network.
	Code string `json:"code,omitzero"`
}

// PasswordChange is the body of POST /me/password.
type PasswordChange struct {
	Current string `json:"current"`
	New     string `json:"new"`
}

// ResetRequest is the body of POST /auth/reset.
type ResetRequest struct {
	Name string `json:"name"`
}

// ResetConfirm is the body of POST /auth/reset/confirm.
type ResetConfirm struct {
	Name string `json:"name"`
	// Code is the reset code from the server's log.
	Code string `json:"code"`
	New  string `json:"new"`
	// Cookie asks for the session in a cookie instead of in the response body.
	Cookie bool `json:"cookie,omitzero"`
}

// PasswordSet is the body of POST /admin/users/{id}/password.
type PasswordSet struct {
	New string `json:"new"`
}

// Accounts is the response to GET /admin/users.
type Accounts struct {
	Users []Account `json:"users"`
}

// Account is a user as an administrator sees them: who, and how they sign in.
type Account struct {
	User User `json:"user"`
	// LastSeen is when the newest of their live sessions was last used. Absent with none.
	LastSeen *time.Time `json:"lastSeen,omitzero"`
	// Sessions counts their live sessions.
	Sessions int `json:"sessions"`
}

// User is an account as the API shows it. It has no password-hash field.
type User struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	Role      string    `json:"role"`
	CreatedAt time.Time `json:"createdAt"`
}

// ListenReport is the body of POST /me/listens: a listen's start, or its end.
// The two share ClientID, and a report sent again changes nothing.
type ListenReport struct {
	// ClientID is the client's own ID for the listen.
	ClientID string `json:"clientId"`
	TrackID  string `json:"trackId"`
	// StartedAt is when the listen started, by the client's clock.
	StartedAt time.Time `json:"startedAt"`
	// Shuffle is the shuffle session it plays in, if any.
	Shuffle string `json:"shuffle,omitzero"`
	// Ended is how it ended: completed, skipped, stopped or interrupted.
	// Absent on a start.
	Ended string `json:"ended,omitzero"`
	// ListenedMs is how much of the song played.
	ListenedMs int64 `json:"listenedMs,omitzero"`
}

// PlayReport is the body of POST /me/plays: one listen that counted. It names no
// user: a play is always the caller's.
type PlayReport struct {
	TrackID string `json:"trackId"`
	// PlayedAt is when the listen started, by the client's clock.
	PlayedAt time.Time `json:"playedAt"`
	// Source names the client.
	Source string `json:"source"`
	// ClientID is the client's own ID for the listen, so a retry counts once.
	ClientID string `json:"clientId"`
}

// Rating is the body of PUT /me/ratings/tracks/{id}: the caller's stars, 1 to 5.
type Rating struct {
	Stars int `json:"stars"`
}

// The states of a Last.fm connection in [LastfmStatus].
const (
	LastfmOK        = "ok"        // Everything that counts is sent.
	LastfmWaiting   = "waiting"   // Last.fm isn't answering; the outbox waits until RetryAt.
	LastfmReconnect = "reconnect" // Last.fm rejected the session; nothing goes until the user connects again.
)

// Listening is the response to GET /me/listening: what the caller has done
// with the library, counted.
type Listening struct {
	Plays  int `json:"plays"`
	Hearts int `json:"hearts"`
	Rated  int `json:"rated"`
}

// LastfmStatus is the response to GET and PATCH /me/lastfm. It never carries
// the session key.
type LastfmStatus struct {
	// Configured is false when the server has no Last.fm key.
	Configured bool `json:"configured"`
	Connected  bool `json:"connected"`
	// Name is the Last.fm user name, when connected.
	Name       string `json:"name,omitzero"`
	Scrobble   bool   `json:"scrobble"`
	NowPlaying bool   `json:"nowPlaying"`
	Loves      bool   `json:"loves"`
	// State is LastfmOK, LastfmWaiting or LastfmReconnect.
	State string `json:"state"`
	// Waiting counts the plays the server holds for Last.fm.
	Waiting    int       `json:"waiting"`
	LastSentAt time.Time `json:"lastSentAt,omitzero"`
	// RetryAt is when a waiting outbox is next tried.
	RetryAt time.Time `json:"retryAt,omitzero"`
}

// LastfmSwitches is the body of PATCH /me/lastfm. A switch it leaves out
// stays as it is.
type LastfmSwitches struct {
	Scrobble   *bool `json:"scrobble,omitzero"`
	NowPlaying *bool `json:"nowPlaying,omitzero"`
	Loves      *bool `json:"loves,omitzero"`
}

// LastfmConnect is the response to POST /me/lastfm/connect: the last.fm page
// to send the browser to.
type LastfmConnect struct {
	URL string `json:"url"`
}

// NowPlaying is the body of POST /me/now-playing.
type NowPlaying struct {
	TrackID  string `json:"trackId"`
	ClientID string `json:"clientId,omitempty"`
}

// EventStreamMediaType is the media type of GET /events.
const EventStreamMediaType = "text/event-stream"

// EventLibrary is a library event: albums added, changed or gone from the index.
type EventLibrary struct {
	Albums []string `json:"albums"`
	All    bool     `json:"all"` // Too many to list: read the index again.
}

// EventMarks is a marks event: a heart or a rating changed. Hearted comes with
// a heart and Stars, 0 for none, with a rating.
type EventMarks struct {
	Kind    string `json:"kind"` // tracks, albums or artists, as the routes name them.
	ID      string `json:"id"`
	Hearted *bool  `json:"hearted,omitempty"`
	Stars   *int   `json:"stars,omitempty"`
}

// EventPrefs is a prefs event: the key of the preference that changed.
type EventPrefs struct {
	Key string `json:"key"`
}

// EventListening is a listening event: a device started a track.
type EventListening struct {
	TrackID  string `json:"trackId"`
	ClientID string `json:"clientId,omitempty"`
}

// EventSettings is a settings event: the keys that changed, never a value.
type EventSettings struct {
	Keys []string `json:"keys"`
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
	// DiscSubtitle is the disc's name as this track is tagged.
	DiscSubtitle string   `json:"discSubtitle,omitzero"`
	Year         int      `json:"year,omitzero"`
	Genres       []string `json:"genres"`
	// Etag changes when the file does. It is not the stream's ETag.
	Etag string `json:"etag"`
	// Hearted and Stars are the caller's own. Stars is absent when unrated.
	Hearted bool `json:"hearted"`
	Stars   int  `json:"stars,omitzero"`
}

// TrackDetail is the response to GET /tracks/{id}: a [Track], and its
// album with the album's new If-Match value.
type TrackDetail struct {
	Track `json:",inline"`

	AlbumID   string `json:"albumId"`
	AlbumEtag string `json:"albumEtag"`
}

// TagEdit is a change to a file's tags. Set holds new values by field name, and
// Clear names fields to remove. A field in neither stays as it is. Tags changes
// the tags no field covers.
type TagEdit struct {
	Set   map[string][]string `json:"set,omitzero"`
	Clear []string            `json:"clear,omitzero"`
	Tags  []RawTagEdit        `json:"tags,omitzero"`
}

// RawTagEdit replaces every value of the tag Key, or with none removes it.
type RawTagEdit struct {
	Key    string   `json:"key"`
	Values []string `json:"values"`
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

// TracksEdit is the body of POST /edits: an edit of tracks that may lie on
// several albums. All goes to every track Tracks names, with each one's own on
// top, and Albums gives the etag of every album they are on.
type TracksEdit struct {
	Albums []AlbumETag    `json:"albums"`
	All    TagEdit        `json:"all,omitzero"`
	Tracks []TrackTagEdit `json:"tracks"`
}

// AlbumETag is an album's etag as an edit read it.
type AlbumETag struct {
	ID   string `json:"id"`
	Etag string `json:"etag"`
}

// EditedTracks is the response to POST /edits: where each track is now.
type EditedTracks struct {
	Tracks []EditedTrack `json:"tracks"`
}

// EditedTrack is a track of [EditedTracks] and the album it is on.
type EditedTrack struct {
	ID      string `json:"id"`
	AlbumID string `json:"albumId"`
}

// AlbumTags is the response to GET /albums/{id}/tags: what a tag editor shows.
// Etag is the album's If-Match value.
type AlbumTags struct {
	Etag   string      `json:"etag"`
	Tracks []TrackTags `json:"tracks"`
}

// TrackTags is one track of an [AlbumTags]. Fields holds the values of each field
// the file has, and Tags everything else. File is the file's base name. ReadOnly
// names the fields a client may show but not edit, such as timestamped lyrics.
type TrackTags struct {
	ID       string              `json:"id"`
	File     string              `json:"file"`
	Fields   map[string][]string `json:"fields"`
	ReadOnly []string            `json:"readOnly,omitzero"`
	Tags     []RawTag            `json:"tags"`
}

// RawTag is a tag no field covers, under its format's own key. ReadOnly means an
// edit may not touch it; a value too long to show whole is cut, and read-only.
type RawTag struct {
	Key      string   `json:"key"`
	Values   []string `json:"values"`
	ReadOnly bool     `json:"readOnly,omitzero"`
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
	Discs       []Disc       `json:"discs"`
	DurationMs  int64        `json:"durationMs"`
	Formats     []string     `json:"formats"`
	AddedAt     time.Time    `json:"addedAt"`
	// Genres is every genre of the tracks, each once, in track order.
	Genres []string `json:"genres"`
	// DisplayGenres is as on [AlbumEntry]; Genres stays raw for the tag editor.
	DisplayGenres []string `json:"displayGenres"`
	// Etag is the If-Match value for POST /albums/{id}/edits.
	Etag string `json:"etag"`
	// Hearted is whether the caller has hearted the album.
	Hearted bool `json:"hearted"`
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

// LibraryIndex is the response to GET /library/index: every song, by artist and
// album, in the order a client shows them.
type LibraryIndex struct {
	Artists []LibraryArtist `json:"artists"`
}

// LibraryArtist is one heading of the library index. ID is empty for a group
// with no owner a reader shows, such as Various Artists. The index carries the
// caller's own hearts and stars.
type LibraryArtist struct {
	ID       string         `json:"id,omitzero"`
	Name     string         `json:"name"`
	SortName string         `json:"sortName"`
	Hearted  bool           `json:"hearted"`
	Albums   []LibraryAlbum `json:"albums"`
}

// LibraryAlbum is an album in the library index.
type LibraryAlbum struct {
	ID          string         `json:"id"`
	Title       string         `json:"title"`
	Artist      string         `json:"artist"`
	Compilation bool           `json:"compilation"`
	Hearted     bool           `json:"hearted"`
	Year        int            `json:"year,omitzero"`
	Artwork     *Artwork       `json:"artwork,omitzero"`
	Tracks      []LibraryTrack `json:"tracks"`
	// Discs are the album's discs only when they say what the tracks don't:
	// when there is more than one, or one with a name. Otherwise nil.
	Discs []Disc `json:"discs,omitzero"`
	// After is, in an index of some albums, the album before this one in the
	// whole index; "" when it comes first. Nil in the whole index.
	After *string `json:"after,omitzero"`
}

// Disc is one disc of an album, in disc order: an album on one disc has one.
type Disc struct {
	No int `json:"no"` // From 1: a track with no disc number is on disc 1.
	// Name is the disc's subtitle, from its first song that has one.
	Name       string `json:"name,omitzero"`
	Songs      int    `json:"songs"`
	DurationMs int64  `json:"durationMs"`
}

// LibraryTrack is a song in the library index.
type LibraryTrack struct {
	ID         string   `json:"id"`
	Title      string   `json:"title"`
	DurationMs int64    `json:"durationMs"`
	Artists    []string `json:"artists"`
	DiscNo     int      `json:"discNo,omitzero"`
	TrackNo    int      `json:"trackNo,omitzero"`
	Hearted    bool     `json:"hearted"`
	// Stars is absent when the caller hasn't rated it.
	Stars int `json:"stars,omitzero"`

	// What `fields` asks for, nil when it asks for nothing: a pointer, so an index
	// without it costs what it did.
	*TrackFields `json:",inline"` //nolint:embeddedstructfieldcheck // Last, so its members follow the track's own on the wire.
}

// TrackFields is what GET /library/index?fields= adds to a track. Each is
// absent unless asked for, and when there is nothing to say: no album artists,
// no genre, never played.
type TrackFields struct {
	AlbumArtists  []string  `json:"albumArtists,omitzero"`
	DisplayGenres []string  `json:"displayGenres,omitzero"`
	Year          int       `json:"year,omitzero"`
	Composers     []string  `json:"composers,omitzero"`
	Format        string    `json:"format,omitzero"`
	SampleRate    int       `json:"sampleRate,omitzero"`
	BitDepth      int       `json:"bitDepth,omitzero"`
	Bitrate       int       `json:"bitrate,omitzero"` // kbps.
	AddedAt       time.Time `json:"addedAt,omitzero"`
	Plays         int       `json:"plays,omitzero"` // The caller's.
	LastPlayedAt  time.Time `json:"lastPlayedAt,omitzero"`
	// Shuffle is the caller's shuffle rules on the track, absent with none.
	Shuffle *ShuffleMark `json:"shuffle,omitzero"`
}

// ShuffleMark is what the caller's shuffle rules say of a track: the run it
// plays in, if any, and whether shuffle leaves it out.
type ShuffleMark struct {
	Run      string `json:"run,omitzero"`
	Position int    `json:"position,omitzero"` // From 1.
	Length   int    `json:"length,omitzero"`
	Excluded bool   `json:"excluded,omitzero"`
}

// ShuffleRules is the response to GET /me/shuffle/rules.
type ShuffleRules struct {
	Runs     []ShuffleRun `json:"runs"`
	Excluded []string     `json:"excluded"`
}

// ShuffleRun is songs that always play together in shuffle, in order.
type ShuffleRun struct {
	ID     string   `json:"id"`
	Tracks []string `json:"tracks"`
}

// ShuffleStart is the body of POST /me/shuffles.
type ShuffleStart struct {
	Scope ShuffleScope `json:"scope"`
	// Mode is finite or endless.
	Mode string `json:"mode"`
	// First is the song to lead with, which the client may already be playing.
	First string `json:"first,omitzero"`
	// Seed repeats a shuffle; absent for a new one.
	Seed *uint64 `json:"seed,omitzero"`
}

// ShuffleScope is what a shuffle draws from.
type ShuffleScope struct {
	// Kind is library, hearted, rated, album, artist or tracks.
	Kind string `json:"kind"`
	// ID is the album's or artist's.
	ID string `json:"id,omitzero"`
	// Tracks are the songs, for tracks.
	Tracks []string `json:"tracks,omitzero"`
}

// ShufflePage is a page of a shuffle: the response to POST /me/shuffles and to
// GET /me/shuffles/{id}/next.
type ShufflePage struct {
	ID   string `json:"id"`
	Mode string `json:"mode"`
	// From is the position the page starts at, and To the one after it: the
	// next page is asked for after To.
	From int64 `json:"from"`
	To   int64 `json:"to"`
	// Songs is how many songs shuffle may pick from.
	Songs int `json:"songs"`
	// Done says a finite shuffle has nothing more.
	Done  bool          `json:"done"`
	Picks []ShufflePick `json:"picks"`
}

// ShufflePick is one pick: a song, or songs combined, and why it came up.
type ShufflePick struct {
	Songs []TrackHit `json:"songs"`
	Why   ShuffleWhy `json:"why"`
}

// ShuffleWhy is why a pick came up, as factors that multiply its weight. The
// client words them.
type ShuffleWhy struct {
	Stars      float64     `json:"stars"`
	Rating     float64     `json:"rating"`
	Hearted    bool        `json:"hearted"`
	Heart      float64     `json:"heart"`
	Played     bool        `json:"played"`
	SinceDays  int         `json:"sinceDays"`
	Recency    float64     `json:"recency"`
	Memory     bool        `json:"memory"`
	EarlySkips int         `json:"earlySkips"`
	SkipMemory float64     `json:"skipMemory"`
	Starts     int         `json:"starts"`
	Skips      int         `json:"skips"`
	SkipRate   float64     `json:"skipRate"`
	Listens    int         `json:"listens"`
	Heard      float64     `json:"heard"`
	Listen     float64     `json:"listen"`
	Flow       float64     `json:"flow"`
	After      *ArtistLink `json:"after,omitzero"`
	Similarity float64     `json:"similarity,omitzero"`
}

// EventShuffle is a shuffle event: one of the person's shuffles moved on.
type EventShuffle struct {
	ID string `json:"id"`
	To int64  `json:"to"`
}

// ShuffleTracks is the body of PUT /me/shuffle/runs and of PUT and DELETE
// /me/shuffle/excluded.
type ShuffleTracks struct {
	Tracks []string `json:"tracks"`
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
	// Fingerprints says where the background fingerprint pass stands.
	Fingerprints FingerprintStatus `json:"fingerprints"`
}

// FingerprintStatus is the background fingerprint pass's progress: Done of the
// Total files waiting when it began. Both are 0 while no pass runs.
type FingerprintStatus struct {
	Done  int `json:"done"`
	Total int `json:"total"`
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

// ServerSettings is the response to GET and PATCH /admin/settings.
type ServerSettings struct {
	// Settings holds every setting, by its key as server.toml spells it, such
	// as "lastfm.api_key".
	Settings map[string]Setting `json:"settings"`
	// Listen is the address the server listens on. Only server.toml sets it,
	// since the server needs it before it can open its database.
	Listen string `json:"listen"`
	// ListenSource is SettingFile, or SettingDefault when the file leaves it out.
	ListenSource string `json:"listenSource"`
	// Version is the server's version.
	Version string `json:"version"`
	// Folders are the library.roots, in order, with what each holds now.
	Folders []LibraryFolder `json:"folders"`
}

// The values of Folder.Kind and FolderListing.Kind. Only FolderFree can be added
// to the library; the others are listed so the server's own folders show.
const (
	FolderFree    = "folder"  // Can be added.
	FolderLibrary = "library" // Is a library folder already.
	FolderInside  = "inside"  // Lies inside a library folder.
	FolderHolds   = "holds"   // Holds a library folder.
	FolderData    = "data"    // Is or holds Kitsune's data.
	FolderConfig  = "config"  // Holds Kitsune's config file.
)

// FolderListing is the response to GET /admin/folders: the folders in Path,
// directories only, and what Path itself holds.
type FolderListing struct {
	Path string `json:"path"`
	// Parent is the folder above Path, absent at /.
	Parent  string     `json:"parent,omitzero"`
	Kind    string     `json:"kind"`
	Folders []Folder   `json:"folders"`
	Audio   AudioCount `json:"audio"`
}

// Folder is one folder in a listing.
type Folder struct {
	Name string `json:"name"`
	Path string `json:"path"`
	Kind string `json:"kind"`
}

// AudioCount is how much audio a folder holds, its subfolders included. The
// count stops after a while: Complete is false when it stopped early, and the
// numbers are then at least that.
type AudioCount struct {
	Files    int  `json:"files"`
	Folders  int  `json:"folders"`
	Complete bool `json:"complete"`
}

// LibraryFolder is a library root and what it holds now: 0 and 0 until a scan
// has read it.
type LibraryFolder struct {
	Path   string       `json:"path"`
	Tracks int64        `json:"tracks"`
	Bytes  int64        `json:"bytes"`
	Watch  *FolderWatch `json:"watch,omitempty"`
}

// FolderWatch is how the server follows a library folder for new music.
type FolderWatch struct {
	Mode       string `json:"mode"`                 // watching, polling or off.
	Reason     string `json:"reason,omitempty"`     // network, watch-limit or missing.
	Filesystem string `json:"filesystem,omitempty"` // For network: the filesystem's type.
}

// The values of Setting.Source. Every source but SettingDefault is stored.
const (
	SettingDefault = "default"
	SettingWeb     = "web"
	SettingFile    = "server.toml"
	SettingEnv     = "env"
	SettingFlag    = "flag"
)

// Setting is one server setting. A secret never carries its value: Set says
// whether it has one.
type Setting struct {
	Value jsontext.Value `json:"value,omitzero"`
	Set   *bool          `json:"set,omitzero"`
	// Source is where the value came from, one of the Setting* constants.
	Source string `json:"source"`
	// From names that source for a person, such as KITSUNE_LASTFM_API_KEY.
	From string `json:"from,omitzero"`
	// RestartRequired says a change applies only after a restart.
	RestartRequired bool `json:"restartRequired,omitzero"`
	// PendingRestart says the value has changed since the server started and
	// waits for a restart.
	PendingRestart bool `json:"pendingRestart,omitzero"`
}
