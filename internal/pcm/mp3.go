package pcm

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"

	"github.com/hajimehoshi/go-mp3"
)

// mp3Decoder reads MPEG audio with github.com/hajimehoshi/go-mp3, which always
// gives 16-bit stereo: a mono file's two channels are the same, and only one
// is kept.
// mp3FrameBytes is the size of a sample frame go-mp3 gives: two 16-bit
// samples.
const mp3FrameBytes = 4

type mp3Decoder struct {
	dec      *mp3.Decoder
	channels int
	raw      []byte
	buf      [][]float64
}

func openMP3(r io.Reader, channels int) (*Stream, error) {
	if channels != 1 && channels != 2 {
		return nil, fmt.Errorf("%w: mp3 of %d channels", ErrUnsupported, channels)
	}
	d, err := mp3.NewDecoder(r)
	if err != nil {
		return nil, fmt.Errorf("%w: mp3: %w", ErrCorrupt, err)
	}
	return &Stream{
		Rate: d.SampleRate(), Channels: channels,
		dec: &mp3Decoder{dec: d, channels: channels, raw: make([]byte, chunkFrames*mp3FrameBytes)},
	}, nil
}

func (d *mp3Decoder) next() ([][]float64, error) {
	n, err := io.ReadFull(d.dec, d.raw)
	if n == 0 {
		if err == nil || errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
			return nil, io.EOF
		}
		return nil, fmt.Errorf("%w: mp3: %w", ErrCorrupt, err)
	}
	if err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrUnexpectedEOF) {
		return nil, fmt.Errorf("%w: mp3: %w", ErrCorrupt, err)
	}
	frames := n / mp3FrameBytes
	d.buf = planes(d.buf, d.channels, frames)
	const scale = 1.0 / 32768
	for i := range frames {
		for c := range d.channels {
			d.buf[c][i] = float64(int16(binary.LittleEndian.Uint16(d.raw[mp3FrameBytes*i+2*c:]))) * scale //nolint:gosec // G115: the bits are a two's complement sample.
		}
	}
	return d.buf, nil
}
