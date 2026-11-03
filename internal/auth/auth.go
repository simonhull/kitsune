package auth

import "errors"

// Sentinels callers match with [errors.Is].
var (
	// ErrInvalidCredentials means the name or password was wrong. It is the same for both.
	ErrInvalidCredentials = errors.New("invalid name or password")
	// ErrUnauthenticated means the token is missing, malformed, unknown or expired.
	ErrUnauthenticated = errors.New("not authenticated")
	// ErrInvalidName means the user name breaks the naming rules.
	ErrInvalidName = errors.New("invalid user name")
	// ErrPasswordTooShort means the password has fewer than 8 characters.
	ErrPasswordTooShort = errors.New("password too short: use at least 8 characters")
	// ErrPasswordTooLong means the password is longer than 1024 bytes.
	ErrPasswordTooLong = errors.New("password too long: use at most 1024 bytes")
)
