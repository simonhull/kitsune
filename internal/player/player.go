package player

import (
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gopxl/beep/v2"
	"github.com/gopxl/beep/v2/flac"
	"github.com/gopxl/beep/v2/mp3"
	"github.com/gopxl/beep/v2/speaker"
)

const sampleRate = beep.SampleRate(44100)

// NowPlaying holds info about the currently playing track.
type NowPlaying struct {
	TrackID    string
	Title      string
	Artist     string
	Album      string
	AlbumID    string
	Year       int
	DurationMs int
	Format     string
}

// Player streams and plays audio from a Subsonic server.
type Player struct {
	mu       sync.Mutex
	logger   *slog.Logger
	current  *NowPlaying
	ctrl     *beep.Ctrl
	streamer beep.StreamSeekCloser
	body     io.ReadCloser // HTTP response body
	tracker  *positionTracker
	playing  bool
	done     chan struct{} // signals track ended
}

// New creates a Player and initializes the audio speaker.
func New(logger *slog.Logger) (*Player, error) {
	if logger == nil {
		logger = slog.Default()
	}

	err := speaker.Init(sampleRate, sampleRate.N(time.Second/10))
	if err != nil {
		return nil, fmt.Errorf("initializing speaker: %w", err)
	}

	return &Player{
		logger: logger.With("component", "player"),
		done:   make(chan struct{}, 1),
	}, nil
}

// Play streams and plays a track from the given URL.
func (p *Player) Play(streamURL string, format string, info NowPlaying) error {
	p.Stop()

	p.logger.Info("playing", "title", info.Title, "artist", info.Artist, "format", format)

	// Open HTTP stream.
	resp, err := http.Get(streamURL)
	if err != nil {
		return fmt.Errorf("streaming %s: %w", info.Title, err)
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return fmt.Errorf("stream returned %d", resp.StatusCode)
	}

	// Decode based on format.
	streamer, streamFormat, err := decode(resp.Body, format)
	if err != nil {
		resp.Body.Close()
		return fmt.Errorf("decoding %s (%s): %w", info.Title, format, err)
	}

	// Resample to speaker rate if needed.
	var source beep.Streamer
	if streamFormat.SampleRate != sampleRate {
		source = beep.Resample(4, streamFormat.SampleRate, sampleRate, streamer)
	} else {
		source = streamer
	}

	// Wrap in position tracker.
	tracker := &positionTracker{Streamer: source}

	// Wrap in ctrl for pause/resume.
	ctrl := &beep.Ctrl{Streamer: tracker, Paused: false}

	p.mu.Lock()
	p.current = &info
	p.ctrl = ctrl
	p.streamer = streamer
	p.body = resp.Body
	p.tracker = tracker
	p.playing = true

	// Drain the done channel in case of a leftover signal.
	select {
	case <-p.done:
	default:
	}
	p.mu.Unlock()

	// Play with a callback when the track ends.
	speaker.Play(beep.Seq(ctrl, beep.Callback(func() {
		p.mu.Lock()
		p.playing = false
		p.mu.Unlock()

		select {
		case p.done <- struct{}{}:
		default:
		}
	})))

	return nil
}

// Stop stops the current track.
func (p *Player) Stop() {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.ctrl != nil {
		speaker.Clear()
	}
	p.cleanup()
}

// TogglePause pauses if playing, resumes if paused.
func (p *Player) TogglePause() {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.ctrl == nil {
		return
	}

	speaker.Lock()
	p.ctrl.Paused = !p.ctrl.Paused
	p.playing = !p.ctrl.Paused
	speaker.Unlock()
}

// IsPlaying reports whether audio is currently playing (not paused).
func (p *Player) IsPlaying() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.playing
}

// Current returns info about the current track, or nil.
func (p *Player) Current() *NowPlaying {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.current
}

// Elapsed returns how many seconds of audio have played.
func (p *Player) Elapsed() float64 {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.tracker == nil {
		return 0
	}

	speaker.Lock()
	pos := p.tracker.pos
	speaker.Unlock()

	return float64(pos) / float64(sampleRate)
}

// Done returns a channel that signals when the current track ends.
func (p *Player) Done() <-chan struct{} {
	return p.done
}

// cleanup releases resources. Must be called with mu held.
func (p *Player) cleanup() {
	if p.streamer != nil {
		p.streamer.Close()
		p.streamer = nil
	}
	if p.body != nil {
		p.body.Close()
		p.body = nil
	}
	p.ctrl = nil
	p.current = nil
	p.tracker = nil
	p.playing = false
}

// --- Decoding ---

func decode(r io.ReadCloser, format string) (beep.StreamSeekCloser, beep.Format, error) {
	switch strings.ToLower(format) {
	case "mp3":
		return mp3.Decode(r)
	case "flac":
		return flac.Decode(r)
	default:
		// For unknown formats (m4a, etc.), try MP3 (assumes server transcodes).
		return mp3.Decode(r)
	}
}

// --- Position tracking ---

type positionTracker struct {
	beep.Streamer
	pos int
}

func (p *positionTracker) Stream(samples [][2]float64) (int, bool) {
	n, ok := p.Streamer.Stream(samples)
	p.pos += n
	return n, ok
}
