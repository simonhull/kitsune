package server

import (
	"flag"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/simonhull/kitsune/internal/openapitest"
)

// contract is shared by every test in the package so that TestMain can check,
// after a full run, that each response the contract declares was exercised.
var contract *openapitest.Contract //nolint:gochecknoglobals // TestMain-owned; coverage must span all tests.

func TestMain(m *testing.M) {
	stderr := os.Stderr //nolint:forbidigo // TestMain is the test binary's entry point, like a command's main.
	flag.Parse()
	c, err := openapitest.MustLoad()
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "load contract:", err)
		os.Exit(1)
	}
	contract = c
	code := m.Run()
	if code == 0 && flag.Lookup("test.run").Value.String() == "" {
		if missing := contract.Unexercised(); len(missing) > 0 {
			_, _ = fmt.Fprintf(stderr, "responses declared in api/openapi.yaml but never exercised:\n  %s\n",
				strings.Join(missing, "\n  "))
			code = 1
		}
	}
	os.Exit(code)
}
