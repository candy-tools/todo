package tui

import (
	"strings"
	"testing"

	"github.com/candy-tools/todo/internal/todo"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

const splitDoc = "# Work\n- [ ] a\n## Backend\n- [ ] b\n# Home\n- [ ] c\n"

// sideSelect selects the category titled title in the sidebar, as walking
// the sidebar onto it would, failing when the sidebar doesn't show it.
func sideSelect(t *testing.T, m model, title string) model {
	t.Helper()
	it := find(m.doc, title)
	if it == nil || !m.side.selectID(it.ID) {
		t.Fatalf("%q is not in the sidebar (rows %v)", title, rowDesc(m.side))
	}
	m.syncScope()
	return m
}

func TestSplitToggle(t *testing.T) {
	m, _ := newTestModel(t, splitDoc)
	before := rowTitles(m.tree)
	m = press(m, "v")
	if !m.split || !m.sideFocus || !m.side.onAll() || m.tree.pane != paneTasks {
		t.Fatalf("v: split=%v sideFocus=%v onAll=%v pane=%v", m.split, m.sideFocus, m.side.onAll(), m.tree.pane)
	}
	m = press(m, "v")
	if m.split || m.tree.pane != paneMixed {
		t.Fatalf("v again: split=%v pane=%v", m.split, m.tree.pane)
	}
	if got := rowTitles(m.tree); !equalStrings(got, before) {
		t.Errorf("rows after v v = %v, want %v", got, before)
	}
}

func TestSplitSidebarNarrowsTasks(t *testing.T) {
	m, _ := newTestModel(t, splitDoc)
	m = press(m, "v", "down") // Work
	if got := rowDesc(m.tree); !equalStrings(got, []string{"a", "h:Backend", "b"}) {
		t.Errorf("Work rows = %v", got)
	}
	m = press(m, "down") // Backend
	if got := rowDesc(m.tree); !equalStrings(got, []string{"b"}) {
		t.Errorf("Backend rows = %v", got)
	}
}

func TestSplitXSwitchesFocus(t *testing.T) {
	m, _ := newTestModel(t, splitDoc)
	m = press(m, "v", "x")
	if m.sideFocus {
		t.Fatal("x should move focus to the task list")
	}
	side := m.side.cursor
	m = press(m, "down")
	if got := m.tree.selected(); got == nil || got.Title != "b" {
		t.Errorf("down in the task list selected %v, want b", got)
	}
	if m.side.cursor != side {
		t.Error("down in the task list moved the sidebar")
	}
	m = press(m, "x")
	if !m.sideFocus {
		t.Error("x again should move focus back to the sidebar")
	}
	n, _ := newTestModel(t, splitDoc)
	if n = press(n, "x"); n.split || n.sideFocus || n.mode != modeMain {
		t.Error("x outside the split view should do nothing")
	}
}

func TestSplitLayout(t *testing.T) {
	m, _ := newTestModel(t, splitDoc)
	m = press(m, "v")
	v := m.View()
	if !strings.Contains(v, "Categories") || !strings.Contains(v, "Tasks") {
		t.Errorf("split view should show both panes:\n%s", v)
	}
	if got := m.footer(200); !strings.Contains(got, "Pane") {
		t.Errorf("footer = %q, want the x Pane hint", got)
	}
}

func TestSplitNarrowTerminalFits(t *testing.T) {
	m, _ := newTestModel(t, splitDoc)
	m = send(m, tea.WindowSizeMsg{Width: 40, Height: 20})
	m = press(m, "v")
	for i, line := range strings.Split(m.View(), "\n") {
		if w := ansi.StringWidth(line); w > 40 {
			t.Errorf("line %d is %d wide: %q", i, w, line)
		}
	}
}

func TestSplitSidebarFollowsTab(t *testing.T) {
	m, path := newTestModel(t, splitDoc)
	m = setStatusExternally(t, m, path, "b", todo.Ready)
	m = press(m, "v")
	m = sideSelect(t, m, "Backend")
	m.setColumn(todo.ColNext)
	if got := m.side.selected(); got == nil || got.Title != "Backend" {
		t.Errorf("Backend holds a Next task: it should stay selected, got %v", got)
	}
	if got := rowDesc(m.tree); !equalStrings(got, []string{"b"}) {
		t.Errorf("Next rows for Backend = %v", got)
	}
	m.setColumn(todo.ColDone)
	if !m.side.onAll() {
		t.Errorf("Done shows no Backend: the sidebar should fall back to All, got %v", m.side.selected())
	}
}

func TestSplitSearchNarrowsSidebar(t *testing.T) {
	m, _ := newTestModel(t, splitDoc)
	m = press(m, "v", "/")
	m = typeText(m, "b")
	m = press(m, "enter")
	if got := rowDesc(m.side); !equalStrings(got, []string{"all", "Work", "Backend"}) {
		t.Errorf("sidebar under search = %v", got)
	}
}

func TestSplitLiveSearchFallsBackToAll(t *testing.T) {
	m, _ := newTestModel(t, "# Work\n- [ ] alpha\n- [ ] beta\n# Home\n- [ ] chores\n")
	m = press(m, "v")
	m = sideSelect(t, m, "Home")
	m = press(m, "x", "/")
	m = typeText(m, "a")
	if !m.side.onAll() {
		t.Fatalf("Home has no match: the sidebar should fall back to All, got %v", m.side.selected())
	}
	if got := m.tree.selected(); got == nil || got.Title != "alpha" {
		t.Fatalf("task list selection = %v, want alpha", got)
	}
	m = press(m, "down")
	if got := m.tree.selected(); got == nil || got.Title != "beta" {
		t.Errorf("down in the search bar should move the task list, got %v", got)
	}
}

func TestSplitVOnKeepsTask(t *testing.T) {
	m, _ := newTestModel(t, splitDoc)
	m = selectTitle(t, m, "c")
	m = press(m, "v")
	if got := m.tree.selected(); got == nil || got.Title != "c" {
		t.Errorf("v should keep c selected in the task list, got %v", got)
	}
	if !m.side.onAll() {
		t.Error("the sidebar should start on All")
	}
}

func TestSplitVOffSelectsFocused(t *testing.T) {
	m, _ := newTestModel(t, splitDoc)
	m = press(m, "v", "x")
	m = selectTitle(t, m, "b")
	m = press(m, "v")
	if got := m.tree.selected(); got == nil || got.Title != "b" {
		t.Errorf("v off from the task list should select b, got %v", got)
	}
	m = press(m, "v")
	m = sideSelect(t, m, "Home")
	m = press(m, "v")
	if got := m.tree.selected(); got == nil || got.Title != "Home" {
		t.Errorf("v off from the sidebar should select Home, got %v", got)
	}
}

func TestSplitToggleIgnoredWhileGrabbed(t *testing.T) {
	m, _ := newTestModel(t, splitDoc)
	m = selectTitle(t, m, "a")
	m = press(m, "m", "v", "k")
	if m.split || m.view.all {
		t.Errorf("v and k must be ignored while an item is grabbed: split=%v all=%v", m.split, m.view.all)
	}
}

func TestSplitWithUnified(t *testing.T) {
	m, path := newTestModel(t, splitDoc)
	m = setStatusExternally(t, m, path, "b", todo.Working)
	m = press(m, "v", "k")
	m = sideSelect(t, m, "Backend")
	if got := rowDesc(m.tree); !equalStrings(got, []string{"b"}) {
		t.Errorf("unified Backend rows = %v", got)
	}
	m = press(m, "k")
	if got := m.side.selected(); got == nil || got.Title != "Backend" {
		t.Errorf("Backlog shows every category: Backend should stay, got %v", got)
	}
	if len(m.tree.rows) != 0 {
		t.Errorf("Backlog has no Backend task: rows = %v", rowDesc(m.tree))
	}
}
