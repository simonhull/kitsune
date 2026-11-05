package library

import (
	"context"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"
)

func TestNextDaily(t *testing.T) {
	t.Parallel()
	ny, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Skipf("no time zone data: %v", err)
	}
	at := func(y int, m time.Month, d, h, mi int) time.Time { return time.Date(y, m, d, h, mi, 0, 0, ny) }
	tests := []struct {
		name         string
		now          time.Time
		hour, minute int
		want         time.Time
	}{
		{"before today's time", at(2026, time.June, 10, 2, 0), 3, 30, at(2026, time.June, 10, 3, 30)},
		{"exactly at it", at(2026, time.June, 10, 3, 30), 3, 30, at(2026, time.June, 11, 3, 30)},
		{"after it", at(2026, time.June, 10, 3, 31), 3, 30, at(2026, time.June, 11, 3, 30)},
		{"end of month", at(2026, time.June, 30, 23, 0), 3, 30, at(2026, time.July, 1, 3, 30)},
		{"into the spring DST day", at(2026, time.March, 7, 4, 0), 3, 30, at(2026, time.March, 8, 3, 30)},
		{"after the spring DST day", at(2026, time.March, 8, 4, 0), 3, 30, at(2026, time.March, 9, 3, 30)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := nextDaily(tt.now, tt.hour, tt.minute); !got.Equal(tt.want) {
				t.Errorf("nextDaily(%v, %d, %d) = %v, want %v", tt.now, tt.hour, tt.minute, got, tt.want)
			}
		})
	}
	t.Run("across the spring DST gap", func(t *testing.T) {
		t.Parallel()
		// 02:30 does not exist on 2026-03-08; time.Date normalizes it. Assert only
		// that the next run is still after now.
		now := at(2026, time.March, 8, 0, 0)
		if got := nextDaily(now, 2, 30); !got.After(now) {
			t.Errorf("nextDaily = %v, want a time after %v", got, now)
		}
	})
}

func TestRunDaily(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		var scans, readies atomic.Int32
		scan := func(context.Context) { scans.Add(1) }
		ready := func() {
			readies.Add(1)
			if got := scans.Load(); got != 1 {
				t.Errorf("ready ran after %d scans, want 1", got)
			}
		}
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		done := make(chan struct{})
		go func() {
			defer close(done)
			runDaily(ctx, time.Now, 3, 30, scan, ready)
		}()
		synctest.Wait()
		if scans.Load() != 1 || readies.Load() != 1 {
			t.Fatalf("after start: scans = %d, readies = %d, want 1 and 1", scans.Load(), readies.Load())
		}
		time.Sleep(24 * time.Hour)
		synctest.Wait()
		if scans.Load() != 2 || readies.Load() != 1 {
			t.Errorf("after a day: scans = %d, readies = %d, want 2 and 1", scans.Load(), readies.Load())
		}
		cancel()
		synctest.Wait()
		select {
		case <-done:
		default:
			t.Error("runDaily did not return after cancel")
		}
		if scans.Load() != 2 {
			t.Errorf("scans = %d after cancel, want 2", scans.Load())
		}
	})
}

// scheduledLib is a library that logs to a buffer.
func scheduledLib(t *testing.T) (*testLib, *safeBuffer) {
	t.Helper()
	l := newTestLib(t, 1)
	logs := &safeBuffer{}
	l.opts.Logger = slog.New(slog.NewTextHandler(logs, nil))
	l.scanner = NewScanner(l.st, l.opts)
	return l, logs
}

func TestScheduledScanFollowsARunningOne(t *testing.T) {
	t.Parallel()
	l, logs := scheduledLib(t)
	l.copyTo(l.roots[0], xFLAC, sampleFLAC)
	running, err := l.scanner.Claim(Full)
	if err != nil {
		t.Fatal(err)
	}
	var scheduled sync.WaitGroup
	scheduled.Go(func() { l.scanner.scheduled(t.Context()) })
	waitForLog(t, logs, "the scheduled scan will retry")
	if _, err = running(t.Context()); err != nil {
		t.Fatal(err)
	}
	scheduled.Wait()
	got := logs.String()
	if n := strings.Count(got, "scan finished"); n != 2 {
		t.Errorf("%d scans finished, want the running one and one retry:\n%s", n, got)
	}
	if !strings.Contains(got, "mode=quick") {
		t.Errorf("no quick scan in:\n%s", got)
	}
	if strings.Contains(got, "level=ERROR") {
		t.Errorf("logged an error:\n%s", got)
	}
}

func TestScheduledRetryStopsWithItsContext(t *testing.T) {
	t.Parallel()
	l, logs := scheduledLib(t)
	if _, err := l.scanner.Claim(Full); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	var scheduled sync.WaitGroup
	scheduled.Go(func() { l.scanner.scheduled(ctx) })
	waitForLog(t, logs, "the scheduled scan will retry")
	cancel()
	scheduled.Wait()
	if got := logs.String(); strings.Contains(got, "scan finished") {
		t.Errorf("a scan ran after its context ended:\n%s", got)
	}
}

// waitForLog waits for a line holding want. The goroutine that writes it is real
// work on a real database, so it can't run inside a synctest bubble.
func waitForLog(t *testing.T, logs *safeBuffer, want string) {
	t.Helper()
	const deadline, poll = 5 * time.Second, time.Millisecond
	for start := time.Now(); time.Since(start) < deadline; time.Sleep(poll) {
		if strings.Contains(logs.String(), want) {
			return
		}
	}
	t.Fatalf("no log line with %q in:\n%s", want, logs)
}
