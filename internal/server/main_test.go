package server

import (
	"fmt"
	"os"
	"testing"

	"github.com/simonhull/kitsune/internal/openapitest"
)

// contract is shared by every test in the package so that TestMain can check,
// after a full run, that each response the contract declares was exercised.
var contract *openapitest.Contract //nolint:gochecknoglobals // TestMain-owned; coverage must span all tests.

func TestMain(m *testing.M) {
	stderr := os.Stderr //nolint:forbidigo // TestMain is the test binary's entry point, like a command's main.
	c, err := openapitest.New()
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "load contract:", err)
		os.Exit(1)
	}
	contract = c
	os.Exit(contract.CheckExercised(stderr, m.Run()))
}
