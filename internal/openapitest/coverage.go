package openapitest

import (
	"flag"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strconv"
	"strings"

	"github.com/pb33f/libopenapi-validator/paths"
)

// exitUnexercised is the exit status of a test binary whose tests passed but
// left a declared response unexercised.
const exitUnexercised = 1

// CheckExercised is the coverage gate for a package's TestMain. Given the
// exit status of m.Run, it returns the status to exit with: code itself,
// unless the tests passed in a full run but left declared responses
// unexercised, which it lists on w. Runs narrowed by -run or -skip, and
// -list, skip the check.
//
//	func TestMain(m *testing.M) {
//		...
//		os.Exit(contract.CheckExercised(os.Stderr, m.Run()))
//	}
func (c *Contract) CheckExercised(w io.Writer, code int) int {
	return c.checkExercised(w, code, partialRun())
}

func (c *Contract) checkExercised(w io.Writer, code int, partial bool) int {
	if code != 0 || partial {
		return code
	}
	missing := c.Unexercised()
	if len(missing) == 0 {
		return code
	}
	// Nothing useful can be done if the report itself fails.
	_, _ = fmt.Fprintf(w, "responses declared in the contract but never exercised:\n  %s\n",
		strings.Join(missing, "\n  "))
	return exitUnexercised
}

// partialRun reports whether the test binary was asked to run only some of
// its tests, or none.
func partialRun() bool {
	for _, name := range []string{"test.run", "test.skip", "test.list"} {
		if f := flag.Lookup(name); f != nil && f.Value.String() != "" {
			return true
		}
	}
	return false
}

// Unexercised lists every response the contract declares that no valid
// response has matched yet, as "METHOD /path status", sorted.
//
// Status keys are the declared codes verbatim, so a range ("2XX") or default
// response never counts as exercised; the contract uses neither yet.
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

// record marks the valid response to req with status as exercised.
// Responses to paths outside the contract are ignored.
func (c *Contract) record(req *http.Request, status int) {
	_, _, path := paths.FindPath(req, c.model, c.paths)
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
