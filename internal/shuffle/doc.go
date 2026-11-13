// Package shuffle orders a person's songs for shuffle play. It is pure: it
// takes plain values (songs, their links, the person's settings, a seed and
// the time) and returns an order. It knows nothing of the store, the API or
// any client.
//
// # Units
//
// Shuffle picks units, not songs. [NewPool] builds them: each run of linked
// songs ([Run]) is one unit that plays from its first song, and every other
// song is a unit of its own. A song skipped in shuffle ([Track].Skipped) never
// comes up as a unit of its own, but still plays inside its run. A run whose
// songs are all skipped, or all missing, is dropped.
//
// # Weights
//
// A unit's weight ([Weight], explained by [Explain]) is the product of its
// rating, recency, skip memory, skip rate and listen time factors. Each
// constant behind them is documented where it is declared, with its source.
//
// # Orders
//
// [Finite] orders every unit once, for an album, playlist or selection.
// [Endless] streams the whole library page by page. Both are weighted (heavier
// units come sooner and more often), spaced (the same artist and album don't
// come close together) and, given a [Similarity], lean toward a next unit
// that suits the last one. A finite order also spreads each artist's units
// evenly through it.
//
// The same inputs and seed always give the same order.
package shuffle
