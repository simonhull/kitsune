package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/simonhull/kitsune/internal/config"
	"github.com/simonhull/kitsune/internal/settings"
	"github.com/simonhull/kitsune/internal/store"
)

var (
	// errSetFlag means a -set flag is not key=value.
	errSetFlag = errors.New("-set wants key=value, such as -set log.level=debug")
	// errSecretFlag means a -set flag named a secret, which the process list
	// would show to everyone on the host.
	errSecretFlag = errors.New("a secret can't be a flag, which anyone on the host can read: use the environment or server.toml")
	// errNoDatabase means settings reset found no database to reset.
	errNoDatabase = errors.New("no database in data_dir")
)

// setFlags are the -set key=value flags of serve, in order, as seeds.
type setFlags []settings.Seed

// String implements [flag.Value]. It never prints a value.
func (s *setFlags) String() string {
	keys := make([]string, len(*s))
	for i, seed := range *s {
		keys[i] = string(seed.Key)
	}
	return strings.Join(keys, ",")
}

// Set implements [flag.Value]: it reads one key=value.
func (s *setFlags) Set(arg string) error {
	name, value, ok := strings.Cut(arg, "=")
	if !ok {
		return errSetFlag
	}
	k, err := settings.Lookup(name)
	if err != nil {
		return err //nolint:wrapcheck // It names the key.
	}
	if k.Secret() {
		return fmt.Errorf("-set %s: %w", k, errSecretFlag)
	}
	seed, use, err := settings.TextSeed(k, value, settings.SourceFlag)
	if err != nil {
		return fmt.Errorf("-set: %w", err)
	}
	if use {
		*s = append(*s, seed)
	}
	return nil
}

// seedsFor returns the seed of every key something outside the database sets.
// A flag beats the environment, which beats the file: the most specific wins.
func seedsFor(cfg *config.Config, keys config.Keys, env config.Env, flags setFlags) ([]settings.Seed, error) {
	byKey := map[settings.Key]settings.Seed{}
	for _, s := range settings.FileSeeds(cfg, keys) {
		byKey[s.Key] = s
	}
	for _, k := range settings.Keys() {
		text, ok := env(k.EnvName())
		if !ok {
			continue
		}
		s, use, err := settings.TextSeed(k, text, settings.SourceEnv)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", k.EnvName(), err)
		}
		if use {
			byKey[k] = s
		}
	}
	for _, s := range flags {
		byKey[s.Key] = s
	}
	out := make([]settings.Seed, 0, len(byKey))
	for _, k := range settings.Keys() {
		if s, ok := byKey[k]; ok {
			out = append(out, s)
		}
	}
	return out, nil
}

// runSettings dispatches the settings subcommands.
func runSettings(args []string, env config.Env, stdout io.Writer) error {
	if len(args) == 0 {
		return &usageError{msg: "settings needs a subcommand: reset"}
	}
	switch sub, rest := args[0], args[1:]; sub {
	case "reset":
		return runSettingsReset(rest, env, stdout)
	default:
		return &usageError{msg: fmt.Sprintf("unknown settings command %q", sub)}
	}
}

// runSettingsReset clears one setting in the database, so the next start seeds
// it from server.toml, the environment or a flag, or gives it its default.
func runSettingsReset(args []string, env config.Env, stdout io.Writer) error {
	flags := flag.NewFlagSet("settings reset", flag.ContinueOnError)
	flags.SetOutput(io.Discard) // cli prints the usage itself.
	path := flags.String("config", "", "config file")
	if err := flags.Parse(args); err != nil {
		return &usageError{msg: err.Error()}
	}
	if flags.NArg() != 1 {
		return &usageError{msg: "settings reset takes exactly one KEY, such as lastfm.api_key"}
	}
	k, err := settings.Lookup(flags.Arg(0))
	if err != nil {
		return &usageError{msg: err.Error()}
	}
	cfg, err := config.Load(*path, config.DefaultPaths(env))
	if err != nil {
		return err //nolint:wrapcheck // config's errors already name the file and key.
	}
	had, err := resetSetting(cfg.Storage.DataDir, k)
	if err != nil {
		return fmt.Errorf("settings reset: %w", err)
	}
	msg := "%s had no value in the database; nothing changed\n"
	if had {
		msg = "cleared %s: the next start sets it from server.toml, the environment or a flag, or its default\n"
	}
	if _, err = fmt.Fprintf(stdout, msg, k); err != nil {
		return fmt.Errorf("write confirmation: %w", err)
	}
	return nil
}

// resetSetting deletes k's value from the database in dataDir and reports
// whether it had one. It never migrates or creates a database: a server may be
// running on it, and keeps its live value until it restarts.
func resetSetting(dataDir string, k settings.Key) (bool, error) {
	dbPath := filepath.Join(dataDir, store.FileName)
	if _, err := os.Stat(dbPath); err != nil {
		return false, fmt.Errorf("%w: %w", errNoDatabase, err)
	}
	ctx := context.Background()
	st, err := store.Open(ctx, dbPath, store.Options{})
	if err != nil {
		return false, fmt.Errorf("open database: %w", err)
	}
	had, err := st.DeleteSetting(ctx, string(k))
	if closeErr := st.Close(); closeErr != nil {
		err = errors.Join(err, fmt.Errorf("close database: %w", closeErr))
	}
	return had, err
}
