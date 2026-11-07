package auth

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/simonhull/kitsune/internal/store"
)

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
	// ErrSetupDone means a first administrator was asked for, but an account exists.
	ErrSetupDone = errors.New("already set up: an account exists")

	// errNegativeHashes is the tail of the error for a negative Options.MaxConcurrentHashes.
	errNegativeHashes = errors.New("want 0 or more")
)

// SessionMaxAge is the absolute lifetime of a session, counted from its
// creation. A session also expires after 30 idle days.
const SessionMaxAge = 365 * 24 * time.Hour

// Session lifetime and limits.
const (
	sessionIdle                = 30 * 24 * time.Hour // Sliding idle expiry.
	touchInterval              = time.Hour           // Minimum age of last_seen before it is rewritten.
	maxUserAgentBytes          = 256
	defaultMaxConcurrentHashes = 2
)

// Service authenticates users. It is safe for concurrent use.
type Service struct {
	store  *store.Store
	params Params
	now    func() time.Time
	slots  chan struct{} // One per hash allowed to run at once.
	dummy  string        // Hash verified for unknown names, so timing matches.
}

// Options configures [New]. The zero value is ready to use.
type Options struct {
	Params              Params           // Zero means DefaultParams().
	Now                 func() time.Time // Nil means time.Now.
	MaxConcurrentHashes int              // Zero means 2.
}

// LoginResult is a new session.
type LoginResult struct {
	Token     string // Give to the client once; it is not stored.
	User      store.User
	ExpiresAt time.Time
}

// New returns a Service over st. It returns an error if opts.Params is
// non-zero but outside the accepted bounds, or if opts.MaxConcurrentHashes is
// negative.
func New(st *store.Store, opts Options) (*Service, error) {
	params := opts.Params
	if params == (Params{}) {
		params = DefaultParams()
	}
	if err := checkParams(params); err != nil {
		return nil, fmt.Errorf("auth: Options.Params: %w", err)
	}
	if opts.MaxConcurrentHashes < 0 {
		return nil, fmt.Errorf("auth: Options.MaxConcurrentHashes is %d; %w", opts.MaxConcurrentHashes, errNegativeHashes)
	}
	slots := opts.MaxConcurrentHashes
	if slots == 0 {
		slots = defaultMaxConcurrentHashes
	}
	now := opts.Now
	if now == nil {
		now = time.Now
	}
	return &Service{
		store:  st,
		params: params,
		now:    now,
		slots:  make(chan struct{}, slots),
		dummy:  hashPassword(params, "kitsune dummy password"),
	}, nil
}

// CreateUser validates name and password, hashes the password and stores the
// user. It returns errors wrapping [ErrInvalidName], [ErrPasswordTooShort],
// [ErrPasswordTooLong] or [store.ErrNameTaken], or the context's error if it
// ends while waiting to hash. The returned user carries no password hash.
func (s *Service) CreateUser(ctx context.Context, name, password string, role store.Role) (store.User, error) {
	hash, err := s.newUserHash(ctx, name, password)
	if err != nil {
		return store.User{}, err
	}
	u, err := s.store.CreateUser(ctx, store.NewUser{Name: name, PasswordHash: hash, Role: role})
	if err != nil {
		return store.User{}, fmt.Errorf("auth: create user: %w", err)
	}
	u.PasswordHash = ""
	return u, nil
}

// CreateFirstAdmin validates and hashes as [Service.CreateUser] does, then
// stores an administrator only if there are no users yet, in one write, so
// two racing calls can't both make one. It returns an error wrapping
// [ErrSetupDone] once any user exists, as well as CreateUser's validation
// errors. The returned user carries no password hash.
func (s *Service) CreateFirstAdmin(ctx context.Context, name, password string) (store.User, error) {
	hash, err := s.newUserHash(ctx, name, password)
	if err != nil {
		return store.User{}, err
	}
	u, err := s.store.CreateFirstUser(ctx, store.NewUser{Name: name, PasswordHash: hash, Role: store.RoleAdmin})
	if errors.Is(err, store.ErrNotFirst) {
		return store.User{}, ErrSetupDone
	}
	if err != nil {
		return store.User{}, fmt.Errorf("auth: create first admin: %w", err)
	}
	u.PasswordHash = ""
	return u, nil
}

// Login checks the credentials and starts a session. An unknown name and a
// wrong password both return [ErrInvalidCredentials], as does a password over
// 1024 bytes. A hash made with older parameters is replaced on success.
func (s *Service) Login(ctx context.Context, name, password, userAgent string) (LoginResult, error) {
	u, ph, err := s.checkCredentials(ctx, name, password)
	if err != nil {
		return LoginResult{}, err
	}
	s.rehashIfStale(ctx, u.ID, password, ph)

	now := s.clock()
	minted := newSessionToken()
	expires := now.Add(sessionIdle)
	err = s.store.CreateSession(ctx, &store.NewSession{
		TokenHash: minted.hash,
		UserID:    u.ID,
		CreatedAt: now,
		ExpiresAt: expires,
		UserAgent: truncateUserAgent(userAgent),
	})
	if err != nil {
		return LoginResult{}, fmt.Errorf("auth: login: %w", err)
	}
	u.PasswordHash = ""
	return LoginResult{Token: minted.token, User: u, ExpiresAt: expires}, nil
}

// Authenticate returns the user a session token belongs to, or an error
// wrapping [ErrUnauthenticated]. An expired session is deleted. The user
// carries no password hash.
func (s *Service) Authenticate(ctx context.Context, token string) (store.User, error) {
	if !looksLikeSessionToken(token) {
		return store.User{}, ErrUnauthenticated
	}
	hash := hashToken(token)
	sess, err := s.store.SessionByTokenHash(ctx, hash)
	if errors.Is(err, store.ErrNotFound) {
		return store.User{}, ErrUnauthenticated
	}
	if err != nil {
		return store.User{}, fmt.Errorf("auth: authenticate: %w", err)
	}
	now := s.clock()
	if !now.Before(sess.ExpiresAt) {
		if delErr := s.store.DeleteSession(ctx, hash); delErr != nil {
			return store.User{}, errors.Join(ErrUnauthenticated, fmt.Errorf("auth: delete expired session: %w", delErr))
		}
		return store.User{}, ErrUnauthenticated
	}
	if now.Sub(sess.LastSeen) >= touchInterval {
		expires := earlier(sess.CreatedAt.Add(SessionMaxAge), now.Add(sessionIdle))
		// Best effort: the session is valid either way, and the next request
		// older than touchInterval retries.
		_ = s.store.TouchSession(ctx, hash, now, expires)
	}
	return sess.User, nil
}

// Logout ends the session a token belongs to. An unknown or malformed token
// is not an error.
func (s *Service) Logout(ctx context.Context, token string) error {
	if !looksLikeSessionToken(token) {
		return nil
	}
	if err := s.store.DeleteSession(ctx, hashToken(token)); err != nil {
		return fmt.Errorf("auth: logout: %w", err)
	}
	return nil
}

// PurgeExpiredSessions deletes every expired session and returns how many it deleted.
func (s *Service) PurgeExpiredSessions(ctx context.Context) (int64, error) {
	n, err := s.store.DeleteExpiredSessions(ctx, s.clock())
	if err != nil {
		return 0, fmt.Errorf("auth: purge sessions: %w", err)
	}
	return n, nil
}

// newUserHash validates a new user's name and password and returns the
// password's hash.
func (s *Service) newUserHash(ctx context.Context, name, password string) (string, error) {
	if err := ValidateName(name); err != nil {
		return "", err
	}
	if err := ValidatePassword(password); err != nil {
		return "", err
	}
	var hash string
	if err := s.withSlot(ctx, func() { hash = hashPassword(s.params, password) }); err != nil {
		return "", err
	}
	return hash, nil
}

// checkCredentials looks the user up and verifies the password, spending the
// same hashing work for an unknown name.
func (s *Service) checkCredentials(ctx context.Context, name, password string) (store.User, parsedHash, error) {
	if len(password) > maxPasswordBytes {
		return store.User{}, parsedHash{}, ErrInvalidCredentials
	}
	u, err := s.store.UserByName(ctx, name)
	if errors.Is(err, store.ErrNotFound) {
		if _, _, verr := s.verify(ctx, s.dummy, password); verr != nil {
			return store.User{}, parsedHash{}, verr
		}
		return store.User{}, parsedHash{}, ErrInvalidCredentials
	}
	if err != nil {
		return store.User{}, parsedHash{}, fmt.Errorf("auth: login: %w", err)
	}
	ok, ph, err := s.verify(ctx, u.PasswordHash, password)
	if err != nil {
		return store.User{}, parsedHash{}, err
	}
	if !ok {
		return store.User{}, parsedHash{}, ErrInvalidCredentials
	}
	return u, ph, nil
}

// verify runs verifyPassword in a hash slot.
func (s *Service) verify(ctx context.Context, encoded, password string) (bool, parsedHash, error) {
	var (
		ok  bool
		ph  parsedHash
		err error
	)
	if slotErr := s.withSlot(ctx, func() { ok, ph, err = verifyPassword(encoded, password) }); slotErr != nil {
		return false, parsedHash{}, slotErr
	}
	if err != nil {
		return false, parsedHash{}, fmt.Errorf("auth: stored hash: %w", err)
	}
	return ok, ph, nil
}

// rehashIfStale replaces a hash whose parameters or sizes differ from the
// service's. It is best effort: a failure leaves the old hash, which still
// works, and the next login retries.
func (s *Service) rehashIfStale(ctx context.Context, id int64, password string, ph parsedHash) {
	if ph.params == s.params && len(ph.salt) == saltBytes && len(ph.key) == keyBytes {
		return
	}
	var hash string
	if err := s.withSlot(ctx, func() { hash = hashPassword(s.params, password) }); err != nil {
		return
	}
	_ = s.store.SetPasswordHash(ctx, id, hash)
}

// withSlot runs f once one of the service's hash slots is free, or fails with
// the context's error if ctx ends first.
func (s *Service) withSlot(ctx context.Context, f func()) error {
	select {
	case s.slots <- struct{}{}:
	case <-ctx.Done():
		return fmt.Errorf("wait for hash slot: %w", ctx.Err())
	}
	defer func() { <-s.slots }()
	f()
	return nil
}

// clock returns the current time in UTC, truncated to the milliseconds the store keeps.
func (s *Service) clock() time.Time {
	return s.now().UTC().Truncate(time.Millisecond)
}

// earlier returns the earlier of a and b.
func earlier(a, b time.Time) time.Time {
	if b.Before(a) {
		return b
	}
	return a
}

// truncateUserAgent drops invalid UTF-8 and cuts ua to at most
// maxUserAgentBytes bytes on a rune boundary.
func truncateUserAgent(ua string) string {
	ua = strings.ToValidUTF8(ua, "")
	if len(ua) <= maxUserAgentBytes {
		return ua
	}
	cut := maxUserAgentBytes
	for cut > 0 && !utf8.RuneStart(ua[cut]) {
		cut--
	}
	return ua[:cut]
}
