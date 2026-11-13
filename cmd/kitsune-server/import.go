package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/simonhull/kitsune/internal/config"
	"github.com/simonhull/kitsune/internal/library"
	"github.com/simonhull/kitsune/internal/navidrome"
	"github.com/simonhull/kitsune/internal/rating"
	"github.com/simonhull/kitsune/internal/store"
	"github.com/simonhull/kitsune/internal/store/navidromedb"
)

// importNavidromeName is the import subcommand for a Navidrome database.
const importNavidromeName = "navidrome"

// afterImport is what the import can't do from another process.
const afterImport = "Shuffles see the import within 10 minutes, or at a restart of the server. Reload open tabs to see it.\n"

// errPair means a FROM=TO flag lacked one side.
var errPair = errors.New("want FROM=TO")

// pairs is a repeatable FROM=TO flag.
type pairs map[string]string

func (p pairs) String() string {
	var b strings.Builder
	for _, k := range slices.Sorted(maps.Keys(p)) {
		fmt.Fprintf(&b, "%s=%s ", k, p[k])
	}
	return strings.TrimSpace(b.String())
}

// Set adds one FROM=TO; a second FROM replaces the first.
func (p pairs) Set(v string) error {
	from, to, ok := strings.Cut(v, "=")
	if !ok || from == "" || to == "" {
		return fmt.Errorf("%q: %w", v, errPair)
	}
	p[from] = to
	return nil
}

// importArgs is the parsed command line of import navidrome.
type importArgs struct {
	configPath, file    string
	dryRun, verbose     bool
	users, roots, paths pairs
	heartStars          rating.Rating
}

// defaultHeartStars is what a starred song with no rating is rated unless
// -heart-stars says otherwise: 4★, in halves.
const defaultHeartStars = 8

// errHeartStars means -heart-stars is no number of halves from 0 to 10.
var errHeartStars = errors.New("-heart-stars wants halves from 1 (½★) to 10 (5★), or 0 to leave them unrated")

// runImport dispatches the import subcommands.
func runImport(args []string, env config.Env, stdout io.Writer) error {
	if len(args) == 0 {
		return &usageError{msg: "import needs a source: " + importNavidromeName}
	}
	switch src, rest := args[0], args[1:]; src {
	case importNavidromeName:
		return runImportNavidrome(rest, env, stdout)
	default:
		return &usageError{msg: fmt.Sprintf("unknown import source %q", src)}
	}
}

// parseImport parses the flags and the FILE of import navidrome.
func parseImport(args []string) (importArgs, error) {
	a := importArgs{users: pairs{}, roots: pairs{}, paths: pairs{}}
	flags := flag.NewFlagSet("import navidrome", flag.ContinueOnError)
	flags.SetOutput(io.Discard) // cli prints the usage itself.
	flags.StringVar(&a.configPath, "config", "", "config file")
	flags.BoolVar(&a.dryRun, "dry-run", false, "report what would be imported, and write nothing")
	flags.BoolVar(&a.verbose, "v", false, "list the songs that matched nothing")
	flags.Var(a.users, "user", "a Navidrome user's Kitsune account, NAVIDROME=KITSUNE (repeatable)")
	flags.Var(a.roots, "root", "a Navidrome library's Kitsune root, NAVIDROME=KITSUNE (repeatable)")
	flags.Var(a.paths, "map", "a moved file, OLD=NEW relative to its root (repeatable)")
	halves := flags.Int64("heart-stars", defaultHeartStars, "what a starred song with no rating is rated, in halves (8 is 4★); 0 leaves it unrated")
	if err := flags.Parse(args); err != nil {
		return importArgs{}, &usageError{msg: err.Error()}
	}
	if *halves != 0 {
		r, err := rating.FromHalves(*halves)
		if err != nil {
			return importArgs{}, &usageError{msg: errHeartStars.Error()}
		}
		a.heartStars = r
	}
	if flags.NArg() != 1 {
		return importArgs{}, &usageError{msg: "import navidrome takes exactly one FILE"}
	}
	a.file = flags.Arg(0)
	return a, nil
}

// runImportNavidrome imports plays and ratings from a Navidrome database,
// rating starred songs that have none -heart-stars. It never migrates or
// creates Kitsune's database, and works while a server runs on it: each
// person's import is one write.
func runImportNavidrome(args []string, env config.Env, stdout io.Writer) error {
	a, err := parseImport(args)
	if err != nil {
		return err
	}
	cfg, err := config.Load(a.configPath, config.DefaultPaths(env))
	if err != nil {
		return err //nolint:wrapcheck // config's errors already name the file and key.
	}
	if err = importNavidrome(cfg.Storage.DataDir, &a, stdout); err != nil {
		return fmt.Errorf("import navidrome: %w", err)
	}
	return nil
}

// importNavidrome plans the import of a.file into the database under dataDir,
// reports it, and writes it unless this is a dry run.
func importNavidrome(dataDir string, a *importArgs, stdout io.Writer) (err error) {
	path := filepath.Join(dataDir, store.FileName)
	if _, err = os.Stat(path); errors.Is(err, fs.ErrNotExist) {
		return errNoDatabase
	}
	ctx := context.Background()
	source, err := loadNavidrome(ctx, a.file)
	if err != nil {
		return err
	}
	st, err := store.Open(ctx, path, store.Options{})
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer func() {
		if closeErr := st.Close(); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("close database: %w", closeErr))
		}
	}()
	plan, err := planImport(ctx, st, &source, a)
	if err != nil {
		return err
	}
	return applyImport(ctx, st, &plan, a, stdout)
}

// applyImport writes plan, one transaction per person, and reports it. A dry
// run, or a plan the path gate stops, is reported and not written.
func applyImport(ctx context.Context, st *store.Store, plan *navidrome.Plan, a *importArgs, stdout io.Writer) error {
	gateErr := plan.Gate.Check()
	if a.dryRun || gateErr != nil {
		if err := navidrome.Write(stdout, plan, nil, a.verbose); err != nil {
			return err //nolint:wrapcheck // Already says "write report".
		}
		if !a.dryRun {
			return fmt.Errorf("nothing written: %w", gateErr)
		}
		return writeDryRun(stdout, gateErr)
	}
	written := make([]store.ImportCounts, len(plan.People))
	for i := range plan.People {
		var err error
		if written[i], err = st.ApplyImport(ctx, &plan.People[i].Import); err != nil {
			return fmt.Errorf("%s: %w", plan.People[i].From, err)
		}
	}
	if err := navidrome.Write(stdout, plan, written, a.verbose); err != nil {
		return err //nolint:wrapcheck // Already says "write report".
	}
	if _, err := io.WriteString(stdout, afterImport); err != nil {
		return fmt.Errorf("write report: %w", err)
	}
	return nil
}

// loadNavidrome reads everything the import needs from the Navidrome
// database at path.
func loadNavidrome(ctx context.Context, path string) (navidrome.Source, error) {
	db, err := navidromedb.Open(ctx, path)
	if err != nil {
		return navidrome.Source{}, err //nolint:wrapcheck // Says "open navidrome database".
	}
	src, err := navidrome.Load(ctx, db)
	return src, errors.Join(err, db.Close())
}

// planImport resolves src against the library in st.
func planImport(ctx context.Context, st *store.Store, src *navidrome.Source, a *importArgs) (navidrome.Plan, error) {
	targets, err := st.ImportTargets(ctx, store.MBIDKeys{
		ReleaseTrack: library.ReleaseTrackMBIDKeys(),
		Recording:    library.TrackMBIDKeys(),
	})
	if err != nil {
		return navidrome.Plan{}, err //nolint:wrapcheck // Says "import targets".
	}
	now := time.Now().UTC()
	users, err := st.Users(ctx, now)
	if err != nil {
		return navidrome.Plan{}, err //nolint:wrapcheck // Says "list users".
	}
	accounts := make([]navidrome.Account, len(users))
	for i := range users {
		accounts[i] = navidrome.Account{ID: users[i].User.ID, Name: users[i].User.Name}
	}
	return navidrome.New(src, &targets, &navidrome.Options{ //nolint:wrapcheck // Its errors name the flag at fault.
		Accounts: accounts, Users: a.users, Roots: a.roots, Paths: a.paths, Now: now, HeartStars: a.heartStars,
	})
}

// writeDryRun closes a dry run's report.
func writeDryRun(stdout io.Writer, gateErr error) error {
	msg := "Dry run: nothing was written.\n"
	if gateErr != nil {
		msg = fmt.Sprintf("Dry run: nothing was written. Without -dry-run this would stop: %v\n", gateErr)
	}
	if _, err := io.WriteString(stdout, msg); err != nil {
		return fmt.Errorf("write report: %w", err)
	}
	return nil
}
