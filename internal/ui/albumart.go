package ui

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"image/png"
	"os"
	"strings"
	"sync/atomic"

	_ "image/gif"
	_ "image/jpeg"

	"golang.org/x/image/draw"
)

// imageCounter generates unique IDs for Kitty graphics placements.
var imageCounter atomic.Uint32

// AlbumArt handles terminal image rendering via the Kitty graphics protocol.
type AlbumArt struct {
	supported    bool
	cache        map[string]uint32 // albumID → kitty image ID
	imageData    map[string]string // albumID → base64 encoded PNG
	cellSize     int               // art size in terminal cells (rows/cols)
	currentImgID uint32            // ID of currently displayed image
}

// NewAlbumArt creates an album art renderer.
// cellSize is the number of terminal rows/columns for the art (square).
func NewAlbumArt(cellSize int) *AlbumArt {
	if cellSize < 4 {
		cellSize = 8
	}
	return &AlbumArt{
		supported: detectKittyGraphics(),
		cache:     make(map[string]uint32),
		imageData: make(map[string]string),
		cellSize:  cellSize,
	}
}

// Supported returns whether the terminal supports inline images.
func (a *AlbumArt) Supported() bool {
	return a.supported
}

// CellSize returns the art dimensions in terminal cells.
func (a *AlbumArt) CellSize() int {
	return a.cellSize
}

// Upload transmits the image to the terminal (without displaying it) and returns
// the Kitty image ID. Call Place() separately to position it.
func (a *AlbumArt) Upload(albumID string, imageData []byte) string {
	if !a.supported || len(imageData) == 0 {
		return ""
	}

	// Check cache — already uploaded.
	if _, ok := a.cache[albumID]; ok {
		return ""
	}

	// Decode image.
	img, _, err := image.Decode(bytes.NewReader(imageData))
	if err != nil {
		return ""
	}

	// Resize to target pixel size.
	pixelSize := a.cellSize * 16
	resized := resizeImage(img, pixelSize, pixelSize)

	// Encode as PNG.
	var buf bytes.Buffer
	if err := png.Encode(&buf, resized); err != nil {
		return ""
	}

	// Assign unique ID.
	id := imageCounter.Add(1)
	a.cache[albumID] = id

	// Transmit image to terminal (a=t: transmit only, no display).
	encoded := base64.StdEncoding.EncodeToString(buf.Bytes())
	return kittyTransmit(id, encoded)
}

// Place returns the escape sequence to display a previously uploaded image
// at a specific position. Call this after the frame renders.
func (a *AlbumArt) Place(albumID string) string {
	if !a.supported {
		return ""
	}

	id, ok := a.cache[albumID]
	if !ok {
		return ""
	}

	// Delete old placement if different image.
	var sb strings.Builder
	if a.currentImgID != 0 && a.currentImgID != id {
		sb.WriteString(fmt.Sprintf("\x1b_Ga=d,d=i,i=%d;\x1b\\", a.currentImgID))
	}
	a.currentImgID = id

	// Display with Unicode placeholder (virtual placement).
	// a=p: display, i=id, c=cols, r=rows, U=1: use Unicode placeholders.
	sb.WriteString(fmt.Sprintf("\x1b_Ga=p,i=%d,c=%d,r=%d,U=1;\x1b\\", id, a.cellSize, a.cellSize))

	// Write placeholder characters — the terminal replaces these with the image.
	for row := 0; row < a.cellSize; row++ {
		for col := 0; col < a.cellSize; col++ {
			// Unicode placeholder: U+10EEEE (Kitty private use) encoded as diacritics.
			// Simpler approach: just reserve the space. The terminal renders the image
			// over these cells when using virtual placements.
			sb.WriteByte(' ')
		}
		if row < a.cellSize-1 {
			sb.WriteByte('\n')
		}
	}

	return sb.String()
}

// RenderInline returns the image as an inline Kitty graphics escape sequence.
// This renders at the current cursor position. Text content should reserve
// blank space where the image will appear.
func (a *AlbumArt) RenderInline(albumID string, imageData []byte) string {
	if !a.supported || len(imageData) == 0 {
		return ""
	}

	// Decode image.
	img, _, err := image.Decode(bytes.NewReader(imageData))
	if err != nil {
		return ""
	}

	// Resize.
	pixelSize := a.cellSize * 16
	resized := resizeImage(img, pixelSize, pixelSize)

	// Encode as PNG.
	var buf bytes.Buffer
	if err := png.Encode(&buf, resized); err != nil {
		return ""
	}

	encoded := base64.StdEncoding.EncodeToString(buf.Bytes())
	return kittyInline(encoded, a.cellSize, a.cellSize)
}

// Clear removes cached data for an album.
func (a *AlbumArt) Clear(albumID string) {
	if id, ok := a.cache[albumID]; ok {
		// Tell terminal to delete the image.
		fmt.Printf("\x1b_Ga=d,d=i,i=%d;\x1b\\", id)
	}
	delete(a.cache, albumID)
	delete(a.imageData, albumID)
}

// ClearAll removes all cached art.
func (a *AlbumArt) ClearAll() {
	// Delete all Kitty images.
	if a.supported {
		fmt.Print("\x1b_Ga=d,d=a;\x1b\\")
	}
	a.cache = make(map[string]uint32)
	a.imageData = make(map[string]string)
	a.currentImgID = 0
}

// Placeholder returns a text-based placeholder when art isn't available.
func (a *AlbumArt) Placeholder() string {
	size := a.cellSize
	var lines []string

	lines = append(lines, "┌"+strings.Repeat("─", size-2)+"┐")
	for i := 0; i < size-2; i++ {
		if i == (size-2)/2 {
			pad := (size - 4) / 2
			lines = append(lines, "│"+strings.Repeat(" ", pad)+"♪♫"+strings.Repeat(" ", size-4-pad)+"│")
		} else {
			lines = append(lines, "│"+strings.Repeat(" ", size-2)+"│")
		}
	}
	lines = append(lines, "└"+strings.Repeat("─", size-2)+"┘")

	return strings.Join(lines, "\n")
}

// --- Kitty graphics protocol ---

// kittyTransmit sends image data to terminal without displaying.
func kittyTransmit(id uint32, b64data string) string {
	var sb strings.Builder
	chunkSize := 4096

	for i := 0; i < len(b64data); i += chunkSize {
		end := i + chunkSize
		if end > len(b64data) {
			end = len(b64data)
		}
		chunk := b64data[i:end]

		if i == 0 {
			more := 1
			if end >= len(b64data) {
				more = 0
			}
			// a=t: transmit only, f=100: PNG, i=id, m=more
			sb.WriteString(fmt.Sprintf("\x1b_Ga=t,f=100,i=%d,m=%d;%s\x1b\\", id, more, chunk))
		} else if end >= len(b64data) {
			sb.WriteString(fmt.Sprintf("\x1b_Gm=0;%s\x1b\\", chunk))
		} else {
			sb.WriteString(fmt.Sprintf("\x1b_Gm=1;%s\x1b\\", chunk))
		}
	}

	return sb.String()
}

// kittyInline renders an image inline at the cursor position.
func kittyInline(b64data string, cols, rows int) string {
	var sb strings.Builder
	chunkSize := 4096

	for i := 0; i < len(b64data); i += chunkSize {
		end := i + chunkSize
		if end > len(b64data) {
			end = len(b64data)
		}
		chunk := b64data[i:end]

		if i == 0 {
			more := 1
			if end >= len(b64data) {
				more = 0
			}
			// a=T: transmit+display, f=100: PNG, c=cols, r=rows
			sb.WriteString(fmt.Sprintf("\x1b_Ga=T,f=100,c=%d,r=%d,m=%d;%s\x1b\\", cols, rows, more, chunk))
		} else if end >= len(b64data) {
			sb.WriteString(fmt.Sprintf("\x1b_Gm=0;%s\x1b\\", chunk))
		} else {
			sb.WriteString(fmt.Sprintf("\x1b_Gm=1;%s\x1b\\", chunk))
		}
	}

	return sb.String()
}

// detectKittyGraphics checks if the terminal supports Kitty graphics protocol.
func detectKittyGraphics() bool {
	termProgram := os.Getenv("TERM_PROGRAM")
	switch strings.ToLower(termProgram) {
	case "ghostty", "wezterm":
		return true
	}

	term := os.Getenv("TERM")
	if strings.Contains(strings.ToLower(term), "kitty") {
		return true
	}

	if os.Getenv("KITTY_WINDOW_ID") != "" {
		return true
	}

	return false
}

// --- Image resizing ---

func resizeImage(src image.Image, width, height int) image.Image {
	dst := image.NewRGBA(image.Rect(0, 0, width, height))
	draw.CatmullRom.Scale(dst, dst.Bounds(), src, src.Bounds(), draw.Over, nil)
	return dst
}
