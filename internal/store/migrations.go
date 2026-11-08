package store

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"regexp"
	"strconv"

	sqlitedriver "github.com/ncruces/go-sqlite3/driver"
)

// migrationsDir is where the numbered files live, in [migrationFiles].
const migrationsDir = "migrations"

// manifestName is the file listing every migration with its sha256. Nothing
// reads it at run time: a test checks it, and it makes two branches that add a
// migration conflict in git.
const manifestName = "manifest.txt"

//go:embed migrations
var migrationFiles embed.FS

// migrationName is the only shape a migration file may have: NNNN_name.sql.
var migrationName = regexp.MustCompile(`^(\d{4})_[a-z0-9_]+\.sql$`)

var (
	// errMigrationName means a file in migrations/ isn't a migration.
	errMigrationName = errors.New("want NNNN_name.sql")
	// errMigrationGap means the numbers aren't contiguous from 1.
	errMigrationGap = errors.New("numbers must run from 1 without gaps or repeats")
	// errForeignKeysOn means foreign keys weren't off where a migration needs them off.
	errForeignKeysOn = errors.New("foreign keys are on inside the transaction")
	// errForeignKeysOff means foreign keys didn't come back on after migrating.
	errForeignKeysOff = errors.New("foreign keys did not come back on")
	// errTransactionLost means a migration ended the transaction it runs in.
	errTransactionLost = errors.New("the file ended its transaction, so part of it may be applied: restore the backup")
	// errDanglingRows means a migration left rows pointing at nothing.
	errDanglingRows = errors.New("foreign key check failed")
	// errNotSQLite means the writer's connection isn't this driver's.
	errNotSQLite = errors.New("not a SQLite connection")
)

// migration is one numbered step of the schema.
type migration struct {
	version int64  // The user_version the step leaves behind.
	name    string // The file name, which errors report.
	sql     string
}

// parseMigrations reads the migrations from files, in order. It accepts
// nothing but the manifest and contiguous NNNN_name.sql files.
func parseMigrations(files fs.FS) ([]migration, error) {
	entries, err := fs.ReadDir(files, migrationsDir)
	if err != nil {
		return nil, fmt.Errorf("read migrations: %w", err)
	}
	steps := make([]migration, 0, len(entries))
	for _, e := range entries {
		if e.Name() == manifestName {
			continue
		}
		step, parseErr := parseMigration(files, e.Name())
		if parseErr != nil {
			return nil, parseErr
		}
		if step.version != int64(len(steps)+1) {
			return nil, fmt.Errorf("migration %s: %w", step.name, errMigrationGap)
		}
		steps = append(steps, step)
	}
	return steps, nil
}

// parseMigration reads the migration called name.
func parseMigration(files fs.FS, name string) (migration, error) {
	match := migrationName.FindStringSubmatch(name)
	if match == nil {
		return migration{}, fmt.Errorf("migration %s: %w", name, errMigrationName)
	}
	version, err := strconv.ParseInt(match[1], 10, 64)
	if err != nil {
		return migration{}, fmt.Errorf("migration %s: %w", name, err)
	}
	body, err := fs.ReadFile(files, path.Join(migrationsDir, name))
	if err != nil {
		return migration{}, fmt.Errorf("migration %s: %w", name, err)
	}
	return migration{version: version, name: name, sql: string(body)}, nil
}

// runMigrations applies steps in order on one pinned connection, adopting a
// database from before migrations first. Each step is one transaction, so a
// failure leaves that step undone and the ones before it done.
//
// Foreign keys stay off throughout. With them on, dropping a parent table
// during a rebuild cascades through its children, silently and with nothing
// left for foreign_key_check to find. The pragma can't change inside a
// transaction, so it is set around them, and the writer keeps its one
// connection for life, so it is restored on every path.
func runMigrations(ctx context.Context, w *sql.DB, steps []migration) (err error) {
	conn, err := w.Conn(ctx)
	if err != nil {
		return fmt.Errorf("pin connection: %w", err)
	}
	defer func() {
		if closeErr := conn.Close(); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("unpin connection: %w", closeErr))
		}
	}()
	if _, err = conn.ExecContext(ctx, "PRAGMA foreign_keys = OFF"); err != nil {
		return fmt.Errorf("foreign keys off: %w", err)
	}
	defer func() {
		// A canceled start must still leave the connection as it found it.
		err = errors.Join(err, restoreForeignKeys(context.WithoutCancel(ctx), conn))
	}()
	if adoptErr := adopt(ctx, conn); adoptErr != nil {
		return adoptErr
	}
	for i := range steps {
		if err = applyStep(ctx, conn, &steps[i]); err != nil {
			return fmt.Errorf("migration %s: %w", steps[i].name, err)
		}
	}
	return nil
}

// restoreForeignKeys turns foreign keys back on and checks that they are.
func restoreForeignKeys(ctx context.Context, conn *sql.Conn) error {
	if _, err := conn.ExecContext(ctx, "PRAGMA foreign_keys = ON"); err != nil {
		return fmt.Errorf("foreign keys on: %w", err)
	}
	var on int64
	if err := conn.QueryRowContext(ctx, "PRAGMA foreign_keys").Scan(&on); err != nil {
		return fmt.Errorf("foreign keys on: %w", err)
	}
	if on != 1 {
		return errForeignKeysOff
	}
	return nil
}

// adopt renumbers a database made before migrations as version 1. Its schema
// is 0001's, so there is nothing to apply.
func adopt(ctx context.Context, conn *sql.Conn) error {
	return inTransaction(ctx, conn, func(tx *sql.Tx) error {
		version, err := readPragma(ctx, tx, "user_version")
		if err != nil || version != adoptHash {
			return err
		}
		return stampVersion(ctx, tx, 1)
	})
}

// applyStep runs one migration in its own transaction. Between the caller's
// read of the version and here, another starter may have run it: BEGIN IMMEDIATE
// has the write lock, so the version read here is the one that counts.
func applyStep(ctx context.Context, conn *sql.Conn, m *migration) error {
	return inTransaction(ctx, conn, func(tx *sql.Tx) error {
		version, err := readPragma(ctx, tx, "user_version")
		if err != nil {
			return err
		}
		if version >= m.version {
			return nil
		}
		if bodyErr := runBody(ctx, conn, tx, m); bodyErr != nil {
			return bodyErr
		}
		return stampVersion(ctx, tx, m.version)
	})
}

// runBody runs a migration's SQL and checks what it left behind.
func runBody(ctx context.Context, conn *sql.Conn, tx *sql.Tx, m *migration) error {
	foreignKeys, err := readPragma(ctx, tx, "foreign_keys")
	if err != nil {
		return err
	}
	if foreignKeys != 0 {
		return errForeignKeysOn
	}
	if _, err = tx.ExecContext(ctx, m.sql); err != nil {
		return fmt.Errorf("apply: %w", err)
	}
	if lost := requireTransaction(ctx, conn); lost != nil {
		return lost
	}
	return checkForeignKeys(ctx, tx)
}

// inTransaction runs fn in a transaction and commits if it returns nil. The
// writer's transactions are BEGIN IMMEDIATE. An SQLite error comes back with its
// extended result code named.
func inTransaction(ctx context.Context, conn *sql.Conn, fn func(*sql.Tx) error) (err error) {
	defer func() { err = named(err) }() // Deferred first, so it runs after the rollback.
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer func() {
		if rbErr := tx.Rollback(); rbErr != nil && !errors.Is(rbErr, sql.ErrTxDone) {
			err = errors.Join(err, fmt.Errorf("rollback: %w", rbErr))
		}
	}()
	if fnErr := fn(tx); fnErr != nil {
		return fnErr
	}
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}

// readPragma reads an integer pragma. name is always a literal here.
func readPragma(ctx context.Context, tx *sql.Tx, name string) (int64, error) {
	var v int64
	if err := tx.QueryRowContext(ctx, "PRAGMA "+name).Scan(&v); err != nil {
		return 0, fmt.Errorf("read %s: %w", name, err)
	}
	return v, nil
}

// requireTransaction fails if SQLite is no longer inside a transaction, which
// database/sql can't see: a COMMIT in a migration file ends it behind its back.
func requireTransaction(ctx context.Context, conn *sql.Conn) error {
	return conn.Raw(func(raw any) error { //nolint:wrapcheck // The callback's errors are this package's own.
		c, ok := raw.(sqlitedriver.Conn)
		if !ok {
			return errNotSQLite
		}
		if c.Raw().GetAutocommit() {
			return errTransactionLost
		}
		return ctx.Err()
	})
}

// checkForeignKeys fails if any row points at nothing. It takes Query, not
// Exec, because the pragma reports through rows and Exec would drop them.
func checkForeignKeys(ctx context.Context, tx *sql.Tx) error {
	rows, err := tx.QueryContext(ctx, "PRAGMA foreign_key_check")
	if err != nil {
		return fmt.Errorf("foreign key check: %w", err)
	}
	defer func() { _ = rows.Close() }()
	count := 0
	var table, parent string
	for rows.Next() {
		var rowID sql.NullInt64 // NULL for a WITHOUT ROWID child.
		var fkID int64
		if err = rows.Scan(&table, &rowID, &parent, &fkID); err != nil {
			return fmt.Errorf("foreign key check: %w", err)
		}
		count++
	}
	if err = rows.Err(); err != nil {
		return fmt.Errorf("foreign key check: %w", err)
	}
	if count > 0 {
		return fmt.Errorf("%w: %d rows, the last in %s pointing at %s", errDanglingRows, count, table, parent)
	}
	return nil
}

// stampVersion marks the file as Kitsune's at version. Pragmas can't take bound
// parameters; both values are integers from this package.
func stampVersion(ctx context.Context, tx *sql.Tx, version int64) error {
	for _, stmt := range []string{
		fmt.Sprintf("PRAGMA application_id = %d", kitsuneAppID),
		fmt.Sprintf("PRAGMA user_version = %d", version),
	} {
		if _, err := tx.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("stamp version %d: %w", version, err)
		}
	}
	return nil
}
