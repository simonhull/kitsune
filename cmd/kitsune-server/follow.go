package main

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/simonhull/kitsune/internal/library"
	"github.com/simonhull/kitsune/internal/organize"
	"github.com/simonhull/kitsune/internal/settings"
)

// ownerChanges are the channels that tell each owner of a live setting that it
// changed. base_url and trusted_proxies have none: they wait for a restart.
type ownerChanges struct {
	library  <-chan struct{} // Roots, grace and tag writes: the scanner's.
	lastfm   <-chan struct{} // The key and secret: the Last.fm client's.
	schedule <-chan struct{} // The quick scan's time.
	level    <-chan struct{} // The log level.
	organize <-chan struct{} // The organizer's templates and switches.
	metadata <-chan struct{} // The artist pass's switches, language and keys.
}

// subscribe subscribes every owner to its keys in live.
func subscribe(live *settings.Service) ownerChanges {
	return ownerChanges{
		library:  live.Subscribe(settings.LibraryRoots, settings.LibraryMissingGrace, settings.LibraryTagWrites),
		lastfm:   live.Subscribe(settings.LastfmAPIKey, settings.LastfmSharedSecret),
		schedule: live.Subscribe(settings.LibraryQuickScan),
		level:    live.Subscribe(settings.LogLevel),
		organize: live.Subscribe(settings.OrganizeTemplate, settings.OrganizeCompilations, settings.OrganizeRoots,
			settings.OrganizeOnEdit, settings.OrganizeCompanions, settings.OrganizeRewritePlaylists),
		metadata: live.Subscribe(settings.MetadataRemote, settings.MetadataFanart, settings.MetadataWikipedia,
			settings.MetadataLastfmBios, settings.MetadataArchive, settings.MetadataLanguage,
			settings.FanartAPIKey, settings.FanartClientKey, settings.LastfmAPIKey),
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

// followRest applies every change to the Last.fm key, the schedule, the
// organizer, the artist pass and the log level until ctx ends. None of them
// waits.
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
		case <-a.changes.organize:
			a.applyOrganize(ctx)
		case <-a.changes.metadata:
			a.applyMetadata(ctx)
		case <-a.changes.level:
			if a.level != nil {
				level := a.live.Snapshot().LogLevel
				a.level.Set(level)
				a.log.InfoContext(ctx, "log level changed", "level", level.String())
			}
		}
	}
}

// applyOrganize gives the scanner the organizer's settings. They passed their
// rules when they were stored, so a template that doesn't parse can't happen;
// should one, it is logged and the defaults stand.
func (a *app) applyOrganize(ctx context.Context) {
	c, err := organizeConfig(a.live.Snapshot())
	if err != nil {
		a.log.ErrorContext(ctx, "a stored organize template doesn't parse; the defaults stand", "err", err)
		return
	}
	a.scanner.SetOrganize(&c)
}

// organizeConfig is the organizer's configuration in snap.
func organizeConfig(snap *settings.Snapshot) (library.OrganizeConfig, error) {
	c := library.DefaultOrganize()
	c.Companions, c.OnEdit, c.RewritePlaylists = snap.OrganizeCompanions, snap.OrganizeOnEdit, snap.OrganizeRewritePlaylists
	main, err := organize.Parse(snap.OrganizeTemplate)
	if err != nil {
		return c, fmt.Errorf("organize.template: %w", err)
	}
	comp, err := organize.Parse(snap.OrganizeCompilations)
	if err != nil {
		return c, fmt.Errorf("organize.compilations: %w", err)
	}
	c.Template, c.Compilations = main, comp
	c.Roots = make(map[string]organize.Templates, len(snap.OrganizeRoots))
	for root, ts := range snap.OrganizeRoots {
		rootMain, mainErr := organize.Parse(ts.Template)
		rootComp, compErr := organize.Parse(ts.Compilations)
		if err = errors.Join(mainErr, compErr); err != nil {
			return c, fmt.Errorf("organize.roots: %w", err)
		}
		c.Roots[root] = organize.Templates{Template: rootMain, Compilations: rootComp}
	}
	return c, nil
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
