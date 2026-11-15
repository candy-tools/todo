package tui

import (
	"strings"
	"testing"

	"github.com/candy-tools/todo/internal/todo"
	"github.com/candy-tools/todo/internal/todo/todotest"
)

// hostile is a title holding terminal escapes (a colour, an OSC window-title
// set ended by BEL, a screen clear) that an older version could have stored.
const hostile = "evil\x1b[31m\x1b]0;pwn\x07\x1b[2J"

// assertNoEscapes fails when s carries any piece of the hostile title's escapes.
func assertNoEscapes(t *testing.T, what, s string) {
	t.Helper()
	for _, bad := range []string{"\x1b[31m", "\x1b]0;", "\x07", "\x1b[2J"} {
		if strings.Contains(s, bad) {
			t.Errorf("%s carries the stored escape %q: %q", what, bad, s)
		}
	}
}

func TestStoredControlCharactersAreNotDrawn(t *testing.T) {
	path := todotest.Seed(t, "# Cat\n\n- [ ] task\n")
	db := openRaw(t, path)
	if err := db.Exec("UPDATE items SET title = title || ?", hostile).Error; err != nil {
		t.Fatalf("store raw titles: %v", err)
	}
	st, err := todo.OpenStore(path, todo.WithBusyTimeout(BusyTimeout))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	m, err := newModel(st, "")
	if err != nil {
		t.Fatalf("newModel: %v", err)
	}
	if !strings.Contains(m.doc.Roots[0].Title, "\x1b") {
		t.Fatal("the raw update should have stored an escape in the title")
	}
	for _, pane := range []paneKind{paneMixed, paneTasks} {
		tr := newTree(m.doc, newViewState())
		tr.setPane(pane)
		for i, r := range tr.rows {
			for _, sel := range []bool{false, true} {
				assertNoEscapes(t, "a tree row", tr.rowString(r, sel))
			}
			if i == 0 && pane == paneTasks && r.kind != rowHeader {
				t.Errorf("the task pane should start with the category header, got kind %v", r.kind)
			}
		}
	}
}
