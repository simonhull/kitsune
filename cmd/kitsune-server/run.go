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
  serve     run the server (-config FILE, -set KEY=VALUE)
  config    print the config file's values and exit
  settings reset KEY   clear a setting in the database, so the next start
                   seeds it again (-config FILE)
  users add NAME   create a user (-admin, -password-stdin, -config FILE);
                   the first can also be made in a browser, on a server's first run
  users passwd NAME   set a user's password and sign them out everywhere
                   (-password-stdin, -keep-sessions, -config FILE)
  users list       print every user's name and role (-config FILE)
  version   print the version and exit
  help      print this help and exit

Flags:
  -config FILE   read FILE instead of $XDG_CONFIG_HOME/kitsune/server.toml (config)
  -set KEY=VALUE seed a setting the database has no value for, such as
                 -set log.level=debug (repeatable)

Settings live in the database and are changed on the Server page. server.toml,
KITSUNE_<KEY> variables (KITSUNE_LASTFM_API_KEY) and -set only fill a setting
that has no value yet; listen and data_dir apply at every start.
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
func cli(args []string, env config.Env, stdin io.Reader, stdout, stderr io.Writer) int {
	err := run(args, env, stdin, stdout, stderr)
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
// environment env, reading from stdin and writing normal output to stdout.
// Prompts go to stderr. A *usageError means the command line was wrong.
func run(args []string, env config.Env, stdin io.Reader, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		return &usageError{msg: "no command given"}
	}
	switch cmd, rest := args[0], args[1:]; cmd {
	case serveName:
		return runServe(rest, env, stderr)
	case "config":
		return runConfig(rest, env, stdout)
	case "settings":
		return runSettings(rest, env, stdout)
	case "users":
		return runUsers(rest, env, stdin, stdout, stderr)
	case "version":
		return runVersion(cmd, rest, stdout)
	case "help", "-h", "--help":
		return runHelp(cmd, rest, stdout)
	default:
		return &usageError{msg: fmt.Sprintf("unknown command %q", cmd)}
	}
}

// runVersion prints the version.
func runVersion(cmd string, rest []string, stdout io.Writer) error {
	if err := noArgs(cmd, rest); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(stdout, "kitsune-server", version()); err != nil {
		return fmt.Errorf("write version: %w", err)
	}
	return nil
}

// runHelp prints the usage text.
func runHelp(cmd string, rest []string, stdout io.Writer) error {
	if err := noArgs(cmd, rest); err != nil {
		return err
	}
	if _, err := io.WriteString(stdout, usage); err != nil {
		return fmt.Errorf("write usage: %w", err)
	}
	return nil
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
