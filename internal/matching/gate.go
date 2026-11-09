package matching

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// gate spaces requests to a source at least interval apart. A person's request
// goes before any prefetch waiting with it, so working ahead never makes
// someone wait. It is safe for concurrent use.
type gate struct {
	interval time.Duration

	mu     sync.Mutex
	next   time.Time     // When the next request may go.
	urgent int           // People waiting.
	wake   chan struct{} // Closed whenever the gate changes, so waiters look again.
}

func newGate(interval time.Duration) *gate {
	return &gate{interval: interval, wake: make(chan struct{})}
}

// changed wakes every waiter. The caller holds mu.
func (g *gate) changed() {
	close(g.wake)
	g.wake = make(chan struct{})
}

// wait returns when a request may go, or with ctx's error. A request that
// isn't [Background] goes before every background one.
func (g *gate) wait(ctx context.Context) error {
	behind := isBackground(ctx)
	g.mu.Lock()
	if !behind {
		g.urgent++
	}
	for {
		now := time.Now()
		if (!behind || g.urgent == 0) && !now.Before(g.next) {
			g.next = now.Add(g.interval)
			if !behind {
				g.urgent--
			}
			g.changed()
			g.mu.Unlock()
			return nil
		}
		wake, delay := g.wake, g.next.Sub(now)
		g.mu.Unlock()
		err := sleep(ctx, delay, wake)
		g.mu.Lock()
		if err != nil {
			if !behind {
				g.urgent--
				g.changed()
			}
			g.mu.Unlock()
			return err
		}
	}
}

// sleep waits for d, wake or the end of ctx, whichever comes first. A d of zero
// or less waits for wake or ctx alone.
func sleep(ctx context.Context, d time.Duration, wake <-chan struct{}) error {
	var timeout <-chan time.Time
	if d > 0 {
		timer := time.NewTimer(d)
		defer timer.Stop()
		timeout = timer.C
	}
	select {
	case <-ctx.Done():
		return fmt.Errorf("wait: %w", context.Cause(ctx))
	case <-wake:
	case <-timeout:
	}
	return nil
}

type backgroundKey struct{}

// Background marks ctx's requests as work done ahead, such as the queue asking
// for the next albums: they wait behind every request a person is waiting on.
func Background(ctx context.Context) context.Context {
	return context.WithValue(ctx, backgroundKey{}, true)
}

func isBackground(ctx context.Context) bool {
	behind, _ := ctx.Value(backgroundKey{}).(bool)
	return behind
}
