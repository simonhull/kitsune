package loudness

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math"
)

// A histogram counts blocks by loudness in bins of 1/binsPerLU LU, from the
// absolute gate up to topLUFS; a louder block, which no real audio makes,
// counts in the top bin. Pooling reads each block at its bin's center, at most
// 0.05 LU off, so an album's loudness is at most 0.05 LU off too, and music,
// whose blocks spread across bins, much less.
const (
	binsPerLU     = 10
	topLUFS       = 10
	histogramBins = (topLUFS - absoluteGate) * binsPerLU
	histogramForm = 1 // The first byte of an encoded histogram.
)

// ErrHistogram is returned for bytes that are not an encoded [Histogram].
var ErrHistogram = errors.New("loudness: not a histogram")

// Histogram counts a track's 400 ms blocks above the absolute gate by
// loudness. Its zero value is empty.
type Histogram struct {
	counts [histogramBins]uint32
}

// binMeanSquare is the mean square of bin's center.
func binMeanSquare(bin int) float64 {
	const center = 0.5
	return meanSquareOf(absoluteGate + (float64(bin)+center)/binsPerLU)
}

// MarshalBinary encodes the histogram compactly: its form, then each bin that
// counts something, as the gap from the previous one and its count, both as
// unsigned varints. A three-minute track takes a few hundred bytes.
func (h *Histogram) MarshalBinary() ([]byte, error) {
	out := []byte{histogramForm}
	last := -1
	for bin, n := range &h.counts {
		if n == 0 {
			continue
		}
		out = binary.AppendUvarint(out, uint64(bin-last)) //nolint:gosec // G115: bins rise, so the gap is positive.
		out = binary.AppendUvarint(out, uint64(n))
		last = bin
	}
	return out, nil
}

// UnmarshalBinary decodes what [Histogram.MarshalBinary] encoded.
//
// # Errors
//
// It returns an error wrapping [ErrHistogram] for anything else.
func (h *Histogram) UnmarshalBinary(data []byte) error {
	if len(data) == 0 || data[0] != histogramForm {
		return fmt.Errorf("%w: unknown form", ErrHistogram)
	}
	var counts [histogramBins]uint32
	bin := -1
	for rest := data[1:]; len(rest) > 0; {
		e, err := readEntry(&rest)
		if err != nil {
			return err
		}
		bin += e.gap
		if bin >= len(counts) {
			return fmt.Errorf("%w: bin out of range", ErrHistogram)
		}
		counts[bin] = e.count //nolint:gosec // G602: bin is checked against the length just above.
	}
	h.counts = counts
	return nil
}

// entry is one counting bin of an encoded histogram: its gap from the one
// before, and its count.
type entry struct {
	gap   int
	count uint32
}

// readEntry reads one entry from the front of *rest.
func readEntry(rest *[]byte) (entry, error) {
	g, n := binary.Uvarint(*rest)
	if n <= 0 || g == 0 || g > histogramBins {
		return entry{}, fmt.Errorf("%w: bad bin", ErrHistogram)
	}
	*rest = (*rest)[n:]
	c, m := binary.Uvarint(*rest)
	if m <= 0 || c == 0 || c > math.MaxUint32 {
		return entry{}, fmt.Errorf("%w: bad count", ErrHistogram)
	}
	*rest = (*rest)[m:]
	return entry{gap: int(g), count: uint32(c)}, nil
}

// Pool returns the integrated loudness of the histograms' blocks pooled, as
// one program: the loudness of an album from its tracks'. It reports true
// when no block passes the gates.
func Pool(histograms ...*Histogram) (float64, bool) {
	var counts [histogramBins]uint64
	for _, h := range histograms {
		for bin, n := range &h.counts {
			counts[bin] += uint64(n)
		}
	}
	ungated, ok := binMean(&counts, -1)
	if !ok {
		return 0, true
	}
	gate := ungated * ratio(relativeGate)
	first := 0
	for first < histogramBins && binMeanSquare(first) <= gate {
		first++
	}
	gated, ok := binMean(&counts, first-1)
	if !ok {
		return 0, true
	}
	return lufs(gated), false
}

// add counts a block of mean square ms, unless the absolute gate drops it.
func (h *Histogram) add(ms float64) {
	l := lufs(ms)
	if !(l > absoluteGate) { // NaN and silence are dropped too.
		return
	}
	bin := min(int((l-absoluteGate)*binsPerLU), histogramBins-1)
	h.counts[bin]++
}

// binMean is the mean square of the blocks in the bins after the bin below.
func binMean(counts *[histogramBins]uint64, below int) (float64, bool) {
	var sum float64
	var n uint64
	for bin := below + 1; bin < histogramBins; bin++ {
		if c := counts[bin]; c > 0 {
			sum += float64(c) * binMeanSquare(bin)
			n += c
		}
	}
	if n == 0 {
		return 0, false
	}
	return sum / float64(n), true
}
