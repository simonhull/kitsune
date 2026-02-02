package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

// Config holds all Kitsune configuration.
type Config struct {
	Library LibraryConfig `toml:"library"`
	UI      UIConfig      `toml:"ui"`
}

// LibraryConfig configures music sources.
type LibraryConfig struct {
	// Path is the primary music directory.
	Path string `toml:"path"`
}

// UIConfig configures the user interface.
type UIConfig struct {
	// AlbumArt controls terminal image rendering: auto, kitty, iterm2, sixel, off.
	AlbumArt string `toml:"album_art"`
}

// Default returns a config with sensible defaults.
func Default() Config {
	return Config{
		UI: UIConfig{
			AlbumArt: "auto",
		},
	}
}

// Dir returns the Kitsune config directory, respecting XDG.
func Dir() string {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "kitsune")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "kitsune")
}

// Path returns the full path to the config file.
func Path() string {
	return filepath.Join(Dir(), "config.toml")
}

// Load reads the config file, or returns defaults if it doesn't exist.
func Load() (Config, error) {
	cfg := Default()

	data, err := os.ReadFile(Path())
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return cfg, fmt.Errorf("reading config: %w", err)
	}

	if err := toml.Unmarshal(data, &cfg); err != nil {
		return cfg, fmt.Errorf("parsing config: %w", err)
	}

	// Expand ~ in library path.
	cfg.Library.Path = expandHome(cfg.Library.Path)

	return cfg, nil
}

// Save writes the current config to disk, creating the directory if needed.
func Save(cfg Config) error {
	dir := Dir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("creating config dir: %w", err)
	}

	f, err := os.Create(Path())
	if err != nil {
		return fmt.Errorf("creating config file: %w", err)
	}
	defer f.Close()

	encoder := toml.NewEncoder(f)
	return encoder.Encode(cfg)
}

// expandHome replaces a leading ~ with the user home directory.
func expandHome(path string) string {
	if !strings.HasPrefix(path, "~/") {
		return path
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return path
	}
	return filepath.Join(home, path[2:])
}
