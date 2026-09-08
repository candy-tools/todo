package todo_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/candy-tools/todo/internal/todo"
)

// EnsureFile bootstraps a missing file so `todo` (with no argument, or a new
// filename) starts from a real file on disk instead of only writing one on the
// first edit. The created file is a valid, empty todo file: no tasks, but the
// managed guide is present.
func TestEnsureFileCreatesMissingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "TODO.md")
	if err := todo.EnsureFile(path); err != nil {
		t.Fatalf("EnsureFile: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected the file to be created, stat: %v", err)
	}
	d, err := todo.Load(path)
	if err != nil {
		t.Fatalf("load bootstrapped file: %v", err)
	}
	if len(d.Roots) != 0 {
		t.Errorf("a bootstrapped file must have no tasks, got %d roots", len(d.Roots))
	}
	if got := readTestFile(t, path); !strings.Contains(got, "<!-- todo:guide") {
		t.Errorf("a bootstrapped file must carry the managed guide, got:\n%s", got)
	}
}

// An existing file must never be clobbered by the bootstrap: EnsureFile only
// creates a file that is missing, so a user's tasks are safe on every launch.
func TestEnsureFileLeavesExistingFileUntouched(t *testing.T) {
	path := filepath.Join(t.TempDir(), "TODO.md")
	const existing = "# Work\n\n- [ ] keep me\n"
	if err := os.WriteFile(path, []byte(existing), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := todo.EnsureFile(path); err != nil {
		t.Fatalf("EnsureFile: %v", err)
	}
	if got := readTestFile(t, path); got != existing {
		t.Errorf("EnsureFile must not modify an existing file\n got: %q\nwant: %q", got, existing)
	}
}
