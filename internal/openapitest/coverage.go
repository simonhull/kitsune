package openapitest

import (
	"net/http"
	"slices"
	"strconv"
	"strings"

	"github.com/pb33f/libopenapi-validator/config"
	"github.com/pb33f/libopenapi-validator/paths"
)

// Unexercised lists every response the contract declares that no validated
// response has matched yet, as "METHOD /path status", sorted. A package's
// TestMain calls it after a full run so that each declared response is
// exercised by at least one test.
func (c *Contract) Unexercised() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	var missing []string
	for path, item := range c.model.Paths.PathItems.FromOldest() {
		for method, op := range item.GetOperations().FromOldest() {
			for code := range op.Responses.Codes.KeysFromOldest() {
				if key := responseKey(strings.ToUpper(method), path, code); !c.seen[key] {
					missing = append(missing, key)
				}
			}
		}
	}
	slices.Sort(missing)
	return missing
}

// record marks the response to req with status as exercised. Responses to
// paths outside the contract are ignored: the validator reports those.
func (c *Contract) record(req *http.Request, status int) {
	_, _, path := paths.FindPath(req, c.model, config.NewValidationOptions())
	if path == "" {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.seen[responseKey(req.Method, path, strconv.Itoa(status))] = true
}

// responseKey names one declared response, as Unexercised reports it.
func responseKey(method, path, code string) string {
	return method + " " + path + " " + code
}
