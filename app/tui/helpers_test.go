package tui

import (
	"strconv"
	"testing"

	"github.com/candy-tools/todo/internal/todo"
	"github.com/candy-tools/todo/internal/todo/todotest"
	tea "github.com/charmbracelet/bubbletea"
)

// newTestModel seeds a store from the markdown src, opens it with the TUI's
// BusyTimeout into a model, and sizes the view. It returns the model and the
// store path so tests can assert on what was persisted.
func newTestModel(t *testing.T, src string) (model, string) {
	t.Helper()
	path := todotest.Seed(t, src)
	return modelOn(t, path), path
}

// newJSONModel is newTestModel seeded from a list file's JSON, for a test
// that starts from kanban states or plans markdown cannot express.
func newJSONModel(t *testing.T, json string) (model, string) {
	t.Helper()
	path := todotest.SeedJSON(t, json)
	return modelOn(t, path), path
}

// modelOn opens the list at path with the TUI's BusyTimeout into a model and
// sizes the view.
func modelOn(t *testing.T, path string) model {
	t.Helper()
	st := todo.NewStore(path, todo.WithBusyTimeout(BusyTimeout))
	m, err := newModel(st, "")
	if err != nil {
		t.Fatalf("newModel: %v", err)
	}
	return send(m, tea.WindowSizeMsg{Width: 100, Height: 30})
}

// importedID is the ID of the Imported sprint of m's list: the sprint a
// markdown seed puts its tasks in a sprint state ([/], [x]) in, as an upgrade
// or import does, or the one setStatusExternally makes. Without one it is the
// ID an upgrade gives it.
func importedID(m model) string {
	if s, ok := m.doc.SprintByName(todo.ImportedSprintName); ok {
		return s.ID
	}
	return "0000000"
}

// newTestModelOn is newTestModel switched to the view for column c: the
// Backlog for ColBacklog, else the Imported sprint's board with the cursor in
// c's section, where the seed's tasks outside the Backlog are. Tests use it to act on working or
// done tasks.
func newTestModelOn(t *testing.T, src string, c todo.Column) (model, string) {
	t.Helper()
	m, path := newTestModel(t, src)
	toTab(&m, c)
	return m, path
}

// toTab switches m to the Backlog for ColBacklog, else to the Imported
// sprint's board with the cursor on the first task of c's section (when it
// holds one), without storing the view.
func toTab(m *model, c todo.Column) {
	if c == todo.ColBacklog {
		m.switchScreen(screenBacklog, "")
		return
	}
	if id := importedID(*m); m.view.screen != screenSprint || m.view.sprint != id || m.view.all {
		m.switchScreen(screenSprint, id)
	}
	toSection(m, c)
}

// toSection puts the board's cursor on the first task of section c, when it
// holds one.
func toSection(m *model, c todo.Column) {
	if i, ok := m.tree.sectionStart(c); ok {
		m.tree.showSectionAt(i)
	}
}

// showOn switches m to the view for column c, as newTestModelOn does, and
// selects the row titled title, failing the test when that view doesn't show
// it. Tests use it to follow a task that changed column.
func showOn(t *testing.T, m model, c todo.Column, title string) model {
	t.Helper()
	toTab(&m, c)
	it := find(m.doc, title)
	if it == nil || !m.tree.selectID(it.ID) {
		t.Fatalf("%q is not shown on %s (rows %v)", title, m.view.title(), rowTitles(m.tree))
	}
	return m
}

// openSecond opens another handle on the store at path, standing in for a second
// process (a CLI command or an agent) writing to the same list.
func openSecond(t *testing.T, path string) *todo.Store {
	t.Helper()
	return todo.NewStore(path)
}

// parseDoc parses the markdown src and numbers its items 1, 2, … in document
// order, standing in for the IDs a store load assigns, so trees built from it
// can key folds by ID.
func parseDoc(t testing.TB, src string) *todo.Document {
	t.Helper()
	d := todotest.Parse(t, src)
	for i, it := range d.Enumerate() {
		it.ID = strconv.Itoa(i + 1)
	}
	return d
}

// newTask returns a task titled title, with description desc, in state st,
// not yet placed in a tree.
func newTask(title, desc string, st todo.Status) *todo.Item {
	return &todo.Item{Kind: todo.Task, Title: title, Description: desc, Status: st}
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

// sectionTitles lists the titles of the item rows in board section c of tr,
// in order: what that section shows.
func sectionTitles(tr tree, c todo.Column) []string {
	var out []string
	in := false
	for _, r := range tr.rows {
		switch {
		case r.kind == rowSection:
			in = r.col == c
		case in && r.kind == rowItem:
			out = append(out, r.item.Title)
		}
	}
	return out
}
