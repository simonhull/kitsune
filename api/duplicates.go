package api

import "time"

// The filters of GET /admin/duplicates.
const (
	DuplicatesAll       = "all"
	DuplicatesAudio     = "audio"     // Groups by the same audio.
	DuplicatesRecording = "recording" // Groups by one MusicBrainz recording.
	DuplicatesProbably  = "probably"  // Groups by artist, title and length.
	DuplicatesPairs     = "pairs"     // Album pairs.
	DuplicatesDismissed = "dismissed" // What was said not to be duplicates.
)

// The kinds of GET /admin/duplicates/{group}.
const (
	DuplicateKindGroup = "group"
	DuplicateKindPair  = "pair"
)

// DuplicateCounts are a find's totals. Groups, Songs and Pairs leave out what
// was dismissed; Audio, Recording and Probably count groups by confidence.
type DuplicateCounts struct {
	Groups    int `json:"groups"`
	Songs     int `json:"songs"`
	Pairs     int `json:"pairs"`
	Audio     int `json:"audio"`
	Recording int `json:"recording"`
	Probably  int `json:"probably"`
	Dismissed int `json:"dismissed"`
	// Library counts the live songs, and Checked those the fingerprint pass
	// has read, which alone can be found as the same audio.
	Library int `json:"library"`
	Checked int `json:"checked"`
}

// DuplicateAlbum is an album a copy is on.
type DuplicateAlbum struct {
	ID      string   `json:"id"`
	Title   string   `json:"title"`
	Artist  string   `json:"artist"`
	Year    int      `json:"year,omitzero"`
	Artwork *Artwork `json:"artwork,omitzero"`
}

// DuplicateGroupSummary is a group as the list shows it.
type DuplicateGroupSummary struct {
	Key        string           `json:"key"`
	Title      string           `json:"title"`
	Artist     string           `json:"artist"`
	Confidence string           `json:"confidence"`
	Copies     int              `json:"copies"`
	Formats    []string         `json:"formats"` // Each copy's, in order.
	Albums     []DuplicateAlbum `json:"albums"`  // Each once, in the copies' order.
	DurationMs []int64          `json:"durationMs"`
	Plays      int              `json:"plays"` // Everyone's, across the copies.
	Pair       string           `json:"pair,omitzero"`
	Dismissed  bool             `json:"dismissed,omitzero"`
}

// DuplicatePairSummary is an album pair as the list shows it.
type DuplicatePairSummary struct {
	Key    string            `json:"key"`
	Albums [2]DuplicateAlbum `json:"albums"`
	// Both counts the songs in both, and Only each album's songs the other
	// doesn't have, at the same index.
	Both      int    `json:"both"`
	Only      [2]int `json:"only"`
	Dismissed bool   `json:"dismissed,omitzero"`
}

// DuplicateList is a find's counts and one page of it.
type DuplicateList struct {
	Counts     DuplicateCounts         `json:"counts"`
	Filter     string                  `json:"filter"`
	Matching   int                     `json:"matching"` // Groups and pairs under the filter.
	Pairs      []DuplicatePairSummary  `json:"pairs"`
	Groups     []DuplicateGroupSummary `json:"groups"`
	NextCursor string                  `json:"nextCursor,omitzero"`
}

// DuplicateCopy is one copy of a recording, as a group's page compares it.
// Its path is relative to the library folder Root indexes in library.roots.
type DuplicateCopy struct {
	Track      string         `json:"track"`
	Title      string         `json:"title"`
	Album      DuplicateAlbum `json:"album"`
	Disc       int            `json:"disc,omitzero"`
	TrackNo    int            `json:"trackNo,omitzero"`
	Path       string         `json:"path"`
	Root       int            `json:"root"`
	Format     string         `json:"format"`
	Bitrate    int            `json:"bitrate,omitzero"`
	SampleRate int            `json:"sampleRate,omitzero"`
	BitDepth   int            `json:"bitDepth,omitzero"`
	Lossless   bool           `json:"lossless"`
	DurationMs int64          `json:"durationMs"`
	Plays      int            `json:"plays"`  // Everyone's.
	Stars      float64        `json:"stars"`  // The caller's own; 0 for none.
	Raters     int            `json:"raters"` // How many people rated it.
	Playlists  int            `json:"playlists"`
	AddedAt    time.Time      `json:"addedAt"`
	InPlace    bool           `json:"inPlace"`
	Original   bool           `json:"original"`
}

// MergePlan is what a merge moves, as its confirm words it: counts, the
// caller's own stars, and the names of the playlists the caller sees by name.
type MergePlan struct {
	Plays          int                 `json:"plays"`
	Listens        int                 `json:"listens"`
	Stars          MergeStars          `json:"stars"`
	Loves          int                 `json:"loves"`
	Hearts         int                 `json:"hearts"`
	Entries        int                 `json:"entries"`
	Playlists      []MergePlanPlaylist `json:"playlists"`
	OtherPlaylists int                 `json:"otherPlaylists"`
	Excluded       int                 `json:"excluded"`
	Runs           int                 `json:"runs"`
	Queues         int                 `json:"queues"`
}

// MergeStars is what a merge does with stars.
type MergeStars struct {
	Yours *YourStars `json:"yours,omitzero"`
	Come  int        `json:"come"`
	Stay  int        `json:"stay"`
}

// YourStars is the caller's own stars in a merge of one group; 0 is none.
type YourStars struct {
	Keeper float64 `json:"keeper"`
	Other  float64 `json:"other"`
	After  float64 `json:"after"`
}

// MergePlanPlaylist is a playlist whose entries a merge repoints.
type MergePlanPlaylist struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	File bool   `json:"file,omitzero"`
}

// KeepPlan is what keeping one copy, or one album of a pair, would move.
type KeepPlan struct {
	Keep string    `json:"keep"`
	Plan MergePlan `json:"plan"`
}

// DuplicateGroup is one group, its copies side by side.
type DuplicateGroup struct {
	Key        string          `json:"key"`
	Title      string          `json:"title"`
	Artist     string          `json:"artist"`
	Confidence string          `json:"confidence"`
	Dismissed  bool            `json:"dismissed,omitzero"`
	Pair       string          `json:"pair,omitzero"`
	Keeper     string          `json:"keeper"`  // The copy suggested.
	Reasons    []string        `json:"reasons"` // Why, in order.
	Copies     []DuplicateCopy `json:"copies"`
	Plans      []KeepPlan      `json:"plans"` // One for each copy kept.
}

// DuplicatePairRow is one row of an album pair's track list: the copy on each
// album, at the same index, or nil when only the other has it.
type DuplicatePairRow struct {
	Group  string            `json:"group,omitzero"`
	Copies [2]*DuplicateCopy `json:"copies"`
}

// DuplicatePair is an album pair, its track lists matched row by row.
type DuplicatePair struct {
	Key       string             `json:"key"`
	Albums    [2]DuplicateAlbum  `json:"albums"`
	Dismissed bool               `json:"dismissed,omitzero"`
	Keeper    string             `json:"keeper"` // The album suggested.
	Rows      []DuplicatePairRow `json:"rows"`
	Plans     []KeepPlan         `json:"plans"`
}

// DuplicateDetail is GET /admin/duplicates/{group}: a group or an album pair.
type DuplicateDetail struct {
	Kind  string          `json:"kind"`
	Group *DuplicateGroup `json:"group,omitzero"`
	Pair  *DuplicatePair  `json:"pair,omitzero"`
}

// MergeRequest is the body of POST /admin/duplicates/{group}/merge: the copy
// to keep and those to trash, or for a pair the album to keep and the other,
// and the plan the confirm showed.
type MergeRequest struct {
	Keep  string    `json:"keep"`
	Trash []string  `json:"trash"`
	Plan  MergePlan `json:"plan"`
}

// Merged is what a merge did.
type Merged struct {
	Kept    []string    `json:"kept"` // The keepers, by track.
	Trashed TrashChange `json:"trashed"`
}

// PlanChangedProblem is a merge refused because the library changed since its
// confirm: Plan is what it would move now.
type PlanChangedProblem struct {
	Problem `json:",inline"`

	Plan MergePlan `json:"plan"`
}
