package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/term"

	"github.com/simonhull/kitsune/internal/auth"
	"github.com/simonhull/kitsune/internal/config"
	"github.com/simonhull/kitsune/internal/store"
)

// dataDirMode is the mode of a data_dir that users add has to create.
const dataDirMode = 0o700

// errPasswordMismatch means the two password prompts disagreed.
var errPasswordMismatch = errors.New("passwords do not match")

// authParams returns the Argon2id parameters users add hashes with. Tests
// replace it with cheap ones.
var authParams = auth.DefaultParams //nolint:gochecknoglobals // Test hook so tests can hash cheaply; plan 008 prescribes it.

// runUsers dispatches the users subcommands.
func runUsers(args []string, env config.Env, stdin io.Reader, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		return &usageError{msg: "users needs a subcommand: add"}
	}
	switch sub, rest := args[0], args[1:]; sub {
	case "add":
		return runUsersAdd(rest, env, stdin, stdout, stderr)
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

// createUser makes dataDir if needed, opens the database in it and adds the user.
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
