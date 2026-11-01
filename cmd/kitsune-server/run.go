package main

import (
	"errors"
	"fmt"
	"io"
	"runtime/debug"
)

const usage = `Usage: kitsune-server <command>

Commands:
  version   print the version and exit
  help      print this help and exit
`

// Exit statuses returned by cli.
const (
	exitOK      = 0
	exitFailure = 1 // the command failed
	exitUsage   = 2 // the command line was wrong
)

// usageError is a command-line mistake; cli reports it with the usage text.
type usageError struct{ msg string }

func (e *usageError) Error() string { return e.msg }

// cli runs kitsune-server with args (without the program name) and returns
// the process exit status. Errors go to stderr, followed by the usage text
// when the command line was wrong.
func cli(args []string, stdout, stderr io.Writer) int {
	err := run(args, stdout)
	if err == nil {
		return exitOK
	}
	// Nothing useful can be done if stderr itself fails.
	_, _ = fmt.Fprintf(stderr, "kitsune-server: %v\n", err)
	if _, ok := errors.AsType[*usageError](err); ok {
		_, _ = io.WriteString(stderr, "\n"+usage)
		return exitUsage
	}
	return exitFailure
}

// run executes the command line args (without the program name), writing
// normal output to stdout. A *usageError means the command line was wrong.
func run(args []string, stdout io.Writer) error {
	if len(args) == 0 {
		return &usageError{msg: "no command given"}
	}
	switch cmd, rest := args[0], args[1:]; cmd {
	case "version":
		if err := noArgs(cmd, rest); err != nil {
			return err
		}
		if _, err := fmt.Fprintln(stdout, "kitsune-server", version()); err != nil {
			return fmt.Errorf("write version: %w", err)
		}
		return nil
	case "help", "-h", "--help":
		if err := noArgs(cmd, rest); err != nil {
			return err
		}
		if _, err := io.WriteString(stdout, usage); err != nil {
			return fmt.Errorf("write usage: %w", err)
		}
		return nil
	default:
		return &usageError{msg: fmt.Sprintf("unknown command %q", cmd)}
	}
}

// noArgs rejects arguments after a command that takes none.
func noArgs(cmd string, rest []string) error {
	if len(rest) > 0 {
		return &usageError{msg: cmd + " takes no arguments"}
	}
	return nil
}

// version reports the main module's version as stamped by the go tool: the
// tag for a tagged build, otherwise a pseudo-version or "(devel)".
func version() string {
	if bi, ok := debug.ReadBuildInfo(); ok && bi.Main.Version != "" {
		return bi.Main.Version
	}
	return "(unknown)"
}
