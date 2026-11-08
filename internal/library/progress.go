package library

import (
	"context"
	"path/filepath"
	"strings"
	"time"
)

// Progress is where scanning stands at one moment. Clients poll it, and while no
// scan runs only Last is set.
type Progress struct {
	Running   bool
	Mode      Mode
	Read      int // Files the scan has read.
	Total     int // Files it has to read: 0 until the survey has counted them.
	StartedAt time.Time
	Last      *ScanResult // The last scan to end, since the server started.
}

// ScanResult says how a scan ended.
type ScanResult struct {
	Mode               Mode
	StartedAt, EndedAt time.Time
	Stats              Stats
	Err                string // Why it failed, without the roots. Empty when it did not.
}

// Progress returns the current snapshot. It never waits for a scan.
func (s *Scanner) Progress() Progress { return *s.progress.Load() }

// publish applies change to a copy of the snapshot and swaps it in. Only the
// holder of the claim may call it, so no change is lost.
func (s *Scanner) publish(change func(*Progress)) {
	next := s.Progress()
	change(&next)
	s.progress.Store(&next)
}

// claimed publishes a claim just taken and returns the scan it reserves.
func (s *Scanner) claimed(mode Mode) func(context.Context) (Stats, error) {
	started := s.now()
	s.publish(func(p *Progress) { *p = Progress{Running: true, Mode: mode, StartedAt: started, Last: p.Last} })
	s.notifyScan(true)
	return func(ctx context.Context) (Stats, error) { return s.run(ctx, mode) }
}

// ended publishes how the running scan went.
func (s *Scanner) ended(stats *Stats, err error) {
	s.publish(func(p *Progress) {
		last := &ScanResult{Mode: p.Mode, StartedAt: p.StartedAt, EndedAt: s.now(), Stats: *stats}
		if err != nil {
			last.Err = err.Error()
			for _, root := range s.live().Roots {
				last.Err = StripRoot(last.Err, root)
			}
		}
		*p = Progress{Last: last}
	})
}

// StripRoot removes the library root from msg, which goes to a client: a path
// under root becomes relative to it, and root itself becomes "<root>". Messages
// are stored as valid UTF-8, so root is compared in that form.
func StripRoot(msg, root string) string {
	clean := strings.ToValidUTF8(filepath.Clean(root), "�")
	dir := strings.TrimSuffix(clean, string(filepath.Separator)) + string(filepath.Separator)
	return strings.ReplaceAll(strings.ReplaceAll(msg, dir, ""), clean, "<root>")
}
