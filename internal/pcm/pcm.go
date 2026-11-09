package pcm

import (
	"errors"
	"fmt"
	"io"
)

// Kind is a file format this package can decode.
type Kind int

// The formats this package decodes.
const (
	FLAC Kind = iota + 1
	MP3
	WAV
	AIFF
)

// Errors a [Stream] returns.
var (
	// ErrUnsupported is audio this package can't decode: a format or a codec
	// it doesn't read.
	ErrUnsupported = errors.New("pcm: unsupported audio")
	// ErrCorrupt is a file whose structure is broken.
	ErrCorrupt = errors.New("pcm: corrupt audio")
)

// chunkFrames is about how many sample frames a [Stream.Read] returns.
const chunkFrames = 8192

// decoder is one format's reader: it returns the next chunk of planes, or
// io.EOF after the last.
type decoder interface {
	next() ([][]float64, error)
}

// Stream is the audio of one file. It is not safe for concurrent use.
type Stream struct {
	// Rate is the sample rate in Hz, and Channels how many planes each read
	// returns.
	Rate, Channels int
	dec            decoder
}

// Open starts decoding r as kind. Channels is how many channels the file's
// tags reader found, which an MP3 decoder can't say itself: a decoder that
// knows its file's channels uses those.
//
// # Errors
//
// It returns an error wrapping [ErrUnsupported] for a kind or codec it
// doesn't decode, [ErrCorrupt] for a broken header, or the read error.
func Open(r io.ReadSeeker, kind Kind, channels int) (*Stream, error) {
	switch kind {
	case FLAC:
		return openFLAC(r)
	case MP3:
		return openMP3(r, channels)
	case WAV:
		return openWAV(r)
	case AIFF:
		return openAIFF(r)
	}
	return nil, fmt.Errorf("%w: kind %d", ErrUnsupported, kind)
}

// Read returns the next chunk of audio: one plane per channel, all as long,
// valid until the next Read. After the last chunk it returns io.EOF.
//
// # Errors
//
// Besides io.EOF, it returns an error wrapping [ErrCorrupt] for broken audio,
// or the read error.
func (s *Stream) Read() ([][]float64, error) {
	planes, err := s.dec.next()
	if err != nil {
		if errors.Is(err, io.EOF) {
			return nil, io.EOF
		}
		return nil, err
	}
	return planes, nil
}

// planes returns n frames of channels planes, reusing buf's.
func planes(buf [][]float64, channels, n int) [][]float64 {
	if len(buf) != channels {
		buf = make([][]float64, channels)
	}
	for c := range buf {
		if cap(buf[c]) < n {
			buf[c] = make([]float64, n)
		}
		buf[c] = buf[c][:n]
	}
	return buf
}
