package ui

import (
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
	"github.com/charmbracelet/lipgloss"
)

// Theme holds the resolved color palette for the UI.
type Theme struct {
	Accent    lipgloss.Color
	Fg        lipgloss.Color
	Dim       lipgloss.Color
	Border    lipgloss.Color
	Error     lipgloss.Color
	Surface   lipgloss.Color // cursor/selection background
	BgDim     lipgloss.Color // subtle backgrounds (now playing bar)
}

// ThemeConfig is the user-facing config section in config.toml.
type ThemeConfig struct {
	Accent  string `toml:"accent"`
	Fg      string `toml:"fg"`
	Dim     string `toml:"dim"`
	Border  string `toml:"border"`
	Error   string `toml:"error"`
	Surface string `toml:"surface"`
}

// omarchyColors maps the Omarchy colors.toml format.
type omarchyColors struct {
	Accent     string `toml:"accent"`
	Foreground string `toml:"foreground"`
	Background string `toml:"background"`
	Color0     string `toml:"color0"`
	Color1     string `toml:"color1"`
	Color7     string `toml:"color7"`
	Color8     string `toml:"color8"`
}

// DefaultTheme returns the built-in Kitsune theme (fox orange).
func DefaultTheme() Theme {
	return Theme{
		Accent:  lipgloss.Color("#FF6B35"),
		Fg:      lipgloss.Color("#FFFFFF"),
		Dim:     lipgloss.Color("#666666"),
		Border:  lipgloss.Color("#333333"),
		Error:   lipgloss.Color("#FF4444"),
		Surface: lipgloss.Color("#333333"),
		BgDim:   lipgloss.Color("#1a1a1a"),
	}
}

// LoadTheme resolves the theme with three-tier fallback:
//  1. Explicit overrides from config.toml [theme]
//  2. Omarchy system theme (~/.config/omarchy/current/theme/colors.toml)
//  3. Built-in defaults
func LoadTheme(cfg ThemeConfig) Theme {
	t := DefaultTheme()

	// Tier 2: Try Omarchy theme.
	if oc, err := loadOmarchyColors(); err == nil {
		applyOmarchy(&t, oc)
	}

	// Tier 1: Explicit config overrides (highest priority).
	applyConfig(&t, cfg)

	return t
}

// loadOmarchyColors reads the Omarchy system theme.
func loadOmarchyColors() (*omarchyColors, error) {
	// Check XDG_CONFIG_HOME first, then default.
	configHome := os.Getenv("XDG_CONFIG_HOME")
	if configHome == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, err
		}
		configHome = filepath.Join(home, ".config")
	}

	path := filepath.Join(configHome, "omarchy", "current", "theme", "colors.toml")

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var oc omarchyColors
	if err := toml.Unmarshal(data, &oc); err != nil {
		return nil, err
	}

	return &oc, nil
}

// applyOmarchy maps Omarchy colors to theme roles.
func applyOmarchy(t *Theme, oc *omarchyColors) {
	if oc.Accent != "" {
		t.Accent = lipgloss.Color(oc.Accent)
	}
	if oc.Foreground != "" {
		t.Fg = lipgloss.Color(oc.Foreground)
	}
	if oc.Color8 != "" {
		t.Dim = lipgloss.Color(oc.Color8)
		t.Border = lipgloss.Color(oc.Color8)
	}
	if oc.Color1 != "" {
		t.Error = lipgloss.Color(oc.Color1)
	}
	if oc.Color0 != "" {
		t.Surface = lipgloss.Color(oc.Color0)
	}
	if oc.Background != "" {
		t.BgDim = lipgloss.Color(oc.Background)
	}
}

// applyConfig applies explicit user overrides.
func applyConfig(t *Theme, cfg ThemeConfig) {
	if cfg.Accent != "" {
		t.Accent = lipgloss.Color(cfg.Accent)
	}
	if cfg.Fg != "" {
		t.Fg = lipgloss.Color(cfg.Fg)
	}
	if cfg.Dim != "" {
		t.Dim = lipgloss.Color(cfg.Dim)
	}
	if cfg.Border != "" {
		t.Border = lipgloss.Color(cfg.Border)
	}
	if cfg.Error != "" {
		t.Error = lipgloss.Color(cfg.Error)
	}
	if cfg.Surface != "" {
		t.Surface = lipgloss.Color(cfg.Surface)
	}
}
