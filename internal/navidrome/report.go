package navidrome

import (
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/simonhull/kitsune/internal/store"
)

// notApplicable fills a cell that means nothing for its row.
const notApplicable = "-"

// row is one line of a person's table.
type row struct {
	label  string
	tally  *Tally
	path   bool // Matched by path is a way this kind matches.
	mbid   bool // And by MBID.
	counts *store.ImportCount
	kept   bool // Kept is a thing that can happen to it.
}

// Write prints plan: each person's table and notes, then the Navidrome users
// left out. written holds what [store.Store.ApplyImport] wrote for each
// person, in plan order; nil is a dry run, which prints no write columns.
// verbose lists the paths of the songs that matched nothing.
func Write(w io.Writer, plan *Plan, written []store.ImportCounts, verbose bool) error {
	var b strings.Builder
	for i := range plan.People {
		var counts *store.ImportCounts
		if written != nil {
			counts = &written[i]
		}
		writePerson(&b, &plan.People[i], counts, verbose)
	}
	unmapped := "none"
	if len(plan.Unmapped) > 0 {
		unmapped = strings.Join(plan.Unmapped, ", ") + " (map one with -user NAVIDROME=KITSUNE)"
	}
	fmt.Fprintf(&b, "Navidrome users with no Kitsune account: %s\n", unmapped)
	fmt.Fprintf(&b, "Matched by path: %d of %d songs with plays or marks\n", plan.Gate.ByPath, plan.Gate.Songs)
	if _, err := io.WriteString(w, b.String()); err != nil {
		return fmt.Errorf("write report: %w", err)
	}
	return nil
}

// writePerson prints one person's table and notes.
func writePerson(b *strings.Builder, p *Person, counts *store.ImportCounts, verbose bool) {
	r := &p.Report
	fmt.Fprintf(b, "Navidrome user %s → Kitsune user %s\n", p.From, p.To)
	header := []string{"source", "matched", "by path", "by MBID", "unmatched"}
	if counts != nil {
		header = append(header, "added", "already there", "kept")
	}
	writeCells(b, "", header)
	var c store.ImportCounts
	if counts != nil {
		c = *counts
	}
	dated, undated := c.Plays[SourceDated], c.Plays[SourceUndated]
	plays := store.ImportCount{Added: dated.Added + undated.Added, There: dated.There + undated.There}
	for _, rw := range []row{
		{label: "plays", tally: &r.Plays, path: true, mbid: true, counts: &plays},
		{label: "  dated", tally: &r.Dated, path: true, mbid: true, counts: &dated},
		{label: "  undated", tally: &r.Undated, path: true, mbid: true, counts: &undated},
		{label: "ratings", tally: &r.Ratings, path: true, mbid: true, counts: &c.Ratings, kept: true},
		{label: "unrated hearts", tally: &r.UnratedHearts, path: true, mbid: true, counts: &c.HeartRatings, kept: true},
		{label: "album hearts", tally: &r.AlbumHearts},
		{label: "artist hearts", tally: &r.ArtistHearts, mbid: true},
	} {
		cells := []string{
			strconv.Itoa(rw.tally.Source), strconv.Itoa(rw.tally.Matched()),
			cell(rw.path, rw.tally.ByPath), cell(rw.mbid, rw.tally.ByMBID), strconv.Itoa(rw.tally.Unmatched),
		}
		if counts != nil && rw.counts != nil {
			cells = append(cells, strconv.Itoa(rw.counts.Added), strconv.Itoa(rw.counts.There), cell(rw.kept, rw.counts.Kept))
		} else if counts != nil {
			cells = append(cells, notApplicable, notApplicable, notApplicable)
		}
		writeCells(b, rw.label, cells)
	}
	writeNotes(b, p, verbose)
	b.WriteByte('\n')
}

// writeNotes prints what the table can't say about a person's import.
func writeNotes(b *strings.Builder, p *Person, verbose bool) {
	r := &p.Report
	split := ""
	if r.SplitAlbums > 0 {
		split = fmt.Sprintf(" (%d lie in more than one Kitsune album)", r.SplitAlbums)
	}
	fmt.Fprintf(b, "Albums match through their songs%s; artists by name, then MBID.\n", split)
	if ratings := r.Ratings.Matched() - len(p.Import.Ratings); ratings > 0 {
		fmt.Fprintf(b, "Ratings of a track another Navidrome song also rates, as after a -map onto a kept copy, written once: %d\n", ratings)
	}
	fmt.Fprintf(b, "Starred songs with a rating keep just the rating: %d\n", r.RatedHearts)
	if p.HeartStars.Valid() {
		fmt.Fprintf(b, "Starred songs with no rating are rated %s (-heart-stars %d)\n", p.HeartStars, p.HeartStars.Halves())
	} else {
		fmt.Fprintf(b, "Starred songs with no rating stay unrated (-heart-stars 0)\n")
	}
	fmt.Fprintf(b, "Not imported, since Kitsune likes songs only, in stars: album hearts %d, artist hearts %d (%d of them rated, not starred)\n",
		r.AlbumHearts.Source, r.ArtistHearts.Source, r.ArtistRatings)
	fmt.Fprintf(b, "Not imported: album ratings %d, bookmarks %d, playlists %d (Kitsune has no playlists yet; keep the Navidrome file)\n",
		r.AlbumRatings, r.Left.Bookmarks, r.Left.Playlists)
	if r.NoDatePlays > 0 {
		fmt.Fprintf(b, "Not imported: %d plays with no time at all\n", r.NoDatePlays)
	}
	fmt.Fprintf(b, "Songs with plays or marks that matched nothing: %d\n", len(r.Unmatched))
	if verbose {
		for _, path := range r.Unmatched {
			fmt.Fprintf(b, "  %s\n", path)
		}
	}
}

// labelWidth is the width of the table's first column.
const labelWidth = 15

// cellWidths are the widths of its other columns, each its header and a gap.
var cellWidths = [...]int{8, 9, 9, 9, 11, 7, 15, 6} //nolint:gochecknoglobals // Read-only layout of the table.

// writeCells prints one table line: the label, then the cells right-aligned.
func writeCells(b *strings.Builder, label string, cells []string) {
	fmt.Fprintf(b, "  %-*s", labelWidth, label)
	for i, c := range cells {
		fmt.Fprintf(b, "%*s", cellWidths[min(i, len(cellWidths)-1)], c)
	}
	b.WriteByte('\n')
}

// cell is n, or notApplicable where n can't happen.
func cell(applies bool, n int) string {
	if !applies {
		return notApplicable
	}
	return strconv.Itoa(n)
}
