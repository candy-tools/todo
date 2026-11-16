package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/candy-tools/todo/internal/todo"
	"github.com/spf13/cobra"
)

const (
	// defaultFile is the JSON list file used when no path is given.
	defaultFile = ".todo.json"
	// legacyFile is the markdown file imported automatically on first run.
	legacyFile = "TODO.md"
	// legacyDBFile is the SQLite list file older versions used by default. It
	// is never read, converted or removed; its presence only earns a notice.
	legacyDBFile = ".todo.db"
)

// fileExists reports whether a file exists at path.
func fileExists(path string) (bool, error) {
	_, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// resolvePath returns the list file a command operates on: the positional
// argument, else the --file flag, else defaultFile. isDefault reports that none of them
// was given, which is what allows the automatic TODO.md import. A markdown path
// is a usage error that points at `todo import`.
func resolvePath(cmd *cobra.Command, args []string) (path string, isDefault bool, err error) {
	switch {
	case len(args) > 0:
		path = args[0]
	case cmd.Flags().Changed("file"):
		path, _ = cmd.Flags().GetString("file")
	default:
		path, isDefault = defaultFile, true
	}
	if strings.HasSuffix(strings.ToLower(path), ".md") {
		return "", false, usageErr(fmt.Sprintf("%s is a markdown file; import it once with: todo import %s", path, path))
	}
	return path, isDefault, nil
}

// importNotice formats the result of an import: a summary line followed by one
// line per warning.
func importNotice(rep todo.ImportReport, from string) string {
	lines := []string{fmt.Sprintf("imported %d categories, %d tasks from %s", rep.Categories, rep.Tasks, from)}
	return strings.Join(append(lines, rep.Warnings...), "\n")
}

// firstRun reports whether the default list file is in use and does not
// exist yet: only then is TODO.md imported automatically and a .todo.db
// mentioned.
func firstRun(path string, isDefault bool) (bool, error) {
	if !isDefault {
		return false, nil
	}
	exists, err := fileExists(path)
	if err != nil {
		return false, err
	}
	return !exists, nil
}

// legacyImportable reports whether, on a first run (see firstRun), a TODO.md
// exists to import.
func legacyImportable(first bool) (bool, error) {
	if !first {
		return false, nil
	}
	return fileExists(legacyFile)
}

// legacyDBNotice returns the notice that a .todo.db an older todo left is not
// read, on a first run (see firstRun) when a .todo.db exists; otherwise "".
// The .todo.db is only stat'ed, and one that cannot be stat'ed earns no
// notice: the notice is a courtesy, never a reason to fail.
func legacyDBNotice(first bool) string {
	if !first {
		return ""
	}
	if old, err := fileExists(legacyDBFile); err != nil || !old {
		return ""
	}
	return fmt.Sprintf("found %s from an older todo; it is no longer read (lists are now stored in %s)", legacyDBFile, defaultFile)
}

// joinNotices joins the non-empty notices, one per line.
func joinNotices(notices ...string) string {
	var lines []string
	for _, n := range notices {
		if n != "" {
			lines = append(lines, n)
		}
	}
	return strings.Join(lines, "\n")
}

// openList returns the store of the list file at path; making it does no I/O,
// and the first write creates the file. When isDefault and the file does not
// exist but TODO.md does, it imports TODO.md into it first and returns the
// import notice, after the .todo.db notice (see legacyDBNotice) when that
// applies; otherwise the notice is that one or "". When another process
// creates the list first (it ran the same first-run import), that list is used
// as is, without a notice. An import that fails, say on a list busy past the
// timeout in opts, fails with the CLI error of its class (see storeErr).
func openList(path string, isDefault bool, opts ...todo.Option) (*todo.Store, string, error) {
	first, err := firstRun(path, isDefault)
	if err != nil {
		return nil, "", err
	}
	dbNote := legacyDBNotice(first)
	imp, err := legacyImportable(first)
	if err != nil {
		return nil, "", err
	}
	if !imp {
		return todo.NewStore(path, opts...), dbNote, nil
	}
	s, rep, err := todo.ImportFile(path, legacyFile, false, opts...)
	switch {
	case errors.Is(err, todo.ErrNotEmpty):
		// The list appeared, already filled, after legacyImportable looked.
		return todo.NewStore(path, opts...), "", nil
	case err != nil:
		return nil, "", storeErr(err)
	}
	return s, joinNotices(dbNote, importNotice(rep, legacyFile)), nil
}

// loadList returns the store of the list file at path and the list it holds,
// running the automatic import first (see openList) and printing its notice,
// and the .todo.db notice, to stderr. A missing file is an empty list, and
// nothing is created unless TODO.md is imported.
func loadList(cmd *cobra.Command, path string, isDefault bool) (*todo.Store, *todo.Document, error) {
	s, notice, err := openList(path, isDefault)
	if err != nil {
		return nil, nil, err
	}
	printNotice(cmd, notice)
	doc, _, err := s.Load(storeCtx(cmd))
	if err != nil {
		return nil, nil, storeErr(err)
	}
	return s, doc, nil
}

// withStore loads the list the command resolves to (see loadList) and runs fn
// on its store and document. A write fn makes goes to that store, and only a
// write that succeeds creates a missing file, so a command that fails before
// it writes (a selector that matches nothing, a usage error) creates nothing.
// A store error fn returns becomes the CLI error of its class (see storeErr).
func withStore(cmd *cobra.Command, fn func(ctx context.Context, s *todo.Store, doc *todo.Document) error) error {
	path, isDefault, err := resolvePath(cmd, nil)
	if err != nil {
		return err
	}
	s, doc, err := loadList(cmd, path, isDefault)
	if err != nil {
		return err
	}
	return storeErr(fn(storeCtx(cmd), s, doc))
}

// storeErr returns a store error as the CLI error of its class (see the todo
// package's error classes), keeping the store's message: ErrNotFound is the
// not-found exit ("the item no longer exists": the command found it when it
// read the list), ErrBadRequest the usage exit, ErrWrongKind the wrong-kind
// exit and ErrConflict (busy included) the conflict exit, as re-running the
// command is the fix. ErrInvalidList (a file this todo cannot read),
// ErrListMissing and ErrInternal (a refused write, a bug in todo) are a
// generic failure, exit 1. Any other error, a CLI error among them, is
// returned unchanged; nil stays nil.
func storeErr(err error) error {
	code := 0
	switch {
	case err == nil:
		return nil
	case errors.Is(err, todo.ErrNotFound):
		return notFound("the item no longer exists")
	case errors.Is(err, todo.ErrBadRequest):
		code = exitUsage
	case errors.Is(err, todo.ErrWrongKind):
		code = exitWrongKind
	case errors.Is(err, todo.ErrConflict):
		code = exitConflict
	case errors.Is(err, todo.ErrInvalidList), errors.Is(err, todo.ErrListMissing), errors.Is(err, todo.ErrInternal):
		code = exitGeneric
	default:
		return err
	}
	return &cliError{code, err.Error()}
}

// printNotice writes a non-empty notice to stderr.
func printNotice(cmd *cobra.Command, notice string) {
	if notice != "" {
		_, _ = fmt.Fprintln(cmd.ErrOrStderr(), notice)
	}
}

// storeCtx is the context for store calls.
func storeCtx(cmd *cobra.Command) context.Context {
	if ctx := cmd.Context(); ctx != nil {
		return ctx
	}
	return context.Background()
}
