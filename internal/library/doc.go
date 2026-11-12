// Package library turns audio files into the store's library rows and keeps
// them in step with the library folders.
//
// [NewTrackWrite] maps one opened file to a store write. A [Scanner] runs the
// scans, one at a time, in three phases:
//
//   - Survey: every root is walked and diffed against the store before anything
//     is read. The survey only reads. It lists the files to read and the tracks
//     to mark missing, and skips any root whose state looks like an outage (an
//     unreadable or empty root, a mass disappearance) instead of mistaking it
//     for deletions.
//   - Read: a pool of workers opens and fingerprints the files. One goroutine
//     resolves each track's identity (the row at its path, then a missing track
//     with the same audio) and writes in batches. A file that cannot be read
//     becomes a scan issue, never a failed scan.
//   - Finish: tracks missing past the grace period are deleted, unless an
//     outage may be hiding files, and unused albums and artists are pruned,
//     together with the cached covers no album uses.
//
// # Album art
//
// With [Options.Artwork] set, the read phase gives every album one cover
// ([Stats].Covers). A file looks for it unless a track before it, in the order
// the Library lists the album ([store.Place]), already has one: an embedded front
// cover, then the other embedded pictures, then cover, folder, front or album
// (.jpg, .jpeg, .png or .webp) in the track's folder, or in the parent of a disc
// folder. So the album ends up with the
// cover of its first track that has one, whatever order the workers finish in. Art
// never fails a file: an image that cannot be decoded or is too large becomes a
// warning on the track, and a cache failure is logged.
//
// Known limits:
//
//   - Art is read only when an audio file is read: a replaced cover.jpg or a
//     lost cache waits for a Full scan.
//   - Art removed from every file of an album stays: a track without art keeps
//     the cover the album has.
//   - A changed track with different art replaces the album's cover until the next
//     Full scan: a Quick scan ranks only the files it reads.
//   - A cover a later track rendered and lost stays in the cache.
//
// [Scanner.Run] scans at start and then daily. [Scanner.Claim] lets a caller
// learn at once whether a scan can start, and run it elsewhere.
package library
