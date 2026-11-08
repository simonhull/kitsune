package store

import (
	"errors"
	"strconv"

	"github.com/ncruces/go-sqlite3"
)

// codedError is an SQLite error with its extended result code named after it:
// "disk I/O error" alone can't say whether a write, an fsync or the WAL index failed.
type codedError struct {
	err  error
	code sqlite3.ExtendedErrorCode
}

func (e *codedError) Error() string { return e.err.Error() + " (" + codeName(e.code) + ")" }

func (e *codedError) Unwrap() error { return e.err }

// named adds the extended result code of the first SQLite error in err's chain
// to its message, once. Other errors come back unchanged.
func named(err error) error {
	if err == nil {
		return nil
	}
	if _, done := errors.AsType[*codedError](err); done {
		return err
	}
	code, ok := errors.AsType[sqlite3.ExtendedErrorCode](err)
	if !ok {
		return err
	}
	return &codedError{err: err, code: code}
}

// codeName is SQLite's name for code without its SQLITE_ prefix, such as
// IOERR_FSYNC, or its number for a code this table doesn't know.
func codeName(code sqlite3.ExtendedErrorCode) string {
	if name, ok := codeNames[code]; ok {
		return name
	}
	return "code " + strconv.Itoa(int(code))
}

// codeNames is every result code the driver declares, named as in sqlite3.h.
//
//nolint:gochecknoglobals // A fixed lookup table, never written.
var codeNames = map[sqlite3.ExtendedErrorCode]string{
	sqlite3.ExtendedErrorCode(sqlite3.ERROR):      "ERROR",
	sqlite3.ExtendedErrorCode(sqlite3.INTERNAL):   "INTERNAL",
	sqlite3.ExtendedErrorCode(sqlite3.PERM):       "PERM",
	sqlite3.ExtendedErrorCode(sqlite3.ABORT):      "ABORT",
	sqlite3.ExtendedErrorCode(sqlite3.BUSY):       "BUSY",
	sqlite3.ExtendedErrorCode(sqlite3.LOCKED):     "LOCKED",
	sqlite3.ExtendedErrorCode(sqlite3.NOMEM):      "NOMEM",
	sqlite3.ExtendedErrorCode(sqlite3.READONLY):   "READONLY",
	sqlite3.ExtendedErrorCode(sqlite3.INTERRUPT):  "INTERRUPT",
	sqlite3.ExtendedErrorCode(sqlite3.IOERR):      "IOERR",
	sqlite3.ExtendedErrorCode(sqlite3.CORRUPT):    "CORRUPT",
	sqlite3.ExtendedErrorCode(sqlite3.NOTFOUND):   "NOTFOUND",
	sqlite3.ExtendedErrorCode(sqlite3.FULL):       "FULL",
	sqlite3.ExtendedErrorCode(sqlite3.CANTOPEN):   "CANTOPEN",
	sqlite3.ExtendedErrorCode(sqlite3.PROTOCOL):   "PROTOCOL",
	sqlite3.ExtendedErrorCode(sqlite3.EMPTY):      "EMPTY",
	sqlite3.ExtendedErrorCode(sqlite3.SCHEMA):     "SCHEMA",
	sqlite3.ExtendedErrorCode(sqlite3.TOOBIG):     "TOOBIG",
	sqlite3.ExtendedErrorCode(sqlite3.CONSTRAINT): "CONSTRAINT",
	sqlite3.ExtendedErrorCode(sqlite3.MISMATCH):   "MISMATCH",
	sqlite3.ExtendedErrorCode(sqlite3.MISUSE):     "MISUSE",
	sqlite3.ExtendedErrorCode(sqlite3.NOLFS):      "NOLFS",
	sqlite3.ExtendedErrorCode(sqlite3.AUTH):       "AUTH",
	sqlite3.ExtendedErrorCode(sqlite3.FORMAT):     "FORMAT",
	sqlite3.ExtendedErrorCode(sqlite3.RANGE):      "RANGE",
	sqlite3.ExtendedErrorCode(sqlite3.NOTADB):     "NOTADB",
	sqlite3.ExtendedErrorCode(sqlite3.NOTICE):     "NOTICE",
	sqlite3.ExtendedErrorCode(sqlite3.WARNING):    "WARNING",
	sqlite3.ERROR_MISSING_COLLSEQ:                 "ERROR_MISSING_COLLSEQ",
	sqlite3.ERROR_RETRY:                           "ERROR_RETRY",
	sqlite3.ERROR_SNAPSHOT:                        "ERROR_SNAPSHOT",
	sqlite3.ERROR_RESERVESIZE:                     "ERROR_RESERVESIZE",
	sqlite3.ERROR_KEY:                             "ERROR_KEY",
	sqlite3.ERROR_UNABLE:                          "ERROR_UNABLE",
	sqlite3.IOERR_READ:                            "IOERR_READ",
	sqlite3.IOERR_SHORT_READ:                      "IOERR_SHORT_READ",
	sqlite3.IOERR_WRITE:                           "IOERR_WRITE",
	sqlite3.IOERR_FSYNC:                           "IOERR_FSYNC",
	sqlite3.IOERR_DIR_FSYNC:                       "IOERR_DIR_FSYNC",
	sqlite3.IOERR_TRUNCATE:                        "IOERR_TRUNCATE",
	sqlite3.IOERR_FSTAT:                           "IOERR_FSTAT",
	sqlite3.IOERR_UNLOCK:                          "IOERR_UNLOCK",
	sqlite3.IOERR_RDLOCK:                          "IOERR_RDLOCK",
	sqlite3.IOERR_DELETE:                          "IOERR_DELETE",
	sqlite3.IOERR_BLOCKED:                         "IOERR_BLOCKED",
	sqlite3.IOERR_NOMEM:                           "IOERR_NOMEM",
	sqlite3.IOERR_ACCESS:                          "IOERR_ACCESS",
	sqlite3.IOERR_CHECKRESERVEDLOCK:               "IOERR_CHECKRESERVEDLOCK",
	sqlite3.IOERR_LOCK:                            "IOERR_LOCK",
	sqlite3.IOERR_CLOSE:                           "IOERR_CLOSE",
	sqlite3.IOERR_DIR_CLOSE:                       "IOERR_DIR_CLOSE",
	sqlite3.IOERR_SHMOPEN:                         "IOERR_SHMOPEN",
	sqlite3.IOERR_SHMSIZE:                         "IOERR_SHMSIZE",
	sqlite3.IOERR_SHMLOCK:                         "IOERR_SHMLOCK",
	sqlite3.IOERR_SHMMAP:                          "IOERR_SHMMAP",
	sqlite3.IOERR_SEEK:                            "IOERR_SEEK",
	sqlite3.IOERR_DELETE_NOENT:                    "IOERR_DELETE_NOENT",
	sqlite3.IOERR_MMAP:                            "IOERR_MMAP",
	sqlite3.IOERR_GETTEMPPATH:                     "IOERR_GETTEMPPATH",
	sqlite3.IOERR_CONVPATH:                        "IOERR_CONVPATH",
	sqlite3.IOERR_VNODE:                           "IOERR_VNODE",
	sqlite3.IOERR_AUTH:                            "IOERR_AUTH",
	sqlite3.IOERR_BEGIN_ATOMIC:                    "IOERR_BEGIN_ATOMIC",
	sqlite3.IOERR_COMMIT_ATOMIC:                   "IOERR_COMMIT_ATOMIC",
	sqlite3.IOERR_ROLLBACK_ATOMIC:                 "IOERR_ROLLBACK_ATOMIC",
	sqlite3.IOERR_DATA:                            "IOERR_DATA",
	sqlite3.IOERR_CORRUPTFS:                       "IOERR_CORRUPTFS",
	sqlite3.IOERR_IN_PAGE:                         "IOERR_IN_PAGE",
	sqlite3.IOERR_BADKEY:                          "IOERR_BADKEY",
	sqlite3.IOERR_CODEC:                           "IOERR_CODEC",
	sqlite3.LOCKED_SHAREDCACHE:                    "LOCKED_SHAREDCACHE",
	sqlite3.LOCKED_VTAB:                           "LOCKED_VTAB",
	sqlite3.BUSY_RECOVERY:                         "BUSY_RECOVERY",
	sqlite3.BUSY_SNAPSHOT:                         "BUSY_SNAPSHOT",
	sqlite3.BUSY_TIMEOUT:                          "BUSY_TIMEOUT",
	sqlite3.CANTOPEN_NOTEMPDIR:                    "CANTOPEN_NOTEMPDIR",
	sqlite3.CANTOPEN_ISDIR:                        "CANTOPEN_ISDIR",
	sqlite3.CANTOPEN_FULLPATH:                     "CANTOPEN_FULLPATH",
	sqlite3.CANTOPEN_CONVPATH:                     "CANTOPEN_CONVPATH",
	sqlite3.CANTOPEN_SYMLINK:                      "CANTOPEN_SYMLINK",
	sqlite3.CORRUPT_VTAB:                          "CORRUPT_VTAB",
	sqlite3.CORRUPT_SEQUENCE:                      "CORRUPT_SEQUENCE",
	sqlite3.CORRUPT_INDEX:                         "CORRUPT_INDEX",
	sqlite3.READONLY_RECOVERY:                     "READONLY_RECOVERY",
	sqlite3.READONLY_CANTLOCK:                     "READONLY_CANTLOCK",
	sqlite3.READONLY_ROLLBACK:                     "READONLY_ROLLBACK",
	sqlite3.READONLY_DBMOVED:                      "READONLY_DBMOVED",
	sqlite3.READONLY_CANTINIT:                     "READONLY_CANTINIT",
	sqlite3.READONLY_DIRECTORY:                    "READONLY_DIRECTORY",
	sqlite3.ABORT_ROLLBACK:                        "ABORT_ROLLBACK",
	sqlite3.CONSTRAINT_CHECK:                      "CONSTRAINT_CHECK",
	sqlite3.CONSTRAINT_COMMITHOOK:                 "CONSTRAINT_COMMITHOOK",
	sqlite3.CONSTRAINT_FOREIGNKEY:                 "CONSTRAINT_FOREIGNKEY",
	sqlite3.CONSTRAINT_FUNCTION:                   "CONSTRAINT_FUNCTION",
	sqlite3.CONSTRAINT_NOTNULL:                    "CONSTRAINT_NOTNULL",
	sqlite3.CONSTRAINT_PRIMARYKEY:                 "CONSTRAINT_PRIMARYKEY",
	sqlite3.CONSTRAINT_TRIGGER:                    "CONSTRAINT_TRIGGER",
	sqlite3.CONSTRAINT_UNIQUE:                     "CONSTRAINT_UNIQUE",
	sqlite3.CONSTRAINT_VTAB:                       "CONSTRAINT_VTAB",
	sqlite3.CONSTRAINT_ROWID:                      "CONSTRAINT_ROWID",
	sqlite3.CONSTRAINT_PINNED:                     "CONSTRAINT_PINNED",
	sqlite3.CONSTRAINT_DATATYPE:                   "CONSTRAINT_DATATYPE",
	sqlite3.NOTICE_RECOVER_WAL:                    "NOTICE_RECOVER_WAL",
	sqlite3.NOTICE_RECOVER_ROLLBACK:               "NOTICE_RECOVER_ROLLBACK",
	sqlite3.NOTICE_RBU:                            "NOTICE_RBU",
	sqlite3.WARNING_AUTOINDEX:                     "WARNING_AUTOINDEX",
	sqlite3.AUTH_USER:                             "AUTH_USER",
}
