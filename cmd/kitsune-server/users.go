package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"
	"time"

	"golang.org/x/term"

	"github.com/simonhull/kitsune/internal/auth"
	"github.com/simonhull/kitsune/internal/config"
	"github.com/simonhull/kitsune/internal/store"
)

// listPadding is the spaces users list puts between its columns.
const listPadding = 2

// dataDirMode is the mode of a data_dir that users add has to create.
const dataDirMode = 0o700

// errPasswordMismatch means the two password prompts disagreed.
var errPasswordMismatch = errors.New("passwords do not match")

// errNoSuchUser means users passwd was given a name no account has.
var errNoSuchUser = errors.New("no such user")

// authParams returns the Argon2id parameters users add hashes with. Tests
// replace it with cheap ones.
var authParams = auth.DefaultParams //nolint:gochecknoglobals // Test hook so tests can hash cheaply; plan 008 prescribes it.

// runUsers dispatches the users subcommands.
func runUsers(args []string, env config.Env, stdin io.Reader, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		return &usageError{msg: "users needs a subcommand: add, passwd or list"}
	}
	switch sub, rest := args[0], args[1:]; sub {
	case "add":
		return runUsersAdd(rest, env, stdin, stdout, stderr)
	case "passwd":
		return runUsersPasswd(rest, env, stdin, stdout, stderr)
	case "list":
		return runUsersList(rest, env, stdout)
	default:
		return &usageError{msg: fmt.Sprintf("unknown users command %q", sub)}
	}
}

// usersAddArgs is the parsed command line of users add.
type usersAddArgs struct {
	configPath string
	name       string
	role       store.Role
	fromStdin  bool
}

// parseUsersAdd parses the flags and the NAME of users add.
func parseUsersAdd(args []string) (usersAddArgs, error) {
	flags := flag.NewFlagSet("users add", flag.ContinueOnError)
	flags.SetOutput(io.Discard) // cli prints the usage itself.
	path := flags.String("config", "", "config file")
	admin := flags.Bool("admin", false, "create an administrator")
	fromStdin := flags.Bool("password-stdin", false, "read the password from one line of stdin")
	if err := flags.Parse(args); err != nil {
		return usersAddArgs{}, &usageError{msg: err.Error()}
	}
	if flags.NArg() != 1 {
		return usersAddArgs{}, &usageError{msg: "users add takes exactly one NAME"}
	}
	role := store.RoleUser
	if *admin {
		role = store.RoleAdmin
	}
	return usersAddArgs{configPath: *path, name: flags.Arg(0), role: role, fromStdin: *fromStdin}, nil
}

// runUsersAdd creates a user in the database under data_dir.
func runUsersAdd(args []string, env config.Env, stdin io.Reader, stdout, stderr io.Writer) error {
	a, err := parseUsersAdd(args)
	if err != nil {
		return err
	}
	password, err := readPassword(stdin, stderr, a.fromStdin)
	if err != nil {
		return err
	}
	if err = errors.Join(auth.ValidateName(a.name), auth.ValidatePassword(password)); err != nil {
		return fmt.Errorf("users add: %w", err)
	}
	cfg, err := config.Load(a.configPath, config.DefaultPaths(env))
	if err != nil {
		return err //nolint:wrapcheck // config's errors already name the file and key.
	}
	if err = createUser(cfg.Storage.DataDir, a.name, password, a.role); err != nil {
		return fmt.Errorf("users add: %w", err)
	}
	kind := "user"
	if a.role == store.RoleAdmin {
		kind = "admin"
	}
	if _, err = fmt.Fprintf(stdout, "created %s %q\n", kind, a.name); err != nil {
		return fmt.Errorf("write confirmation: %w", err)
	}
	return nil
}

// createUser makes dataDir if needed, opens the database in it and adds the
// user. It never migrates: a server may be running on that database.
func createUser(dataDir, name, password string, role store.Role) (err error) {
	if err = os.MkdirAll(dataDir, dataDirMode); err != nil {
		return fmt.Errorf("create data_dir: %w", err)
	}
	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(dataDir, store.FileName), store.Options{})
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer func() {
		if closeErr := st.Close(); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("close database: %w", closeErr))
		}
	}()
	svc, err := auth.New(st, auth.Options{Params: authParams()})
	if err != nil {
		return fmt.Errorf("start auth: %w", err)
	}
	if _, err = svc.CreateUser(ctx, name, password, role); err != nil {
		return fmt.Errorf("create user: %w", err)
	}
	return nil
}

// usersPasswdArgs is the parsed command line of users passwd.
type usersPasswdArgs struct {
	configPath   string
	name         string
	fromStdin    bool
	keepSessions bool
}

// parseUsersPasswd parses the flags and the NAME of users passwd.
func parseUsersPasswd(args []string) (usersPasswdArgs, error) {
	flags := flag.NewFlagSet("users passwd", flag.ContinueOnError)
	flags.SetOutput(io.Discard) // cli prints the usage itself.
	path := flags.String("config", "", "config file")
	fromStdin := flags.Bool("password-stdin", false, "read the password from one line of stdin")
	keep := flags.Bool("keep-sessions", false, "leave the user signed in where they are")
	if err := flags.Parse(args); err != nil {
		return usersPasswdArgs{}, &usageError{msg: err.Error()}
	}
	if flags.NArg() != 1 {
		return usersPasswdArgs{}, &usageError{msg: "users passwd takes exactly one NAME"}
	}
	return usersPasswdArgs{configPath: *path, name: flags.Arg(0), fromStdin: *fromStdin, keepSessions: *keep}, nil
}

// runUsersPasswd sets a user's password and, unless -keep-sessions is given,
// signs them out everywhere. It works while a server runs on the database.
func runUsersPasswd(args []string, env config.Env, stdin io.Reader, stdout, stderr io.Writer) error {
	a, err := parseUsersPasswd(args)
	if err != nil {
		return err
	}
	cfg, err := config.Load(a.configPath, config.DefaultPaths(env))
	if err != nil {
		return err //nolint:wrapcheck // config's errors already name the file and key.
	}
	out := auth.SignOutEverywhere()
	if a.keepSessions {
		out = auth.StaySignedIn()
	}
	// The name is checked before the prompts, so a typo costs no typing.
	err = withUsers(cfg.Storage.DataDir, func(ctx context.Context, st *store.Store, svc *auth.Service) error {
		u, findErr := st.UserByName(ctx, a.name)
		if errors.Is(findErr, store.ErrNotFound) {
			return fmt.Errorf("%w named %q", errNoSuchUser, a.name)
		}
		if findErr != nil {
			return fmt.Errorf("find user: %w", findErr)
		}
		password, readErr := readPassword(stdin, stderr, a.fromStdin)
		if readErr != nil {
			return readErr
		}
		return svc.SetPassword(ctx, u.ID, password, out)
	})
	if err != nil {
		return fmt.Errorf("users passwd: %w", err)
	}
	msg := fmt.Sprintf("set a new password for %q and signed them out everywhere\n", a.name)
	if a.keepSessions {
		msg = fmt.Sprintf("set a new password for %q\n", a.name)
	}
	if _, err = io.WriteString(stdout, msg); err != nil {
		return fmt.Errorf("write confirmation: %w", err)
	}
	return nil
}

// runUsersList prints every user's name and role, one per line.
func runUsersList(args []string, env config.Env, stdout io.Writer) error {
	flags := flag.NewFlagSet("users list", flag.ContinueOnError)
	flags.SetOutput(io.Discard) // cli prints the usage itself.
	path := flags.String("config", "", "config file")
	if err := flags.Parse(args); err != nil {
		return &usageError{msg: err.Error()}
	}
	if flags.NArg() != 0 {
		return &usageError{msg: "users list takes no arguments"}
	}
	cfg, err := config.Load(*path, config.DefaultPaths(env))
	if err != nil {
		return err //nolint:wrapcheck // config's errors already name the file and key.
	}
	var users []store.Account
	err = withUsers(cfg.Storage.DataDir, func(ctx context.Context, st *store.Store, _ *auth.Service) error {
		var listErr error
		users, listErr = st.Users(ctx, time.Now())
		return listErr //nolint:wrapcheck // Wrapped once below, with the command's name.
	})
	if err != nil {
		return fmt.Errorf("users list: %w", err)
	}
	tw := tabwriter.NewWriter(stdout, 0, 0, listPadding, ' ', 0)
	for _, u := range users {
		_, _ = fmt.Fprintf(tw, "%s\t%s\n", u.User.Name, u.User.Role)
	}
	if err = tw.Flush(); err != nil {
		return fmt.Errorf("write users: %w", err)
	}
	return nil
}

// withUsers opens the existing database under dataDir and runs fn with it. It
// never migrates or creates a database: a server may be running on it.
func withUsers(dataDir string, fn func(context.Context, *store.Store, *auth.Service) error) (err error) {
	path := filepath.Join(dataDir, store.FileName)
	if _, err = os.Stat(path); errors.Is(err, fs.ErrNotExist) {
		return errNoDatabase
	}
	ctx := context.Background()
	st, err := store.Open(ctx, path, store.Options{})
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer func() {
		if closeErr := st.Close(); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("close database: %w", closeErr))
		}
	}()
	svc, err := auth.New(st, auth.Options{Params: authParams()})
	if err != nil {
		return fmt.Errorf("start auth: %w", err)
	}
	return fn(ctx, st, svc)
}

// readPassword reads the new user's password: one line of stdin when fromStdin
// is set, otherwise two no-echo prompts on the terminal that stdin is.
func readPassword(stdin io.Reader, stderr io.Writer, fromStdin bool) (string, error) {
	if fromStdin {
		line, err := bufio.NewReader(stdin).ReadString('\n')
		if err != nil && !errors.Is(err, io.EOF) {
			return "", fmt.Errorf("read password: %w", err)
		}
		return strings.TrimSuffix(strings.TrimSuffix(line, "\n"), "\r"), nil
	}
	f, ok := stdin.(*os.File)
	if !ok || !term.IsTerminal(int(f.Fd())) {
		return "", &usageError{msg: "stdin is not a terminal; pass -password-stdin to read the password from it"}
	}
	first, err := prompt(f, stderr, "Password: ")
	if err != nil {
		return "", err
	}
	second, err := prompt(f, stderr, "Repeat password: ")
	if err != nil {
		return "", err
	}
	if first != second {
		return "", errPasswordMismatch
	}
	return first, nil
}

// prompt asks for a line without echoing it.
func prompt(f *os.File, stderr io.Writer, label string) (string, error) {
	if _, err := io.WriteString(stderr, label); err != nil {
		return "", fmt.Errorf("write prompt: %w", err)
	}
	b, err := term.ReadPassword(int(f.Fd()))
	// ReadPassword swallows the newline the user typed.
	_, _ = io.WriteString(stderr, "\n")
	if err != nil {
		return "", fmt.Errorf("read password: %w", err)
	}
	return string(b), nil
}
