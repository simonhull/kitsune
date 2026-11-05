package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

const nextStep = "CREATE TABLE next (x INTEGER) STRICT;"

// backupClock is the time every backup test takes its backups at.
func backupClock() time.Time { return time.Date(2026, 11, 5, 14, 30, 12, 0, time.UTC) }

// testUpgrade migrates on the real disk at backupClock.
func testUpgrade() *upgrade { return newUpgrade(&Options{Migrate: true}, backupClock) }

// dirNames lists path's directory, apart from the database's own files.
func dirNames(t *testing.T, path string) []string {
	t.Helper()
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	names := []string{}
	for _, e := range entries {
		if n := e.Name(); n != FileName && n != FileName+"-wal" && n != FileName+"-shm" {
			names = append(names, n)
		}
	}
	return names
}

func TestBackupBeforeMigrating(t *testing.T) {
	t.Parallel()
	d, steps := seededDB(t)
	before := libraryFacts(t, d.w)

	if err := settleSchemaWith(t.Context(), d.w, d.path, withStep(steps, "next", nextStep), testUpgrade()); err != nil {
		t.Fatal(err)
	}

	want := fmt.Sprintf("%s.v%d.20261105T143012Z.bak", FileName, len(steps))
	if got := dirNames(t, d.path); !slices.Equal(got, []string{want}) {
		t.Fatalf("beside the database: %q, want %q", got, want)
	}
	backup := filepath.Join(filepath.Dir(d.path), want)
	fi, err := os.Stat(backup)
	if err != nil {
		t.Fatal(err)
	}
	if perm := fi.Mode().Perm(); perm != privateFileMode {
		t.Errorf("backup mode = %o, want %o", perm, privateFileMode)
	}
	saved := rawDB(t, backup)
	if got := rawInt(t, saved, "PRAGMA user_version"); got != int64(len(steps)) {
		t.Errorf("backup user_version = %d, want %d", got, len(steps))
	}
	if got := rawInt(t, saved, "SELECT count(*) FROM sqlite_schema WHERE name = 'next'"); got != 0 {
		t.Error("the backup holds the migration it was meant to precede")
	}
	expectFacts(t, saved, before)
}

// TestBackupRestores follows the steps in AGENTS.md: the backup, without the
// database's -wal and -shm, opens under a binary at its version.
func TestBackupRestores(t *testing.T) {
	t.Parallel()
	d, steps := seededDB(t)
	before := libraryFacts(t, d.w)
	bad := withStep(steps, "next", "CREATE TABLE half (x INTEGER) STRICT;\nDELETE FROM users;\nDROP TABLE nowhere;")
	if err := settleSchemaWith(t.Context(), d.w, d.path, bad, testUpgrade()); err == nil {
		t.Fatal("the bad migration succeeded")
	}
	backups, err := listBackups(d.path)
	if err != nil || len(backups) != 1 {
		t.Fatalf("listBackups = %v, %v, want one", backups, err)
	}
	if err = d.w.Close(); err != nil {
		t.Fatal(err)
	}
	if err = os.Rename(backups[0].path, d.path); err != nil {
		t.Fatal(err)
	}
	if err = errors.Join(removeIfPresent(d.path+"-wal"), removeIfPresent(d.path+"-shm")); err != nil {
		t.Fatal(err)
	}

	s, err := Open(t.Context(), d.path, Options{Migrate: true})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	expectFacts(t, s.w, before)
}

func TestNoBackupWithoutData(t *testing.T) {
	t.Parallel()
	d := newScratchDB(t)
	d.mustSettle(t, shippedSteps(t))
	if got := dirNames(t, d.path); len(got) != 0 {
		t.Errorf("creating a database left %q", got)
	}
}

func TestNoBackupWhenCurrent(t *testing.T) {
	t.Parallel()
	d, steps := seededDB(t)
	d.mustSettle(t, steps)
	if got := dirNames(t, d.path); len(got) != 0 {
		t.Errorf("a current database left %q", got)
	}
}

// expectUpgradeRefused runs one pending step under u and checks that it stopped
// before touching anything.
func expectUpgradeRefused(t *testing.T, d scratchDB, steps []migration, u *upgrade, want error) {
	t.Helper()
	err := settleSchemaWith(t.Context(), d.w, d.path, withStep(steps, "next", nextStep), u)
	if !errors.Is(err, want) {
		t.Fatalf("err = %v, want %v", err, want)
	}
	if got := rawInt(t, d.w, "PRAGMA user_version"); got != int64(len(steps)) {
		t.Errorf("user_version = %d, want %d", got, len(steps))
	}
	if got := rawInt(t, d.w, "SELECT count(*) FROM sqlite_schema WHERE name = 'next'"); got != 0 {
		t.Error("the migration ran without a backup")
	}
	if got := dirNames(t, d.path); len(got) != 0 {
		t.Errorf("left behind: %q", got)
	}
}

func TestLowDiskStopsTheUpgrade(t *testing.T) {
	t.Parallel()
	d, steps := seededDB(t)
	u := testUpgrade()
	u.freeSpace = func(string) (uint64, error) { return 1024, nil }
	expectUpgradeRefused(t, d, steps, u, errBackupSpace)
}

func TestSpaceCheckWantsThreeTimesTheDatabase(t *testing.T) {
	t.Parallel()
	d, _ := seededDB(t)
	size, err := databaseSize(d.path)
	if err != nil {
		t.Fatal(err)
	}
	for free, want := range map[uint64]error{3*size - 1: errBackupSpace, 3 * size: nil} {
		u := testUpgrade()
		u.freeSpace = func(string) (uint64, error) { return free, nil }
		if got := u.requireSpace(d.path, filepath.Dir(d.path)); !errors.Is(got, want) {
			t.Errorf("free %d of size %d: err = %v, want %v", free, size, got, want)
		}
	}
}

func TestFailedBackupLeavesNoPartialFile(t *testing.T) {
	t.Parallel()
	d, steps := seededDB(t)
	diskFull := errors.New("disk I/O error")
	u := testUpgrade()
	u.vacuumInto = func(_ context.Context, _ *sql.DB, target string) error {
		if err := os.WriteFile(target, []byte("partial"), privateFileMode); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(target+"-journal", []byte("partial"), privateFileMode); err != nil {
			t.Fatal(err)
		}
		return diskFull
	}
	expectUpgradeRefused(t, d, steps, u, diskFull)
}

func TestSameSecondRetryKeepsOneBackup(t *testing.T) {
	t.Parallel()
	d, steps := seededDB(t)
	bad := withStep(steps, "next", "INSERT INTO nowhere VALUES (1);")
	for range 2 {
		if err := settleSchemaWith(t.Context(), d.w, d.path, bad, testUpgrade()); err == nil || strings.Contains(err.Error(), "back up") {
			t.Fatalf("err = %v, want the migration's own failure", err)
		}
	}
	if got := dirNames(t, d.path); len(got) != 1 {
		t.Errorf("beside the database: %q, want one backup", got)
	}
}

// touchBackups creates empty backups named by their suffixes.
func touchBackups(t *testing.T, path string, suffixes ...string) {
	t.Helper()
	for _, s := range suffixes {
		if err := os.WriteFile(path+s, nil, privateFileMode); err != nil {
			t.Fatal(err)
		}
	}
}

func TestPruneKeepsTheLastThreeFromVersions(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), FileName)
	touchBackups(t, path,
		".v1.20260101T000000Z.bak",
		".v2.20260201T000000Z.bak",
		".v3.20260301T000000Z.bak",
		".v4.20260401T000000Z.bak",
		".v4.20260402T000000Z.bak",
		".v9.notastamp.bak",
		".bak",
	)
	if err := pruneBackups(path); err != nil {
		t.Fatal(err)
	}
	want := []string{
		FileName + ".bak",
		FileName + ".v2.20260201T000000Z.bak",
		FileName + ".v3.20260301T000000Z.bak",
		FileName + ".v4.20260402T000000Z.bak",
		FileName + ".v9.notastamp.bak",
	}
	if got := dirNames(t, path); !slices.Equal(got, want) {
		t.Errorf("after pruning: %q, want %q", got, want)
	}
}

// TestCrashLoopKeepsGoodBackups is why pruning waits for success: however often
// one bad version fails, the backups from earlier versions stay.
func TestCrashLoopKeepsGoodBackups(t *testing.T) {
	t.Parallel()
	d, steps := seededDB(t)
	old := []string{".v1.20260101T000000Z.bak", ".v2.20260201T000000Z.bak", ".v3.20260301T000000Z.bak"}
	touchBackups(t, d.path, old...)
	bad := withStep(steps, "next", "INSERT INTO nowhere VALUES (1);")
	for i := range 5 {
		u := testUpgrade()
		u.now = func() time.Time { return backupClock().Add(time.Duration(i) * time.Minute) }
		if err := settleSchemaWith(t.Context(), d.w, d.path, bad, u); err == nil {
			t.Fatal("the bad migration succeeded")
		}
	}
	got := strings.Join(dirNames(t, d.path), " ")
	for _, s := range old {
		if !strings.Contains(got, FileName+s) {
			t.Errorf("%s went during a crash loop: %s", s, got)
		}
	}
}

func TestSuccessPrunes(t *testing.T) {
	t.Parallel()
	d, steps := seededDB(t)
	touchBackups(t, d.path,
		".v7.20260101T000000Z.bak",
		".v8.20260201T000000Z.bak",
		".v9.20260301T000000Z.bak",
	)
	if err := settleSchemaWith(t.Context(), d.w, d.path, withStep(steps, "next", nextStep), testUpgrade()); err != nil {
		t.Fatal(err)
	}
	backups, err := listBackups(d.path)
	if err != nil {
		t.Fatal(err)
	}
	if len(backups) != backupsKept || backups[backupsKept-1].from != 8 {
		t.Errorf("backups = %+v, want the v7 one gone", backups)
	}
}

func TestFreeSpaceReadsTheDisk(t *testing.T) {
	t.Parallel()
	free, err := freeSpace(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if free == 0 {
		t.Error("freeSpace = 0 on a directory a test just wrote")
	}
}
