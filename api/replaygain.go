package api

import "time"

// The kinds of ReplayGain run.
const (
	GainKindRun  = "run"
	GainKindUndo = "undo"
)

// The states of a song or an album in a run.
const (
	GainAlbumWritten = "written"
	GainAlbumWriting = "writing"
	GainAlbumWaiting = "waiting"
	GainAlbumLeft    = "left" // Nothing of it was written.
)

// ReplayGainStatus is the response to GET /admin/replaygain: what Kitsune
// measured and could write into the files now, what waits, the run going and
// the latest runs.
type ReplayGainStatus struct {
	Songs        int         `json:"songs"`
	Albums       int         `json:"albums"`
	Formats      []string    `json:"formats"`
	Biggest      []GainAlbum `json:"biggest"`
	Waiting      int         `json:"waiting"`
	Unmeasurable int         `json:"unmeasurable"`
	Run          *GainRun    `json:"run,omitzero"`
	Runs         []GainRun   `json:"runs"`
}

// GainAlbum is an album with songs a run writes, or wrote.
type GainAlbum struct {
	ID      string   `json:"id"`
	Title   string   `json:"title"`
	Artist  string   `json:"artist"`
	Year    int      `json:"year,omitzero"`
	Artwork *Artwork `json:"artwork,omitzero"`
	Formats []string `json:"formats"`
	Songs   int      `json:"songs"`
}

// GainRunRequest is the body of POST /admin/replaygain/runs.
type GainRunRequest struct {
	Albums []string `json:"albums,omitzero"`
}

// GainRun is a ReplayGain run, or an undo of one.
type GainRun struct {
	ID         string     `json:"id"`
	Kind       string     `json:"kind"`
	State      string     `json:"state"`
	StartedAt  time.Time  `json:"startedAt"`
	FinishedAt *time.Time `json:"finishedAt,omitzero"`
	By         string     `json:"by,omitzero"`
	Total      int        `json:"total"`
	Done       int        `json:"done"`
	Written    int        `json:"written"`
	Left       int        `json:"left"`
	Failed     int        `json:"failed"`
	Albums     int        `json:"albums"`
	AlbumsDone int        `json:"albumsDone"`
	Album      string     `json:"album,omitzero"`
	Stopping   bool       `json:"stopping,omitzero"`
	Undoable   bool       `json:"undoable"`
	UndoOf     string     `json:"undoOf,omitzero"`
	UndoneBy   string     `json:"undoneBy,omitzero"`
}

// GainRunAlbum is one album of a run, and how far the run got with it.
type GainRunAlbum struct {
	GainAlbum

	State   string `json:"state"`
	Written int    `json:"written"`
	Left    int    `json:"left"`
}

// GainLeft is a song a run left as it was, and why.
type GainLeft struct {
	Track  string `json:"track"`
	Title  string `json:"title"`
	Path   string `json:"path"`
	Root   int    `json:"root"`
	Reason string `json:"reason"`
}

// GainRunDetail is the response to GET /admin/replaygain/runs/{id}.
type GainRunDetail struct {
	Run    GainRun        `json:"run"`
	Albums []GainRunAlbum `json:"albums"`
	Left   []GainLeft     `json:"left"`
}

// EventReplayGain is a replaygain event: a run's progress, to administrators.
type EventReplayGain struct {
	Run        string `json:"run"`
	State      string `json:"state"`
	Done       int    `json:"done"`
	Total      int    `json:"total"`
	AlbumsDone int    `json:"albumsDone"`
	Album      string `json:"album,omitzero"`
}
