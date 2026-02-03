package subsonic

import (
	"fmt"
	"io"
)

// GetCoverArt fetches cover art bytes for the given ID.
// Size is the desired dimension in pixels (square). Use 0 for original size.
func (c *Client) GetCoverArt(id string, size int) ([]byte, error) {
	artURL := c.CoverArtURL(id, size)

	resp, err := c.http.Get(artURL)
	if err != nil {
		return nil, fmt.Errorf("fetching cover art: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("cover art returned status %d", resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading cover art: %w", err)
	}

	return data, nil
}
