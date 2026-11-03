package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"runtime/debug"

	"github.com/simonhull/kitsune/internal/config"
)

const usage = `Usage: kitsune-server <command> [flags]

Commands:
  config    print the configuration in effect and exit
  version   print the version and exit
  help      print this help and exit

Flags:
  -config FILE   read FILE instead of $XDG_CONFIG_HOME/kitsune/server.toml (config)
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

// cli runs kitsune-server with args (without the program name) in the
// environment env and returns the process exit status. Errors go to stderr,
// followed by the usage text when the command line was wrong.
func cli(args []string, env config.Env, stdout, stderr io.Writer) int {
	err := run(args, env, stdout)
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

// run executes the command line args (without the program name) in the
// environment env, writing normal output to stdout. A *usageError means the
// command line was wrong.
func run(args []string, env config.Env, stdout io.Writer) error {
	if len(args) == 0 {
		return &usageError{msg: "no command given"}
	}
	switch cmd, rest := args[0], args[1:]; cmd {
	case "config":
		return runConfig(rest, env, stdout)
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

// runConfig prints the configuration in effect as TOML.
func runConfig(args []string, env config.Env, stdout io.Writer) error {
	flags := flag.NewFlagSet("config", flag.ContinueOnError)
	flags.SetOutput(io.Discard) // cli prints the usage itself.
	path := flags.String("config", "", "config file")
	if err := flags.Parse(args); err != nil {
		return &usageError{msg: err.Error()}
	}
	if err := noArgs("config", flags.Args()); err != nil {
		return err
	}
	cfg, err := config.Load(*path, config.DefaultPaths(env))
	if err != nil {
		return err //nolint:wrapcheck // config's errors already name the file and key.
	}
	return cfg.WriteTOML(stdout) //nolint:wrapcheck // Already wrapped as "write config".
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
