package main

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/simonhull/kitsune/internal/app"
	"github.com/simonhull/kitsune/internal/config"
	"github.com/simonhull/kitsune/internal/db"
	"github.com/simonhull/kitsune/internal/player"
	"github.com/simonhull/kitsune/internal/subsonic"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "config error: %v\n", err)
		os.Exit(1)
	}

	// Log to file so it doesn't corrupt the TUI.
	logger := setupLogger()

	database, err := db.Open(logger)
	if err != nil {
		fmt.Fprintf(os.Stderr, "database error: %v\n", err)
		os.Exit(1)
	}
	defer database.Close()

	// Create Subsonic client if configured.
	var client *subsonic.Client
	if cfg.HasSubsonic() {
		client = subsonic.NewClient(cfg.Subsonic.URL, cfg.Subsonic.Username, cfg.Subsonic.Password)

		if err := client.Ping(); err != nil {
			fmt.Fprintf(os.Stderr, "subsonic connection failed: %v\n", err)
			fmt.Fprintf(os.Stderr, "check your config at %s\n", config.Path())
			os.Exit(1)
		}
	}

	// Initialize audio player.
	p, err := player.New(logger)
	if err != nil {
		fmt.Fprintf(os.Stderr, "audio init failed: %v\n", err)
		os.Exit(1)
	}

	prog := tea.NewProgram(
		app.New(cfg, database, client, p),
		tea.WithAltScreen(),
	)

	if _, err := prog.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func setupLogger() *slog.Logger {
	logDir := db.DataDir()
	os.MkdirAll(logDir, 0o755)

	logPath := filepath.Join(logDir, "kitsune.log")
	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		// Fall back to discard if we can't open the log file.
		return slog.New(slog.NewTextHandler(os.NewFile(0, os.DevNull), nil))
	}

	logger := slog.New(slog.NewTextHandler(f, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)
	return logger
}
