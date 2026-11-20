package tui

import (
	"strings"
	"testing"

	"github.com/candy-tools/todo/internal/todo"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

const splitDoc = "# Work\n- [ ] a\n## Backend\n- [ ] b\n# Home\n- [ ] c\n"

// splitAll opens the split view on All with the sidebar focused, as the app
// does when it opens on a stored split. The v key opens it on the selection
// instead (see TestVOnTaskOpensSplitOnTaskList).
func splitAll(m model) model {
	m.toggleSplit()
	return m
}

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
	m = press(m, "v") // the cursor is on the Work category
	if sel := m.side.selected(); !m.split || !m.sideFocus || sel == nil || sel.Title != "Work" || m.tree.pane != paneTasks {
		t.Fatalf("v: split=%v sideFocus=%v side=%v pane=%v", m.split, m.sideFocus, sel, m.tree.pane)
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
	m = press(splitAll(m), "down") // Work
	if got := rowDesc(m.tree); !equalStrings(got, []string{"h:Backend", "b", "h:Work", "a"}) {
		t.Errorf("Work rows = %v", got)
	}
	m = press(m, "down") // Backend
	if got := rowDesc(m.tree); !equalStrings(got, []string{"b"}) {
		t.Errorf("Backend rows = %v", got)
	}
}

func TestSplitTabSwitchesFocus(t *testing.T) {
	m, _ := newTestModel(t, splitDoc)
	m = press(m, "v", "tab")
	if m.sideFocus {
		t.Fatal("tab should move focus to the task list")
	}
	side := m.side.cursor
	m = press(m, "down")
	// The list opens on b (Work › Backend comes first), so down reaches a.
	if got := m.tree.selected(); got == nil || got.Title != "a" {
		t.Errorf("down in the task list selected %v, want a", got)
	}
	if m.side.cursor != side {
		t.Error("down in the task list moved the sidebar")
	}
	m = press(m, "tab")
	if !m.sideFocus {
		t.Error("tab again should move focus back to the sidebar")
	}
	n, _ := newTestModel(t, splitDoc)
	if n = press(n, "tab"); n.split || n.sideFocus || n.mode != modeMain {
		t.Error("tab outside the split view should do nothing")
	}
}

func TestSplitLayout(t *testing.T) {
	m, _ := newTestModel(t, splitDoc)
	m = press(m, "v")
	v := m.View()
	if !strings.Contains(v, "╭─ Categories") || !strings.Contains(v, "╭─ Work") {
		t.Errorf("split view should show both panes:\n%s", v)
	}
	if got := m.footer(200); !strings.Contains(got, "Pane") {
		t.Errorf("footer = %q, want the tab Pane hint", got)
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

func TestSplitSidebarFollowsView(t *testing.T) {
	m, path := newTestModel(t, splitDoc)
	m = setStatusExternally(t, m, path, "b", todo.Ready)
	m = press(m, "v")
	m = sideSelect(t, m, "Work")
	m = press(m, "u") // the unified view shows every task
	if got := m.side.selected(); got == nil || got.Title != "Work" {
		t.Errorf("Work holds tasks in the unified view: it should stay selected, got %v", got)
	}
	m = sideSelect(t, m, "Backend")
	if got := rowDesc(m.tree); !equalStrings(got, []string{"b"}) {
		t.Errorf("unified rows for Backend = %v", got)
	}
	m = press(m, "u") // the Backlog lists every category, but not b, now in a sprint
	if got := m.side.selected(); got == nil || got.Title != "Backend" || len(rowDesc(m.tree)) != 0 {
		t.Errorf("Backlog: sidebar on %v, rows %v, want Backend kept with no task", got, rowDesc(m.tree))
	}
	m.setFilter("Home")
	if !m.side.onAll() {
		t.Errorf("a search with no match in Backend: the sidebar should fall back to All, got %v", m.side.selected())
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
	m = press(m, "tab", "/")
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
	m = press(splitAll(m), "tab")
	m = selectTitle(t, m, "b")
	m = press(m, "v")
	if got := m.tree.selected(); got == nil || got.Title != "b" {
		t.Errorf("v off from the task list should select b, got %v", got)
	}
	m = splitAll(m)
	m = sideSelect(t, m, "Home")
	m = press(m, "v")
	if got := m.tree.selected(); got == nil || got.Title != "Home" {
		t.Errorf("v off from the sidebar should select Home, got %v", got)
	}
}

func TestSplitToggleIgnoredWhileGrabbed(t *testing.T) {
	m, _ := newTestModel(t, splitDoc)
	m = selectTitle(t, m, "a")
	m = press(m, "m", "v", "u")
	if m.split || m.view.all {
		t.Errorf("v and a must be ignored while an item is grabbed: split=%v all=%v", m.split, m.view.all)
	}
}

func TestSplitWithUnified(t *testing.T) {
	m, path := newTestModel(t, splitDoc)
	m = setStatusExternally(t, m, path, "b", todo.Working)
	m = press(m, "v", "u")
	m = sideSelect(t, m, "Backend")
	if got := rowDesc(m.tree); !equalStrings(got, []string{"b"}) {
		t.Errorf("unified Backend rows = %v", got)
	}
	m = press(m, "u")
	if got := m.side.selected(); got == nil || got.Title != "Backend" {
		t.Errorf("Backlog shows every category: Backend should stay, got %v", got)
	}
	if len(m.tree.rows) != 0 {
		t.Errorf("Backlog has no Backend task: rows = %v", rowDesc(m.tree))
	}
}
