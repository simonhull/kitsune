package store

import (
	"context"
	"database/sql"
	"fmt"
)

// kitsuneAppID marks a SQLite file as Kitsune's ("KTSN").
const kitsuneAppID = 0x4B54534E

// adoptHash is the user_version of a database made before migrations existed:
// a hash of 0001_baseline.sql, which is that schema byte for byte. Keep it while
// backups from before adoption may exist.
const adoptHash = 852615662

// hashFloor splits user_version: below it, a count of migrations; from it up,
// a hash left by a build from before migrations. Every known hash is far above.
const hashFloor = 1 << 20

// probeSQL reads what settleSchema decides on: the number of schema rows, the
// application_id and the user_version.
const probeSQL = `SELECT (SELECT count(*) FROM sqlite_schema), application_id, user_version
FROM pragma_application_id, pragma_user_version`

// schemaState is what the probe found.
type schemaState struct {
	objects, appID, version int64
}

// refusal says why the file can't be Kitsune's at any version.
func (st *schemaState) refusal() error {
	switch {
	case st.objects > 0 && st.appID != kitsuneAppID:
		return ErrNotKitsune
	case st.objects > 0 && st.version == 0:
		return ErrUnversioned
	case st.objects == 0 && st.version != 0:
		return ErrStaleSchema
	}
	return nil
}

// startVersion returns the migration count the database stands at, adopting a
// hash as version 1, or the reason a binary at latest can't use it.
func (st *schemaState) startVersion(latest int64) (int64, error) {
	if err := st.refusal(); err != nil {
		return 0, err
	}
	switch v := st.version; {
	case v == adoptHash:
		return 1, nil
	case v >= 0 && v <= latest:
		return v, nil
	case v > latest && v < hashFloor:
		return 0, fmt.Errorf("at version %d, this binary stops at %d: %w", v, latest, ErrNewerSchema)
	default:
		return 0, ErrStaleSchema
	}
}

// settleSchema brings the database to the embedded schema, or says why it
// can't. Without u.migrate it only creates an empty database, and refuses one
// that has migrations pending.
func settleSchema(ctx context.Context, w *sql.DB, path string, u *upgrade) error {
	steps, err := parseMigrations(migrationFiles)
	if err != nil {
		return fmt.Errorf("open %s: %w", path, err)
	}
	return settleSchemaWith(ctx, w, path, steps, u)
}

// settleSchemaWith is settleSchema with the migrations injected, so tests can
// try broken ones and stop short of the latest.
func settleSchemaWith(ctx context.Context, w *sql.DB, path string, steps []migration, u *upgrade) error {
	var st schemaState
	if err := w.QueryRowContext(ctx, probeSQL).Scan(&st.objects, &st.appID, &st.version); err != nil {
		return fmt.Errorf("open %s: probe: %w", path, err)
	}
	latest := int64(len(steps))
	from, err := st.startVersion(latest)
	if err != nil {
		return fmt.Errorf("open %s: %w", path, err)
	}
	if st.version == latest {
		return nil
	}
	if st.objects == 0 {
		if err = runMigrations(ctx, w, steps); err != nil {
			return fmt.Errorf("open %s: %w", path, err)
		}
		return nil
	}
	if !u.migrate {
		return fmt.Errorf("open %s: at version %d, this binary needs %d: %w", path, st.version, latest, ErrMigrationsPending)
	}
	saved, err := u.apply(ctx, w, path, steps[from:], from)
	if err != nil {
		return fmt.Errorf("open %s: %w", path, err)
	}
	u.log.InfoContext(ctx, "database migrated", "from", st.version, "to", latest, "backup", saved)
	return nil
}

// apply upgrades a database with data in it from version from, and returns the
// backup's path. Adoption alone changes nothing but the number, so only real
// steps get a backup.
func (u *upgrade) apply(ctx context.Context, w *sql.DB, path string, steps []migration, from int64) (string, error) {
	if len(steps) == 0 {
		return "", runMigrations(ctx, w, steps)
	}
	saved, err := u.backup(ctx, w, path, from)
	if err != nil {
		return "", fmt.Errorf("back up before migrating, which did not start: %w", err)
	}
	if migrateErr := runMigrations(ctx, w, steps); migrateErr != nil {
		return "", migrateErr
	}
	if pruneErr := pruneBackups(path); pruneErr != nil {
		u.log.WarnContext(ctx, "pruning old database backups failed", "err", pruneErr)
	}
	return saved, nil
}
