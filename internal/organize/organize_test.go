package organize_test

import (
	"encoding/json/v2"
	"errors"
	"os"
	"path"
	"slices"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/simonhull/kitsune/internal/organize"
)

// fixtures are the cases the web client's port is held to as well.
type fixtures struct {
	Parse []struct {
		Template string
		Error    *struct {
			At     int
			Reason string
		}
	}
	Paths []struct {
		Name, Template, Ext, Path, Error string
		Song                             song
	}
	Safe []struct{ In, Out string }
}

// song is a [organize.Song] as the fixtures write it.
type song struct {
	Title, Album, AlbumArtist, Genre, DiscName, Format string
	Artists                                            []string
	Year, OriginalYear, Disc, Discs, Track, DiscSongs  int
	Compilation                                        bool
}

func (s *song) organize() organize.Song {
	return organize.Song{
		Title: s.Title, Album: s.Album, AlbumArtist: s.AlbumArtist, Genre: s.Genre, DiscName: s.DiscName, Format: s.Format,
		Artists: s.Artists, Year: s.Year, OriginalYear: s.OriginalYear, Disc: s.Disc, Discs: s.Discs, Track: s.Track,
		DiscSongs: s.DiscSongs, Compilation: s.Compilation,
	}
}

func loadFixtures(t *testing.T) *fixtures {
	t.Helper()
	data, err := os.ReadFile("testdata/templates.json")
	if err != nil {
		t.Fatal(err)
	}
	var f fixtures
	if err = json.Unmarshal(data, &f); err != nil {
		t.Fatal(err)
	}
	return &f
}

func TestParseFixtures(t *testing.T) {
	t.Parallel()
	for _, tc := range loadFixtures(t).Parse {
		_, err := organize.Parse(tc.Template)
		if tc.Error == nil {
			if err != nil {
				t.Errorf("Parse(%q) = %v, want nil", tc.Template, err)
			}
			continue
		}
		te, ok := errors.AsType[*organize.TemplateError](err)
		if !ok {
			t.Errorf("Parse(%q) = %v, want a TemplateError", tc.Template, err)
			continue
		}
		if te.At != tc.Error.At || te.Reason != tc.Error.Reason {
			t.Errorf("Parse(%q) = at %d %q, want at %d %q", tc.Template, te.At, te.Reason, tc.Error.At, tc.Error.Reason)
		}
		if !errors.Is(err, organize.ErrTemplate) {
			t.Errorf("Parse(%q) error doesn't unwrap to ErrTemplate", tc.Template)
		}
	}
}

func TestPathFixtures(t *testing.T) {
	t.Parallel()
	for _, tc := range loadFixtures(t).Paths {
		tmpl, err := organize.Parse(tc.Template)
		if err != nil {
			t.Fatalf("%s: Parse = %v", tc.Name, err)
		}
		s := tc.Song.organize()
		got, err := tmpl.Path(&s, tc.Ext)
		switch {
		case tc.Error == "noTags":
			if !errors.Is(err, organize.ErrNoName) {
				t.Errorf("%s: Path = %q, %v; want ErrNoName", tc.Name, got, err)
			}
		case err != nil || got != tc.Path:
			t.Errorf("%s: Path = %q, %v; want %q", tc.Name, got, err, tc.Path)
		}
	}
}

func TestSafeFixtures(t *testing.T) {
	t.Parallel()
	for _, tc := range loadFixtures(t).Safe {
		if got := organize.Safe(tc.In); got != tc.Out {
			t.Errorf("Safe(%q) = %q, want %q", tc.In, got, tc.Out)
		}
	}
}

func TestSafeCutsLongNamesKeepingTheExtension(t *testing.T) {
	t.Parallel()
	// 59 three-byte characters and two one-byte ones: a cut at 180 lands in the middle of one.
	long := strings.Repeat("あ", 59) + "ab" + strings.Repeat("い", 20) + ".flac"
	got := organize.Safe(long)
	if len(got) > organize.MostSegmentBytes || !utf8.ValidString(got) || !strings.HasSuffix(got, ".flac") {
		t.Fatalf("Safe = %q (%d bytes), want at most %d valid bytes ending .flac", got, len(got), organize.MostSegmentBytes)
	}
	if want := strings.Repeat("あ", 58) + ".flac"; got != want { // 175 bytes for the stem: the cut falls inside the 59th.
		t.Errorf("Safe = %q, want %q", got, want)
	}
	folder := organize.Safe(strings.Repeat("é", 200))
	if len(folder) != organize.MostSegmentBytes || !utf8.ValidString(folder) {
		t.Errorf("Safe(200 é) = %d bytes, want %d", len(folder), organize.MostSegmentBytes)
	}
}

func TestPathCutsTitleThenAlbumPastTheLimit(t *testing.T) {
	t.Parallel()
	tmpl, err := organize.Parse("<Album Artist>/<Genre>/<Disc Name>/<Album>/<Track> - <Title>")
	if err != nil {
		t.Fatal(err)
	}
	s := organize.Song{
		AlbumArtist: strings.Repeat("a", 170), Genre: strings.Repeat("g", 170), DiscName: strings.Repeat("d", 170),
		Album: strings.Repeat("b", 170), Title: strings.Repeat("t", 400), Track: 1, Discs: 1, DiscSongs: 1,
	}
	got, err := tmpl.Path(&s, ".flac")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) > organize.MostPathBytes {
		t.Fatalf("len = %d, want at most %d", len(got), organize.MostPathBytes)
	}
	parts := strings.Split(got, "/")
	if parts[3] != strings.Repeat("b", 170) {
		t.Errorf("album = %q, want it whole: the title is cut first", parts[3])
	}
	s.Title = "t"
	s.Album = strings.Repeat("b", 400)
	s.Genre = strings.Repeat("g", 179)
	s.DiscName = strings.Repeat("d", 179)
	s.AlbumArtist = strings.Repeat("a", 179)
	got, err = tmpl.Path(&s, ".flac")
	if err != nil || len(got) > organize.MostPathBytes {
		t.Errorf("Path = %d bytes, %v; want the album cut to fit", len(got), err)
	}
}

func FuzzSafeIsIdempotent(f *testing.F) {
	for _, s := range []string{"", ".", "..", "CON.", "a/b", "  .x. ", "é́", strings.Repeat("あ", 70) + ".flac", "@ .", "COM1 .flac"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		once := organize.Safe(s)
		if twice := organize.Safe(once); twice != once {
			t.Errorf("Safe(Safe(%q)) = %q, want %q", s, twice, once)
		}
		if len(once) > organize.MostSegmentBytes || once == "" || strings.ContainsAny(once, `<>:"/\|?*`) ||
			strings.HasPrefix(once, ".") || strings.HasPrefix(once, "@") || strings.HasSuffix(once, ".") || strings.HasSuffix(once, " ") {
			t.Errorf("Safe(%q) = %q, which is not safe", s, once)
		}
	})
}

func TestSafeIsIdempotentOnEveryFixture(t *testing.T) {
	t.Parallel()
	fx := loadFixtures(t)
	for _, tc := range fx.Safe {
		if once := organize.Safe(tc.In); organize.Safe(once) != once {
			t.Errorf("Safe is not idempotent on %q", tc.In)
		}
	}
	for _, tc := range fx.Paths {
		for seg := range strings.SplitSeq(tc.Path, "/") {
			if seg != "" && organize.Safe(seg) != seg {
				t.Errorf("%s: segment %q is not a fixed point of Safe", tc.Name, seg)
			}
		}
	}
}

// defaults are the default templates, parsed.
func defaults(t *testing.T) organize.Templates {
	t.Helper()
	main, err := organize.Parse(organize.DefaultTemplate)
	if err != nil {
		t.Fatal(err)
	}
	comp, err := organize.Parse(organize.DefaultCompilations)
	if err != nil {
		t.Fatal(err)
	}
	return organize.Templates{Template: main, Compilations: comp}
}

// in is one song to plan, keyed by key, at p under "/music".
func in(key int64, p string, s *organize.Song) organize.Input {
	return organize.Input{Key: key, Root: "/music", Path: p, Song: *s}
}

// album is a one-disc album's song.
func album(artist, name string, year, track, songs int, title string) organize.Song {
	return organize.Song{AlbumArtist: artist, Artists: []string{artist}, Album: name, Year: year, Track: track, Disc: 1, Discs: 1, DiscSongs: songs, Title: title}
}

// TestDefaultOnTheOwnersShapes holds the default templates to the shapes the
// owner's library has, from plan 068.
func TestDefaultOnTheOwnersShapes(t *testing.T) {
	t.Parallel()
	fangs := album("Falling Up", "Fangs!", 2009, 5, 12, "Golden Arrows")
	church := album("David Crowder Band", "Church Music", 2009, 1, 17, "Phos Hilaron (Hail Gladdening Light)")
	braid := album("Braid", "Frame & Canvas", 1998, 3, 10, "Killing a Camera")
	archers := album("Archers", "Blood & Guts", 2015, 4, 11, "Lost")
	sleep := album("Sleep Token", "Even in Arcadia", 2025, 2, 10, "Emergence")
	giants := album("Kingdom Of Giants", "Edge of a Dream", 2026, 2, 10, "Respawn")
	mechina := album("Mechina", "Bellum Interruptum", 2025, 1, 11, "The World We Saved 2025")
	narnia := album("Harry Gregson-Williams", "The Chronicles of Narnia", 2005, 13, 34, "Only the Beginning of the Adventure")
	comp := album("Various Artists", "Now 45", 2001, 7, 20, "A Song")
	comp.Compilation = true
	songs := []organize.Input{
		in(1, "Falling Up/2009 - Fangs!/05 - Golden Arrows.flac", &fangs),
		in(2, "David Crowder Band/2009 - Church Music/01 Phos Hilaron (Hail Gladdening Light).flac", &church),
		in(3, "Braid/1998 - Frame & Canvas/03 Killing a Camera.flac", &braid),
		in(4, "Archers/2015 - Blood & Guts/04. Lost.flac", &archers),
		in(5, "Sleep Token/2025 - Even In Arcadia/01-02 Emergence.flac", &sleep),
		in(6, "Kingdom Of Giants/2026- Edge of a Dream/02 Respawn.flac", &giants),
		in(7, "Mechina/2025 - Bellum Interruptum/Mechina - Bellum Interruptum - 01 The World We Saved 2025.flac", &mechina),
		in(8, "Harry Gregson-Williams/13 Only the Beginning of the Adventure.flac", &narnia),
		in(9, "Compilations/Now 45/07 - A Song.flac", &comp),
	}
	want := []organize.Planned{
		{Kind: organize.InPlace, To: "Falling Up/2009 - Fangs!/05 - Golden Arrows.flac"},
		{Kind: organize.Rename, To: "David Crowder Band/2009 - Church Music/01 - Phos Hilaron (Hail Gladdening Light).flac"},
		{Kind: organize.Rename, To: "Braid/1998 - Frame & Canvas/03 - Killing a Camera.flac"},
		{Kind: organize.Rename, To: "Archers/2015 - Blood & Guts/04 - Lost.flac"},
		{Kind: organize.Move, To: "Sleep Token/2025 - Even in Arcadia/02 - Emergence.flac"},
		{Kind: organize.Move, To: "Kingdom Of Giants/2026 - Edge of a Dream/02 - Respawn.flac"},
		{Kind: organize.Rename, To: "Mechina/2025 - Bellum Interruptum/01 - The World We Saved 2025.flac"},
		{Kind: organize.Move, To: "Harry Gregson-Williams/2005 - The Chronicles of Narnia/13 - Only the Beginning of the Adventure.flac"},
		{Kind: organize.InPlace, To: "Compilations/Now 45/07 - A Song.flac"},
	}
	p := organize.MakePlan(songs, &organize.Options{Templates: defaults(t)})
	for i, w := range want {
		if got := p.Songs[i]; got.Kind != w.Kind || got.To != w.To || got.CaseOnly {
			t.Errorf("%s: got %v %q case %v, want %v %q", songs[i].Path, got.Kind, got.To, got.CaseOnly, w.Kind, w.To)
		}
	}
	if c, want := p.Counts, (organize.Counts{Songs: 9, InPlace: 2, Renames: 4, Moves: 3}); c != want || p.Folders != 7 {
		t.Errorf("counts = %+v in %d folders, want %+v in 7", c, p.Folders, want)
	}
}

func TestCaseOnly(t *testing.T) {
	t.Parallel()
	s := album("Sleep Token", "Even in Arcadia", 2025, 1, 10, "Look to Windward")
	p := organize.MakePlan([]organize.Input{in(1, "Sleep Token/2025 - Even In Arcadia/01 - Look to Windward.flac", &s)}, &organize.Options{Templates: defaults(t)})
	if got := p.Songs[0]; got.Kind != organize.Move || !got.CaseOnly || got.Kind == organize.Conflict {
		t.Errorf("got %+v, want a case-only move", got)
	}
	s = album("Sleep Token", "Even In Arcadia", 2025, 1, 10, "Look To Windward")
	p = organize.MakePlan([]organize.Input{in(1, "Sleep Token/2025 - Even In Arcadia/01 - Look to Windward.flac", &s)}, &organize.Options{
		Templates: defaults(t),
		List: func(_, dir string) []organize.Entry {
			if dir == "Sleep Token/2025 - Even In Arcadia" {
				return []organize.Entry{{Name: "01 - Look to Windward.flac"}}
			}
			return nil
		},
	})
	if got := p.Songs[0]; got.Kind != organize.Rename || !got.CaseOnly {
		t.Errorf("got %+v, want a case-only rename, never a conflict with its own file", got)
	}
}

func TestConflicts(t *testing.T) {
	t.Parallel()
	plain := album("Celldweller", "Wish Upon a Blackstar", 2012, 12, 16, "The Best It's Gonna Get")
	deluxe := plain
	twinA := album("Band", "Same", 2000, 1, 2, "Song")
	twinB := twinA
	twinB.Title = "SONG" // The same path to a case-insensitive filesystem.
	unknown := album("Other", "Album", 2001, 1, 1, "Taken")
	notOurs := album("Elsewhere", "Somewhere", 2002, 1, 1, "Blocked")
	songs := []organize.Input{
		in(1, "Celldweller/2012 - Wish Upon a Blackstar/12 - The Best It's Gonna Get.flac", &plain),
		in(2, "Celldweller/2012 - Wish Upon a Blackstar (Deluxe)/12 - The Best It's Gonna Get.flac", &deluxe),
		in(3, "Band/x/a.flac", &twinA),
		in(4, "Band/y/b.flac", &twinB),
		in(5, "Other/old.flac", &unknown),
		in(6, "Elsewhere/old.flac", &notOurs),
	}
	p := organize.MakePlan(songs, &organize.Options{
		Templates: defaults(t),
		Others:    []organize.Item{{Key: 99, Root: "/music", Path: "Elsewhere/2002 - Somewhere/01 - BLOCKED.flac"}},
		List: func(_, dir string) []organize.Entry {
			if dir == "Other/2001 - Album" {
				return []organize.Entry{{Name: "01 - Taken.flac.bak"}, {Name: "01 - taken.FLAC"}}
			}
			return nil
		},
	})
	want := []organize.Planned{
		{Kind: organize.InPlace}, // The song already there.
		{Kind: organize.Conflict, Reason: organize.Claimed, With: []int64{1}}, // The deluxe twin.
		{Kind: organize.Conflict, Reason: organize.Claimed, With: []int64{4}}, // Twins by case,
		{Kind: organize.Conflict, Reason: organize.Claimed, With: []int64{3}}, // each in the other's way.
		{Kind: organize.Conflict, Reason: organize.UnknownFile, Unknown: "01 - taken.FLAC"},
		{Kind: organize.Conflict, Reason: organize.Claimed, With: []int64{99}}, // A song not planned.
	}
	for i, w := range want {
		got := p.Songs[i]
		if got.Kind != w.Kind || got.Reason != w.Reason || !slices.Equal(got.With, w.With) || got.Unknown != w.Unknown {
			t.Errorf("song %d = %+v, want %+v", i, got, w)
		}
	}
	if p.Counts.Conflicts != 5 || p.Counts.InPlace != 1 {
		t.Errorf("counts = %+v", p.Counts)
	}
}

func TestRootsAndCompilationsTakeTheirTemplates(t *testing.T) {
	t.Parallel()
	other, err := organize.Parse("<Album>/<Title>")
	if err != nil {
		t.Fatal(err)
	}
	comp, err := organize.Parse("VA/<Album>/<Title>")
	if err != nil {
		t.Fatal(err)
	}
	const books = "/books"
	s := album("A", "B", 2000, 1, 1, "C")
	c := s
	c.Compilation = true
	p := organize.MakePlan([]organize.Input{
		in(1, "x.flac", &s),
		{Key: 2, Root: books, Path: "x.flac", Song: s},
		{Key: 3, Root: books, Path: "y.flac", Song: c},
	}, &organize.Options{Templates: defaults(t), Roots: map[string]organize.Templates{books: {Template: other, Compilations: comp}}})
	for i, want := range []string{"A/2000 - B/01 - C.flac", "B/C.flac", "VA/B/C.flac"} {
		if got := p.Songs[i].To; got != want {
			t.Errorf("song %d = %q, want %q", i, got, want)
		}
	}
}

func TestCompanions(t *testing.T) {
	t.Parallel()
	a := album("Falling Up", "Fangs!", 2009, 1, 2, "One")
	b := album("Falling Up", "Fangs!", 2009, 2, 2, "Two")
	lone := album("Solo", "Kept", 2001, 1, 2, "Moves")
	stays := album("Solo", "Kept", 2001, 2, 2, "Stays")
	listing := map[string][]organize.Entry{
		"old fangs": {
			{Name: "1 One.flac"},
			{Name: "2 Two.flac"},
			{Name: "cover.jpg"},
			{Name: "Fangs!.cue"},
			{Name: "rip.log"},
			{Name: "1 One.lrc"},
			{Name: "desktop.ini"},
			{Name: "@eaDir", Dir: true},
			{Name: "notes.txt"},
		},
		"Falling Up/2009 - Fangs!": {{Name: "notes.txt"}},
		"Solo/2001 - Kept":         {{Name: "02 - Stays.flac"}, {Name: "cover.jpg"}, {Name: "01 Moves.lrc"}, {Name: "01 Moves.flac"}},
	}
	p := organize.MakePlan([]organize.Input{
		in(1, "old fangs/1 One.flac", &a), in(2, "old fangs/2 Two.flac", &b),
		in(3, "Solo/2001 - Kept/01 Moves.flac", &lone), in(4, "Solo/2001 - Kept/02 - Stays.flac", &stays),
	}, &organize.Options{
		Templates: defaults(t), Companions: true,
		List: func(_, dir string) []organize.Entry { return listing[dir] },
	})
	got := make([]string, 0, len(p.Companions))
	for _, c := range p.Companions {
		got = append(got, c.From+" → "+c.To)
	}
	slices.Sort(got)
	want := []string{
		"Solo/2001 - Kept/01 Moves.lrc → Solo/2001 - Kept/01 - Moves.lrc",
		"old fangs/1 One.lrc → Falling Up/2009 - Fangs!/01 - One.lrc",
		"old fangs/Fangs!.cue → Falling Up/2009 - Fangs!/Fangs!.cue",
		"old fangs/cover.jpg → Falling Up/2009 - Fangs!/cover.jpg",
		"old fangs/rip.log → Falling Up/2009 - Fangs!/rip.log",
	}
	if !slices.Equal(got, want) {
		t.Errorf("companions =\n%s\nwant\n%s\n(notes.txt is taken at the target; a folder a song stays in keeps its cover)",
			strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	p = organize.MakePlan([]organize.Input{in(1, "old fangs/1 One.flac", &a), in(2, "old fangs/2 Two.flac", &b)}, &organize.Options{
		Templates: defaults(t), List: func(_, dir string) []organize.Entry { return listing[dir] },
	})
	if len(p.Companions) != 0 {
		t.Errorf("companions without the option = %v", p.Companions)
	}
}

func TestSkipped(t *testing.T) {
	t.Parallel()
	tmpl, err := organize.Parse("<Album>/<Track>")
	if err != nil {
		t.Fatal(err)
	}
	s := organize.Song{Title: "No track", Album: "A"}
	p := organize.MakePlan([]organize.Input{in(1, "a.flac", &s)}, &organize.Options{Template: tmpl, Compilations: tmpl})
	if got := p.Songs[0]; got.Kind != organize.Skipped || got.Reason != organize.NoTags || p.Counts.Skipped != 1 {
		t.Errorf("got %+v, want skipped for no tags", got)
	}
}

func TestKindNames(t *testing.T) {
	t.Parallel()
	for k, want := range map[organize.Kind]string{
		organize.InPlace: "inPlace", organize.Rename: "rename", organize.Move: "move", organize.Conflict: "conflict", organize.Skipped: "skipped",
	} {
		if k.String() != want {
			t.Errorf("%d = %q, want %q", k, k.String(), want)
		}
	}
	if !organize.IsCompanion("Cover.JPG") || organize.IsCompanion("desktop.ini") || organize.IsCompanion(".cover.jpg") ||
		organize.IsCompanion(path.Base("a/b.flac")) {
		t.Error("IsCompanion is wrong")
	}
}
