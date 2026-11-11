package pcm

import (
	"bufio"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
)

// sampleFormat is how a WAV or AIFF file stores its samples.
type sampleFormat struct {
	channels int
	width    int // Bytes per sample.
	float    bool
	unsigned bool // 8-bit WAV.
	order    binary.ByteOrder
}

// MaxChannels is the most channels a stream may have: loudness.NewMeter's
// limit, checked before a decoder sizes its buffers by the count.
const MaxChannels = 8

// check refuses what [pcmDecoder] can't read.
func (f *sampleFormat) check() error {
	switch {
	case f.channels < 1:
		return fmt.Errorf("%w: %d channels", ErrCorrupt, f.channels)
	case f.channels > MaxChannels:
		return fmt.Errorf("%w: %d channels", ErrUnsupported, f.channels)
	case f.float && f.width != float32Bytes && f.width != float64Bytes:
		return fmt.Errorf("%w: %d-byte floats", ErrUnsupported, f.width)
	case !f.float && (f.width < 1 || f.width > int32Bytes):
		return fmt.Errorf("%w: %d-byte samples", ErrUnsupported, f.width)
	}
	return nil
}

// pcmDecoder reads uncompressed samples, interleaved, from a data chunk.
type pcmDecoder struct {
	sampleFormat

	r    io.Reader // Holds only the samples.
	raw  []byte
	buf  [][]float64
	read func(b []byte) float64
}

// newPCM returns the stream of the samples r holds.
func newPCM(r io.Reader, rate int, f *sampleFormat) (*Stream, error) {
	if err := f.check(); err != nil {
		return nil, err
	}
	if rate <= 0 {
		return nil, fmt.Errorf("%w: rate %d", ErrCorrupt, rate)
	}
	d := &pcmDecoder{sampleFormat: *f, r: r, raw: make([]byte, chunkFrames*f.channels*f.width)}
	d.read = d.reader()
	return &Stream{Rate: rate, Channels: f.channels, dec: d}, nil
}

// Integer samples are read into the top of a 32-bit word, as they are
// left-justified in their bytes: a sample's width alone gives its scale.
const (
	wordShift = 24 // Where a sample's first, most significant, byte goes.
	fullScale = 1 << 31
	signBit   = 1 << 31
)

// reader returns what reads one sample of this format, scaled to full scale
// at 1.
func (f *sampleFormat) reader() func(b []byte) float64 {
	o := f.order
	switch {
	case f.float && f.width == float32Bytes:
		return func(b []byte) float64 { return float64(math.Float32frombits(o.Uint32(b))) }
	case f.float:
		return func(b []byte) float64 { return math.Float64frombits(o.Uint64(b)) }
	}
	return f.integer
}

// integer reads an integer sample.
func (f *sampleFormat) integer(b []byte) float64 {
	big := f.order == binary.BigEndian
	var v uint32
	for k := range f.width {
		i := k
		if !big {
			i = f.width - 1 - k
		}
		v |= uint32(b[i]) << (wordShift - bitsPerByte*k)
	}
	if f.unsigned {
		v ^= signBit // Offset binary to two's complement.
	}
	return float64(int32(v)) / fullScale
}

func (d *pcmDecoder) next() ([][]float64, error) {
	n, err := io.ReadFull(d.r, d.raw)
	frame := d.channels * d.width
	frames := n / frame
	if frames == 0 {
		if err == nil || errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
			return nil, io.EOF // A cut-off last frame is dropped.
		}
		return nil, fmt.Errorf("pcm: %w", err)
	}
	if err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrUnexpectedEOF) {
		return nil, fmt.Errorf("pcm: %w", err)
	}
	d.buf = planes(d.buf, d.channels, frames)
	for i := range frames {
		at := i * frame
		for c := range d.channels {
			d.buf[c][i] = d.read(d.raw[at+c*d.width:])
		}
	}
	return d.buf, nil
}

// chunks reads the chunks of a RIFF or IFF file in turn.
type chunks struct {
	r     *bufio.Reader
	order binary.ByteOrder
}

// chunkHeader is a chunk's id and size.
type chunkHeader struct {
	id   string
	size uint64
}

// next reads the header of the next chunk.
func (cs *chunks) next() (chunkHeader, error) {
	var b [8]byte
	if _, err := io.ReadFull(cs.r, b[:]); err != nil {
		return chunkHeader{}, fmt.Errorf("%w: no audio chunk", ErrCorrupt)
	}
	return chunkHeader{id: string(b[:4]), size: uint64(cs.order.Uint32(b[4:]))}, nil
}

// body reads at least want and at most limit bytes of the chunk ch, and
// moves past the rest of it.
func (cs *chunks) body(ch chunkHeader, want, limit uint64) ([]byte, error) {
	if ch.size < want {
		return nil, fmt.Errorf("%w: %q chunk of %d bytes", ErrCorrupt, ch.id, ch.size)
	}
	b := make([]byte, min(ch.size, limit))
	if _, err := io.ReadFull(cs.r, b); err != nil {
		return nil, fmt.Errorf("%w: %q chunk cut short", ErrCorrupt, ch.id)
	}
	return b, cs.skip(ch, uint64(len(b)))
}

// skip moves past the rest of the chunk ch, of which read bytes are read, and
// its pad byte.
func (cs *chunks) skip(ch chunkHeader, read uint64) error {
	for rest := ch.size - read + ch.size&1; rest > 0; {
		n, err := cs.r.Discard(int(min(rest, math.MaxInt32)))
		if err != nil {
			return fmt.Errorf("%w: %q chunk cut short", ErrCorrupt, ch.id)
		}
		rest -= uint64(n) //nolint:gosec // G115: Discard returns no more than asked.
	}
	return nil
}

// header is what a RIFF or IFF file starts with: its magic, such as RIFF or
// FORM, and its form, such as WAVE or AIFF.
type header struct{ magic, form string }

// readHeader reads the 12 bytes a RIFF or IFF file starts with, and wraps r
// for its chunks.
func readHeader(r io.Reader, order binary.ByteOrder) (*chunks, header, error) {
	br := bufio.NewReaderSize(r, readBuffer)
	var head [12]byte
	if _, err := io.ReadFull(br, head[:]); err != nil {
		return nil, header{}, fmt.Errorf("%w: header: %w", ErrCorrupt, err)
	}
	return &chunks{r: br, order: order}, header{magic: string(head[:4]), form: string(head[8:])}, nil
}

// until reads chunks up to the first one of id, which it returns, handing
// each before it to other.
func (cs *chunks) until(id string, other func(chunkHeader) error) (chunkHeader, error) {
	for {
		ch, err := cs.next()
		if err != nil {
			return chunkHeader{}, err
		}
		if ch.id == id {
			return ch, nil
		}
		if otherErr := other(ch); otherErr != nil {
			return chunkHeader{}, otherErr
		}
	}
}

// Sizes in WAV and AIFF files.
const (
	float32Bytes  = 4
	float64Bytes  = 8
	int32Bytes    = 4
	readBuffer    = 1 << 16
	bitsPerByte   = 8
	fmtMin        = 16 // A fmt chunk's fixed part.
	fmtExtensible = 40 // WAVE_FORMAT_EXTENSIBLE's.
	ds64Min       = 16 // As far as the data size.
	commSize      = 18 // AIFF's COMM chunk.
	aifcCommSize  = 22 // AIFF-C's, with its compression type.
	ssndHeader    = 8  // SSND's offset and block size.
)

// The WAVE format tags read here.
const (
	wavePCM        = 0x0001
	waveFloat      = 0x0003
	waveExtensible = 0xFFFE
)

// wavHeaders is what a WAVE file's chunks before its data say.
type wavHeaders struct {
	format *sampleFormat
	rate   int
	size64 uint64 // The data size of an RF64 file, from its ds64 chunk.
}

// read takes in one chunk before the data.
func (w *wavHeaders) read(cs *chunks, ch chunkHeader) error {
	switch ch.id {
	case "ds64":
		b, err := cs.body(ch, ds64Min, ds64Min)
		if err != nil {
			return err
		}
		w.size64 = binary.LittleEndian.Uint64(b[8:])
		return nil
	case "fmt ":
		b, err := cs.body(ch, fmtMin, fmtExtensible)
		if err != nil {
			return err
		}
		w.format, w.rate, err = parseFmt(b)
		return err
	}
	return cs.skip(ch, 0)
}

// openWAV reads a RIFF or RF64 WAVE file's fmt chunk and stops at its data.
func openWAV(r io.Reader) (*Stream, error) {
	cs, h, err := readHeader(r, binary.LittleEndian)
	if err != nil {
		return nil, err
	}
	if (h.magic != "RIFF" && h.magic != "RF64") || h.form != "WAVE" {
		return nil, fmt.Errorf("%w: not a RIFF WAVE file", ErrUnsupported)
	}
	var w wavHeaders
	data, err := cs.until("data", func(ch chunkHeader) error { return w.read(cs, ch) })
	if err != nil {
		return nil, err
	}
	if w.format == nil {
		return nil, fmt.Errorf("%w: wav data before fmt", ErrCorrupt)
	}
	if h.magic == "RF64" && data.size == math.MaxUint32 {
		data.size = w.size64
	}
	return newPCM(io.LimitReader(cs.r, int64(min(data.size, math.MaxInt64))), w.rate, w.format)
}

// parseFmt reads a fmt chunk.
func parseFmt(b []byte) (*sampleFormat, int, error) {
	le := binary.LittleEndian
	tag := le.Uint16(b)
	if tag == waveExtensible && len(b) == fmtExtensible {
		tag = le.Uint16(b[24:]) // The sub-format GUID starts with the format tag.
	}
	if tag != wavePCM && tag != waveFloat {
		return nil, 0, fmt.Errorf("%w: wave format 0x%04x", ErrUnsupported, tag)
	}
	channels := int(le.Uint16(b[2:]))
	align := int(le.Uint16(b[12:]))
	if channels == 0 || align == 0 || align%channels != 0 {
		return nil, 0, fmt.Errorf("%w: block align %d for %d channels", ErrCorrupt, align, channels)
	}
	width := align / channels
	f := &sampleFormat{channels: channels, width: width, float: tag == waveFloat, unsigned: tag == wavePCM && width == 1, order: le}
	return f, int(le.Uint32(b[4:])), nil
}

// openAIFF reads an AIFF or AIFF-C file's COMM chunk and stops at its sound
// data. AIFF-C is read when uncompressed: big- or little-endian integers, or
// floats.
func openAIFF(r io.Reader) (*Stream, error) {
	cs, h, err := readHeader(r, binary.BigEndian)
	if err != nil {
		return nil, err
	}
	if h.magic != "FORM" || (h.form != "AIFF" && h.form != "AIFC") {
		return nil, fmt.Errorf("%w: not an AIFF file", ErrUnsupported)
	}
	comm := uint64(commSize)
	if h.form == "AIFC" {
		comm = aifcCommSize
	}
	var f *sampleFormat
	var rate int
	ssnd, err := cs.until("SSND", func(ch chunkHeader) error {
		if ch.id != "COMM" {
			return cs.skip(ch, 0)
		}
		b, bodyErr := cs.body(ch, comm, comm)
		if bodyErr != nil {
			return bodyErr
		}
		f, rate, bodyErr = parseComm(b)
		return bodyErr
	})
	if err != nil {
		return nil, err
	}
	if f == nil {
		return nil, fmt.Errorf("%w: aiff sound data before COMM", ErrCorrupt)
	}
	return soundData(cs, ssnd, rate, f)
}

// soundData starts the stream of an SSND chunk, past its offset.
func soundData(cs *chunks, ch chunkHeader, rate int, f *sampleFormat) (*Stream, error) {
	if ch.size < ssndHeader {
		return nil, fmt.Errorf("%w: short SSND", ErrCorrupt)
	}
	var b [ssndHeader]byte
	if _, err := io.ReadFull(cs.r, b[:]); err != nil {
		return nil, fmt.Errorf("%w: SSND cut short", ErrCorrupt)
	}
	offset := uint64(binary.BigEndian.Uint32(b[:]))
	if offset > ch.size-ssndHeader {
		return nil, fmt.Errorf("%w: SSND offset past its end", ErrCorrupt)
	}
	if _, err := cs.r.Discard(int(offset)); err != nil { //nolint:gosec // G115: a uint32.
		return nil, fmt.Errorf("%w: SSND cut short", ErrCorrupt)
	}
	return newPCM(io.LimitReader(cs.r, int64(ch.size-ssndHeader-offset)), rate, f) //nolint:gosec // G115: a uint32's worth.
}

// parseComm reads a COMM chunk, with the compression type when it holds one.
func parseComm(b []byte) (*sampleFormat, int, error) {
	be := binary.BigEndian
	bits := int(be.Uint16(b[6:]))
	f := &sampleFormat{channels: int(be.Uint16(b)), width: (bits + bitsPerByte - 1) / bitsPerByte, order: be}
	if len(b) >= aifcCommSize {
		if err := f.compression(string(b[18:22])); err != nil {
			return nil, 0, err
		}
	}
	rate := extended([extendedSize]byte(b[8:18]))
	if !(rate >= 1 && rate < math.MaxInt32) {
		return nil, 0, fmt.Errorf("%w: rate %v", ErrCorrupt, rate)
	}
	return f, int(math.Round(rate)), nil
}

// compression sets what an AIFF-C compression type says of the samples.
func (f *sampleFormat) compression(kind string) error {
	switch kind {
	case "NONE", "twos", "in24", "in32":
	case "sowt":
		f.order = binary.LittleEndian
	case "fl32", "FL32":
		f.float, f.width = true, float32Bytes
	case "fl64", "FL64":
		f.float, f.width = true, float64Bytes
	default:
		return fmt.Errorf("%w: aiff-c compression %q", ErrUnsupported, kind)
	}
	return nil
}

// An 80-bit IEEE 754 extended float, as AIFF gives its rate: a sign bit, 15
// bits of exponent and a 64-bit mantissa with its integer bit.
const (
	extendedSize     = 10
	extendedExponent = 0x7FFF
	extendedBias     = 16383
	mantissaBits     = 63
)

// extended decodes an 80-bit extended float.
func extended(b [extendedSize]byte) float64 {
	se := binary.BigEndian.Uint16(b[:])
	mantissa := binary.BigEndian.Uint64(b[2:])
	v := math.Ldexp(float64(mantissa), int(se&extendedExponent)-extendedBias-mantissaBits)
	if se&^extendedExponent != 0 {
		v = -v
	}
	return v
}
