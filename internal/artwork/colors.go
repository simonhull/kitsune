package artwork

import (
	"bytes"
	"fmt"
	"image/jpeg"
	"math"
)

// Thresholds of the color rules.
const (
	bucketBits     = 4 // Bits kept of each channel.
	channelBits    = 8
	bucketShift    = channelBits - bucketBits
	channels       = 3
	half           = 2
	bucketCount    = 1 << (channels * bucketBits)
	minAccentSat   = 0.35 // HSL saturation an accent needs...
	minAccentLum   = 0.25 // ...and its lightness range.
	maxAccentLum   = 0.80
	minAccentFrac  = 50  // An accent needs at least 1/50 (2%) of the pixels.
	hueBins        = 12  // Accent candidates: one bin per 30 degrees of hue.
	minContrast    = 4.5 // WCAG AA for text on the ground.
	lumStep        = 0.005
	fallbackAccent = "#f28c28" // The fox.
)

// Color-space constants (sRGB, WCAG 2, HSL).
const (
	byteMax       = 255
	hueSector     = 60
	hueTurn       = 360
	hueSectors    = 6
	linearKnee    = 0.03928
	linearSlope   = 12.92
	gammaOffset   = 0.055
	gammaScale    = 1.055
	gamma         = 2.4
	flare         = 0.05
	lumRed        = 0.2126
	lumGreen      = 0.7152
	lumBlue       = 0.0722
	textR         = 0xF5 // The UI's text color, #F5F1EC.
	textG         = 0xF1
	textB         = 0xEC
	sectorGreen   = 1
	sectorCyan    = 2
	sectorBlue    = 3
	sectorMagenta = 4
)

// rgb is an 8-bit sRGB color.
type rgb struct{ r, g, b uint8 }

// String formats c as lowercase #rrggbb.
func (c rgb) String() string { return fmt.Sprintf("#%02x%02x%02x", c.r, c.g, c.b) }

// bucket collects the pixels of one 4-bit-per-channel cell.
type bucket struct {
	n       int
	r, g, b int // Channel sums.
}

// add counts one pixel.
func (b *bucket) add(red, green, blue int) {
	b.n++
	b.r += red
	b.g += green
	b.b += blue
}

func (b *bucket) mean() rgb {
	//nolint:gosec // A mean of bytes is a byte.
	return rgb{uint8(b.r / b.n), uint8(b.g / b.n), uint8(b.b / b.n)}
}

// palette is a cover's accent and ground, as #rrggbb.
type palette struct{ accent, ground string }

// hsl is a color as hue in degrees, saturation and lightness in 0..1.
type hsl struct{ h, s, l float64 }

// colorsOf returns the accent and ground of an encoded JPEG. The colors come
// from what is stored, never from pre-encode pixels, so the first Put and
// every later one agree.
func colorsOf(encoded []byte) (palette, error) {
	img, err := jpeg.Decode(bytes.NewReader(encoded))
	if err != nil {
		return palette{}, fmt.Errorf("decode: %w", err)
	}
	var buckets [bucketCount]bucket
	var hues [hueBins]bucket
	bounds := img.Bounds()
	total := 0
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			r, g, b, _ := img.At(x, y).RGBA()
			r8, g8, b8 := int(r>>channelBits), int(g>>channelBits), int(b>>channelBits)
			k := (r8>>bucketShift)<<(bucketBits+bucketBits) | (g8>>bucketShift)<<bucketBits | b8>>bucketShift
			buckets[k].add(r8, g8, b8)
			//nolint:gosec // Each channel is a byte: it was shifted down from 16 bits.
			if v := toHSL(rgb{uint8(r8), uint8(g8), uint8(b8)}); vivid(v) {
				hues[hueBin(v)].add(r8, g8, b8)
			}
			total++
		}
	}
	return palette{accent: pickAccent(&hues, total), ground: pickGround(&buckets)}, nil
}

// vivid reports whether a pixel is colorful enough to be part of an accent.
func vivid(v hsl) bool {
	return v.s >= minAccentSat && v.l >= minAccentLum && v.l <= maxAccentLum
}

// hueBin returns the bin of a hue. A cover's color is spread over many shades,
// so an accent is found by hue: exact-color cells split one red across dozens.
func hueBin(v hsl) int {
	return int(v.h/(hueTurn/hueBins)) % hueBins
}

// pickAccent is the mean color of the largest hue bin holding enough of the
// image. Ties go to the lowest hue.
func pickAccent(hues *[hueBins]bucket, total int) string {
	best := -1
	for k := range hues {
		bin := &hues[k]
		if bin.n == 0 || bin.n*minAccentFrac < total {
			continue
		}
		if best < 0 || bin.n > hues[best].n {
			best = k
		}
	}
	if best < 0 {
		return fallbackAccent
	}
	return hues[best].mean().String()
}

// pickGround is the largest bucket, darkened until text reads on it. Ties go
// to the lowest key.
func pickGround(buckets *[bucketCount]bucket) string {
	best := 0
	for k := range buckets {
		if buckets[k].n > buckets[best].n {
			best = k
		}
	}
	if buckets[best].n == 0 {
		return rgb{}.String()
	}
	c := buckets[best].mean()
	if contrast(c) >= minContrast {
		return c.String()
	}
	v := toHSL(c)
	for v.l > 0 {
		v.l = max(v.l-lumStep, 0)
		if c = fromHSL(v); contrast(c) >= minContrast {
			return c.String()
		}
	}
	return rgb{}.String()
}

// luminance is the WCAG relative luminance.
func luminance(c rgb) float64 {
	lin := func(v uint8) float64 {
		x := float64(v) / byteMax
		if x <= linearKnee {
			return x / linearSlope
		}
		return math.Pow((x+gammaOffset)/gammaScale, gamma)
	}
	return lumRed*lin(c.r) + lumGreen*lin(c.g) + lumBlue*lin(c.b)
}

// contrast is the WCAG contrast ratio of c against the text color.
func contrast(c rgb) float64 {
	a, b := luminance(c), luminance(rgb{textR, textG, textB})
	return (max(a, b) + flare) / (min(a, b) + flare)
}

// toHSL converts c to HSL.
func toHSL(c rgb) hsl {
	r, g, b := float64(c.r)/byteMax, float64(c.g)/byteMax, float64(c.b)/byteMax
	hi, lo := max(r, g, b), min(r, g, b)
	l := (hi + lo) / half
	d := hi - lo
	if d == 0 {
		return hsl{l: l}
	}
	s := d / (1 - math.Abs(2*l-1))
	var h float64
	switch hi {
	case r:
		h = math.Mod((g-b)/d, hueSectors)
	case g:
		h = (b-r)/d + sectorCyan
	default:
		h = (r-g)/d + sectorMagenta
	}
	h *= hueSector
	if h < 0 {
		h += hueTurn
	}
	return hsl{h, s, l}
}

// fromHSL is the inverse of [toHSL].
func fromHSL(v hsl) rgb {
	h, s, l := v.h, v.s, v.l
	c := (1 - math.Abs(2*l-1)) * s
	x := c * (1 - math.Abs(math.Mod(h/hueSector, half)-1))
	m := l - c/half
	var r, g, b float64
	switch int(h/hueSector) % hueSectors {
	case 0:
		r, g, b = c, x, 0
	case sectorGreen:
		r, g, b = x, c, 0
	case sectorCyan:
		r, g, b = 0, c, x
	case sectorBlue:
		r, g, b = 0, x, c
	case sectorMagenta:
		r, g, b = x, 0, c
	default:
		r, g, b = c, 0, x
	}
	to8 := func(v float64) uint8 {
		return uint8(math.Round(min(max((v+m)*byteMax, 0), byteMax)))
	}
	return rgb{to8(r), to8(g), to8(b)}
}
