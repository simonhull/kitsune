// Command kitsune-server serves a music library to Kitsune clients.
package main

import (
	"errors"
	"fmt"
	"os"
)

// Exit statuses: 1 for a failed command, 2 for a command-line mistake.
const (
	exitFailure = 1
	exitUsage   = 2
)

func main() {
	err := run(os.Args[1:], os.Stdout)
	if err == nil {
		return
	}
	fmt.Fprintf(os.Stderr, "kitsune-server: %v\n", err)
	if _, ok := errors.AsType[*usageError](err); ok {
		fmt.Fprint(os.Stderr, "\n"+usage)
		os.Exit(exitUsage)
	}
	os.Exit(exitFailure)
}
