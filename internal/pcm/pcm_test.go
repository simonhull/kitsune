package pcm_test

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"math"
	"os"
	"testing"

	"github.com/simonhull/kitsune/internal/loudness"
	"github.com/simonhull/kitsune/internal/pcm"
)

// decoded is a whole stream's audio.
type decoded struct {
	stream *pcm.Stream
	planes [][]float64
}

// decodeAll reads every chunk of the stream of r.
func decodeAll(t *testing.T, r io.ReadSeeker, kind pcm.Kind, channels int) decoded {
	t.Helper()
	s, err := pcm.Open(r, kind, channels)
	if err != nil {
		t.Fatal(err)
	}
	all := make([][]float64, s.Channels)
	for {
		planes, readErr := s.Read()
		if errors.Is(readErr, io.EOF) {
			return decoded{stream: s, planes: all}
		}
		if readErr != nil {
			t.Fatal(readErr)
		}
		for c := range planes {
			all[c] = append(all[c], planes[c]...)
		}
	}
}

// loudnessOf meters planes.
func loudnessOf(t *testing.T, rate int, planes [][]float64) loudness.Result {
	t.Helper()
	m, err := loudness.NewMeter(rate, len(planes))
	if err != nil {
		t.Fatal(err)
	}
	if err = m.Push(planes...); err != nil {
		t.Fatal(err)
	}
	r, err := m.Finish()
	if err != nil {
		t.Fatal(err)
	}
	return r
}

// The fixtures in testdata are a 1 kHz sine at -20 dBFS, made with ffmpeg.
// Their reference loudness is ffmpeg's ebur128: an MP3 starts and ends with
// the encoder's silence, which the decoders here keep, as ffmpeg does.
func TestFixtures(t *testing.T) {
	t.Parallel()
	cases := []struct {
		file     string
		kind     pcm.Kind
		channels int
		rate     int
		want     float64
	}{
		{"sine_24.flac", pcm.FLAC, 2, 48_000, -20.0},
		{"sine_stereo.mp3", pcm.MP3, 2, 44_100, -20.4},
		{"sine_mono.mp3", pcm.MP3, 1, 44_100, -23.5},
	}
	for _, tc := range cases {
		t.Run(tc.file, func(t *testing.T) {
			t.Parallel()
			f, err := os.Open("testdata/" + tc.file)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = f.Close() }()
			d := decodeAll(t, f, tc.kind, tc.channels)
			s, planes := d.stream, d.planes
			if s.Rate != tc.rate || s.Channels != tc.channels {
				t.Fatalf("%d Hz, %d channels; want %d Hz, %d", s.Rate, s.Channels, tc.rate, tc.channels)
			}
			r := loudnessOf(t, s.Rate, planes)
			if math.Abs(r.Integrated-tc.want) > 0.1 {
				t.Errorf("I = %.2f, want %.1f", r.Integrated, tc.want)
			}
		})
	}
}

// pcmFile is a WAV or AIFF file being written.
type pcmFile struct {
	order binary.ByteOrder
	buf   bytes.Buffer
}

func (p *pcmFile) str(s string) { p.buf.WriteString(s) }

func (p *pcmFile) u16(v int) { _ = binary.Write(&p.buf, p.order, uint16(v)) } //nolint:gosec // G115: test values fit.

func (p *pcmFile) u32(v int) { _ = binary.Write(&p.buf, p.order, uint32(v)) } //nolint:gosec // G115: as above.

// sineSamples is a stereo sine at half of full scale, its channels in
// opposite phase.
func sineSamples(n int) [][2]float64 {
	const amplitude = 0.5
	out := make([][2]float64, n)
	for i := range out {
		v := amplitude * math.Sin(2*math.Pi*1000*float64(i)/48_000)
		out[i] = [2]float64{v, -v}
	}
	return out
}

// encodeSample writes v as width bytes of an integer or float sample.
func encodeSample(w *bytes.Buffer, order binary.ByteOrder, v float64, width int, float, unsigned bool) {
	switch {
	case float && width == 4:
		_ = binary.Write(w, order, math.Float32bits(float32(v)))
	case float:
		_ = binary.Write(w, order, math.Float64bits(v))
	case unsigned:
		w.WriteByte(byte(int(math.Round(v*127)) + 128)) //nolint:gosec // G115: within a byte.
	default:
		full := int64(math.Round(v * float64(int64(1)<<(8*width-1)-1)))
		var b [8]byte
		order.PutUint64(b[:], uint64(full)) //nolint:gosec // G115: two's complement on purpose.
		if order == binary.BigEndian {
			w.Write(b[8-width:])
		} else {
			w.Write(b[:width])
		}
	}
}

// wav writes a WAV file of samples, padded with an odd chunk before the data
// to check the pad byte is skipped.
func wav(samples [][2]float64, tag, width int, extensible bool) []byte {
	var data bytes.Buffer
	for _, s := range samples {
		for _, v := range s {
			encodeSample(&data, binary.LittleEndian, v, width, tag == 3, tag == 1 && width == 1)
		}
	}
	p := &pcmFile{order: binary.LittleEndian}
	fmtSize := 16
	if extensible {
		fmtSize = 40
	}
	p.str("RIFF")
	p.u32(4 + 8 + fmtSize + 8 + 3 + 1 + 8 + data.Len())
	p.str("WAVE")
	p.str("fmt ")
	p.u32(fmtSize)
	if extensible {
		p.u16(0xFFFE)
	} else {
		p.u16(tag)
	}
	p.u16(2)
	p.u32(48_000)
	p.u32(48_000 * 2 * width)
	p.u16(2 * width)
	p.u16(8 * width)
	if extensible {
		p.u16(22)
		p.u16(8 * width)
		p.u32(3)
		p.u16(tag)
		p.str("\x00\x00\x00\x00\x10\x00\x80\x00\x00\xaa\x00\x38\x9b\x71")
	}
	p.str("odd ")
	p.u32(3)
	p.str("abc\x00")
	p.str("data")
	p.u32(data.Len())
	p.buf.Write(data.Bytes())
	return p.buf.Bytes()
}

// aiff writes an AIFF or AIFF-C file of samples.
func aiff(samples [][2]float64, compression string, width int) []byte {
	order := binary.ByteOrder(binary.BigEndian)
	if compression == "sowt" {
		order = binary.LittleEndian
	}
	float := compression == "fl32" || compression == "fl64"
	var data bytes.Buffer
	for _, s := range samples {
		for _, v := range s {
			encodeSample(&data, order, v, width, float, false)
		}
	}
	p := &pcmFile{order: binary.BigEndian}
	form, comm := "AIFF", 18
	if compression != "" {
		form, comm = "AIFC", 22
	}
	p.str("FORM")
	p.u32(4 + 8 + comm + 8 + 8 + 4 + data.Len())
	p.str(form)
	p.str("COMM")
	p.u32(comm)
	p.u16(2)
	p.u32(len(samples))
	p.u16(8 * width)
	p.str("\x40\x0e\xbb\x80\x00\x00\x00\x00\x00\x00") // 48,000 as an 80-bit float.
	if compression != "" {
		p.str(compression)
	}
	p.str("SSND")
	p.u32(8 + 4 + data.Len())
	p.u32(4) // An offset, to check it is skipped.
	p.u32(0)
	p.str("zzzz")
	p.buf.Write(data.Bytes())
	return p.buf.Bytes()
}

func TestWAVAndAIFF(t *testing.T) {
	t.Parallel()
	samples := sineSamples(48_000)
	cases := []struct {
		name  string
		file  []byte
		kind  pcm.Kind
		error float64 // The most a sample may be off: its quantization.
	}{
		{"wav 8-bit", wav(samples, 1, 1, false), pcm.WAV, 1.0 / 64},
		{"wav 16-bit", wav(samples, 1, 2, false), pcm.WAV, 1.0 / (1 << 14)},
		{"wav 24-bit extensible", wav(samples, 1, 3, true), pcm.WAV, 1.0 / (1 << 22)},
		{"wav 32-bit", wav(samples, 1, 4, false), pcm.WAV, 1.0 / (1 << 30)},
		{"wav float", wav(samples, 3, 4, false), pcm.WAV, 1e-7},
		{"wav double", wav(samples, 3, 8, true), pcm.WAV, 1e-15},
		{"aiff 16-bit", aiff(samples, "", 2), pcm.AIFF, 1.0 / (1 << 14)},
		{"aiff 24-bit", aiff(samples, "", 3), pcm.AIFF, 1.0 / (1 << 22)},
		{"aifc sowt", aiff(samples, "sowt", 2), pcm.AIFF, 1.0 / (1 << 14)},
		{"aifc fl32", aiff(samples, "fl32", 4), pcm.AIFF, 1e-7},
		{"aifc fl64", aiff(samples, "fl64", 8), pcm.AIFF, 1e-15},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			d := decodeAll(t, bytes.NewReader(tc.file), tc.kind, 0)
			s, planes := d.stream, d.planes
			if s.Rate != 48_000 || s.Channels != 2 || len(planes[0]) != len(samples) {
				t.Fatalf("%d Hz, %d channels, %d frames", s.Rate, s.Channels, len(planes[0]))
			}
			for i, want := range samples {
				for c := range want {
					if d := math.Abs(planes[c][i] - want[c]); d > tc.error {
						t.Fatalf("frame %d channel %d: %v, want %v", i, c, planes[c][i], want[c])
					}
				}
			}
		})
	}
}

// wavWithChannels is a valid WAV whose fmt chunk claims n channels of two bytes.
func wavWithChannels(t *testing.T, n int) []byte {
	t.Helper()
	b := wav(sineSamples(10), 1, 2, false)
	binary.LittleEndian.PutUint16(b[22:], uint16(n))
	binary.LittleEndian.PutUint16(b[32:], uint16(2*n))
	return b
}

func TestRefusals(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		file []byte
		kind pcm.Kind
		want error
	}{
		{"a-law wav", wav(sineSamples(10), 6, 1, false), pcm.WAV, pcm.ErrUnsupported},
		{"compressed aifc", aiff(sineSamples(10), "ima4", 2), pcm.AIFF, pcm.ErrUnsupported},
		{"not a wav", []byte("RIFF\x04\x00\x00\x00AVI "), pcm.WAV, pcm.ErrUnsupported},
		{"truncated wav", wav(sineSamples(10), 1, 2, false)[:30], pcm.WAV, pcm.ErrCorrupt},
		{"wav with 12 channels", wavWithChannels(t, 12), pcm.WAV, pcm.ErrUnsupported},
		{"aac", nil, pcm.Kind(99), pcm.ErrUnsupported},
		{"5.1 mp3", nil, pcm.MP3, pcm.ErrUnsupported},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			channels := 0
			if tc.kind == pcm.MP3 {
				channels = 6
			}
			_, err := pcm.Open(bytes.NewReader(tc.file), tc.kind, channels)
			if !errors.Is(err, tc.want) {
				t.Errorf("err = %v, want %v", err, tc.want)
			}
		})
	}
}
