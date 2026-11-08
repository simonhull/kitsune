package settings

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"

	"github.com/simonhull/kitsune/internal/config"
	"github.com/simonhull/kitsune/internal/store"
)

// A Seed is a value from server.toml, the environment or a flag, for a key the
// database may not have yet.
type Seed struct {
	Key    Key
	Source Source
	value  []byte // Canonical JSON.
}

// FileSeeds returns a seed for every key the config file set, as keys lists
// them. A key it set to an empty value seeds nothing.
func FileSeeds(cfg *config.Config, keys config.Keys) []Seed {
	var seeds []Seed
	all := defs()
	for _, k := range Keys() {
		if !keys.Has(string(k)) {
			continue
		}
		d := all[k]
		raw := d.file(cfg)
		if d.empty(raw) {
			continue
		}
		var v Values
		if d.decode(&v, raw) != nil {
			continue // Load has checked the file; a value it took that this refuses can't seed.
		}
		seeds = append(seeds, Seed{Key: k, Source: SourceFile, value: d.encode(&v)})
	}
	return seeds
}

// TextSeed returns the seed for k from text, the value of an environment
// variable or a flag. Text that is empty seeds nothing: the second result is
// false.
//
// # Errors
//
// A *[KeyError] wrapping [ErrBadValue] when k doesn't take text. Its message
// never holds the text.
func TextSeed(k Key, text string, src Source) (Seed, bool, error) {
	d, ok := defs()[k]
	if !ok {
		return Seed{}, false, &KeyError{Key: k, Err: ErrUnknownKey}
	}
	raw, err := d.text(text)
	if err != nil {
		return Seed{}, false, badValue(k, err)
	}
	if d.empty(raw) {
		return Seed{}, false, nil
	}
	return Seed{Key: k, Source: src, value: raw}, true, nil
}

// Apply stores each seed whose key the database has no value for, recording its
// source. Where the database has a value already, it wins: a seed that differs
// from it logs one warning naming the key and the seed's source, and never the
// value. Pass at most one seed per key: the one that should win.
//
// Only cmd/ seeds, at startup, before [Open].
func Apply(ctx context.Context, st *store.Store, seeds []Seed, log *slog.Logger) error {
	rows, err := st.Settings(ctx)
	if err != nil {
		return fmt.Errorf("seed settings: %w", err)
	}
	stored := make(map[Key][]byte, len(rows))
	for _, r := range rows {
		stored[Key(r.Key)] = r.Value
	}
	all := defs()
	for _, s := range seeds {
		have, ok := stored[s.Key]
		if !ok {
			if _, err = st.SeedSetting(ctx, string(s.Key), s.value, string(s.Source)); err != nil {
				return fmt.Errorf("seed settings: %w", err)
			}
			log.InfoContext(ctx, "setting seeded", "key", string(s.Key), "from", s.Source.From(s.Key))
			continue
		}
		var v Values
		if all[s.Key].decode(&v, have) == nil && bytes.Equal(all[s.Key].encode(&v), s.value) {
			continue
		}
		log.WarnContext(ctx, "a setting is set in the database, which wins: this value doesn't apply. "+
			"Clear it on the Server page, or run kitsune-server settings reset, to seed it again at the next start",
			"key", string(s.Key), "from", s.Source.From(s.Key))
	}
	return nil
}
