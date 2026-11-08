package shuffle

const (
	// polishReach is how far, in slots either way, polish looks for a fix
	// for a break. It bounds the work on a large set, where the breaks it
	// meets are few and near their fix.
	polishReach = 256
	// polishPasses is how many times polish goes over an order. Each fix
	// leaves strictly fewer breaks, so this only bounds the work.
	polishPasses = 8
)

// polisher fixes the spacing breaks a one-pass repair leaves. Placing units
// one by one can paint itself into a corner, most often at the end, where
// the units left over are all one artist's, or one album's within that
// artist's turns.
type polisher struct {
	p     *Pool
	picks []Pick
	from  int    // the picks before from stay where they are: a lead keeps its place
	saved []Pick // a span of picks as it was before a fix was tried
}

// op is a change to try on the picks at k and j.
type op func(picks []Pick, k, j int)

// polish fixes what it can of picks' breaks, leaving the picks before from
// where they are. For each break, it takes the nearest fix that leaves
// fewer breaks: moving the pick to another slot; swapping it with another
// pick, which can trade two songs of one artist between albums without
// disturbing the artist's turns; or pulling another pick in front of it,
// which puts a third artist between two of one. A fix can move a break
// behind the slot it was found at, so polish goes over the order again
// while a pass fixes anything, up to polishPasses. It returns how many
// breaks are left.
func polish(p *Pool, picks []Pick, from int) int {
	x := polisher{p: p, picks: picks, from: from}
	for range polishPasses {
		left, fixed := 0, false
		for k := from; k < len(picks); k++ {
			if !x.breaksAt(k) {
				continue
			}
			if x.nearest(k, move) || x.nearest(k, swap) || x.nearest(k, pull) {
				fixed = true
			} else {
				left++
			}
		}
		if !fixed {
			return left
		}
	}
	return x.breaksIn(0, len(picks)-1)
}

// nearest tries o on the pick at k and each slot out from it, either way
// and not before from, and keeps the first that leaves fewer breaks. It
// reports whether it found one.
func (x *polisher) nearest(k int, o op) bool {
	for step := 1; step <= polishReach; step++ {
		if j := k - step; j >= x.from && x.improves(k, j, o) {
			return true
		}
		if j := k + step; j < len(x.picks) && x.improves(k, j, o) {
			return true
		}
	}
	return false
}

// improves applies o to the picks at k and j, and keeps the change if it
// leaves fewer breaks. Every op only reorders the picks between the two
// slots, so only breaks from the lower slot to windowSize past the higher
// one can change, and that span holds the same picks before and after.
func (x *polisher) improves(k, j int, o op) bool {
	lo, hi := min(j, k), min(len(x.picks)-1, max(j, k)+windowSize)
	before := x.breaksIn(lo, hi)
	x.saved = append(x.saved[:0], x.picks[lo:hi+1]...)
	o(x.picks, k, j)
	if x.breaksIn(lo, hi) < before {
		return true
	}
	copy(x.picks[lo:], x.saved)
	return false
}

// breaksIn counts the picks from lo to hi that break a rule.
func (x *polisher) breaksIn(lo, hi int) int {
	n := 0
	for k := lo; k <= hi; k++ {
		if x.breaksAt(k) {
			n++
		}
	}
	return n
}

// breaksAt reports whether the pick at k comes too close to one before it.
// The first pick never does, which is as well: a lead from outside the pool
// has none of its indexes.
func (x *polisher) breaksAt(k int) bool {
	u := x.picks[k].Unit
	for d := 1; d <= min(k, windowSize); d++ {
		prev := x.picks[k-d].Unit
		if clashes(x.p, u, prev.artists, prev.albums, d) {
			return true
		}
	}
	return false
}

// move takes the pick at k out and puts it in at j.
func move(picks []Pick, k, j int) {
	pk := picks[k]
	if j < k {
		copy(picks[j+1:k+1], picks[j:k])
	} else {
		copy(picks[k:j], picks[k+1:j+1])
	}
	picks[j] = pk
}

// swap exchanges the picks at k and j.
func swap(picks []Pick, k, j int) { picks[k], picks[j] = picks[j], picks[k] }

// pull takes the pick at j out and puts it in just before the pick at k.
func pull(picks []Pick, k, j int) {
	if j < k {
		move(picks, j, k-1)
	} else {
		move(picks, j, k)
	}
}

// relean records flow's lean on every pick after the first, from the pick
// before it, once polish has settled the order.
func relean(c *Config, strength float64, picks []Pick) {
	for k := 1; k < len(picks); k++ {
		after, ok := picks[k-1].Unit.primary()
		f := newFlow(c, strength, after, ok)
		r := &picks[k].Reason
		r.Flow, r.After, r.Similarity = 1, 0, 0
		f.lean(r, picks[k].Unit)
	}
}
