package cmd

import (
	"bytes"
	"context"
	"os"
	"strings"
	"testing"

	"github.com/candy-tools/todo/internal/todo"
	"github.com/candy-tools/todo/internal/todo/todotest"
)

// runSplit runs the root command capturing stdout and stderr separately.
func runSplit(t *testing.T, args ...string) (stdout, stderr string, err error) {
	t.Helper()
	c := newRootCommand()
	var out, errb bytes.Buffer
	c.SetOut(&out)
	c.SetErr(&errb)
	c.SetArgs(args)
	err = c.Execute()
	return out.String(), errb.String(), err
}

func contains(s, sub string) bool { return strings.Contains(s, sub) }

func TestListMissingDBCreatesNothing(t *testing.T) {
	t.Chdir(t.TempDir())
	out, err := runCmd(t, "list")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if !contains(out, "  #    id  state     item") {
		t.Errorf("missing header:\n%s", out)
	}
	if _, err := os.Stat(defaultFile); !os.IsNotExist(err) {
		t.Errorf("list must not create %s (stat err: %v)", defaultFile, err)
	}
}

func TestAutoImportOnFirstRun(t *testing.T) {
	t.Chdir(t.TempDir())
	md := []byte("# Work\n\n- [ ] alpha\n")
	if err := os.WriteFile(legacyFile, md, 0o600); err != nil {
		t.Fatal(err)
	}
	out, errOut, err := runSplit(t, "list")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if !contains(out, "alpha") {
		t.Errorf("imported items missing:\n%s", out)
	}
	if !contains(errOut, "imported 1 categories, 1 tasks from TODO.md") {
		t.Errorf("stderr notice missing: %q", errOut)
	}
	if _, err := os.Stat(defaultFile); err != nil {
		t.Errorf("%s should exist: %v", defaultFile, err)
	}
	if got, _ := os.ReadFile(legacyFile); !bytes.Equal(got, md) {
		t.Errorf("TODO.md must be unchanged, got %q", got)
	}
	_, errOut, err = runSplit(t, "list")
	if err != nil || errOut != "" {
		t.Errorf("second run: err=%v stderr=%q, want no notice", err, errOut)
	}
}

func TestAutoImportOnlyForDefaultPath(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := os.WriteFile(legacyFile, []byte("# Work\n\n- [ ] alpha\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	out, errOut, err := runSplit(t, "list", "-f", "other.db")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if contains(out, "alpha") || errOut != "" {
		t.Errorf("no import expected: out=%q err=%q", out, errOut)
	}
	if _, err := os.Stat(defaultFile); !os.IsNotExist(err) {
		t.Error("the default db must not be created")
	}
	// A write on the explicit path starts an empty db too.
	if _, err := runCmd(t, "add-category", "-f", "other.db", "--title", "X"); err != nil {
		t.Fatal(err)
	}
	out, _ = runCmd(t, "list", "-f", "other.db")
	if contains(out, "alpha") {
		t.Errorf("other.db should not contain imported items:\n%s", out)
	}
}

func TestFailingWriteCreatesNoDB(t *testing.T) {
	t.Chdir(t.TempDir())
	if _, err := runCmd(t, "done", "--id", "3"); codeOf(t, err) != exitNotFound {
		t.Errorf("done on an empty dir: want not-found, got %v", err)
	}
	if _, err := runCmd(t, "add", "--title", "x", "--parent-title", "Work"); codeOf(t, err) != exitNotFound {
		t.Errorf("add under a missing parent: want not-found, got %v", err)
	}
	if _, err := runCmd(t, "add-category"); codeOf(t, err) != exitUsage {
		t.Errorf("add-category without a title: want usage, got %v", err)
	}
	if _, err := os.Stat(defaultFile); !os.IsNotExist(err) {
		t.Errorf("a failing write must not create %s (stat err: %v)", defaultFile, err)
	}
	// A write that succeeds on the empty list does create it.
	if _, err := runCmd(t, "add-category", "--title", "Work"); err != nil {
		t.Fatalf("add-category: %v", err)
	}
	if _, err := os.Stat(defaultFile); err != nil {
		t.Errorf("%s should exist after a successful write: %v", defaultFile, err)
	}
}

func TestImportNoticeSkippedIsEmpty(t *testing.T) {
	if got := importNotice(todo.ImportReport{Skipped: true}, legacyFile); got != "" {
		t.Errorf("a skipped import must have no notice, got %q", got)
	}
	if got := importNotice(todo.ImportReport{}, legacyFile); got != "imported 0 categories, 0 tasks from TODO.md" {
		t.Errorf("an empty import must still be reported, got %q", got)
	}
}

func TestAddCategoryAutoImportsFirst(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := os.WriteFile(legacyFile, []byte("# Work\n\n- [ ] alpha\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	out, errOut, err := runSplit(t, "add-category", "--title", "Sub", "--parent-title", "Work")
	if err != nil {
		t.Fatal(err)
	}
	if out != "add-category: ## Sub\n" || !contains(errOut, "imported") {
		t.Errorf("out=%q err=%q", out, errOut)
	}
}

// A tree rule the store enforces itself (one a race got past the command's own
// kind checks) surfaces as the CLI's wrong-kind error.
func TestStoreTreeRuleIsWrongKind(t *testing.T) {
	t.Chdir(t.TempDir())
	for _, want := range []error{todo.ErrWrongParent, todo.ErrNotTask} {
		c := newRootCommand()
		err := withStore(c, func(context.Context, storeOpener, *todo.Document) error { return want })
		if codeOf(t, err) != exitWrongKind || err.Error() != want.Error() {
			t.Errorf("withStore with %v = %v, want a wrong-kind error with its message", want, err)
		}
	}
}

// A store conflict (the tree changed under the command since it read it)
// surfaces as the CLI's conflict error with the store's message.
func TestStoreConflictIsConflict(t *testing.T) {
	t.Chdir(t.TempDir())
	c := newRootCommand()
	err := withStore(c, func(context.Context, storeOpener, *todo.Document) error { return todo.ErrConflict })
	if codeOf(t, err) != exitConflict || err.Error() != todo.ErrConflict.Error() {
		t.Errorf("withStore with ErrConflict = %v, want a conflict error with its message", err)
	}
}

// A store invalid move (an item placed into its own subtree) surfaces as the
// CLI's usage error with the store's message, not a conflict: re-running the
// command would fail the same way.
func TestStoreInvalidMoveIsUsage(t *testing.T) {
	t.Chdir(t.TempDir())
	c := newRootCommand()
	err := withStore(c, func(context.Context, storeOpener, *todo.Document) error { return todo.ErrInvalidMove })
	if codeOf(t, err) != exitUsage || err.Error() != todo.ErrInvalidMove.Error() {
		t.Errorf("withStore with ErrInvalidMove = %v, want a usage error with its message", err)
	}
}

// A busy store (another process held the writer lock past the busy timeout)
// surfaces as the CLI's conflict error with the store's message: re-running
// the command is the fix for both.
func TestStoreBusyIsConflict(t *testing.T) {
	t.Chdir(t.TempDir())
	c := newRootCommand()
	err := withStore(c, func(context.Context, storeOpener, *todo.Document) error { return todo.ErrBusy })
	if codeOf(t, err) != exitConflict || err.Error() != todo.ErrBusy.Error() {
		t.Errorf("withStore with ErrBusy = %v, want a conflict error with its message", err)
	}
}

// Each write command opens the database once and writes through the handle it
// loaded from, rather than reopening it for the write.
func TestWriteCommandsOpenTheStoreOnce(t *testing.T) {
	for _, args := range [][]string{
		{"done", "--title", "a"},
		{"status", "--title", "a", "--to", "review"},
		{"add", "--parent-title", "Work", "--title", "c"},
		{"add-category", "--title", "Other"},
		{"edit", "--title", "a", "--set-title", "A"},
		{"rm", "--title", "b"},
		{"prune"},
	} {
		t.Run(args[0], func(t *testing.T) {
			db := todotest.Seed(t, "# Work\n\n- [ ] a\n- [x] b\n")
			opens := 0
			orig := openDB
			openDB = func(path string, opts ...todo.Option) (*todo.Store, error) {
				opens++
				return orig(path, opts...)
			}
			t.Cleanup(func() { openDB = orig })
			if _, err := runCmd(t, append(args, "-f", db)...); err != nil {
				t.Fatalf("%v: %v", args, err)
			}
			if opens != 1 {
				t.Errorf("%v opened the store %d times, want 1", args, opens)
			}
		})
	}
}
