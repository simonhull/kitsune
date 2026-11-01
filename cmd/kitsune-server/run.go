package main

import (
	"fmt"
	"io"
	"runtime/debug"
)

const usage = `Usage: kitsune-server <command>

Commands:
  version   print the version and exit
  help      print this help and exit
`

// usageError is a command-line mistake; main prints it with the usage text
// and exits with status 2.
type usageError struct{ msg string }

func (e *usageError) Error() string { return e.msg }

// run executes the command line args (without the program name), writing
// normal output to stdout. A *usageError means the command line was wrong.
func run(args []string, stdout io.Writer) error {
	if len(args) == 0 {
		return &usageError{msg: "no command given"}
	}
	switch args[0] {
	case "version":
		if _, err := fmt.Fprintln(stdout, "kitsune-server", version()); err != nil {
			return fmt.Errorf("write version: %w", err)
		}
		return nil
	case "help", "-h", "--help":
		if _, err := io.WriteString(stdout, usage); err != nil {
			return fmt.Errorf("write usage: %w", err)
		}
		return nil
	default:
		return &usageError{msg: fmt.Sprintf("unknown command %q", args[0])}
	}
}

// version reports the main module's version as stamped by the go tool: the
// tag for a tagged build, otherwise a pseudo-version or "(devel)".
func version() string {
	if bi, ok := debug.ReadBuildInfo(); ok && bi.Main.Version != "" {
		return bi.Main.Version
	}
	return "(unknown)"
}
