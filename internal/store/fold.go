package store

import "strings"

// searchFolder replaces the letters that the FTS tokenizer's diacritic removal
// leaves alone. The tokenizer handles combining marks; this handles the rest.
var searchFolder = strings.NewReplacer( //nolint:gochecknoglobals // Immutable after init; strings.Replacer is safe for concurrent use.
	"ø", "o", "Ø", "O",
	"ł", "l", "Ł", "L",
	"đ", "d", "Đ", "D", "ð", "d", "Ð", "D",
	"ħ", "h", "Ħ", "H",
	"ı", "i",
	"æ", "ae", "Æ", "AE",
	"œ", "oe", "Œ", "OE",
	"ß", "ss", "ẞ", "SS",
	"þ", "th", "Þ", "TH",
)

// foldSearch prepares text for the tracks_fts table, and search queries must
// fold the same way. It returns s unchanged when it holds none of the letters.
func foldSearch(s string) string { return searchFolder.Replace(s) }
