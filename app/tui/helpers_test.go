package tui

import (
	"testing"

	"github.com/candy-tools/todo/internal/todo"
	"github.com/candy-tools/todo/internal/todo/todotest"
	tea "github.com/charmbracelet/bubbletea"
)

// newTestModel seeds a store from the markdown src, opens it with the TUI's
// BusyTimeout into a model (closed when the test ends), and sizes the view. It
// returns the model and the store path so tests can assert on what was
// persisted.
func newTestModel(t *testing.T, src string) (model, string) {
	t.Helper()
	path := todotest.Seed(t, src)
	st, err := todo.OpenStore(path, todo.WithBusyTimeout(BusyTimeout))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	m, err := newModel(st, "")
	if err != nil {
		t.Fatalf("newModel: %v", err)
	}
	return send(m, tea.WindowSizeMsg{Width: 100, Height: 30}), path
}

// newTestModelOn is newTestModel switched to the board tab for column c, for
// tests that act on items outside the Backlog (working or done tasks).
func newTestModelOn(t *testing.T, src string, c todo.Column) (model, string) {
	t.Helper()
	m, path := newTestModel(t, src)
	m.setColumn(c)
	return m, path
}

// showOn switches m to the tab for column c and selects the row titled title,
// failing the test when that tab doesn't show it. Tests use it to follow a task
// that changed column (and so left the tab it was in).
func showOn(t *testing.T, m model, c todo.Column, title string) model {
	t.Helper()
	m.setColumn(c)
	it := find(m.doc, title)
	if it == nil || !m.tree.selectID(it.ID) {
		t.Fatalf("%q is not shown on %s (rows %v)", title, c.Title(), rowTitles(m.tree))
	}
	return m
}

// openSecond opens another handle on the store at path, standing in for a second
// process (a CLI command or an agent) writing to the same list. It is closed when
// the test ends.
func openSecond(t *testing.T, path string) *todo.Store {
	t.Helper()
	st, err := todo.OpenStore(path)
	if err != nil {
		t.Fatalf("open second store: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}

// parseDoc parses the markdown src and numbers its items 1, 2, … in document
// order, standing in for the IDs a store load assigns, so trees built from it
// can key folds by ID.
func parseDoc(t testing.TB, src string) *todo.Document {
	t.Helper()
	d := todotest.Parse(t, src)
	for i, it := range d.Enumerate() {
		it.ID = uint(i + 1)
	}
	return d
}

// send applies msg and returns the updated model.
func send(m model, msg tea.Msg) model {
	nm, _ := m.Update(msg)
	return nm.(model)
}

// press sends a keystroke by its dibs-style name ("up", "enter", "a", "A", …).
func press(m model, keys ...string) model {
	for _, k := range keys {
		m = send(m, keyMsg(k))
	}
	return m
}

// typeText inserts a run of characters into the focused text input.
func typeText(m model, s string) model {
	return send(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)})
}

func keyMsg(s string) tea.KeyMsg {
	switch s {
	case "up":
		return tea.KeyMsg{Type: tea.KeyUp}
	case "down":
		return tea.KeyMsg{Type: tea.KeyDown}
	case "left":
		return tea.KeyMsg{Type: tea.KeyLeft}
	case "right":
		return tea.KeyMsg{Type: tea.KeyRight}
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
	case "tab":
		return tea.KeyMsg{Type: tea.KeyTab}
	case "shift+tab":
		return tea.KeyMsg{Type: tea.KeyShiftTab}
	case "space", " ":
		return tea.KeyMsg{Type: tea.KeySpace}
	default:
		return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
	}
}

// find returns the first item in the document whose title matches, or nil.
func find(d *todo.Document, title string) *todo.Item {
	var out *todo.Item
	var walk func([]*todo.Item)
	walk = func(items []*todo.Item) {
		for _, it := range items {
			if it.Title == title {
				out = it
				return
			}
			walk(it.Children)
		}
	}
	walk(d.Roots)
	return out
}

// pickState sets the selected task's state through the t dialog: it opens the
// picker, puts the cursor on st and presses enter. It fails the test when the
// dialog doesn't open or doesn't offer st.
func pickState(t *testing.T, m model, st todo.Status) model {
	t.Helper()
	m = press(m, "t")
	if m.mode != modeStatePick {
		t.Fatalf("t should open the state dialog, mode = %v", m.mode)
	}
	for i, r := range m.pick.rows {
		if r.header == "" && r.state == st {
			m.pick.cursor = i
			return press(m, "enter")
		}
	}
	t.Fatalf("the state dialog doesn't offer %v", st)
	return m
}

// setTreeFilter applies a search query to a bare tree (one built in a test
// without a model), the way model.setFilter does.
func setTreeFilter(tr *tree, q string) {
	tr.vs.filter = q
	tr.cursor = 0
	tr.rebuild()
}
