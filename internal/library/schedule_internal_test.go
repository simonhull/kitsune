package library

import (
	"context"
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
