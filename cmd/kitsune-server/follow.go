package main

import (
	"context"
	"slices"

	"github.com/simonhull/kitsune/internal/library"
	"github.com/simonhull/kitsune/internal/settings"
)

// ownerChanges are the channels that tell each owner of a live setting that it
// changed. base_url and trusted_proxies have none: they wait for a restart.
type ownerChanges struct {
	library  <-chan struct{} // Roots, grace and tag writes: the scanner's.
	lastfm   <-chan struct{} // The key and secret: the Last.fm client's.
	schedule <-chan struct{} // The quick scan's time.
	level    <-chan struct{} // The log level.
}

// subscribe subscribes every owner to its keys in live.
func subscribe(live *settings.Service) ownerChanges {
	return ownerChanges{
		library:  live.Subscribe(settings.LibraryRoots, settings.LibraryMissingGrace, settings.LibraryTagWrites),
		lastfm:   live.Subscribe(settings.LastfmAPIKey, settings.LastfmSharedSecret),
		schedule: live.Subscribe(settings.LibraryQuickScan),
		level:    live.Subscribe(settings.LogLevel),
	}
}

// followLibrary hands the scanner every change to its settings until ctx ends.
// A change waits for the scan, edit or fingerprint batch holding the library;
// new roots then start a scan, or follow the one running. It runs on its own,
// so a long scan never holds up a change to anything else.
func (a *app) followLibrary(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-a.changes.library:
		}
		snap := a.live.Snapshot()
		before := a.scanner.Roots()
		next := library.Live{Roots: snap.Roots, MissingGrace: snap.MissingGrace, TagWrites: snap.TagWrites}
		if err := a.scanner.Configure(ctx, next); err != nil {
			return // Only shutdown ends the wait.
		}
		a.log.InfoContext(ctx, "library settings applied",
			"roots", len(next.Roots), "tagWrites", next.TagWrites, "missingGrace", next.MissingGrace.String())
		if !slices.Equal(before, next.Roots) {
			a.log.InfoContext(ctx, "library folders changed: scanning")
			a.scanner.ScanWhenFree(ctx) // It logs how the scan went.
		}
	}
}

// followRest applies every change to the Last.fm key, the schedule and the log
// level until ctx ends. None of them waits.
func (a *app) followRest(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-a.changes.lastfm:
			a.applyLastfm(ctx)
		case <-a.changes.schedule:
			t := a.live.Snapshot().QuickScan
			a.scanner.SetDaily(library.Daily{Hour: t.Hour, Minute: t.Minute})
			a.log.InfoContext(ctx, "quick scan rescheduled", "at", t.String())
		case <-a.changes.level:
			if a.level != nil {
				level := a.live.Snapshot().LogLevel
				a.level.Set(level)
				a.log.InfoContext(ctx, "log level changed", "level", level.String())
			}
		}
	}
}

// applyLastfm gives the sender a client for the live key and secret, or none
// while either is missing. It logs whether Last.fm is on, never the key.
func (a *app) applyLastfm(ctx context.Context) {
	client, err := lastfmClient(lastfmConfig(a.live.Snapshot()), a.lastfmURL, a.log)
	if err != nil { // Only a stand-in's URL can be wrong, and startup has checked it.
		a.log.ErrorContext(ctx, "last.fm client", "err", err)
		return
	}
	a.lastfm.SetClient(client)
	a.log.InfoContext(ctx, "last.fm settings applied", "enabled", client != nil)
}
