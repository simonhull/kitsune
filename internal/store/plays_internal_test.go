package store

import (
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/ncruces/go-sqlite3"
	sqlitedriver "github.com/ncruces/go-sqlite3/driver"
)

// playSource is the client seededStore's plays come from.
const playSource = "web"

// firstTrack is the ULID of the first track seedLibrary writes.
var firstTrack = fmt.Sprintf("T%025d", 1) //nolint:gochecknoglobals // Read-only fixture shared by the tests below.

// seededStore is a store with seedLibrary's rows in it, but no plays.
func seededStore(t *testing.T) *Store {
	t.Helper()
	s := openTest(t)
	seedLibrary(t, s.w)
	if _, err := s.w.ExecContext(t.Context(), "DELETE FROM plays"); err != nil {
		t.Fatal(err)
	}
	return s
}

// addUser writes a second user and returns its ID.
func addUser(t *testing.T, s *Store) int64 {
	t.Helper()
	if _, err := s.w.ExecContext(t.Context(), "INSERT INTO users (id, name, password_hash, role, created_at) VALUES (2, 'ada', 'h', 'user', 1)"); err != nil {
		t.Fatal(err)
	}
	return 2
}

func play(userID int64, clientID string) *NewPlay {
	return &NewPlay{UserID: userID, TrackID: firstTrack, PlayedAt: time.UnixMilli(1000).UTC(), Source: playSource, ClientID: clientID}
}

func mustRecord(t *testing.T, s *Store, p *NewPlay) {
	t.Helper()
	if err := s.RecordPlay(t.Context(), p); err != nil {
		t.Fatal(err)
	}
}

func recent(t *testing.T, s *Store, userID int64) []Play {
	t.Helper()
	plays, err := s.RecentPlays(t.Context(), userID, 100)
	if err != nil {
		t.Fatal(err)
	}
	return plays
}

func TestRecordPlayCountsARetryOnce(t *testing.T) {
	t.Parallel()
	s := seededStore(t)
	mustRecord(t, s, play(1, "a"))
	mustRecord(t, s, play(1, "a"))
	if got := recent(t, s, 1); !slices.Equal(got, []Play{{TrackID: firstTrack, PlayedAt: time.UnixMilli(1000).UTC(), Source: playSource}}) {
		t.Errorf("plays = %+v, want the one", got)
	}
	mustRecord(t, s, play(1, "b"))
	if got := len(recent(t, s, 1)); got != 2 {
		t.Errorf("plays = %d, want 2: a new client ID is a new listen", got)
	}
}

func TestRecordPlayKeepsUsersApart(t *testing.T) {
	t.Parallel()
	s := seededStore(t)
	other := addUser(t, s)
	mustRecord(t, s, play(1, "same"))
	mustRecord(t, s, play(other, "same"))
	mustRecord(t, s, play(other, "more"))
	if got := len(recent(t, s, 1)); got != 1 {
		t.Errorf("first user's plays = %d, want 1", got)
	}
	if got := len(recent(t, s, other)); got != 2 {
		t.Errorf("second user's plays = %d, want 2: a client ID is only unique per user", got)
	}
}

func TestRecordPlayFindsAMissingTrack(t *testing.T) {
	t.Parallel()
	s := seededStore(t)
	if _, err := s.w.ExecContext(t.Context(), "UPDATE tracks SET missing_since = 1"); err != nil {
		t.Fatal(err)
	}
	mustRecord(t, s, play(1, "a"))
	if got := len(recent(t, s, 1)); got != 1 {
		t.Errorf("plays = %d, want 1", got)
	}
}

func TestRecordPlayRefusesAnUnknownTrack(t *testing.T) {
	t.Parallel()
	s := seededStore(t)
	p := play(1, "a")
	p.TrackID = "nope"
	if err := s.RecordPlay(t.Context(), p); !errors.Is(err, ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

func TestRecentPlaysAreNewestFirstAndBounded(t *testing.T) {
	t.Parallel()
	s := seededStore(t)
	for i, at := range []int64{2000, 3000, 1000} {
		p := play(1, strconv.Itoa(i))
		p.PlayedAt = time.UnixMilli(at)
		mustRecord(t, s, p)
	}
	got, err := s.RecentPlays(t.Context(), 1, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].PlayedAt.UnixMilli() != 3000 || got[1].PlayedAt.UnixMilli() != 2000 {
		t.Errorf("plays = %+v, want the newest two, newest first", got)
	}
}

func TestDeletingATrackDeletesItsPlays(t *testing.T) {
	t.Parallel()
	s := seededStore(t)
	mustRecord(t, s, play(1, "a"))
	if _, err := s.w.ExecContext(t.Context(), "UPDATE tracks SET missing_since = 1 WHERE ulid = ?", firstTrack); err != nil {
		t.Fatal(err)
	}
	if n, err := s.DeleteMissingBefore(t.Context(), time.UnixMilli(2)); err != nil || n != 1 {
		t.Fatalf("DeleteMissingBefore = %d, %v, want 1", n, err)
	}
	if got := len(recent(t, s, 1)); got != 0 {
		t.Errorf("plays = %d, want 0", got)
	}
}

func TestDeletingAUserDeletesTheirPlays(t *testing.T) {
	t.Parallel()
	s := seededStore(t)
	other := addUser(t, s)
	mustRecord(t, s, play(other, "a"))
	if _, err := s.w.ExecContext(t.Context(), "DELETE FROM users WHERE id = ?", other); err != nil {
		t.Fatal(err)
	}
	if got := rawInt(t, s.w, "SELECT count(*) FROM plays"); got != 0 {
		t.Errorf("plays = %d, want 0", got)
	}
}

// cascadePlan returns the query plan of deleting a track, the cascades in it.
func cascadePlan(t *testing.T, db *sql.DB) string {
	t.Helper()
	conn := pinned(t, db)
	err := conn.Raw(func(raw any) error {
		c, ok := raw.(sqlitedriver.Conn)
		if !ok {
			return errNotSQLite
		}
		if _, configErr := c.Raw().Config(sqlite3.DBCONFIG_TRIGGER_EQP, true); configErr != nil {
			return fmt.Errorf("list cascades: %w", configErr)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	rows, err := conn.QueryContext(t.Context(), "EXPLAIN QUERY PLAN DELETE FROM tracks WHERE id = 1")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	var plan strings.Builder
	for rows.Next() {
		var id, parent, unused int64
		var detail sql.NullString // NULL on the lines that only open a cascade.
		if err = rows.Scan(&id, &parent, &unused, &detail); err != nil {
			t.Fatal(err)
		}
		fmt.Fprintln(&plan, detail.String)
	}
	if err = rows.Err(); err != nil {
		t.Fatal(err)
	}
	return plan.String()
}

// cascadeSeeks is the line of a plan that says the cascade found its rows through
// plays_track. At 2M plays, a scan held the writer for 46.8 s per 100 tracks.
const cascadeSeeks = "SEARCH plays USING COVERING INDEX plays_track (track_id=?)"

func TestCascadeUsesPlaysTrack(t *testing.T) {
	t.Parallel()
	s := openTest(t)
	if plan := cascadePlan(t, s.w); !strings.Contains(plan, cascadeSeeks) {
		t.Errorf("deleting a track doesn't seek plays_track:\n%s", plan)
	}
}

func TestCascadeCheckNoticesALostIndex(t *testing.T) {
	t.Parallel()
	s := openTest(t)
	if _, err := s.w.ExecContext(t.Context(), "DROP INDEX plays_track"); err != nil {
		t.Fatal(err)
	}
	plan := cascadePlan(t, s.w)
	if strings.Contains(plan, cascadeSeeks) {
		t.Errorf("the check passed with no index:\n%s", plan)
	}
	if !strings.Contains(plan, "SCAN plays") {
		t.Errorf("the plan doesn't show the cascade at all, so it proves nothing:\n%s", plan)
	}
}
