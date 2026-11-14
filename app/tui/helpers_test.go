package tui

import (
	"testing"

	"github.com/candy-tools/todo/internal/todo"
	"github.com/candy-tools/todo/internal/todo/todotest"
	tea "github.com/charmbracelet/bubbletea"
)

// newTestModel seeds a store from the markdown src, opens it into a model (closed
// when the test ends), and sizes the view. It returns the model and the store
// path so tests can assert on what was persisted.
func newTestModel(t *testing.T, src string) (model, string) {
	t.Helper()
	path := todotest.Seed(t, src)
	st, err := todo.OpenStore(path)
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
