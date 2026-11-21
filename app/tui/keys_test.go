package tui

import (
	"strings"
	"testing"

	"github.com/candy-tools/todo/internal/todo"
)

// storedStatus is the status of the task with id in the list file at path,
// read with a fresh load.
func storedStatus(t *testing.T, path, id string) todo.Status {
	t.Helper()
	it, ok := loadSprintDoc(t, path).ByID(id)
	if !ok {
		t.Fatalf("task %s not stored", id)
	}
	return it.Status
}

func TestAAdvancesAndAReturnsColumn(t *testing.T) {
	m, path := twoSprints(t)
	m = onBoard(t, m, "s000001", todo.ColInProgress, "working")
	m = press(m, "A")
	if got := storedStatus(t, path, "t000002"); got != todo.New {
		t.Fatalf("A from In Progress stored %v, want new (Next's first state)", got)
	}
	m = onBoard(t, m, "s000001", todo.ColNext, "working")
	press(m, "a")
	if got := storedStatus(t, path, "t000002"); got != todo.Working {
		t.Fatalf("a from Next stored %v, want working", got)
	}
}

func TestAOnBacklogTaskOpensAssignPicker(t *testing.T) {
	m, _ := twoSprints(t)
	m = selectTitle(t, m, "backlogTask")
	m = press(m, "a")
	if m.mode != modeSprintPick || m.sprintPick.purpose != pickAssign {
		t.Fatalf("a on a backlog task: mode %v purpose %v", m.mode, m.sprintPick.purpose)
	}
}

func TestPNoLongerMovesColumns(t *testing.T) {
	m, _ := twoSprints(t)
	m = onBoard(t, m, "s000001", todo.ColInProgress, "working")
	before := storeFP(t, m)
	m = press(m, "P")
	if storeFP(t, m) != before {
		t.Fatal("P must not write any more")
	}
	// p opened Planning, which is gone: it does nothing, and stays on the board.
	m = press(m, "p")
	if storeFP(t, m) != before || m.view.screen != screenSprint {
		t.Fatalf("p must do nothing: screen %v", m.view.screen)
	}
}

func TestUTogglesUnified(t *testing.T) {
	m, _ := twoSprints(t)
	m = press(m, "u")
	if !m.view.all {
		t.Fatal("u should open the unified view")
	}
	m = press(m, "u")
	if m.view.all {
		t.Fatal("u again should close it")
	}
}

func TestBracketsSkipEmptySectionsTabDoesNot(t *testing.T) {
	m, _ := twoSprints(t)
	m.switchScreen(screenSprint, "s000001") // only In Progress holds a task
	for _, key := range []string{"]", "[", "tab"} {
		if m = press(m, key); m.boardColumn() != todo.ColInProgress || m.status != "" {
			t.Fatalf("%s → %v (footer %q), want In Progress, the only section with tasks", key, m.boardColumn(), m.status)
		}
	}
}

func TestTabSwitchesSplitPanesXDoesNot(t *testing.T) {
	m, _ := newTestModel(t, "# Work\n- [ ] a\n")
	m = press(m, "v")
	if !m.sideFocus {
		t.Fatal("v starts on the sidebar")
	}
	if m = press(m, "x"); !m.sideFocus {
		t.Fatal("x must not switch panes any more")
	}
	if m = press(m, "tab"); m.sideFocus {
		t.Fatal("tab should move to the task list")
	}
	if m = press(m, "shift+tab"); !m.sideFocus {
		t.Fatal("shift+tab should move back to the sidebar")
	}
	if got := m.footer(200); !strings.Contains(got, "tab") || !strings.Contains(got, "Pane") {
		t.Errorf("footer = %q, want the tab Pane hint", got)
	}
}

func TestTabDoesNothingWithoutSplit(t *testing.T) {
	m, _ := newTestModel(t, "# Work\n- [ ] a\n")
	m = press(m, "tab")
	if m.split || m.sideFocus {
		t.Fatal("tab must do nothing outside the split view")
	}
}

func TestVOnTaskOpensSplitOnTaskList(t *testing.T) {
	m, _ := newTestModel(t, "# Work\n- [ ] a\n- [ ] b\n")
	m = selectTitle(t, m, "b")
	m = press(m, "v")
	if !m.split || m.sideFocus {
		t.Fatalf("v on a task: split %v sideFocus %v, want the task list focused", m.split, m.sideFocus)
	}
	if !m.side.onAll() {
		t.Error("the sidebar should be on All")
	}
	if got := m.tree.selected(); got == nil || got.Title != "b" {
		t.Errorf("selected %v, want task b", got)
	}
}

func TestVOnCategoryFocusesItInSidebar(t *testing.T) {
	m, _ := newTestModel(t, "# Work\n- [ ] a\n# Home\n- [ ] h\n")
	m = selectTitle(t, m, "Home")
	m = press(m, "v")
	if !m.split || !m.sideFocus {
		t.Fatalf("v on a category: split %v sideFocus %v, want the sidebar focused", m.split, m.sideFocus)
	}
	if got := m.side.selected(); got == nil || got.Title != "Home" {
		t.Errorf("sidebar selected %v, want Home", got)
	}
	if got := m.tree.selected(); got == nil || got.Title != "h" {
		t.Errorf("task list selected %v, want h (Home's task list)", got)
	}
}
