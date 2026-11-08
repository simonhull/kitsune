package library

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/simonhull/audiometa"

	"github.com/simonhull/kitsune/internal/store"
)

// olderReader is a version no audiometa of this build reports.
const olderReader = "v0.0.0-older"

// issueLib is a library holding one readable file, rel, a copy of sample, that
// the store has as an unsupported file, judged by reader.
func issueLib(t *testing.T, rel, sample, reader string) *testLib {
	t.Helper()
	l := newTestLib(t, 1)
	root := l.roots[0]
	l.copyTo(root, rel, sample)
	info, err := os.Stat(filepath.Join(root, rel))
	if err != nil {
		t.Fatal(err)
	}
	l.seedIssueBy(root, rel, info.Size(), info.ModTime(), reader)
	return l
}

// issues returns every scan issue the store holds.
func (l *testLib) issues() []store.ScanIssue {
	l.t.Helper()
	issues, err := l.st.ScanIssues(l.t.Context())
	if err != nil {
		l.t.Fatal(err)
	}
	return issues
}

// A file judged unsupported or corrupted is read again once the reader changes,
// though the file hasn't: a newer audiometa may take it.
func TestIssueIsRetriedUnderANewReader(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct{ name, reader string }{
		{"an older reader", olderReader},
		{"no reader, from before readers were kept", ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			l := issueLib(t, aFLAC, sampleFLAC, tt.reader)
			stats := l.scan(Quick)
			if issues := l.issues(); stats.Read != 1 || stats.Added != 1 || len(issues) != 0 {
				t.Errorf("read %d, added %d, issues %+v; want the file read and its issue gone", stats.Read, stats.Added, issues)
			}
			if st := l.states(l.roots[0])[aFLAC]; st.TrackID == 0 || st.Issue {
				t.Errorf("state = %+v, want a track", st)
			}
		})
	}
}

// Under the reader that judged it, a quick scan leaves an unchanged file alone.
func TestIssueIsLeftAloneUnderItsReader(t *testing.T) {
	t.Parallel()
	l := issueLib(t, aFLAC, sampleFLAC, audiometa.GetVersion())
	if stats, issues := l.scan(Quick), l.issues(); stats.Read != 0 || len(issues) != 1 {
		t.Errorf("read %d, issues %+v; want the file left alone", stats.Read, issues)
	}
}

// A file the reader can't take keeps that reader's version with its issue.
func TestIssueKeepsItsReader(t *testing.T) {
	t.Parallel()
	l := newTestLib(t, 1)
	root := l.roots[0]
	l.write(t, root, "broken.flac", []byte("not audio at all"))
	l.scan(Quick)
	st := l.states(root)["broken.flac"]
	if !st.Issue || st.Reader != l.scanner.reader || st.Reader == "" {
		t.Errorf("state = %+v, want an issue by reader %q", st, l.scanner.reader)
	}
	if stats := l.scan(Quick); stats.Read != 0 {
		t.Errorf("a second quick scan read %d files, want none", stats.Read)
	}
}
