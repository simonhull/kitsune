package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/ncruces/go-sqlite3"

	"github.com/simonhull/kitsune/internal/store/sqlcdb"
)

// Role is a user's permission level.
type Role string

const (
	// RoleAdmin can manage users and the library.
	RoleAdmin Role = "admin"
	// RoleUser can browse, play and keep their own favorites and playlists.
	RoleUser Role = "user"
)

// User is a Kitsune account.
type User struct {
	ID           int64
	Name         string
	PasswordHash string // An encoded hash from package auth; never a password.
	Role         Role
	CreatedAt    time.Time
}

// NewUser is the input to [Store.CreateUser].
type NewUser struct {
	Name         string
	PasswordHash string
	Role         Role
}

// CreateUser adds a user. It returns an error wrapping [ErrNameTaken] if the
// name is in use, ignoring ASCII case.
func (s *Store) CreateUser(ctx context.Context, u NewUser) (User, error) {
	createdAt := s.now().UnixMilli()
	id, err := s.wq.CreateUser(ctx, sqlcdb.CreateUserParams{
		Name:         u.Name,
		PasswordHash: u.PasswordHash,
		Role:         string(u.Role),
		CreatedAt:    createdAt,
	})
	if err != nil {
		if errors.Is(err, sqlite3.CONSTRAINT_UNIQUE) {
			return User{}, fmt.Errorf("create user %q: %w", u.Name, ErrNameTaken)
		}
		return User{}, fmt.Errorf("create user: %w", err)
	}
	return User{
		ID:           id,
		Name:         u.Name,
		PasswordHash: u.PasswordHash,
		Role:         u.Role,
		CreatedAt:    time.UnixMilli(createdAt).UTC(),
	}, nil
}

// CreateFirstUser adds u only while there are no users, in one statement, so
// of two concurrent calls on an empty store exactly one succeeds. It returns
// an error wrapping [ErrNotFirst] once any user exists.
func (s *Store) CreateFirstUser(ctx context.Context, u NewUser) (User, error) {
	createdAt := s.now().UnixMilli()
	res, err := s.wq.CreateFirstUser(ctx, sqlcdb.CreateFirstUserParams{
		Name:         u.Name,
		PasswordHash: u.PasswordHash,
		Role:         string(u.Role),
		CreatedAt:    createdAt,
	})
	if err != nil {
		return User{}, fmt.Errorf("create first user: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return User{}, fmt.Errorf("create first user: %w", err)
	}
	if n == 0 {
		return User{}, ErrNotFirst
	}
	id, err := res.LastInsertId()
	if err != nil {
		return User{}, fmt.Errorf("create first user: %w", err)
	}
	return User{
		ID:           id,
		Name:         u.Name,
		PasswordHash: u.PasswordHash,
		Role:         u.Role,
		CreatedAt:    time.UnixMilli(createdAt).UTC(),
	}, nil
}

// UserByName returns the user with name, ignoring ASCII case, or an error
// wrapping [ErrNotFound].
func (s *Store) UserByName(ctx context.Context, name string) (User, error) {
	row, err := s.rq.UserByName(ctx, name)
	if errors.Is(err, sql.ErrNoRows) {
		return User{}, fmt.Errorf("user %q: %w", name, ErrNotFound)
	}
	if err != nil {
		return User{}, fmt.Errorf("user by name: %w", err)
	}
	return User{
		ID:           row.ID,
		Name:         row.Name,
		PasswordHash: row.PasswordHash,
		Role:         Role(row.Role),
		CreatedAt:    time.UnixMilli(row.CreatedAt).UTC(),
	}, nil
}

// CountUsers returns the number of users. At zero, serve opens first-run setup.
func (s *Store) CountUsers(ctx context.Context) (int, error) {
	n, err := s.rq.CountUsers(ctx)
	if err != nil {
		return 0, fmt.Errorf("count users: %w", err)
	}
	return int(n), nil
}

// SetPasswordHash replaces a user's password hash. It returns an error
// wrapping [ErrNotFound] if no user has id.
func (s *Store) SetPasswordHash(ctx context.Context, id int64, hash string) error {
	n, err := s.wq.SetPasswordHash(ctx, sqlcdb.SetPasswordHashParams{PasswordHash: hash, ID: id})
	if err != nil {
		return fmt.Errorf("set password hash: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("user %d: %w", id, ErrNotFound)
	}
	return nil
}

// SetPassword replaces a user's password hash and ends their sessions, in one
// transaction, so no session made with the old password outlives it. The
// session whose token hash is keep stays; a nil keep ends them all. It returns
// an error wrapping [ErrNotFound] if no user has id.
func (s *Store) SetPassword(ctx context.Context, id int64, hash string, keep []byte) error {
	err := s.inTx(ctx, func(q *sqlcdb.Queries, _ *sql.Tx) error {
		n, err := q.SetPasswordHash(ctx, sqlcdb.SetPasswordHashParams{PasswordHash: hash, ID: id})
		if err != nil {
			return fmt.Errorf("set password hash: %w", err)
		}
		if n == 0 {
			return fmt.Errorf("user %d: %w", id, ErrNotFound)
		}
		if keep == nil {
			_, err = q.DeleteSessionsOf(ctx, id)
		} else {
			_, err = q.DeleteOtherSessionsOf(ctx, sqlcdb.DeleteOtherSessionsOfParams{UserID: id, TokenHash: keep})
		}
		if err != nil {
			return fmt.Errorf("end sessions: %w", err)
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("set password: %w", err)
	}
	return nil
}

// Users returns every user by name, without their password hashes.
func (s *Store) Users(ctx context.Context) ([]User, error) {
	rows, err := s.rq.ListUsers(ctx)
	if err != nil {
		return nil, fmt.Errorf("list users: %w", err)
	}
	users := make([]User, 0, len(rows))
	for _, r := range rows {
		users = append(users, User{ID: r.ID, Name: r.Name, Role: Role(r.Role), CreatedAt: time.UnixMilli(r.CreatedAt).UTC()})
	}
	return users, nil
}
