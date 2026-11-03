package server

import (
	"net/netip"
	"strings"
	"sync"
	"time"
)

// Login limiter tuning.
const (
	maxFreeFailures = 5                // Failures before backoff starts.
	maxBackoffShift = 10               // 1<<10 seconds exceeds maxBackoff.
	maxBackoff      = 15 * time.Minute // Longest block.
	failureDecay    = time.Hour        // A key quiet this long starts again at zero.
	maxLimiterKeys  = 10_000           // Entries kept in production.
	maxNameKeyBytes = 256              // Longest account name in a key.
	ipv6KeyBits     = 64               // IPv6 clients are counted per /64.
)

// limitEntry is the failure state of one key.
type limitEntry struct {
	failures     int
	blockedUntil time.Time
	lastFailure  time.Time
}

// loginLimiter counts consecutive login failures per key and blocks a key
// with exponential backoff once it has too many. It is safe for concurrent
// use; the caller passes the time in.
type loginLimiter struct {
	maxKeys int

	mu      sync.Mutex
	entries map[string]*limitEntry
}

// newLoginLimiter returns a limiter that tracks at most maxKeys keys.
func newLoginLimiter(maxKeys int) *loginLimiter {
	return &loginLimiter{maxKeys: maxKeys, entries: make(map[string]*limitEntry)}
}

// attempt counts a login attempt on every key, as a failure: the caller
// reports success with [loginLimiter.succeeded]. Counting up front means
// concurrent attempts cannot all slip through before any is recorded.
//
// If any key is blocked, attempt counts nothing and returns the longest
// remaining wait and false.
func (l *loginLimiter) attempt(keys []string, now time.Time) (time.Duration, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()

	var wait time.Duration
	for _, k := range keys {
		if e, ok := l.entries[k]; ok && now.Before(e.blockedUntil) {
			wait = max(wait, e.blockedUntil.Sub(now))
		}
	}
	if wait > 0 {
		return wait, false
	}
	for _, k := range keys {
		e := l.entry(k, now)
		if e == nil {
			continue // Every entry is blocked; this key goes unrecorded.
		}
		if e.lastFailure.Add(failureDecay).Before(now) {
			e.failures = 0
		}
		e.failures++
		e.lastFailure = now
		if e.failures > maxFreeFailures {
			e.blockedUntil = now.Add(backoff(e.failures))
		}
	}
	return 0, true
}

// succeeded records that the attempt counted on nameKey and ipKey logged in.
// The name key is forgotten. The IP key only gets this attempt back: its
// earlier failures stand, so one valid account cannot wipe its IP's count
// between password-spray guesses, and many successful logins from one IP
// never block it.
func (l *loginLimiter) succeeded(nameKey, ipKey string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.entries, nameKey)
	l.uncount(ipKey)
}

// refund gives back an attempt counted on keys that was neither a success nor
// a failed guess, such as one that hit an internal error.
func (l *loginLimiter) refund(keys ...string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, k := range keys {
		l.uncount(k)
	}
}

// uncount undoes one attempt counted on key, lifting a block that only that
// attempt could have caused. The caller holds l.mu.
func (l *loginLimiter) uncount(key string) {
	e, ok := l.entries[key]
	if !ok {
		return
	}
	e.failures = max(e.failures-1, 0)
	if e.failures <= maxFreeFailures {
		e.blockedUntil = time.Time{}
	}
}

// entry returns key's entry, adding it if needed. When the map is full it
// first makes room by evicting stale, then the oldest, unblocked entries; a
// blocked entry is never evicted. It returns nil if there is no room. The
// caller holds l.mu.
func (l *loginLimiter) entry(key string, now time.Time) *limitEntry {
	if e, ok := l.entries[key]; ok {
		return e
	}
	if len(l.entries) >= l.maxKeys {
		l.evict(now)
	}
	if len(l.entries) >= l.maxKeys {
		return nil
	}
	e := &limitEntry{}
	l.entries[key] = e
	return e
}

// evict drops every stale unblocked entry and, if that frees nothing, the
// unblocked entry with the oldest failure. The caller holds l.mu.
func (l *loginLimiter) evict(now time.Time) {
	before := len(l.entries)
	oldestKey, oldest := "", time.Time{}
	for k, e := range l.entries {
		if now.Before(e.blockedUntil) {
			continue
		}
		if e.lastFailure.Add(failureDecay).Before(now) {
			delete(l.entries, k)
			continue
		}
		if oldestKey == "" || e.lastFailure.Before(oldest) {
			oldestKey, oldest = k, e.lastFailure
		}
	}
	if len(l.entries) == before && oldestKey != "" {
		delete(l.entries, oldestKey)
	}
}

// backoff returns how long to block a key after failures consecutive
// failures: 1 s for the first one past the free failures, doubling to the cap.
func backoff(failures int) time.Duration {
	n := failures - maxFreeFailures - 1 // 0 for the 6th failure.
	if n >= maxBackoffShift {
		return maxBackoff
	}
	return min(time.Second<<n, maxBackoff)
}

// ipKey returns the limiter key for a client IP. IPv6 clients share a key per
// /64, so spreading guesses over one interface's addresses does not help; an
// unparsable address is used as is.
func ipKey(clientIP string) string {
	addr, err := netip.ParseAddr(clientIP)
	if err != nil {
		return "ip:" + clientIP
	}
	addr = addr.WithZone("").Unmap()
	if addr.Is4() {
		return "ip:" + addr.String()
	}
	prefix, err := addr.Prefix(ipv6KeyBits)
	if err != nil {
		return "ip:" + clientIP
	}
	return "ip:" + prefix.String()
}

// nameKey returns the limiter key for an account name. Lower-casing folds a
// superset of the store's ASCII case-insensitivity, so it can only merge the
// counters of two names, never split one account's. Unknown and existing names
// are counted alike.
func nameKey(name string) string {
	lower := strings.ToLower(name)
	return "name:" + lower[:min(len(lower), maxNameKeyBytes)]
}
