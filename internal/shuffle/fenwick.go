package shuffle

import "math/bits"

// fenwick is a Fenwick tree of weights: it changes one weight and draws a
// unit in proportion to weight, each in O(log n), so an endless page never
// rescans the whole pool for a pick.
type fenwick struct {
	tree   []float64 // 1-based partial sums
	weight []float64 // each unit's weight, exactly
	top    int       // the highest power of two at most len(weight)
}

// newFenwick builds a tree over weight, which it takes ownership of.
func newFenwick(weight []float64) *fenwick {
	n := len(weight)
	f := &fenwick{tree: make([]float64, n+1), weight: weight}
	for i, w := range weight {
		f.tree[i+1] += w
		if j := (i + 1) + (i+1)&-(i+1); j <= n {
			f.tree[j] += f.tree[i+1]
		}
	}
	if n > 0 {
		f.top = 1 << (bits.Len(uint(n)) - 1)
	}
	return f
}

// total is the sum of all weights.
func (f *fenwick) total() float64 {
	t := 0.0
	for i := len(f.weight); i > 0; i -= i & -i {
		t += f.tree[i]
	}
	return t
}

// set changes unit i's weight to w.
func (f *fenwick) set(i int, w float64) {
	delta := w - f.weight[i]
	f.weight[i] = w
	for j := i + 1; j < len(f.tree); j += j & -j {
		f.tree[j] += delta
	}
}

// find returns the unit where the running sum of weights passes target,
// from 0 to total. Rounding can land it on a unit of weight zero; callers
// check.
func (f *fenwick) find(target float64) int {
	pos := 0
	for step := f.top; step > 0; step >>= 1 {
		if next := pos + step; next < len(f.tree) && f.tree[next] <= target {
			pos = next
			target -= f.tree[next]
		}
	}
	return min(pos, len(f.weight)-1)
}
