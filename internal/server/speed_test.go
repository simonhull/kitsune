package server

import (
	"bytes"
	"cmp"
	"crypto/rand"
	"encoding/json/v2"
	"fmt"
	"io"
	"net/http"
	"net/http/httptrace"
	"os"
	"slices"
	"strconv"
	"testing"
	"time"
)

// budgetsFile holds every speed budget, the browser suite's included.
const budgetsFile = "../../perf/budgets.json"

// budgets is the part of budgetsFile the server's tests read.
type budgets struct {
	CIMultiplier float64           `json:"ciMultiplier"`
	Server       map[string]budget `json:"server"`
}

type budget struct {
	Interaction string  `json:"interaction"`
	P95Ms       float64 `json:"p95Ms"`
}

// speedBudget returns the budget for id, scaled as the browser suite scales
// it: by ciMultiplier on CI, or by KITSUNE_PERF_MULTIPLIER when that is set.
// It skips tb under the race detector or -short, where timing means nothing.
func speedBudget(tb testing.TB, id string) (budget, time.Duration) {
	tb.Helper()
	if raceOn || testing.Short() {
		tb.Skip("timing needs a plain build: the race detector slows everything several times over")
	}
	raw, err := os.ReadFile(budgetsFile)
	if err != nil {
		tb.Fatal(err)
	}
	var all budgets
	if err = json.Unmarshal(raw, &all); err != nil {
		tb.Fatal(err)
	}
	b, ok := all.Server[id]
	if !ok {
		tb.Fatalf("no server budget %q in %s", id, budgetsFile)
	}
	multiplier := 1.0
	if os.Getenv("CI") != "" {
		multiplier = all.CIMultiplier
	}
	if set := os.Getenv("KITSUNE_PERF_MULTIPLIER"); set != "" {
		if multiplier, err = strconv.ParseFloat(set, 64); err != nil {
			tb.Fatalf("KITSUNE_PERF_MULTIPLIER: %v", err)
		}
	}
	return b, time.Duration(b.P95Ms * multiplier * float64(time.Millisecond))
}

// holdSpeed fails tb when the p95 of took is over limit, and reports the
// median and p95 in the log and, on CI, in the job summary.
func holdSpeed(tb testing.TB, b budget, limit time.Duration, took []time.Duration) {
	tb.Helper()
	slices.Sort(took)
	median, p95 := percentile(took, 50), percentile(took, 95)
	row := fmt.Sprintf("| %s | %.2f | %.2f | %g ms |\n", b.Interaction, ms(median), ms(p95), b.P95Ms)
	tb.Logf("%d runs: median %v, p95 %v, budget %v", len(took), median, p95, limit)
	if summary := os.Getenv("GITHUB_STEP_SUMMARY"); summary != "" {
		appendSummary(tb, summary, row)
	}
	if p95 > limit {
		tb.Errorf("%s: p95 %v, over its budget of %v", b.Interaction, p95, limit)
	}
}

func appendSummary(tb testing.TB, path, row string) {
	tb.Helper()
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0) //nolint:gosec // G703: the path is CI's own job summary, from GITHUB_STEP_SUMMARY.
	if err != nil {
		tb.Fatal(err)
	}
	_, err = f.WriteString(row)
	if err = cmp.Or(err, f.Close()); err != nil {
		tb.Fatal(err)
	}
}

// percentile is the nearest-rank percentile of sorted: a value that was measured.
func percentile(sorted []time.Duration, p int) time.Duration {
	rank := max(1, (p*len(sorted)+99)/100)
	return sorted[rank-1]
}

func ms(d time.Duration) float64 { return float64(d) / float64(time.Millisecond) }

// streamFileSize is the size of the file the stream budget serves: a few
// minutes of CD-quality FLAC.
const streamFileSize = 24 << 20

// firstByte requests a track's stream and returns how long the first byte of
// the response took, reading the rest as a player would.
func (ss *streamServer) firstByte(tb testing.TB, id string) time.Duration {
	tb.Helper()
	var start time.Time
	var first time.Duration
	trace := &httptrace.ClientTrace{GotFirstResponseByte: func() { first = time.Since(start) }}
	req, err := http.NewRequestWithContext(httptrace.WithClientTrace(tb.Context(), trace),
		http.MethodGet, ss.URL+"/tracks/"+id+"/stream", http.NoBody)
	if err != nil {
		tb.Fatal(err)
	}
	req.Header.Set(authorizationHeader, "Bearer "+ss.token)
	start = time.Now()
	resp, err := ss.Server.Client().Do(req)
	if err != nil {
		tb.Fatal(err)
	}
	n, err := io.Copy(io.Discard, resp.Body)
	if err = cmp.Or(err, resp.Body.Close()); err != nil {
		tb.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK || n != streamFileSize {
		tb.Fatalf("stream: %d with %d bytes", resp.StatusCode, n)
	}
	return first
}

// newSpeedStream is a stream server holding one FLAC-sized file.
func newSpeedStream(tb testing.TB) *streamServer {
	tb.Helper()
	ts := newTestServer(tb)
	ss := &streamServer{testServer: ts, token: ts.sessionFor(tb).token, root: tb.TempDir()}
	rel := ss.seed(tb, streamTrackID, streamFormat)
	body := make([]byte, streamFileSize)
	_, _ = rand.Read(body) // Never fails.
	ss.write(tb, rel, string(body), time.Now())
	return ss
}

func TestStreamFirstByteSpeed(t *testing.T) { //nolint:paralleltest // Timed: a speed test measures a server alone, never beside another.
	b, limit := speedBudget(t, "stream-first-byte")
	ss := newSpeedStream(t)
	ss.firstByte(t, streamTrackID) // Warm: the connection, the statement cache, the page cache.
	took := make([]time.Duration, 200)
	for i := range took {
		took[i] = ss.firstByte(t, streamTrackID)
	}
	holdSpeed(t, b, limit, took)
}

func BenchmarkStreamFirstByte(b *testing.B) {
	ss := newSpeedStream(b)
	var total time.Duration
	for b.Loop() {
		total += ss.firstByte(b, streamTrackID)
	}
	b.ReportMetric(ms(total)/float64(b.N), "ms/first-byte")
}

func TestLibraryIndexSpeed(t *testing.T) { //nolint:paralleltest // Timed: a speed test measures a server alone, never beside another.
	b, limit := speedBudget(t, "library-index")
	ts := newTestServer(t)
	shape := benchShapes[len(benchShapes)-1]
	token := ts.seedBench(t, shape.tracks, shape.albums, shape.artists)
	holdSpeed(t, b, limit, ts.timeGets(t, token, indexPath+defaultIndexFields))
}

// TestArtistIndexSpeed times the artist page's list: one artist's songs, of a
// library as large as the whole index's budget reads.
func TestArtistIndexSpeed(t *testing.T) { //nolint:paralleltest // Timed: a speed test measures a server alone, never beside another.
	b, limit := speedBudget(t, "artist-index")
	ts := newTestServer(t)
	shape := benchShapes[len(benchShapes)-1]
	token := ts.seedBench(t, shape.tracks-prolificSongs, shape.albums, shape.artists)
	artist := ts.seedProlific(t, shape.albums)
	holdSpeed(t, b, limit, ts.timeGets(t, token, indexPath+defaultIndexFields+"&artist="+artist))
}

// timeGets times 20 gzipped GETs of path, after one to warm up.
func (ts *testServer) timeGets(t *testing.T, token, path string) []time.Duration {
	t.Helper()
	fetch := func() time.Duration {
		req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, ts.URL+path, http.NoBody)
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set(authorizationHeader, "Bearer "+token)
		req.Header.Set("Accept-Encoding", "gzip")
		start := time.Now()
		resp, err := ts.Server.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		var body bytes.Buffer
		_, err = io.Copy(&body, resp.Body)
		if err = cmp.Or(err, resp.Body.Close()); err != nil {
			t.Fatal(err)
		}
		took := time.Since(start)
		if resp.StatusCode != http.StatusOK || body.Len() == 0 {
			t.Fatalf("GET %s: %d", path, resp.StatusCode)
		}
		return took
	}
	fetch() // Warm.
	took := make([]time.Duration, 20)
	for i := range took {
		took[i] = fetch()
	}
	return took
}
