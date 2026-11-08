// Package bench measures internal/shuffle: it is where "the best shuffle"
// is a number rather than a claim. It holds only tests.
//
// The report runs thousands of shuffles over generated libraries (skewed,
// compilation-heavy, a tiny album, run-heavy and 50,000 songs), with
// simulated listeners, and prints every metric the shuffle spec lists, next
// to a naive Fisher–Yates shuffle's:
//
//	go test -run Report -v ./internal/shuffle/bench
//
// UPDATE_BASELINE=1 rewrites BASELINE.md, the report a change is compared
// against. A broken hard rule (a run out of order, a skipped song picked, a
// spacing break the set had room to avoid, flow off that isn't neutral) fails
// the test. The speed figures come from the benchmarks:
//
//	go test -run '^$' -bench . ./internal/shuffle/bench
package bench
