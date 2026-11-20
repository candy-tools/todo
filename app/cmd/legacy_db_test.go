package cmd

import (
	"bytes"
	"errors"
	"os"
	"strings"
	"testing"
)

// oldDB is stand-in content for a .todo.db an older todo left; nothing may
// read or change it.
var oldDB = []byte("SQLite format 3\x00 not really")

// writeOldDB puts a .todo.db in the current directory.
func writeOldDB(t *testing.T) {
	t.Helper()
	if err := os.WriteFile(legacyDBFile, oldDB, 0o600); err != nil {
		t.Fatal(err)
	}
}

// assertOldDBUntouched fails unless .todo.db still holds oldDB.
func assertOldDBUntouched(t *testing.T) {
	t.Helper()
	if got, err := os.ReadFile(legacyDBFile); err != nil || !bytes.Equal(got, oldDB) {
		t.Errorf(".todo.db must be left as it was: %q, %v", got, err)
	}
}

const wantDBNotice = "found .todo.db from an older todo; it is no longer read (lists are now stored in .todo.json)"

func TestLegacyDBNoticeOnRead(t *testing.T) {
	t.Chdir(t.TempDir())
	writeOldDB(t)
	_, errOut, err := runSplit(t, "list")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if errOut != wantDBNotice+"\n" {
		t.Errorf("stderr = %q, want the one-line .todo.db notice", errOut)
	}
	if _, err := os.Stat(defaultFile); !os.IsNotExist(err) {
		t.Errorf("list must not create %s (stat err: %v)", defaultFile, err)
	}
	assertOldDBUntouched(t)
}

// Under --json a failing command's stderr is its one JSON error line: the
// .todo.db notice is left out.
func TestLegacyDBNoticeQuietUnderJSON(t *testing.T) {
	t.Chdir(t.TempDir())
	writeOldDB(t)
	code, _, stderr := runExecute("show", "--id", "nope", "--json")
	if code != exitNotFound {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
	if je := decodeJSONError(t, stderr); je.Error.Code != "not_found" {
		t.Errorf("unexpected error object: %+v", je.Error)
	}
	assertOldDBUntouched(t)
}

func TestLegacyDBNoticeOnWriteThenGone(t *testing.T) {
	t.Chdir(t.TempDir())
	writeOldDB(t)
	_, errOut, err := runSplit(t, "add-category", "--title", "Work")
	if err != nil {
		t.Fatalf("add-category: %v", err)
	}
	if errOut != wantDBNotice+"\n" {
		t.Errorf("stderr = %q, want the .todo.db notice once", errOut)
	}
	// .todo.json exists now: the notice is not repeated.
	_, errOut, err = runSplit(t, "list")
	if err != nil || errOut != "" {
		t.Errorf("second run: err=%v stderr=%q, want no notice", err, errOut)
	}
	assertOldDBUntouched(t)
}

func TestLegacyDBNoticeWithAutoImport(t *testing.T) {
	t.Chdir(t.TempDir())
	writeOldDB(t)
	if err := os.WriteFile(legacyFile, []byte("# Work\n\n- [ ] alpha\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	out, errOut, err := runSplit(t, "list")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if !contains(out, "alpha") {
		t.Errorf("TODO.md must still be imported:\n%s", out)
	}
	if !contains(errOut, wantDBNotice) || !contains(errOut, "imported 1 categories, 1 tasks from TODO.md") {
		t.Errorf("stderr = %q, want both notices", errOut)
	}
	assertOldDBUntouched(t)
}

func TestLegacyDBNoticeDefaultPathOnly(t *testing.T) {
	t.Chdir(t.TempDir())
	writeOldDB(t)
	for _, args := range [][]string{{"list", "-f", "other.json"}, {"list", "-f", defaultFile}} {
		if _, errOut, err := runSplit(t, args...); err != nil || errOut != "" {
			t.Errorf("%v: err=%v stderr=%q, want no notice for an explicit path", args, err, errOut)
		}
	}
	assertOldDBUntouched(t)
}

func TestLegacyDBNoticeForTUI(t *testing.T) {
	t.Chdir(t.TempDir())
	writeOldDB(t)
	_, notice, err := openList(defaultFile, true)
	if err != nil {
		t.Fatalf("openList: %v", err)
	}
	if notice != wantDBNotice {
		t.Errorf("notice = %q, want the .todo.db notice", notice)
	}

	if err := os.WriteFile(legacyFile, []byte("# Work\n\n- [ ] alpha\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, notice, err = openList(defaultFile, true)
	if err != nil {
		t.Fatalf("openList with TODO.md: %v", err)
	}
	if lines := strings.Split(notice, "\n"); len(lines) != 2 || lines[0] != wantDBNotice ||
		lines[1] != "imported 1 categories, 1 tasks from TODO.md" {
		t.Errorf("notice = %q, want the .todo.db notice and the import notice", notice)
	}
	assertOldDBUntouched(t)
}

func TestLegacyDBNoticeOnImport(t *testing.T) {
	t.Chdir(t.TempDir())
	writeOldDB(t)
	if err := os.WriteFile("list.md", []byte("# Work\n\n- [ ] alpha\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, errOut, err := runSplit(t, "import", "list.md")
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if errOut != wantDBNotice+"\n" {
		t.Errorf("stderr = %q, want the .todo.db notice", errOut)
	}
	assertOldDBUntouched(t)
}

// A .todo.db that cannot be stat'ed (here a symlink to itself) earns no
// notice and fails no command: the notice is only a courtesy.
func TestLegacyDBStatErrorIsIgnored(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := os.Symlink(legacyDBFile, legacyDBFile); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if _, err := os.Stat(legacyDBFile); err == nil || errors.Is(err, os.ErrNotExist) {
		t.Skipf("precondition: stat of a self-link should fail otherwise, got %v", err)
	}
	for _, args := range [][]string{{"list"}, {"add-category", "--title", "Work"}, {"import", "list.md"}} {
		if args[0] == "import" {
			if err := os.WriteFile("list.md", []byte("# Home\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.Remove(defaultFile); err != nil {
				t.Fatal(err)
			}
		}
		if _, errOut, err := runSplit(t, args...); err != nil || errOut != "" {
			t.Errorf("%v: err=%v stderr=%q, want success and no notice", args, err, errOut)
		}
	}
}
