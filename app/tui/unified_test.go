package tui

import (
	"strings"
	"testing"

	"github.com/candy-tools/todo/internal/todo"
)

// newUnifiedFixture seeds one task per column: open o, ready r, working w,
// done d, all under category C.
func newUnifiedFixture(t *testing.T) (model, string) {
	t.Helper()
	return newJSONModel(t, `{"version": 3, "categories": [{"id": "c000000", "title": "C"}], "tasks": [
		{"id": "t000001", "category": "c000000", "title": "o", "status": "open"},
		{"id": "t000002", "category": "c000000", "title": "r", "status": "ready"},
		{"id": "t000003", "category": "c000000", "title": "w", "status": "working"},
		{"id": "t000004", "category": "c000000", "title": "d", "status": "done"}]}`)
}

// filterTick opens the f dialog, ticks the states sts and applies them.
func filterTick(t *testing.T, m model, sts ...todo.Status) model {
	t.Helper()
	m = press(m, "f")
	if m.mode != modeStateFilter {
		t.Fatalf("f should open the filter dialog, mode = %v", m.mode)
	}
	for _, st := range sts {
		m.pick.focus(st)
		m = press(m, " ")
	}
	return press(m, "enter")
}

func TestUnifiedShowsEveryState(t *testing.T) {
	m, _ := newUnifiedFixture(t)
	m = press(m, "u")
	if !m.view.all {
		t.Fatal("a should turn the unified view on")
	}
	if got := rowTitles(m.tree); !equalStrings(got, []string{"C", "o", "r", "w", "d"}) {
		t.Errorf("rows = %v, want every task", got)
	}
	if h := m.header(100); !strings.Contains(h, "All states · 4 tasks") || strings.Contains(h, "Backlog") {
		t.Errorf("header = %q, want the all-states label instead of the screen's", h)
	}
	m = press(m, "u")
	if m.view.all || m.view.screen != screenBacklog {
		t.Errorf("a again should return to Backlog, all=%v screen=%v", m.view.all, m.view.screen)
	}
	if got := rowTitles(m.tree); !equalStrings(got, []string{"C", "o"}) {
		t.Errorf("back on Backlog rows = %v", got)
	}
}

func TestUnifiedReturnsToLastTab(t *testing.T) {
	m, _ := newUnifiedFixture(t)
	toTab(&m, todo.ColNext)
	m = press(m, "u", "u")
	if m.view.screen != screenSprint || m.view.column != todo.ColNext {
		t.Errorf("a twice should return to the board's Next, got screen %v tab %v", m.view.screen, m.view.column)
	}
}

func TestUnifiedTabKeysHint(t *testing.T) {
	m, _ := newUnifiedFixture(t)
	toTab(&m, todo.ColNext)
	m = press(m, "u", "2")
	if !m.view.all {
		t.Error("a tab key must not leave the unified view")
	}
	if got := m.footer(200); !strings.Contains(got, "press u to show the board") {
		t.Errorf("footer = %q, want the hint", got)
	}
}

func TestUnifiedEditsCategories(t *testing.T) {
	m, _ := newUnifiedFixture(t)
	toTab(&m, todo.ColNext)
	m = press(m, "u")
	m = selectTitle(t, m, "C")
	if last := m.tree.rows[len(m.tree.rows)-1]; last.kind != rowNewCategory {
		t.Error("the unified view should end with the + new category row")
	}
	m = press(m, "e")
	if m.mode != modeForm {
		t.Errorf("e on a category in the unified view should open the form, status %q", m.status)
	}
}

func TestUnifiedNewTaskIsOpen(t *testing.T) {
	m, path := newUnifiedFixture(t)
	m = press(m, "u")
	m = selectTitle(t, m, "w")
	m = press(m, "n")
	m = typeText(m, "z")
	m = press(m, "enter")
	if m.mode != modeMain {
		t.Errorf("enter should save and close the form, mode %v status %q", m.mode, m.status)
	}
	if sel := m.tree.selected(); sel == nil || sel.Title != "z" {
		t.Errorf("the new task should be selected in the unified view, got %v", sel)
	}
	if st := storedItem(t, path, "z").Status; st != todo.Open {
		t.Errorf("a new task in the unified view stored %v, want open", st)
	}
}

func TestUnifiedOwnStateFilter(t *testing.T) {
	m, _ := newUnifiedFixture(t)
	m = filterTick(t, m, todo.Deferred) // Backlog: only deferred (none)
	m = press(m, "u")
	if len(m.view.activeStates()) != 0 || len(rowTitles(m.tree)) != 5 {
		t.Fatalf("a tab's filter must not apply in the unified view: states %v rows %v", m.view.activeStates(), rowTitles(m.tree))
	}
	m = press(m, "f")
	offers := map[todo.Status]bool{}
	for _, r := range m.pick.rows {
		if r.header == "" {
			offers[r.state] = true
		}
	}
	if !offers[todo.Ready] || !offers[todo.Done] || !offers[todo.Open] {
		t.Errorf("the unified filter should offer every state, got %v", offers)
	}
	m = press(m, "esc")
	m = filterTick(t, m, todo.Ready)
	if got := rowTitles(m.tree); !equalStrings(got, []string{"C", "r"}) {
		t.Errorf("unified filter on ready: rows = %v", got)
	}
	m = press(m, "esc")
	if len(m.view.activeStates()) != 0 {
		t.Errorf("esc should clear the unified filter, got %v", m.view.activeStates())
	}
	m = press(m, "u")
	if got := m.view.activeStates(); len(got) != 1 || got[0] != todo.Deferred {
		t.Errorf("Backlog's filter should be back, got %v", got)
	}
}

func TestUnifiedSelectionCarriesOver(t *testing.T) {
	m, _ := newUnifiedFixture(t)
	toTab(&m, todo.ColInProgress)
	m = selectTitle(t, m, "w")
	m = press(m, "u")
	if got := m.tree.selected(); got == nil || got.Title != "w" {
		t.Fatalf("a should keep w selected, got %v", got)
	}
	m = selectTitle(t, m, "r")
	m = press(m, "u")
	if m.tree.cursor != 0 {
		t.Errorf("r isn't on In Progress: the cursor should go to the top, got %d", m.tree.cursor)
	}
}
