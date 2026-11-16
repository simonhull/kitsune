package api

import "time"

// OrganizeRequest is the body of POST /admin/organize/preview. Empty templates
// are the saved ones; no tracks or albums is the whole library.
type OrganizeRequest struct {
	Template     string   `json:"template,omitzero"`
	Compilations string   `json:"compilations,omitzero"`
	Tracks       []string `json:"tracks,omitzero"`
	Albums       []string `json:"albums,omitzero"`
}

// The filters of a preview's pages, and the kinds of its songs and albums.
const (
	OrganizeKindAll      = "all"
	OrganizeKindRename   = "rename"
	OrganizeKindMove     = "move"
	OrganizeKindConflict = "conflict"
	OrganizeKindSkipped  = "skipped"
	OrganizeKindInPlace  = "inPlace"
	OrganizeKindMixed    = "mixed" // An album's kind when its songs don't share one.
)

// OrganizeCounts are a preview's totals. Moves is Renames and NewFolders.
type OrganizeCounts struct {
	Songs      int `json:"songs"`
	InPlace    int `json:"inPlace"`
	Moves      int `json:"moves"`
	Renames    int `json:"renames"`
	NewFolders int `json:"newFolders"`
	Conflicts  int `json:"conflicts"`
	Skipped    int `json:"skipped"`
	Folders    int `json:"folders"`
}

// OrganizeScope says which songs a preview of some songs holds.
type OrganizeScope struct {
	Songs  int    `json:"songs"`
	Albums int    `json:"albums"`
	Album  string `json:"album,omitzero"` // The album's title, when the songs are on one.
}

// OrganizePreview is a preview's counts and one page of its albums.
type OrganizePreview struct {
	ID           string          `json:"id"`
	Template     string          `json:"template"`
	Compilations string          `json:"compilations"`
	Counts       OrganizeCounts  `json:"counts"`
	Scope        *OrganizeScope  `json:"scope,omitzero"`
	Filter       string          `json:"filter"`
	Matching     int             `json:"matching"`
	Albums       []OrganizeAlbum `json:"albums"`
	NextCursor   string          `json:"nextCursor,omitzero"`
}

// OrganizeAlbum is one album's songs in a preview. Every path is relative to
// the library folder Root indexes in library.roots.
type OrganizeAlbum struct {
	ID         string              `json:"id"`
	Title      string              `json:"title"`
	Artist     string              `json:"artist"`
	Artwork    *Artwork            `json:"artwork,omitzero"`
	Root       int                 `json:"root"`
	Kind       string              `json:"kind"`
	Songs      int                 `json:"songs"`
	FromFolder string              `json:"fromFolder,omitzero"`
	ToFolder   string              `json:"toFolder,omitzero"`
	CaseOnly   bool                `json:"caseOnly,omitzero"`
	Files      []OrganizeFile      `json:"files"`
	Companions []OrganizeCompanion `json:"companions"`
}

// OrganizeCompanion is a file that comes along with an album's songs.
type OrganizeCompanion struct {
	From string `json:"from"`
	To   string `json:"to"`
}

// OrganizeFile is one song in a preview.
type OrganizeFile struct {
	Track    string            `json:"track"`
	Title    string            `json:"title"`
	From     string            `json:"from"`
	To       string            `json:"to,omitzero"`
	Kind     string            `json:"kind"`
	CaseOnly bool              `json:"caseOnly,omitzero"`
	Reason   string            `json:"reason,omitzero"`
	Unknown  string            `json:"unknown,omitzero"`
	Conflict *OrganizeConflict `json:"conflict,omitzero"`
}

// OrganizeConflict is a song beside the songs in its way.
type OrganizeConflict struct {
	Self          OrganizeClaimant   `json:"self"`
	With          []OrganizeClaimant `json:"with"`
	SameRecording bool               `json:"sameRecording"`
	Why           string             `json:"why,omitzero"`   // A ConflictWhy.
	Group         string             `json:"group,omitzero"` // The group of duplicates the songs are in.
}

// What makes the songs of a conflict look like one recording: the same audio,
// one MusicBrainz recording, or the same artist, title and length.
const (
	ConflictWhyDigest    = "digest"
	ConflictWhyRecording = "recording"
	ConflictWhyTags      = "tags"
)

// OrganizeClaimant is a song in a conflict. Plays and Stars are the caller's.
type OrganizeClaimant struct {
	Track      string    `json:"track"`
	Title      string    `json:"title"`
	Album      string    `json:"album"`
	AlbumID    string    `json:"albumId"`
	Path       string    `json:"path"`
	Root       int       `json:"root"`
	Format     string    `json:"format"`
	Bitrate    int       `json:"bitrate,omitzero"`
	DurationMs int64     `json:"durationMs"`
	Plays      int       `json:"plays"`
	Stars      float64   `json:"stars"`
	AddedAt    time.Time `json:"addedAt"`
	Here       bool      `json:"here"`
}

// OrganizeRunRequest is the body of POST /admin/organize/runs.
type OrganizeRunRequest struct {
	PreviewID string   `json:"previewId"`
	Tracks    []string `json:"tracks,omitzero"`
	Albums    []string `json:"albums,omitzero"`
}

// The values of OrganizeRun.State and EventOrganize.State.
const (
	OrganizeRunning = "running"
	OrganizeDone    = "done"
	OrganizeStopped = "stopped"
	OrganizeUndone  = "undone"
)

// OrganizeRun is a run, an undo or a song moved after an edit.
type OrganizeRun struct {
	ID         string     `json:"id"`
	Kind       string     `json:"kind"`
	State      string     `json:"state"`
	StartedAt  time.Time  `json:"startedAt"`
	FinishedAt *time.Time `json:"finishedAt,omitzero"`
	By         string     `json:"by,omitzero"`
	Template   string     `json:"template"`
	Total      int        `json:"total"`
	Done       int        `json:"done"`
	Moved      int        `json:"moved"`
	Folders    int        `json:"folders"`
	Conflicts  int        `json:"conflicts"`
	Left       int        `json:"left"`
	Failed     int        `json:"failed"`
	Companions int        `json:"companions"`
	Playlists  int        `json:"playlists,omitzero"`
	Album      string     `json:"album,omitzero"`
	Stopping   bool       `json:"stopping,omitzero"`
	Undoable   bool       `json:"undoable"`
	UndoOf     string     `json:"undoOf,omitzero"`
	UndoneBy   string     `json:"undoneBy,omitzero"`
}

// OrganizeRuns is the response to GET /admin/organize/runs.
type OrganizeRuns struct {
	Runs []OrganizeRun `json:"runs"`
}

// OrganizeLeft is a song a run left where it was, and why.
type OrganizeLeft struct {
	Track  string `json:"track"`
	Title  string `json:"title"`
	From   string `json:"from"`
	To     string `json:"to,omitzero"`
	Root   int    `json:"root"`
	Reason string `json:"reason"`
}

// OrganizeRunDetail is the response to GET /admin/organize/runs/{id}.
type OrganizeRunDetail struct {
	Run  OrganizeRun    `json:"run"`
	Left []OrganizeLeft `json:"left"`
}

// OrganizeTemplateProblem is the [Problem] of a template that doesn't parse:
// Field says which, At where, in characters.
type OrganizeTemplateProblem struct {
	Problem `json:",inline"`

	Field string `json:"field"`
	At    int    `json:"at"`
}

// OrganizeExamples is the response to GET /admin/organize/examples.
type OrganizeExamples struct {
	Songs []OrganizeExample `json:"songs"`
}

// OrganizeExample is a song to show a template on: who and what it is, where
// it is now, and what a template reads of it.
type OrganizeExample struct {
	Track  string       `json:"track"`
	Title  string       `json:"title"`
	Artist string       `json:"artist"`
	Album  string       `json:"album"`
	From   string       `json:"from"`
	Root   int          `json:"root"`
	Song   OrganizeSong `json:"song"`
}

// OrganizeSong is what a template reads of a song.
type OrganizeSong struct {
	Title        string   `json:"title"`
	Album        string   `json:"album"`
	AlbumArtist  string   `json:"albumArtist"`
	Artists      []string `json:"artists"`
	Genre        string   `json:"genre"`
	DiscName     string   `json:"discName"`
	Format       string   `json:"format"`
	Year         int      `json:"year"`
	OriginalYear int      `json:"originalYear"`
	Disc         int      `json:"disc"`
	Discs        int      `json:"discs"`
	Track        int      `json:"track"`
	DiscSongs    int      `json:"discSongs"`
	Compilation  bool     `json:"compilation"`
}

// EventOrganize is an organize event: a run's progress, to administrators.
type EventOrganize struct {
	Run        string `json:"run"`
	State      string `json:"state"`
	Done       int    `json:"done"`
	Total      int    `json:"total"`
	AlbumsDone int    `json:"albumsDone"`
	Album      string `json:"album,omitzero"`
}
