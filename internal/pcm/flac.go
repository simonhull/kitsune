package pcm

import (
	"errors"
	"fmt"
	"io"

	"github.com/mewkiz/flac"
)

// flacDecoder reads FLAC frames with github.com/mewkiz/flac.
type flacDecoder struct {
	stream *flac.Stream
	scale  float64
	buf    [][]float64
}

func openFLAC(r io.Reader) (*Stream, error) {
	s, err := flac.New(r)
	if err != nil {
		return nil, fmt.Errorf("%w: flac: %w", ErrCorrupt, err)
	}
	info := s.Info
	if info.BitsPerSample < 4 || info.BitsPerSample > 32 || info.NChannels == 0 {
		return nil, fmt.Errorf("%w: flac of %d bits, %d channels", ErrUnsupported, info.BitsPerSample, info.NChannels)
	}
	return &Stream{
		Rate: int(info.SampleRate), Channels: int(info.NChannels),
		dec: &flacDecoder{stream: s, scale: 1 / float64(uint64(1)<<(info.BitsPerSample-1))},
	}, nil
}

func (d *flacDecoder) next() ([][]float64, error) {
	f, err := d.stream.ParseNext()
	if errors.Is(err, io.EOF) {
		return nil, io.EOF
	}
	if err != nil {
		return nil, fmt.Errorf("%w: flac frame: %w", ErrCorrupt, err)
	}
	n := int(f.BlockSize)
	d.buf = planes(d.buf, len(f.Subframes), n)
	for c, sub := range f.Subframes {
		if len(sub.Samples) < n {
			return nil, fmt.Errorf("%w: flac subframe of %d samples, want %d", ErrCorrupt, len(sub.Samples), n)
		}
		out := d.buf[c]
		for i, x := range sub.Samples[:n] {
			out[i] = float64(x) * d.scale
		}
	}
	return d.buf, nil
}
