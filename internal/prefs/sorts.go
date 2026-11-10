package prefs

import (
	"encoding/json/v2"
	"errors"
	"fmt"
	"slices"
)

// Direction is which way an order runs.
type Direction string

// The two directions.
const (
	Ascending  Direction = "asc"
	Descending Direction = "desc"
)

// Sort is one order and the way it runs: a list's choice, or one part of it.
type Sort struct {
	By  string    `json:"by"`
	Dir Direction `json:"dir"`
}

// LibrarySortValue is the value of [LibrarySort]: how the Library orders its
// artists, and each artist's albums. Artist pages take the albums' order too.
type LibrarySortValue struct {
	Artists Sort `json:"artists"`
	Albums  Sort `json:"albums"`
}

// ArtistSongsValue is the value of [ArtistSongs]: how an artist page's full song
// list groups and orders its songs. By album, the albums go by [LibrarySort]'s.
type ArtistSongsValue struct {
	Group Grouping `json:"group"`
	Sort  Sort     `json:"sort"` // The flat list's order.
}

// Grouping is whether a song list runs flat or by album.
type Grouping string

// The two groupings.
const (
	Flat    Grouping = "flat"
	ByAlbum Grouping = "album"
)

// The orders a list may name in [Sort.By]. A client words them; the server only checks.
const (
	OrderName   = "name"
	OrderArtist = "artist"
	OrderYear   = "year"
	OrderAdded  = "added"
	OrderPlays  = "plays"
	OrderRating = "rating"
	OrderAlbums = "albums"
	OrderTop    = "top"
	OrderTitle  = "title"
)

// The orders each list offers.
//
//nolint:gochecknoglobals // Read-only tables of what each sort may name.
var (
	libraryArtistOrders = []string{OrderName, OrderAdded, OrderPlays}
	libraryAlbumOrders  = []string{OrderYear, OrderName, OrderAdded}
	albumsOrders        = []string{OrderName, OrderArtist, OrderYear, OrderAdded, OrderPlays, OrderRating}
	artistsOrders       = []string{OrderName, OrderAlbums, OrderAdded, OrderPlays}
	artistSongOrders    = []string{OrderTop, OrderPlays, OrderRating, OrderTitle, OrderYear, OrderAdded}
)

var (
	errOrder     = errors.New("no such order")
	errDirection = errors.New("dir is not asc or desc")
	errSortParts = errors.New("by and dir are both required")
	errSortHalf  = errors.New("artists and albums are both required")
	errGrouping  = errors.New("group is not flat or album")
	errSongsHalf = errors.New("group and sort are both required")
)

// sortIn is a [Sort] as it arrives, so a missing member can be told from an empty one.
type sortIn struct {
	By  *string    `json:"by"`
	Dir *Direction `json:"dir"`
}

func (in *sortIn) check(orders []string) (Sort, error) {
	if in == nil || in.By == nil || in.Dir == nil {
		return Sort{}, errSortParts
	}
	if !slices.Contains(orders, *in.By) {
		return Sort{}, fmt.Errorf("%q: %w", *in.By, errOrder)
	}
	if *in.Dir != Ascending && *in.Dir != Descending {
		return Sort{}, errDirection
	}
	return Sort{By: *in.By, Dir: *in.Dir}, nil
}

// checkSortOf returns the check of a list's [Sort], one of orders.
func checkSortOf(orders []string) func([]byte) ([]byte, error) {
	return func(value []byte) ([]byte, error) {
		var in sortIn
		if err := json.Unmarshal(value, &in, json.RejectUnknownMembers(true)); err != nil {
			return nil, fmt.Errorf("decode: %w", err)
		}
		out, err := in.check(orders)
		if err != nil {
			return nil, err
		}
		return encode(out)
	}
}

// checkLibrarySort accepts a [LibrarySortValue].
func checkLibrarySort(value []byte) ([]byte, error) {
	var in struct {
		Artists *sortIn `json:"artists"`
		Albums  *sortIn `json:"albums"`
	}
	if err := json.Unmarshal(value, &in, json.RejectUnknownMembers(true)); err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}
	if in.Artists == nil || in.Albums == nil {
		return nil, errSortHalf
	}
	artists, err := in.Artists.check(libraryArtistOrders)
	if err != nil {
		return nil, fmt.Errorf("artists: %w", err)
	}
	albums, err := in.Albums.check(libraryAlbumOrders)
	if err != nil {
		return nil, fmt.Errorf("albums: %w", err)
	}
	return encode(LibrarySortValue{Artists: artists, Albums: albums})
}

// checkArtistSongs accepts an [ArtistSongsValue].
func checkArtistSongs(value []byte) ([]byte, error) {
	var in struct {
		Group *Grouping `json:"group"`
		Sort  *sortIn   `json:"sort"`
	}
	if err := json.Unmarshal(value, &in, json.RejectUnknownMembers(true)); err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}
	if in.Group == nil || in.Sort == nil {
		return nil, errSongsHalf
	}
	if *in.Group != Flat && *in.Group != ByAlbum {
		return nil, errGrouping
	}
	sort, err := in.Sort.check(artistSongOrders)
	if err != nil {
		return nil, fmt.Errorf("sort: %w", err)
	}
	return encode(ArtistSongsValue{Group: *in.Group, Sort: sort})
}

func encode(v any) ([]byte, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("encode: %w", err)
	}
	return b, nil
}
