package server

import (
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// ipA is a client IP key used across the limiter tests.
const ipA = "ip:a"

// epoch is the instant the limiter tests start at.
func epoch() time.Time { return time.UnixMilli(1_762_000_000_000).UTC() }

// failN counts n attempts on keys at now and reports how many were allowed.
func failN(l *loginLimiter, n int, now time.Time, keys ...string) int {
	allowed := 0
	for range n {
		if _, ok := l.attempt(keys, now); ok {
			allowed++
		}
	}
	return allowed
}

func TestLimiterAllowsSixAttempts(t *testing.T) {
	t.Parallel()
	l := newLoginLimiter(100)
	if got := failN(l, 6, epoch(), ipA); got != 6 {
		t.Fatalf("allowed %d of the first 6 attempts, want 6", got)
	}
	wait, ok := l.attempt([]string{ipA}, epoch())
	if ok || wait != time.Second {
		t.Errorf("7th attempt = (%v, %t), want (1s, false)", wait, ok)
	}
}

func TestLimiterBackoff(t *testing.T) {
	t.Parallel()
	l := newLoginLimiter(100)
	failN(l, 6, epoch(), "k")
	var now time.Time
	for _, want := range []time.Duration{2 * time.Second, 4 * time.Second, 8 * time.Second, 16 * time.Second} {
		now = l.entries["k"].blockedUntil // The previous block has just ended.
		if _, ok := l.attempt([]string{"k"}, now); !ok {
			t.Fatalf("attempt after the block at %v was refused", now)
		}
		if got := l.entries["k"].blockedUntil.Sub(now); got != want {
			t.Errorf("failure %d blocks for %v, want %v", l.entries["k"].failures, got, want)
		}
	}
}

func TestLimiterCap(t *testing.T) {
	t.Parallel()
	l := newLoginLimiter(100)
	now := epoch()
	for i := 1; i <= 100; i++ {
		if e, ok := l.entries["k"]; ok {
			now = e.blockedUntil // The previous block has just ended.
		}
		if _, ok := l.attempt([]string{"k"}, now); !ok {
			t.Fatalf("attempt %d was refused", i)
		}
		if i != 30 && i != 100 {
			continue
		}
		e := l.entries["k"]
		if !e.blockedUntil.After(now) {
			t.Fatalf("after %d failures blockedUntil %v is not after now %v", i, e.blockedUntil, now)
		}
		if got := e.blockedUntil.Sub(now); got != maxBackoff {
			t.Errorf("after %d failures the block is %v, want %v", i, got, maxBackoff)
		}
	}
}

func TestLimiterDecay(t *testing.T) {
	t.Parallel()
	l := newLoginLimiter(100)
	failN(l, 5, epoch(), "k")
	if got := failN(l, 5, epoch().Add(61*time.Minute), "k"); got != 5 {
		t.Errorf("allowed %d of 5 attempts after the decay, want 5", got)
	}
}

func TestLimiterSucceededResetsNameAndRefundsIP(t *testing.T) {
	t.Parallel()
	l := newLoginLimiter(100)
	failN(l, 5, epoch(), ipA, "name:x")
	// The successful attempt is counted up front, as the 6th, and blocks...
	if _, ok := l.attempt([]string{ipA, "name:x"}, epoch()); !ok {
		t.Fatal("6th attempt refused")
	}
	// ...until succeeded gives it back.
	l.succeeded("name:x", ipA)
	if _, ok := l.entries["name:x"]; ok {
		t.Error("name:x still has an entry after succeeded")
	}
	e := l.entries[ipA]
	if e.failures != 5 || !e.blockedUntil.IsZero() {
		t.Errorf("ip:a = %d failures, blocked until %v; want its 5 earlier failures and no block", e.failures, e.blockedUntil)
	}
}

func TestLimiterRefund(t *testing.T) {
	t.Parallel()
	l := newLoginLimiter(100)
	failN(l, 6, epoch(), ipA) // The 6th sets a block.
	l.refund(ipA, "ip:absent")
	e := l.entries[ipA]
	if e.failures != 5 || !e.blockedUntil.IsZero() {
		t.Errorf("ip:a = %d failures, blocked until %v; want 5 and no block", e.failures, e.blockedUntil)
	}
}

func TestLimiterAttemptIsAtomic(t *testing.T) {
	t.Parallel()
	l := newLoginLimiter(100)
	failN(l, 7, epoch(), ipA) // The 7th is refused, but the 6th set a block.
	if _, ok := l.attempt([]string{ipA, "name:x"}, epoch()); ok {
		t.Fatal("attempt over a blocked key was allowed")
	}
	if e, ok := l.entries["name:x"]; ok {
		t.Errorf("name:x was counted (%d) by a refused attempt", e.failures)
	}
}

func TestLimiterWaitIsLongest(t *testing.T) {
	t.Parallel()
	l := newLoginLimiter(100)
	l.entries["a"] = &limitEntry{failures: 7, blockedUntil: epoch().Add(2 * time.Second), lastFailure: epoch()}
	l.entries["b"] = &limitEntry{failures: 9, blockedUntil: epoch().Add(8 * time.Second), lastFailure: epoch()}
	wait, ok := l.attempt([]string{"a", "b"}, epoch())
	if ok || wait != 8*time.Second {
		t.Errorf("attempt = (%v, %t), want (8s, false)", wait, ok)
	}
}

func TestLimiterEvictsStaleFirst(t *testing.T) {
	t.Parallel()
	l := newLoginLimiter(3)
	old := epoch().Add(-2 * time.Hour)
	for _, k := range []string{"a", "b", "c"} {
		l.entries[k] = &limitEntry{failures: 3, lastFailure: old}
	}
	if _, ok := l.attempt([]string{"d"}, epoch()); !ok {
		t.Fatal("attempt refused")
	}
	if _, ok := l.entries["d"]; !ok {
		t.Error("the new key was not recorded")
	}
	if len(l.entries) > 3 {
		t.Errorf("%d entries, want at most 3", len(l.entries))
	}
}

func TestLimiterNeverEvictsBlocked(t *testing.T) {
	t.Parallel()
	l := newLoginLimiter(3)
	for _, k := range []string{"a", "b", "c"} {
		l.entries[k] = &limitEntry{failures: 7, blockedUntil: epoch().Add(time.Minute), lastFailure: epoch()}
	}
	if _, ok := l.attempt([]string{"d"}, epoch()); !ok {
		t.Fatal("attempt on a new key was refused")
	}
	if _, ok := l.entries["d"]; ok {
		t.Error("the new key was recorded although every entry is blocked")
	}
	for _, k := range []string{"a", "b", "c"} {
		if e, ok := l.entries[k]; !ok || !e.blockedUntil.After(epoch()) {
			t.Errorf("block on %s was lost", k)
		}
	}
}

func TestLimiterEvictsOldestUnblocked(t *testing.T) {
	t.Parallel()
	l := newLoginLimiter(3)
	l.entries["blocked"] = &limitEntry{failures: 7, blockedUntil: epoch().Add(time.Minute), lastFailure: epoch().Add(-time.Minute)}
	l.entries["older"] = &limitEntry{failures: 2, lastFailure: epoch().Add(-10 * time.Minute)}
	l.entries["newer"] = &limitEntry{failures: 2, lastFailure: epoch().Add(-5 * time.Minute)}
	if _, ok := l.attempt([]string{"d"}, epoch()); !ok {
		t.Fatal("attempt refused")
	}
	if _, ok := l.entries["older"]; ok {
		t.Error("the older unblocked entry survived")
	}
	for _, k := range []string{"blocked", "newer", "d"} {
		if _, ok := l.entries[k]; !ok {
			t.Errorf("entry %s was evicted", k)
		}
	}
}

func TestIPKey(t *testing.T) {
	t.Parallel()
	if got := ipKey("192.0.2.7"); got != "ip:192.0.2.7" {
		t.Errorf("ipKey(IPv4) = %q", got)
	}
	if a, b := ipKey("2001:db8::1"), ipKey("2001:db8::ffff"); a != b {
		t.Errorf("addresses in one /64 map to %q and %q, want one key", a, b)
	}
	if a, b := ipKey("2001:db8::1"), ipKey("2001:db8:0:1::1"); a == b {
		t.Errorf("addresses in different /64s share the key %q", a)
	}
	if got := ipKey("::ffff:192.0.2.7"); got != "ip:192.0.2.7" {
		t.Errorf("ipKey(4in6) = %q, want the IPv4 key", got)
	}
	if got := ipKey("garbage"); got != "ip:garbage" {
		t.Errorf("ipKey(garbage) = %q", got)
	}
}

func TestNameKey(t *testing.T) {
	t.Parallel()
	if a, b := nameKey("Simon"), nameKey("simon"); a != b {
		t.Errorf("nameKey folds Simon and simon to %q and %q", a, b)
	}
	if a, b := nameKey("\u212a"), nameKey("k"); a != b { // The Kelvin sign is shorter once lowered.
		t.Errorf("nameKey folds the Kelvin sign and k to %q and %q", a, b)
	}
	if got := nameKey(strings.Repeat("x", 5<<10)); len(got) > 261 {
		t.Errorf("long name key is %d bytes, want at most 261", len(got))
	}
}

func TestLimiterConcurrent(t *testing.T) {
	t.Parallel()
	const (
		workers  = 50
		attempts = 100
		keys     = 7
	)
	l := newLoginLimiter(100)
	var allowed [keys]atomic.Int64
	var wg sync.WaitGroup
	for w := range workers {
		wg.Go(func() {
			for i := range attempts {
				k := (w + i) % keys
				if _, ok := l.attempt([]string{fmt.Sprintf("ip:%d", k)}, epoch()); ok {
					allowed[k].Add(1)
				}
			}
		})
	}
	wg.Wait()
	for k := range allowed {
		if got := allowed[k].Load(); got > 6 {
			t.Errorf("key %d allowed %d attempts in one instant, want at most 6", k, got)
		}
	}
}
