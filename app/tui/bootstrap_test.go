package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/candy-tools/todo/internal/todo"
	"github.com/candy-tools/todo/internal/todo/todotest"
)

// Launching the app shows the store's base name in the header, the open
// notice (e.g. the automatic TODO.md import summary) in the footer, and starts
// from the fingerprint of the store's file.
func TestNewModelShowsNoticeAndStoreName(t *testing.T) {
	st := todo.NewStore(filepath.Join(t.TempDir(), ".todo.json"))
	notice := "imported 1 categories, 2 tasks from TODO.md"
	m, err := newModel(st, notice)
	if err != nil {
		t.Fatalf("newModel: %v", err)
	}
	if got := m.footer(200); !strings.Contains(got, notice) {
		t.Errorf("footer should show the notice %q, got %q", notice, got)
	}
	if got := m.header(80); !strings.Contains(got, ".todo.json") {
		t.Errorf("header should show the store's base name, got %q", got)
	}
	if !m.lastFP.IsMissing() {
		t.Errorf("lastFP = %x, want missing for a store with no file yet", m.lastFP)
	}
	if !m.tree.onPlaceholder() {
		t.Errorf("an empty store should start on the placeholder row")
	}

	// An import notice with warnings spans lines; the footer stays one line.
	m, err = newModel(st, notice+"\nline 3: skipped")
	if err != nil {
		t.Fatalf("newModel: %v", err)
	}
	if got := m.footer(200); strings.Contains(got, "\n") || !strings.Contains(got, "line 3: skipped") {
		t.Errorf("a multi-line notice should be joined onto the footer line, got %q", got)
	}
}

// Opening a list only to look at it leaves the file as it was: an older
// version without todo's note is neither upgraded nor given the note, so it
// does not show as changed and older todo binaries still read it.
func TestNewModelLeavesFileAlone(t *testing.T) {
	const v6 = `{"version": 6, "sprints": [], "categories": [{"id": "aaaaaaa", "title": "A"}], "tasks": []}`
	path := todotest.SeedJSON(t, v6)
	if _, err := newModel(todo.NewStore(path), ""); err != nil {
		t.Fatalf("newModel: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != v6 {
		t.Errorf("opening the list rewrote it:\n%s\nwant it as it was\n%s", got, v6)
	}
}
