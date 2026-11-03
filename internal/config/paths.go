package config

import "path/filepath"

// Env looks up an environment variable. [os.LookupEnv] is the production
// implementation.
type Env func(key string) (string, bool)

// Paths are the default locations of the config file and the data directory.
// A field is empty when it can't be resolved from the environment.
type Paths struct {
	ConfigFile string
	DataDir    string
}

// DefaultPaths resolves the XDG base directories from env. The config file is
// $XDG_CONFIG_HOME/kitsune/server.toml and the data directory is
// $XDG_DATA_HOME/kitsune-server. An unset or relative variable falls back to
// $HOME/.config or $HOME/.local/share, as the XDG spec requires.
func DefaultPaths(env Env) Paths {
	var p Paths
	if dir := xdgDir(env, "XDG_CONFIG_HOME", ".config"); dir != "" {
		p.ConfigFile = filepath.Join(dir, "kitsune", "server.toml")
	}
	if dir := xdgDir(env, "XDG_DATA_HOME", filepath.Join(".local", "share")); dir != "" {
		p.DataDir = filepath.Join(dir, "kitsune-server")
	}
	return p
}

// xdgDir returns the absolute directory in the variable key, else fallback
// under an absolute $HOME, else "".
func xdgDir(env Env, key, fallback string) string {
	if dir, ok := env(key); ok && filepath.IsAbs(dir) {
		return dir
	}
	if home, ok := env("HOME"); ok && filepath.IsAbs(home) {
		return filepath.Join(home, fallback)
	}
	return ""
}
