package main

import (
	"fmt"
	"log/slog"
	"os"

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

	logger := slog.Default()

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
