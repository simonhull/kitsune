package matching_test

import (
	"fmt"
	"time"

	"github.com/simonhull/kitsune/internal/matching"
	"github.com/simonhull/kitsune/internal/matching/musicbrainztest"
)

// The board's album: Glasshouse by Low Tide Choir, two discs of 9 and 8 songs.
var glasshouseSongs = []struct { //nolint:gochecknoglobals // A fixed fixture, never written.
	title  string
	length time.Duration
}{
	{"Breakwater", 238 * time.Second},
	{"Salt in the Lungs", 271 * time.Second},
	{"Greenhouse Effect", 192 * time.Second},
	{"Condensation", 227 * time.Second},
	{"Small Weather", 306 * time.Second},
	{"Fogbound", 242 * time.Second},
	{"Panes", 164 * time.Second},
	{"Low Tide", 362 * time.Second},
	{"Ribbons", 213 * time.Second},
	{"Breakwater (Room Version)", 252 * time.Second},
	{"Fogbound (Room Version)", 262 * time.Second},
	{"Kelp & Glass", 201 * time.Second},
	{"Panes (Room Version)", 177 * time.Second},
	{"Harbor Lights (Intro)", 51 * time.Second},
	{"Harbor Wall", 224 * time.Second},
	{"Neap", 220 * time.Second},
	{"The Long Drain", 432 * time.Second},
}

const (
	glasshouse   = "Glasshouse"
	lowTide      = "Low Tide Choir"
	discOne      = 9
	released     = "2021-03-12"
	roomSessions = "The Room Sessions"
)

// glasshouseAlbum is the album as Kitsune holds it: the files call the room
// versions "(Room)", and Kelp and Panes are swapped on disc 2.
func glasshouseAlbum() *matching.Album {
	album := &matching.Album{Title: glasshouse, Artists: []string{lowTide}}
	for i, s := range glasshouseSongs {
		disc, number := 1, i+1
		if i >= discOne {
			disc, number = 2, i-discOne+1
		}
		album.Tracks = append(album.Tracks, matching.Track{
			ID: fmt.Sprintf("01JTRACK%018d", i), Disc: disc, Number: number, Title: s.title, Length: s.length + 400*time.Millisecond,
		})
	}
	album.Tracks[11], album.Tracks[12] = album.Tracks[12], album.Tracks[11]
	album.Tracks[11].Number, album.Tracks[12].Number = 3, 4
	return album
}

// release builds a release of the songs, split onto media of the given sizes, in format.
func release(name, title, artist, format string, sizes []int, songs int) matching.Release {
	r := matching.Release{
		Source: matching.SourceMusicBrainz, ID: musicbrainztest.ID(name), GroupID: musicbrainztest.ID("group " + title),
		Title: title, Artists: []matching.Credit{{Name: artist, ID: musicbrainztest.ID("artist " + artist)}},
		Date: released, Original: released, Country: "GB", Barcode: "5060000000417",
		Labels: []matching.Label{{Name: "Saltmarsh Records", CatalogNumber: "SALT 041"}},
		Genres: []string{"dream pop", "ambient pop"}, Cover: true, Status: "Official",
	}
	n := 0
	for i, size := range sizes {
		m := matching.Medium{Position: i + 1, Format: format, TrackCount: size}
		if i == 1 {
			m.Title = roomSessions
		}
		for j := range size {
			if n >= songs {
				break
			}
			s := glasshouseSongs[n%len(glasshouseSongs)]
			song := s.title
			if n >= len(glasshouseSongs) {
				song = fmt.Sprintf("Bonus %d", n-len(glasshouseSongs)+1)
			}
			m.Tracks = append(m.Tracks, matching.ReleaseTrack{
				ID: musicbrainztest.ID(fmt.Sprintf("%s track %d", name, n)), RecordingID: musicbrainztest.ID(fmt.Sprintf("recording %d", n)),
				Position: j + 1, Title: song, Artists: []matching.Credit{{Name: artist, ID: musicbrainztest.ID("artist " + artist)}},
				Length: s.length, ISRCs: []string{fmt.Sprintf("GBSLT21%05d", n)},
			})
			n++
		}
		out := m
		out.TrackCount = len(m.Tracks)
		r.Media = append(r.Media, out)
	}
	return r
}

// The board's four releases.
func glasshouseCD() matching.Release {
	return release("cd", glasshouse, lowTide, "CD", []int{9, 8}, 17)
}

func glasshouseDeluxe() matching.Release {
	r := release("deluxe", "Glasshouse (Deluxe Edition)", lowTide, "Digital Media", []int{9, 12}, 21)
	r.Date = "2022-05-06"
	return r
}

func glasshouseVinyl() matching.Release {
	r := release("vinyl", glasshouse, lowTide, `12" Vinyl`, []int{5, 4, 4, 4}, 17)
	// Two songs run long on vinyl.
	r.Media[0].Tracks[1].Length += 20 * time.Second
	r.Media[1].Tracks[0].Length += 9 * time.Second
	return r
}

func glassHouseChorus() matching.Release {
	r := release("chorus", "Glass House", "Low Tide Chorus", "CD", []int{11}, 11)
	for i := range r.Media[0].Tracks {
		if i%4 != 0 {
			r.Media[0].Tracks[i].Length += time.Minute
		}
	}
	return r
}
