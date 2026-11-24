package scrape

import (
	"hash/fnv"
	"slices"
	"time"

	"github.com/simonhull/kitsune/internal/matching"
	"github.com/simonhull/kitsune/internal/store"
)

// When a source is asked again about an artist.
const (
	// foundEvery is how often an artist something was found for is asked
	// again, give or take spread, which a hash of its MBID decides so the
	// refreshes of a library fetched in one go don't arrive in one go.
	foundEvery = 27 * 24 * time.Hour
	spread     = 6 * 24 * time.Hour
	// noneFirst and noneAfter are how long nothing found waits: a week the
	// first time, a month after.
	noneFirst = 7 * 24 * time.Hour
	noneAfter = 30 * 24 * time.Hour
)

// How long a failure waits, by failures in a row: an hour, six, a day, then a
// week.
const (
	backoffFirst  = time.Hour
	backoffSecond = 6 * time.Hour
	backoffThird  = 24 * time.Hour
	backoffLast   = 7 * 24 * time.Hour
)

// failureBackoff is how long the failures-th failure in a row waits.
func failureBackoff(failures int) time.Duration {
	steps := []time.Duration{backoffFirst, backoffSecond, backoffThird, backoffLast}
	return steps[min(max(failures, 1), len(steps))-1]
}

// NextAsk is the state an answer of status leaves, asked at now, after prev
// (nil when never asked). msg says what failed, for a failure.
func NextAsk(status store.ScrapeStatus, prev *store.ScrapeState, mbid, msg string, now time.Time) store.ScrapeState {
	st := store.ScrapeState{Status: status, FetchedAt: now}
	switch status {
	case store.ScrapeFound:
		st.NextAt = now.Add(foundEvery + spreadOf(mbid))
	case store.ScrapeNone:
		wait := noneFirst
		if prev != nil && prev.Status == store.ScrapeNone {
			wait = noneAfter
		}
		st.NextAt = now.Add(wait)
	case store.ScrapeFailed:
		st.Failures = 1
		if prev != nil && prev.Status == store.ScrapeFailed {
			st.Failures = prev.Failures + 1
		}
		st.NextAt, st.Error = now.Add(failureBackoff(st.Failures)), msg
	}
	return st
}

// spreadOf is mbid's share of the spread, the same every time.
func spreadOf(mbid string) time.Duration {
	h := fnv.New64a()
	_, _ = h.Write([]byte(mbid)) // A hash never fails.
	return time.Duration(h.Sum64() % uint64(spread))
}

// Due reports whether a source with state st (nil when never asked) is to be
// asked at now.
func Due(st *store.ScrapeState, now time.Time) bool {
	return st == nil || !st.NextAt.After(now)
}

// specialArtists are MusicBrainz's special-purpose artists: nothing is ever
// fetched for one, since no picture or bio is theirs.
func specialArtists() []string {
	return []string{
		"89ad4ac3-39f7-470e-963a-56509c546377", // Various Artists
		"f731ccc4-e22a-43af-a747-64213329e088", // [anonymous]
		"33cf029c-63b0-41a0-9855-be2a3665fb3b", // [data]
		"314e1c25-dde7-4e4d-b2f4-0a7b9f7c56dc", // spoken words
		"eec63d3c-3b81-4ad4-b1e4-7c147d4d2b61", // [no artist]
		"9be7f096-97ec-4615-8957-8d40b5dcbc41", // [traditional]
		"125ec42a-7229-4250-afc5-e057484327fe", // [unknown]
		"66ea0139-149f-4a0c-8fbf-5ea9ec4a6e49", // [Disney]
		"a0ef7e1d-44ff-4039-9435-7d5fefdeecc9", // stage shows
	}
}

// Askable reports whether an artist with mbid may be asked about: a
// well-formed MBID, and not a special-purpose artist.
func Askable(mbid string) bool {
	return matching.IsMBID(mbid) && !slices.Contains(specialArtists(), mbid)
}
