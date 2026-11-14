package tui

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/candy-tools/todo/internal/todo"
)

// Launching the app shows the store's base name in the header, the open
// notice (e.g. the automatic TODO.md import summary) in the footer, and starts
// from the store's current revision.
func TestNewModelShowsNoticeAndStoreName(t *testing.T) {
	st, err := todo.OpenStore(filepath.Join(t.TempDir(), ".todo.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	notice := "imported 1 categories, 2 tasks from TODO.md"
	m, err := newModel(st, notice)
	if err != nil {
		t.Fatalf("newModel: %v", err)
	}
	if got := m.footer(200); !strings.Contains(got, notice) {
		t.Errorf("footer should show the notice %q, got %q", notice, got)
	}
	if got := m.header(80); !strings.Contains(got, ".todo.db") {
		t.Errorf("header should show the store's base name, got %q", got)
	}
	rev, err := st.Revision(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if m.lastRev != rev {
		t.Errorf("lastRev = %d, want the store's revision %d", m.lastRev, rev)
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
