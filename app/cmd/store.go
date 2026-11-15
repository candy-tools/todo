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

// openDB opens an existing store or creates it; tests replace it to observe
// how often a command opens the database.
var openDB = todo.OpenStore

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

// openStore opens the store at path with opts, creating it when missing. When
// isDefault and the store does not exist but TODO.md does, it imports TODO.md
// instead and returns the import notice; otherwise the notice is "". When
// another process creates the store first (it ran the same first-run import),
// that store is opened as is, without a notice.
func openStore(_ *cobra.Command, path string, isDefault bool, opts ...todo.Option) (*todo.Store, string, error) {
	imp, err := legacyImportable(path, isDefault)
	if err != nil {
		return nil, "", err
	}
	if imp {
		s, rep, err := todo.ImportFile(path, legacyFile, false, opts...)
		if errors.Is(err, todo.ErrNotEmpty) {
			// The store appeared, already filled, after legacyImportable looked.
			s, err := openDB(path, opts...)
			return s, "", err
		}
		if err != nil {
			return nil, "", err
		}
		return s, importNotice(rep, legacyFile), nil
	}
	s, err := openDB(path, opts...)
	return s, "", err
}

// openForRead loads the document without creating anything: with no store and
// nothing to import it returns an empty document. An automatic import notice is
// written to stderr.
func openForRead(cmd *cobra.Command, path string, isDefault bool) (*todo.Document, error) {
	s, doc, err := loadStore(cmd, path, isDefault)
	if s != nil {
		_ = s.Close()
	}
	return doc, err
}

// loadStore is openForRead that keeps the store open: it returns the open store
// and its document, or a nil store and an empty document when there is no store
// and nothing to import. The caller closes a non-nil store.
func loadStore(cmd *cobra.Command, path string, isDefault bool) (*todo.Store, *todo.Document, error) {
	exists, err := todo.StoreExists(path)
	if err != nil {
		return nil, nil, err
	}
	if !exists {
		if imp, err := legacyImportable(path, isDefault); err != nil || !imp {
			return nil, &todo.Document{}, err
		}
	}
	s, notice, err := openStore(cmd, path, isDefault)
	if err != nil {
		return nil, nil, err
	}
	printNotice(cmd, notice)
	doc, _, err := s.Load(storeCtx(cmd))
	if err != nil {
		_ = s.Close()
		return nil, nil, err
	}
	return s, doc, nil
}

// storeOpener returns the store a write command writes to: the one its
// document was loaded from, or, when there was none, a new one created on the
// first call.
type storeOpener func() (*todo.Store, error)

// withStore loads the document the command resolves to (running the automatic
// import and printing its notice to stderr, like openForRead) and runs fn on
// it. open returns the store the document was loaded from; only when there was
// none does it create one, and fn calls open only once it is about to write, so
// a command that fails earlier — a selector that matches nothing, a usage error
// — never creates the store. The store is closed afterwards. A store
// ErrNotFound becomes the CLI's not-found error, a store ErrWrongParent or
// ErrNotTask its wrong-kind one, and a store ErrConflict or ErrBusy its conflict
// one.
func withStore(cmd *cobra.Command, fn func(ctx context.Context, open storeOpener, doc *todo.Document) error) error {
	path, isDefault, err := resolvePath(cmd, nil)
	if err != nil {
		return err
	}
	s, doc, err := loadStore(cmd, path, isDefault)
	if err != nil {
		return err
	}
	defer func() {
		if s != nil {
			_ = s.Close()
		}
	}()
	open := func() (*todo.Store, error) {
		if s != nil {
			return s, nil
		}
		st, err := openDB(path)
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
		if errors.Is(err, todo.ErrTooDeep) || errors.Is(err, todo.ErrSubtaskStatus) {
			return usageErr(err.Error())
		}
		if errors.Is(err, todo.ErrWrongParent) || errors.Is(err, todo.ErrNotTask) {
			return &cliError{exitWrongKind, err.Error()}
		}
		if errors.Is(err, todo.ErrConflict) || errors.Is(err, todo.ErrBusy) {
			return &cliError{exitConflict, err.Error()}
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
