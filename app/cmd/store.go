package cmd

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/candy-tools/todo/internal/todo"
	"github.com/spf13/cobra"
)

const (
	// defaultFile is the SQLite store used when no path is given.
	defaultFile = ".todo.db"
	// legacyFile is the markdown file imported automatically on first run.
	legacyFile = "TODO.md"
)

// resolvePath returns the store a command operates on: the positional argument,
// else the --file flag, else defaultFile. isDefault reports that none of them
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
// line per warning. A skipped import (another process created the store first)
// has no notice.
func importNotice(rep todo.ImportReport, from string) string {
	if rep.Skipped {
		return ""
	}
	lines := []string{fmt.Sprintf("imported %d categories, %d tasks from %s", rep.Categories, rep.Tasks, from)}
	return strings.Join(append(lines, rep.Warnings...), "\n")
}

// legacyImportable reports whether the store at path is missing while the
// default path is in use and a TODO.md exists to import.
func legacyImportable(path string, isDefault bool) (bool, error) {
	if !isDefault {
		return false, nil
	}
	exists, err := todo.StoreExists(path)
	if err != nil || exists {
		return false, err
	}
	return todo.StoreExists(legacyFile)
}

// openStore opens the store at path, creating it when missing. When isDefault
// and the store does not exist but TODO.md does, it imports TODO.md instead and
// returns the import notice; otherwise the notice is "". When another process
// creates the store first (it ran the same first-run import), that store is
// opened as is, without a notice.
func openStore(_ *cobra.Command, path string, isDefault bool) (*todo.Store, string, error) {
	imp, err := legacyImportable(path, isDefault)
	if err != nil {
		return nil, "", err
	}
	if imp {
		s, rep, err := todo.ImportFile(path, legacyFile, false)
		if errors.Is(err, todo.ErrNotEmpty) {
			// The store appeared, already filled, after legacyImportable looked.
			s, err := todo.OpenStore(path)
			return s, "", err
		}
		if err != nil {
			return nil, "", err
		}
		return s, importNotice(rep, legacyFile), nil
	}
	s, err := todo.OpenStore(path)
	return s, "", err
}

// openForRead loads the document without creating anything: with no store and
// nothing to import it returns an empty document. An automatic import notice is
// written to stderr.
func openForRead(cmd *cobra.Command, path string, isDefault bool) (*todo.Document, error) {
	exists, err := todo.StoreExists(path)
	if err != nil {
		return nil, err
	}
	if !exists {
		if imp, err := legacyImportable(path, isDefault); err != nil || !imp {
			return &todo.Document{}, err
		}
	}
	s, notice, err := openStore(cmd, path, isDefault)
	if err != nil {
		return nil, err
	}
	defer func() { _ = s.Close() }()
	printNotice(cmd, notice)
	doc, _, err := s.Load(storeCtx(cmd))
	return doc, err
}

// storeOpener returns the store a write command writes to, opening (and, when
// it is missing, creating) it on the first call.
type storeOpener func() (*todo.Store, error)

// withStore loads the document the command resolves to (running the automatic
// import and printing its notice to stderr, like openForRead) and runs fn on
// it. fn calls open only once it is about to write, so a command that fails
// earlier — a selector that matches nothing, a usage error — never creates the
// store. The store is closed afterwards. A store ErrNotFound becomes the CLI's
// not-found error.
func withStore(cmd *cobra.Command, fn func(ctx context.Context, open storeOpener, doc *todo.Document) error) error {
	path, isDefault, err := resolvePath(cmd, nil)
	if err != nil {
		return err
	}
	doc, err := openForRead(cmd, path, isDefault)
	if err != nil {
		return err
	}
	var s *todo.Store
	defer func() {
		if s != nil {
			_ = s.Close()
		}
	}()
	open := func() (*todo.Store, error) {
		if s != nil {
			return s, nil
		}
		st, err := todo.OpenStore(path)
		if err != nil {
			return nil, err
		}
		s = st
		return s, nil
	}
	if err := fn(storeCtx(cmd), open, doc); err != nil {
		if errors.Is(err, todo.ErrNotFound) {
			return notFound("the item no longer exists")
		}
		return err
	}
	return nil
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
