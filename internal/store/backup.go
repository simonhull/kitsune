package store

import (
	"cmp"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
)

const (
	// backupSpaceFactor is how many times the database's size must be free
	// before a backup. Measured peak use is about 2.7 times.
	backupSpaceFactor = 3
	// backupsKept is how many distinct from-versions keep a backup.
	backupsKept = 3
	// backupStampLayout names a backup to the second, so a retry on the same
	// day gets a name of its own.
	backupStampLayout = "20060102T150405Z"
)

// backupSuffix is what follows the database's name in a backup's: the version
// the backup holds and when it was taken.
var backupSuffix = regexp.MustCompile(`^\.v(\d+)\.(\d{8}T\d{6}Z)\.bak$`)

// errBackupSpace means the disk is too full to back up.
var errBackupSpace = errors.New("not enough free disk space")

// upgrade is how settleSchema treats a database behind this binary. Tests
// replace the disk's answers.
type upgrade struct {
	migrate    bool
	log        *slog.Logger
	now        func() time.Time
	freeSpace  func(dir string) (uint64, error)
	vacuumInto func(ctx context.Context, w *sql.DB, target string) error
}

// newUpgrade returns the upgrade opts ask for, on the real disk.
func newUpgrade(opts *Options, now func() time.Time) *upgrade {
	return &upgrade{migrate: opts.Migrate, log: opts.logger(), now: now, freeSpace: freeSpace, vacuumInto: vacuumInto}
}

// vacuumInto writes a compact copy of the database to target, WAL included.
func vacuumInto(ctx context.Context, w *sql.DB, target string) error {
	if _, err := w.ExecContext(ctx, "VACUUM INTO ?", target); err != nil {
		return fmt.Errorf("vacuum into %s: %w", target, err)
	}
	return nil
}

// backup copies the database at path, at version from, to a file beside it and
// returns that file's path. Any failure leaves no partial file behind, and must
// stop the upgrade.
func (u *upgrade) backup(ctx context.Context, w *sql.DB, path string, from int64) (string, error) {
	dir := filepath.Dir(path)
	if err := u.requireSpace(path, dir); err != nil {
		return "", err
	}
	// CreateTemp makes the file 0600. VACUUM INTO would make it 0644 beside a
	// database of password hashes, and it accepts an empty file.
	f, err := os.CreateTemp(dir, filepath.Base(path)+".backup-*")
	if err != nil {
		return "", fmt.Errorf("create: %w", err)
	}
	temp := f.Name()
	name := fmt.Sprintf("%s.v%d.%s.bak", path, from, u.now().UTC().Format(backupStampLayout))
	err = u.fill(ctx, w, f)
	if err == nil {
		err = os.Rename(temp, name)
	}
	if err != nil {
		return "", errors.Join(fmt.Errorf("write: %w", err), removeIfPresent(temp), removeIfPresent(temp+"-journal"))
	}
	return name, nil
}

// fill writes the backup into f, makes it durable and closes f.
func (u *upgrade) fill(ctx context.Context, w *sql.DB, f *os.File) error {
	err := u.vacuumInto(ctx, w, f.Name())
	if err == nil {
		err = f.Sync()
	}
	return errors.Join(err, f.Close())
}

// requireSpace fails unless dir has room for a backup of the database at path.
func (u *upgrade) requireSpace(path, dir string) error {
	size, err := databaseSize(path)
	if err != nil {
		return err
	}
	free, err := u.freeSpace(dir)
	if err != nil {
		return fmt.Errorf("check free space: %w", err)
	}
	if free/backupSpaceFactor < size {
		return fmt.Errorf("%w: %d bytes free, want %d times the database's %d", errBackupSpace, free, backupSpaceFactor, size)
	}
	return nil
}

// databaseSize adds up the database and its WAL.
func databaseSize(path string) (uint64, error) {
	var total uint64
	for _, p := range []string{path, path + "-wal"} {
		fi, err := os.Stat(p)
		switch {
		case errors.Is(err, fs.ErrNotExist):
			continue
		case err != nil:
			return 0, fmt.Errorf("measure database: %w", err)
		}
		total += uint64(fi.Size()) //nolint:gosec // G115: a file's size is never negative.
	}
	return total, nil
}

// removeIfPresent removes path, which may not exist.
func removeIfPresent(path string) error {
	if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("remove partial backup: %w", err)
	}
	return nil
}

// backupFile is a backup found beside the database.
type backupFile struct {
	path  string
	from  int64
	stamp string
}

// listBackups returns the backups of the database at path, newest first.
func listBackups(path string) ([]backupFile, error) {
	dir, base := filepath.Dir(path), filepath.Base(path)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("list backups: %w", err)
	}
	var backups []backupFile
	for _, e := range entries {
		suffix, ok := strings.CutPrefix(e.Name(), base)
		if !ok {
			continue
		}
		m := backupSuffix.FindStringSubmatch(suffix)
		if m == nil {
			continue
		}
		from, parseErr := strconv.ParseInt(m[1], 10, 64)
		if parseErr != nil {
			continue // Too long to be a version, so not one of ours.
		}
		backups = append(backups, backupFile{path: filepath.Join(dir, e.Name()), from: from, stamp: m[2]})
	}
	slices.SortFunc(backups, func(a, b backupFile) int { return cmp.Compare(b.stamp, a.stamp) })
	return backups, nil
}

// pruneBackups keeps the newest backup of each of the last backupsKept
// from-versions. It runs only after a successful upgrade, so a crash loop on one
// bad version can't push the good backups out.
func pruneBackups(path string) error {
	backups, err := listBackups(path)
	if err != nil {
		return err
	}
	kept := make(map[int64]struct{}, backupsKept)
	var errs []error
	for i := range backups {
		_, seen := kept[backups[i].from]
		if !seen && len(kept) < backupsKept {
			kept[backups[i].from] = struct{}{}
			continue
		}
		if err = os.Remove(backups[i].path); err != nil {
			errs = append(errs, fmt.Errorf("prune backup: %w", err))
		}
	}
	return errors.Join(errs...)
}
