package store

import (
	"bufio"
	"database/sql"
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

const (
	memReaders   = 8
	memUsers     = 20_000
	memHashBytes = 200
	kib          = 1024
)

// vmRSS returns this process's resident set size in bytes. ncruces maps each
// connection's Wasm memory outside the Go heap, so runtime.MemStats can't see it.
func vmRSS(tb testing.TB) int64 {
	tb.Helper()
	f, err := os.Open("/proc/self/status")
	if err != nil {
		tb.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		rest, ok := strings.CutPrefix(sc.Text(), "VmRSS:")
		if !ok {
			continue
		}
		fields := strings.Fields(rest)
		if len(fields) == 0 {
			break
		}
		kb, convErr := strconv.ParseInt(fields[0], 10, 64)
		if convErr != nil {
			tb.Fatal(convErr)
		}
		return kb * kib
	}
	tb.Fatal("no VmRSS line in /proc/self/status")
	return 0
}

// BenchmarkReaderPoolMemory reports the resident memory each reader connection
// gains once its page cache is warm (B/conn), and what an idle connection costs
// right after Open (B/idle-conn, writer included). It measures once; run it with -benchtime 1x.
func BenchmarkReaderPoolMemory(b *testing.B) {
	if runtime.GOOS != "linux" {
		b.Skip("reads VmRSS from /proc/self/status")
	}
	ctx := b.Context()
	runtime.GC()
	beforeOpen := vmRSS(b)
	s, err := Open(ctx, b.TempDir()+"/kitsune.db", Options{Readers: memReaders})
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { _ = s.Close() })
	runtime.GC()
	// Open leaves every reader idle in the pool, so the baseline below already
	// holds their base cost; this is that cost, the writer's included.
	idle := float64(vmRSS(b)-beforeOpen) / (memReaders + 1)

	fillUsers(b, s)
	runtime.GC()
	before := vmRSS(b)

	for b.Loop() {
		warmReaders(b, s)
		runtime.GC()
		b.ReportMetric(float64(vmRSS(b)-before)/memReaders, "B/conn")
		b.ReportMetric(idle, "B/idle-conn")
	}
}

// fillUsers inserts enough rows to outgrow a connection's default page cache.
func fillUsers(b *testing.B, s *Store) {
	b.Helper()
	ctx := b.Context()
	tx, err := s.w.BeginTx(ctx, nil)
	if err != nil {
		b.Fatal(err)
	}
	hash := strings.Repeat("h", memHashBytes)
	for i := range memUsers {
		if _, err = tx.ExecContext(ctx,
			"INSERT INTO users (name, password_hash, role, created_at) VALUES (?, ?, 'user', 0)",
			fmt.Sprintf("user%05d", i), hash); err != nil {
			b.Fatal(err)
		}
	}
	if err = tx.Commit(); err != nil {
		b.Fatal(err)
	}
}

// warmReaders holds every reader at once and scans the table twice on each, so
// the second pass reads from a warm cache.
func warmReaders(b *testing.B, s *Store) {
	b.Helper()
	ctx := b.Context()
	conns := make([]*sql.Conn, 0, memReaders)
	for range memReaders {
		c, err := s.r.Conn(ctx)
		if err != nil {
			b.Fatal(err)
		}
		conns = append(conns, c)
		for range 2 {
			var total int64
			if err = c.QueryRowContext(ctx, "SELECT sum(length(password_hash)) FROM users").Scan(&total); err != nil {
				b.Fatal(err)
			}
		}
	}
	for _, c := range conns {
		if err := c.Close(); err != nil {
			b.Fatal(err)
		}
	}
}
