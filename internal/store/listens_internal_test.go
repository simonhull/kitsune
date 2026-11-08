package store

import (
	"errors"
	"slices"
	"testing"
	"time"
)

func listen(clientID string, listened time.Duration, ended Ending) *NewListen {
	return &NewListen{
		UserID: 1, TrackID: firstTrack, ClientID: clientID, Shuffle: "S1",
		StartedAt: time.UnixMilli(5000).UTC(), Listened: listened, Ended: ended,
	}
}

func listenRows(t *testing.T, s *Store) []string {
	t.Helper()
	return rawRows(t, s.w, "SELECT client_id, shuffle, started_at, listened_ms, duration_ms, coalesce(ended, '-') FROM listens ORDER BY client_id")
}

func TestRecordListenJoinsAStartToItsEnd(t *testing.T) {
	t.Parallel()
	s := seededStore(t)
	for _, l := range []*NewListen{
		listen("a", 0, UnderWay),
		listen("a", 0, UnderWay), // a start sent twice
		listen("a", 12*time.Second, Skipped),
		listen("a", 12*time.Second, Skipped), // an end sent twice
		listen("a", 0, UnderWay),             // a start arriving late changes nothing
		listen("a", 99*time.Second, Completed),
		listen("b", 800*time.Millisecond, Completed), // an end with no start
	} {
		if err := s.RecordListen(t.Context(), l); err != nil {
			t.Fatal(err)
		}
	}
	want := []string{"a S1 5000 12000 1000 skipped", "b S1 5000 800 1000 completed"}
	if got := listenRows(t, s); !slices.Equal(got, want) {
		t.Errorf("listens = %q, want %q", got, want)
	}
}

func TestRecordListenKeepsTheMostHeardWhileUnderWay(t *testing.T) {
	t.Parallel()
	s := seededStore(t)
	for _, l := range []*NewListen{listen("a", 5*time.Second, UnderWay), listen("a", 2*time.Second, UnderWay)} {
		if err := s.RecordListen(t.Context(), l); err != nil {
			t.Fatal(err)
		}
	}
	if got := listenRows(t, s); !slices.Equal(got, []string{"a S1 5000 5000 1000 -"}) {
		t.Errorf("listens = %q", got)
	}
}

func TestRecordListenRefuses(t *testing.T) {
	t.Parallel()
	s := seededStore(t)
	unknown := listen("a", 0, UnderWay)
	unknown.TrackID = "T9999999999999999999999999"
	if err := s.RecordListen(t.Context(), unknown); !errors.Is(err, ErrNotFound) {
		t.Errorf("unknown track: err = %v, want ErrNotFound", err)
	}
	for _, l := range []*NewListen{listen("b", 0, "paused"), listen("c", -time.Second, Completed)} {
		if err := s.RecordListen(t.Context(), l); !errors.Is(err, ErrInvalidWrite) {
			t.Errorf("%+v: err = %v, want ErrInvalidWrite", l, err)
		}
	}
	if got := listenRows(t, s); len(got) != 0 {
		t.Errorf("listens = %q, want none", got)
	}
}
