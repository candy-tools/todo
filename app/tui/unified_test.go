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
	m, path := newTestModel(t, "# C\n- [ ] o\n- [ ] r\n- [ ] w\n- [x] d\n")
	m = setStatusExternally(t, m, path, "r", todo.Ready)
	m = setStatusExternally(t, m, path, "w", todo.Working)
	m.status = ""
	return m, path
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
	m = press(m, "k")
	if !m.view.all {
		t.Fatal("k should turn the unified view on")
	}
	if got := rowTitles(m.tree); !equalStrings(got, []string{"C", "o", "r", "w", "d"}) {
		t.Errorf("rows = %v, want every task", got)
	}
	if h := m.header(100); !strings.Contains(h, "All states · 4 tasks") || strings.Contains(h, "Backlog") {
		t.Errorf("header = %q, want the all-states label instead of the tabs", h)
	}
	m = press(m, "k")
	if m.view.all || m.view.column != todo.ColBacklog {
		t.Errorf("k again should return to Backlog, all=%v column=%v", m.view.all, m.view.column)
	}
	if got := rowTitles(m.tree); !equalStrings(got, []string{"C", "o"}) {
		t.Errorf("back on Backlog rows = %v", got)
	}
}

func TestUnifiedReturnsToLastTab(t *testing.T) {
	m, _ := newUnifiedFixture(t)
	m.setColumn(todo.ColNext)
	m = press(m, "k", "k")
	if m.view.column != todo.ColNext {
		t.Errorf("k twice should return to Next, got %v", m.view.column)
	}
}

func TestUnifiedTabKeysHint(t *testing.T) {
	m, _ := newUnifiedFixture(t)
	m = press(m, "k", "2")
	if !m.view.all {
		t.Error("a tab key must not leave the unified view")
	}
	if got := m.footer(200); !strings.Contains(got, "press k to show the board") {
		t.Errorf("footer = %q, want the hint", got)
	}
}

func TestUnifiedEditsCategories(t *testing.T) {
	m, _ := newUnifiedFixture(t)
	m.setColumn(todo.ColNext)
	m = press(m, "k")
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
	m = press(m, "k")
	m = selectTitle(t, m, "w")
	m = press(m, "n")
	m = typeText(m, "z")
	m = press(m, "enter")
	if st := storedItem(t, path, "z").Status; st != todo.Open {
		t.Errorf("a new task in the unified view stored %v, want open", st)
	}
}

func TestUnifiedOwnStateFilter(t *testing.T) {
	m, _ := newUnifiedFixture(t)
	m = filterTick(t, m, todo.Deferred) // Backlog: only deferred (none)
	m = press(m, "k")
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
	m = press(m, "k")
	if got := m.view.activeStates(); len(got) != 1 || got[0] != todo.Deferred {
		t.Errorf("Backlog's filter should be back, got %v", got)
	}
}

func TestUnifiedSelectionCarriesOver(t *testing.T) {
	m, _ := newUnifiedFixture(t)
	m.setColumn(todo.ColInProgress)
	m = selectTitle(t, m, "w")
	m = press(m, "k")
	if got := m.tree.selected(); got == nil || got.Title != "w" {
		t.Fatalf("k should keep w selected, got %v", got)
	}
	m = selectTitle(t, m, "r")
	m = press(m, "k")
	if m.tree.cursor != 0 {
		t.Errorf("r isn't on In Progress: the cursor should go to the top, got %d", m.tree.cursor)
	}
}
