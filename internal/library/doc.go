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
//     outage may be hiding files, and unused albums and artists are pruned.
//
// [Scanner.Run] scans at start and then daily.
package library
