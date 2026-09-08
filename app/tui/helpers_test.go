package tui

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/candy-tools/todo/internal/todo"
	tea "github.com/charmbracelet/bubbletea"
)

// newTestModel writes src to a temp file, loads it into a model, and sizes the
// view. It returns the model and the file path so tests can assert on what was
// persisted.
func newTestModel(t *testing.T, src string) (model, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "todo.md")
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatalf("seed file: %v", err)
	}
	m, err := newModel(path)
	if err != nil {
		t.Fatalf("newModel: %v", err)
	}
	return send(m, tea.WindowSizeMsg{Width: 100, Height: 30}), path
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

// readFile returns the on-disk contents of the model's file.
func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	return string(b)
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
