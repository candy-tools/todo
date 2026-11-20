package tui

import (
	"context"
	"strings"
	"testing"

	"github.com/candy-tools/todo/internal/todo"
)

const actionsDoc = "# Work\n- [ ] a\n## Backend\n- [ ] b\n# Home\n- [ ] c\n# Empty\n"

// addTyped opens the add-task dialog with key, types title and saves it.
func addTyped(t *testing.T, m model, key, title string) model {
	t.Helper()
	m = press(m, key)
	if m.mode != modeForm {
		t.Fatalf("%s should open the form, status %q", key, m.status)
	}
	m = typeText(m, title)
	return press(m, "enter")
}

func TestSplitAddTaskFromSidebar(t *testing.T) {
	m, path := newTestModel(t, actionsDoc)
	m = press(m, "v")
	m = sideSelect(t, m, "Work")
	m = addTyped(t, m, "n", "z")
	z := storedItem(t, path, "z")
	if z.Parent == nil || z.Parent.Title != "Work" || z.Status != todo.Open {
		t.Errorf("z stored under %v with %v, want Work / open", z.Parent, z.Status)
	}
	if !hasRow(m, "z") {
		t.Errorf("the task list should show z, rows %v", rowDesc(m.tree))
	}
	if got := m.side.selected(); got == nil || got.Title != "Work" {
		t.Errorf("the sidebar should stay on Work, got %v", got)
	}
}

func TestSplitAddTaskIntoEmptyScope(t *testing.T) {
	m, path := newTestModel(t, actionsDoc)
	m = press(m, "v")
	m = sideSelect(t, m, "Empty")
	m = press(m, "tab")
	m = addTyped(t, m, "n", "z")
	if z := storedItem(t, path, "z"); z.Parent == nil || z.Parent.Title != "Empty" {
		t.Errorf("z stored under %v, want Empty", z.Parent)
	}
	if got := m.tree.selected(); got == nil || got.Title != "z" {
		t.Errorf("the new task should be selected in the list, got %v", got)
	}
}

func TestSplitAddOnAllRefused(t *testing.T) {
	m, _ := newTestModel(t, actionsDoc)
	m = press(splitAll(m), "n")
	if m.mode != modeMain || !strings.Contains(m.footer(200), "pick a category in the sidebar") {
		t.Errorf("n on All: mode %v footer %q", m.mode, m.footer(200))
	}
	toTab(&m, todo.ColNext) // nothing in Next: the list is empty
	m = press(m, "tab", "n")
	if m.mode != modeMain || !strings.Contains(m.footer(200), "pick a category in the sidebar") {
		t.Errorf("n in an empty list on All: mode %v footer %q", m.mode, m.footer(200))
	}
}

func TestSplitCInRightPane(t *testing.T) {
	m, _ := newTestModel(t, actionsDoc)
	m = press(m, "v", "tab", "c")
	if m.mode != modeMain || !strings.Contains(m.footer(200), "categories are managed in the sidebar") {
		t.Errorf("c in the list: mode %v footer %q", m.mode, m.footer(200))
	}
}

func TestSplitEditCategoryInSidebar(t *testing.T) {
	m, path := newTestModel(t, actionsDoc)
	m = setStatusExternally(t, m, path, "a", todo.Ready)
	m = press(m, "v")
	m = sideSelect(t, m, "Work")
	m = press(m, "e")
	if m.mode != modeForm || m.editing == nil || m.editing.Title != "Work" {
		t.Fatalf("e on Work should edit it, mode %v", m.mode)
	}
	m = press(m, "esc")
	toTab(&m, todo.ColNext)
	m = sideSelect(t, m, "Work")
	m = press(m, "e")
	if m.mode != modeMain || !strings.Contains(m.footer(200), msgCategoriesInBacklog) {
		t.Errorf("outside Backlog e on a category: mode %v footer %q", m.mode, m.footer(200))
	}
}

func TestSplitDeleteScopeFallsBackToAll(t *testing.T) {
	m, _ := newTestModel(t, actionsDoc)
	m = press(m, "v")
	m = sideSelect(t, m, "Home")
	m = press(m, "d", "y")
	if !m.side.onAll() {
		t.Errorf("deleting the scope should fall back to All, got %v", m.side.selected())
	}
	if !hasRow(m, "a") || !hasRow(m, "b") {
		t.Errorf("All should list the remaining tasks, rows %v", rowDesc(m.tree))
	}
}

func TestSplitDeleteLastTaskInScope(t *testing.T) {
	m, _ := newTestModel(t, actionsDoc)
	m = press(m, "v")
	m = sideSelect(t, m, "Home")
	m = press(m, "tab", "d", "y")
	if len(m.tree.rows) != 0 {
		t.Fatalf("Home's list should be empty, rows %v", rowDesc(m.tree))
	}
	if got := m.tree.view(40, 5); !strings.Contains(got, "no tasks here") {
		t.Errorf("empty list view = %q", got)
	}
	m = press(m, "down", "up", "t", "e", "enter", "a", "m", "y", "d")
	if m.mode != modeMain {
		t.Errorf("keys on an empty list opened %v", m.mode)
	}
	if got := m.side.selected(); got == nil || got.Title != "Home" {
		t.Errorf("Home itself stays selected, got %v", got)
	}
}

func TestSplitGrabLeavesScope(t *testing.T) {
	m, path := newTestModel(t, actionsDoc)
	m = press(m, "v")
	m = sideSelect(t, m, "Backend")
	// Work's own tasks come after its subcategory Backend, so b steps down
	// into Work.
	m = press(m, "tab", "m", "down")
	if p := storedItem(t, path, "b").Parent; p == nil || p.Title != "Work" {
		t.Fatalf("b should move down into Work, parent %v", p)
	}
	if m.tree.grabbed {
		t.Error("the grab should end once b left the list")
	}
	if got := m.footer(200); !strings.Contains(got, "moved to Work") {
		t.Errorf("footer = %q, want where b went", got)
	}
}

func TestSplitAddCategoryFromPlaceholder(t *testing.T) {
	m, _ := newTestModel(t, actionsDoc)
	m = press(m, "v")
	for i := 0; !m.side.onPlaceholder() && i < 20; i++ {
		m = press(m, "down")
	}
	m = press(m, "c")
	m = typeText(m, "New")
	m = press(m, "enter")
	if got := m.side.selected(); got == nil || got.Title != "New" {
		t.Errorf("the new category should be selected in the sidebar, got %v", got)
	}
}

func TestSplitReloadKeepsScope(t *testing.T) {
	m, path := newTestModel(t, actionsDoc)
	m = press(m, "v")
	m = sideSelect(t, m, "Backend")
	backend := find(m.doc, "Backend").ID
	if _, _, err := openSecond(t, path).AddTask(context.Background(), todo.Fields{Title: "ext"}, todo.Open, backend, ""); err != nil {
		t.Fatalf("external add: %v", err)
	}
	m = pollOnce(t, m)
	if got := m.side.selected(); got == nil || got.Title != "Backend" {
		t.Errorf("the reload should keep Backend, got %v", got)
	}
	if !hasRow(m, "ext") {
		t.Errorf("the list should show ext, rows %v", rowDesc(m.tree))
	}
}

func TestSplitReloadScopeDeleted(t *testing.T) {
	m, path := newTestModel(t, actionsDoc)
	m = press(m, "v")
	m = sideSelect(t, m, "Home")
	if _, err := openSecond(t, path).Delete(context.Background(), find(m.doc, "Home").ID); err != nil {
		t.Fatalf("external delete: %v", err)
	}
	m = pollOnce(t, m)
	if !m.side.onAll() {
		t.Errorf("a deleted scope should fall back to All, got %v", m.side.selected())
	}
}

func TestSplitEmptyDocument(t *testing.T) {
	m, _ := newTestModel(t, "")
	m = press(m, "v")
	if got := rowDesc(m.side); !equalStrings(got, []string{"all", "new"}) {
		t.Errorf("empty sidebar rows = %v", got)
	}
	if got := m.tree.view(40, 5); !strings.Contains(got, "no tasks here") {
		t.Errorf("empty list view = %q", got)
	}
	m = press(m, "tab", "n")
	if m.mode != modeMain || !strings.Contains(m.footer(200), "pick a category in the sidebar") {
		t.Errorf("n in an empty list: mode %v footer %q", m.mode, m.footer(200))
	}
}

func TestSplitFilteredStateChangeNamesColumn(t *testing.T) {
	m, _ := newJSONModel(t, `{"version": 3, "categories": [{"id": "c000000", "title": "C"}], "tasks": [
		{"id": "t000001", "category": "c000000", "title": "o", "status": "open"},
		{"id": "t000002", "category": "c000000", "title": "r", "status": "ready"}]}`)
	toTab(&m, todo.ColNext)
	m = filterTick(t, m, todo.Ready)
	m = press(m, "v", "tab")
	m = selectTitle(t, m, "r")
	m = pickState(t, m, todo.New) // still in Next, but the filter hides it now
	if got := m.footer(200); !strings.Contains(got, "moved to Next — new") {
		t.Errorf("footer = %q, want the state change, not a category move", got)
	}
}

func TestSplitEnterOnAllAddsCategory(t *testing.T) {
	m, _ := newTestModel(t, actionsDoc)
	m = press(splitAll(m), "enter")
	if m.mode != modeForm || m.target != targetAddCategory {
		t.Errorf("enter on All should open the add-category form, mode %v target %v", m.mode, m.target)
	}
}
