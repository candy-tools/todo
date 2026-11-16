package cmd

import (
	"bytes"
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
	_, notice, err := openOrImport(defaultFile, true)
	if err != nil {
		t.Fatalf("openOrImport: %v", err)
	}
	if notice != wantDBNotice {
		t.Errorf("notice = %q, want the .todo.db notice", notice)
	}

	if err := os.WriteFile(legacyFile, []byte("# Work\n\n- [ ] alpha\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, notice, err = openOrImport(defaultFile, true)
	if err != nil {
		t.Fatalf("openOrImport with TODO.md: %v", err)
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
