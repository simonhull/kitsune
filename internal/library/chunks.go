package library

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
)

// Sizes of the parts of a WAV or AIFF file that [chunkID3Version] reads.
const (
	formHeaderSize  = 12 // Magic, size and form type.
	chunkHeaderSize = 8  // ID and size.
	// ds64DataSize is where an RF64 file keeps its data chunk's 64-bit size:
	// in the ds64 chunk that follows the header, after the RIFF size.
	ds64DataSize   = formHeaderSize + chunkHeaderSize + 8
	maxChunkWalk   = 1 << 12
	unsetChunkSize = 0xFFFFFFFF
	id3HeaderSize  = 4 // "ID3" and the major version.
)

// errNoID3Chunk means a WAV or AIFF file has no ID3v2 chunk, so a write gives it
// an ID3v2.4 one.
var errNoID3Chunk = errors.New("no ID3v2 chunk")

// chunkID3Version returns the major version of the ID3v2 tag in the "id3 " or
// "ID3 " chunk of the WAV (RIFF, RIFX, RF64) or AIFF file at path, which
// audiometa doesn't report. It walks the chunk headers only. A file without the
// chunk returns [errNoID3Chunk].
func chunkID3Version(path string) (byte, error) {
	file, err := os.Open(path) //nolint:gosec // G304: audiometa just opened this path.
	if err != nil {
		return 0, fmt.Errorf("read the ID3 version: %w", err)
	}
	defer func() { _ = file.Close() }() // Only read.
	order, rf64Data, err := readForm(file)
	if err != nil {
		return 0, fmt.Errorf("read the ID3 version: %w", err)
	}
	off := int64(formHeaderSize)
	for range maxChunkWalk {
		var ch [chunkHeaderSize + id3HeaderSize]byte
		if n, _ := file.ReadAt(ch[:], off); n < chunkHeaderSize {
			break
		}
		id, size := string(ch[:4]), uint64(order.Uint32(ch[4:8]))
		if id == "id3 " || id == "ID3 " {
			if string(ch[8:11]) != "ID3" {
				break
			}
			return ch[11], nil
		}
		if size == unsetChunkSize && id == "data" {
			size = rf64Data
		}
		off += chunkHeaderSize + int64(size) + int64(size&1) //nolint:gosec // G115: a 32-bit size, or RF64's, which a real file never takes past int64.
	}
	return 0, errNoID3Chunk
}

// readForm reads the container header: the byte order of its sizes and, for
// RF64, the 64-bit size of the data chunk.
func readForm(file io.ReaderAt) (binary.ByteOrder, uint64, error) {
	var hdr [ds64DataSize + 8]byte
	n, err := file.ReadAt(hdr[:], 0)
	if n < formHeaderSize {
		if err == nil || errors.Is(err, io.EOF) {
			err = io.ErrUnexpectedEOF
		}
		return nil, 0, err
	}
	switch magic := string(hdr[:4]); {
	case magic == "RF64" && n == len(hdr):
		return binary.LittleEndian, binary.LittleEndian.Uint64(hdr[ds64DataSize:]), nil
	case magic == "RIFF" || magic == "RF64":
		return binary.LittleEndian, 0, nil
	}
	return binary.BigEndian, 0, nil
}
