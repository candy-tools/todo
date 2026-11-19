package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

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
		t.Error("the default list file must not be created")
	}
	// A write on the explicit path starts an empty list too.
	if _, err := runCmd(t, "add-category", "-f", "other.json", "--title", "X"); err != nil {
		t.Fatal(err)
	}
	out, _ = runCmd(t, "list", "-f", "other.json")
	if contains(out, "alpha") {
		t.Errorf("other.json should not contain imported items:\n%s", out)
	}
}

func TestFailingWriteCreatesNoList(t *testing.T) {
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

// Reading commands work while another process holds the list's write lock: a
// read takes no lock.
func TestListWhileAnotherProcessWrites(t *testing.T) {
	path := todotest.Seed(t, "# Work\n\n- [ ] a\n")
	todotest.HoldWriteLock(t, path)
	out, err := runCmd(t, "list", "-f", path)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if !contains(out, "a") || !contains(out, "Work") {
		t.Errorf("list output misses the items:\n%s", out)
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

// The automatic TODO.md import writes the JSON list, and no .todo.db.
func TestAutoImportWritesJSON(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := os.WriteFile(legacyFile, []byte("# Work\n\n- [ ] alpha\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := runCmd(t, "list"); err != nil {
		t.Fatal(err)
	}
	raw := bytesOf(t, ".todo.json")
	if !contains(string(raw), `"version": 6`) || !contains(string(raw), `"title": "alpha"`) {
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

// A store error reaches the user by its class: every member of a class gets
// the class's exit code and keeps its message, wrapped or not.
func TestStoreErrorsMapByClass(t *testing.T) {
	refused := fmt.Errorf("refused to write an invalid list: %w", todo.ErrInternal)
	for _, c := range []struct {
		err  error
		code int
		msg  string
	}{
		{todo.ErrNotFound, exitNotFound, "the item no longer exists"},
		{fmt.Errorf("item x: %w", todo.ErrNotFound), exitNotFound, "the item no longer exists"},
		{todo.ErrTooDeep, exitUsage, todo.ErrTooDeep.Error()},
		{todo.ErrSubtaskStatus, exitUsage, todo.ErrSubtaskStatus.Error()},
		{todo.ErrInvalidMove, exitUsage, todo.ErrInvalidMove.Error()},
		{fmt.Errorf("title: %w", todo.ErrInvalidTitle), exitUsage, "title: " + todo.ErrInvalidTitle.Error()},
		{todo.ErrRestoreAndCascade, exitUsage, todo.ErrRestoreAndCascade.Error()},
		{todo.ErrWrongParent, exitWrongKind, todo.ErrWrongParent.Error()},
		{todo.ErrNotTask, exitWrongKind, todo.ErrNotTask.Error()},
		{todo.ErrConflict, exitConflict, todo.ErrConflict.Error()},
		{todo.ErrBusy, exitConflict, todo.ErrBusy.Error()},
		{todo.ErrInvalidList, exitGeneric, todo.ErrInvalidList.Error()},
		{fmt.Errorf("x.json: %w", todo.ErrNewerFormat), exitGeneric, "x.json: " + todo.ErrNewerFormat.Error()},
		{todo.ErrListMissing, exitGeneric, todo.ErrListMissing.Error()},
		{refused, exitGeneric, refused.Error()},
	} {
		t.Run(c.err.Error(), func(t *testing.T) {
			t.Chdir(t.TempDir())
			err := withStore(newRootCommand(), func(context.Context, *todo.Store, *todo.Document) error { return c.err })
			if codeOf(t, err) != c.code || err.Error() != c.msg {
				t.Errorf("withStore with %v = %v (exit %d), want exit %d with %q", c.err, err, codeOf(t, err), c.code, c.msg)
			}
		})
	}
}

// The first-run import of TODO.md waits for the list's write lock like any
// write: when another process holds it past the timeout, opening the list
// fails with the conflict exit and the busy message, and creates no list.
func TestFirstRunImportBusyIsConflict(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := os.WriteFile(legacyFile, []byte("# Work\n\n- [ ] alpha\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	todotest.HoldWriteLock(t, defaultFile)
	_, _, err := openList(defaultFile, true, todo.WithBusyTimeout(time.Millisecond))
	if codeOf(t, err) != exitConflict || err.Error() != todo.ErrBusy.Error() {
		t.Errorf("first-run import on a busy list = %v, want a conflict error with the busy message", err)
	}
	if _, err := os.Stat(defaultFile); !os.IsNotExist(err) {
		t.Errorf("a busy import must not create %s (stat err: %v)", defaultFile, err)
	}
}
