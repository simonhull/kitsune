package subsonic

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

const (
	apiVersion = "1.16.1"
	clientName = "kitsune"
)

// Client talks to a Subsonic-compatible server (Navidrome, etc.).
type Client struct {
	baseURL  string
	user     string
	password string
	http     *http.Client
}

// NewClient creates a Subsonic API client.
func NewClient(baseURL, user, password string) *Client {
	return &Client{
		baseURL:  baseURL,
		user:     user,
		password: password,
		http:     &http.Client{Timeout: 30 * time.Second},
	}
}

// StreamURL returns the URL to stream a track by ID.
func (c *Client) StreamURL(id string) string {
	return c.buildURL("stream", url.Values{"id": {id}})
}

// CoverArtURL returns the URL for cover art by ID.
func (c *Client) CoverArtURL(id string, size int) string {
	params := url.Values{"id": {id}}
	if size > 0 {
		params.Set("size", fmt.Sprintf("%d", size))
	}
	return c.buildURL("getCoverArt", params)
}

// Ping tests the connection and authentication.
func (c *Client) Ping() error {
	var resp pingResponse
	if err := c.get("ping", nil, &resp); err != nil {
		return err
	}
	if resp.Response.Status != "ok" {
		return fmt.Errorf("ping failed: %s", resp.Response.Status)
	}
	return nil
}

// GetArtists returns all artists from the library, indexed alphabetically.
func (c *Client) GetArtists() ([]Artist, error) {
	var resp artistsResponse
	if err := c.get("getArtists", nil, &resp); err != nil {
		return nil, fmt.Errorf("getArtists: %w", err)
	}
	if resp.Response.Status != "ok" {
		return nil, apiErr(resp.Response.Error)
	}

	var artists []Artist
	for _, idx := range resp.Response.Artists.Index {
		artists = append(artists, idx.Artist...)
	}
	return artists, nil
}

// GetArtist returns an artist and their albums.
func (c *Client) GetArtist(id string) (*ArtistDetail, error) {
	var resp artistResponse
	if err := c.get("getArtist", url.Values{"id": {id}}, &resp); err != nil {
		return nil, fmt.Errorf("getArtist(%s): %w", id, err)
	}
	if resp.Response.Status != "ok" {
		return nil, apiErr(resp.Response.Error)
	}
	return &resp.Response.Artist, nil
}

// GetAlbum returns an album and its tracks.
func (c *Client) GetAlbum(id string) (*AlbumDetail, error) {
	var resp albumResponse
	if err := c.get("getAlbum", url.Values{"id": {id}}, &resp); err != nil {
		return nil, fmt.Errorf("getAlbum(%s): %w", id, err)
	}
	if resp.Response.Status != "ok" {
		return nil, apiErr(resp.Response.Error)
	}
	return &resp.Response.Album, nil
}

// --- HTTP plumbing ---

func (c *Client) buildURL(endpoint string, params url.Values) string {
	if params == nil {
		params = url.Values{}
	}
	params.Set("u", c.user)
	params.Set("p", c.password)
	params.Set("v", apiVersion)
	params.Set("c", clientName)
	params.Set("f", "json")
	return fmt.Sprintf("%s/rest/%s.view?%s", c.baseURL, endpoint, params.Encode())
}

func (c *Client) get(endpoint string, params url.Values, dest any) error {
	resp, err := c.http.Get(c.buildURL(endpoint, params))
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("reading response: %w", err)
	}

	return json.Unmarshal(body, dest)
}

func apiErr(e *APIError) error {
	if e == nil {
		return fmt.Errorf("unknown API error")
	}
	return fmt.Errorf("subsonic error %d: %s", e.Code, e.Message)
}

// --- API response types ---

type APIError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type Artist struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	AlbumCount int    `json:"albumCount"`
	CoverArt   string `json:"coverArt"`
}

type ArtistDetail struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Album []Album `json:"album"`
}

type Album struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Artist    string `json:"artist"`
	ArtistID  string `json:"artistId"`
	CoverArt  string `json:"coverArt"`
	SongCount int    `json:"songCount"`
	Duration  int    `json:"duration"` // seconds
	Year      int    `json:"year"`
	Genre     string `json:"genre"`
}

type AlbumDetail struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Artist    string `json:"artist"`
	ArtistID  string `json:"artistId"`
	CoverArt  string `json:"coverArt"`
	SongCount int    `json:"songCount"`
	Year      int    `json:"year"`
	Song      []Song `json:"song"`
}

type Song struct {
	ID         string `json:"id"`
	Title      string `json:"title"`
	Album      string `json:"album"`
	Artist     string `json:"artist"`
	AlbumID    string `json:"albumId"`
	ArtistID   string `json:"artistId"`
	TrackNum   int    `json:"track"`
	DiscNum    int    `json:"discNumber"`
	Year       int    `json:"year"`
	Genre      string `json:"genre"`
	Duration   int    `json:"duration"` // seconds
	BitRate    int    `json:"bitRate"`
	Suffix     string `json:"suffix"` // file extension (mp3, flac, etc.)
	CoverArt   string `json:"coverArt"`
}

// --- JSON response envelopes ---

type baseResponse struct {
	Status string   `json:"status"`
	Error  *APIError `json:"error,omitempty"`
}

type pingResponse struct {
	Response baseResponse `json:"subsonic-response"`
}

type artistsResponse struct {
	Response struct {
		baseResponse
		Artists struct {
			Index []struct {
				Name   string   `json:"name"`
				Artist []Artist `json:"artist"`
			} `json:"index"`
		} `json:"artists"`
	} `json:"subsonic-response"`
}

type artistResponse struct {
	Response struct {
		baseResponse
		Artist ArtistDetail `json:"artist"`
	} `json:"subsonic-response"`
}

type albumResponse struct {
	Response struct {
		baseResponse
		Album AlbumDetail `json:"album"`
	} `json:"subsonic-response"`
}
