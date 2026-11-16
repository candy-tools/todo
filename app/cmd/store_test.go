package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
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

func TestListNoFileCreatesNothing(t *testing.T) {
	t.Chdir(t.TempDir())
	out, err := runCmd(t, "list")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if !contains(out, "  #      id  state     item") {
		t.Errorf("missing header:\n%s", out)
	}
	if _, err := os.Stat(defaultFile); !os.IsNotExist(err) {
		t.Errorf("list must not create %s (stat err: %v)", defaultFile, err)
	}
	if entries, _ := os.ReadDir("."); len(entries) != 0 {
		t.Errorf("list must create nothing, found %v", entries)
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
	if _, err := os.Stat(".todo.db"); !os.IsNotExist(err) {
		t.Errorf("the import must not create .todo.db (stat err: %v)", err)
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
	out, errOut, err := runSplit(t, "list", "-f", "other.json")
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
	if _, err := runCmd(t, "add-category", "-f", "other.json", "--title", "X"); err != nil {
		t.Fatal(err)
	}
	out, _ = runCmd(t, "list", "-f", "other.json")
	if contains(out, "alpha") {
		t.Errorf("other.json should not contain imported items:\n%s", out)
	}
}

func TestFailingWriteCreatesNoDB(t *testing.T) {
	t.Chdir(t.TempDir())
	if _, err := runCmd(t, "done", "--id", "zzzzzzz"); codeOf(t, err) != exitNotFound {
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

func TestImportNoticeEmptyImportIsReported(t *testing.T) {
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

// A store that is busy when a command opens it (another process holds the
// writer lock while the open needs it) fails every command, the TUI start
// included, with the conflict error and the message a busy write shows.
func TestOpenBusyIsConflict(t *testing.T) {
	for _, args := range [][]string{
		{"list"},
		{"done", "--title", "a"},
		nil, // the TUI
	} {
		t.Run(fmt.Sprint(args), func(t *testing.T) {
			db := todotest.Seed(t, "# Work\n\n- [ ] a\n")
			orig := openDB
			openDB = func(string, ...todo.Option) (*todo.Store, error) { return nil, todo.ErrBusy }
			t.Cleanup(func() { openDB = orig })
			_, err := runCmd(t, append(args, "-f", db)...)
			if codeOf(t, err) != exitConflict || err.Error() != todo.ErrBusy.Error() {
				t.Errorf("%v with a busy open = %v, want a conflict error with ErrBusy's message", args, err)
			}
		})
	}
}

// Reading commands work while another process holds the writer lock on a store
// already in the current format: opening it writes nothing.
func TestListWhileAnotherProcessWrites(t *testing.T) {
	db := todotest.Seed(t, "# Work\n\n- [ ] a\n")
	todotest.HoldWriteLock(t, db)
	out, err := runCmd(t, "list", "-f", db)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if !contains(out, "a") || !contains(out, "Work") {
		t.Errorf("list output misses the items:\n%s", out)
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

// A .todo.db left by an older todo is neither read nor converted.
func TestIgnoresTodoDB(t *testing.T) {
	t.Chdir(t.TempDir())
	junk := []byte("SQLite format 3\x00 not really")
	if err := os.WriteFile(".todo.db", junk, 0o600); err != nil {
		t.Fatal(err)
	}
	out, err := runCmd(t, "list")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if !contains(out, "  #      id  state     item") {
		t.Errorf("want the empty list:\n%s", out)
	}
	if got, _ := os.ReadFile(".todo.db"); !bytes.Equal(got, junk) {
		t.Error(".todo.db must stay untouched")
	}
}

// The automatic TODO.md import writes the JSON list, not a database.
func TestAutoImportWritesJSON(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := os.WriteFile(legacyFile, []byte("# Work\n\n- [ ] alpha\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := runCmd(t, "list"); err != nil {
		t.Fatal(err)
	}
	raw := bytesOf(t, ".todo.json")
	if !contains(string(raw), `"version": 3`) || !contains(string(raw), `"title": "alpha"`) {
		t.Errorf(".todo.json is not the JSON list:\n%s", raw)
	}
	if _, err := os.Stat(".todo.db"); !os.IsNotExist(err) {
		t.Errorf(".todo.db must not exist (stat err: %v)", err)
	}
}

// An invalid list file fails every command with exit 1 and the validation
// message, and is never rewritten.
func TestInvalidFileExitsOne(t *testing.T) {
	t.Chdir(t.TempDir())
	bad := `{
  "version": 3,
  "categories": [],
  "tasks": [
    {"id": "aaaaaaa", "category": "bbbbbbb", "title": "x", "status": "open"}
  ]
}
`
	if err := os.WriteFile(defaultFile, []byte(bad), 0o600); err != nil {
		t.Fatal(err)
	}
	_, _, err := runSplit(t, "add", "--title", "y", "--parent-title", "x")
	if codeOf(t, err) != exitGeneric {
		t.Fatalf("add on an invalid list = %v, want exit 1", err)
	}
	if want := `task aaaaaaa: category "bbbbbbb" does not exist`; !contains(err.Error(), want) {
		t.Errorf("error %q does not name %q", err, want)
	}
	if got := bytesOf(t, defaultFile); string(got) != bad {
		t.Errorf("the invalid file was rewritten:\n%s", got)
	}
}

// The store's invalid-list and newer-version errors reach the user as exit 1
// with the store's message.
func TestStoreInvalidListIsGeneric(t *testing.T) {
	t.Chdir(t.TempDir())
	for _, want := range []error{todo.ErrInvalidList, todo.ErrNewerSchema} {
		c := newRootCommand()
		err := withStore(c, func(context.Context, storeOpener, *todo.Document) error { return want })
		if codeOf(t, err) != exitGeneric || err.Error() != want.Error() {
			t.Errorf("withStore with %v = %v, want exit 1 with its message", want, err)
		}
	}
}

// --id takes the 7-character ID shown by list --json.
func TestIDSelectorString(t *testing.T) {
	path := todotest.Seed(t, "# Work\n\n- [ ] a\n- [ ] b\n")
	out, err := runCmd(t, "list", "--file", path, "--json")
	if err != nil {
		t.Fatal(err)
	}
	var items []jsonItem
	if err := json.Unmarshal([]byte(out), &items); err != nil {
		t.Fatal(err)
	}
	id := items[0].Children[1].ID
	if len(id) != 7 {
		t.Fatalf("want a 7-character id, got %q", id)
	}
	if _, err := runCmd(t, "done", "--file", path, "--id", id); err != nil {
		t.Fatalf("done --id %s: %v", id, err)
	}
	if got := todotest.Dump(t, path); !contains(got, "- [x] b") || !contains(got, "- [ ] a") {
		t.Errorf("only b should be done:\n%s", got)
	}
	if _, err := runCmd(t, "done", "--file", path, "--id", "zzzzzzz"); codeOf(t, err) != exitNotFound {
		t.Error("an unknown --id should be not-found")
	}
}
