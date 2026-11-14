package tui

import (
	"strings"
	"testing"

	"github.com/candy-tools/todo/internal/todo"
	"github.com/candy-tools/todo/internal/todo/todotest"
	"github.com/charmbracelet/x/ansi"
)

// titles lists the titles of a sibling slice, for order assertions.
func titles(items []*todo.Item) []string {
	out := make([]string, len(items))
	for i, it := range items {
		out[i] = it.Title
	}
	return out
}

func TestMoveModeGrabToggles(t *testing.T) {
	m, _ := newTestModel(t, "# Work\n\n- [ ] a\n")
	m = press(m, "down") // task a
	m = press(m, "m")    // grab
	if !m.tree.grabbed {
		t.Fatal("m should enter move mode")
	}
	m = press(m, "m") // drop
	if m.tree.grabbed {
		t.Error("a second m should exit move mode")
	}
	m = press(m, "m", "esc") // grab then esc
	if m.tree.grabbed {
		t.Error("esc should exit move mode")
	}
	m = press(m, "m", "enter") // grab then enter
	if m.tree.grabbed {
		t.Error("enter should exit move mode")
	}
}

func TestMoveModeGrabRequiresRealItem(t *testing.T) {
	m, _ := newTestModel(t, "") // starts on the "+ new category" placeholder
	if !m.tree.onPlaceholder() {
		t.Fatal("empty document should start on the placeholder")
	}
	m = press(m, "m")
	if m.tree.grabbed {
		t.Error("the placeholder row cannot be grabbed")
	}
}

func TestMoveModeReorderPersists(t *testing.T) {
	m, path := newTestModel(t, "# Work\n\n- [ ] a\n- [ ] b\n")
	m = press(m, "down") // task a
	a := m.tree.selected().ID
	m = press(m, "m", "down") // grab a, move it down past b
	if got := titles(find(m.doc, "Work").Children); got[0] != "b" || got[1] != "a" {
		t.Errorf("in-memory order = %v, want [b a]", got)
	}
	if m.tree.selected().ID != a {
		t.Error("the cursor should follow the moved item")
	}
	reloaded := todotest.Parse(t, todotest.Dump(t, path))
	if got := titles(find(reloaded, "Work").Children); got[0] != "b" || got[1] != "a" {
		t.Errorf("persisted order = %v, want [b a]", got)
	}
}

func TestMoveModeIndentMakesSubtask(t *testing.T) {
	m, path := newTestModel(t, "# Work\n\n- [ ] a\n- [ ] b\n")
	m = press(m, "down", "down") // task b
	m = press(m, "m", "right")   // grab b, indent under a
	a := find(m.doc, "a")
	if len(a.Children) != 1 || a.Children[0].Title != "b" {
		t.Fatalf("b should be a subtask of a, a.children=%v", titles(a.Children))
	}
	if !strings.Contains(todotest.Dump(t, path), "  - [ ] b") {
		t.Errorf("subtask indentation not persisted:\n%s", todotest.Dump(t, path))
	}
}

func TestMoveModeOutdentMakesSibling(t *testing.T) {
	m, _ := newTestModel(t, "# Work\n\n- [ ] a\n  - [ ] c\n")
	m = press(m, "down", "down") // subtask c
	m = press(m, "m", "left")    // grab c, outdent to a's sibling
	c := find(m.doc, "c")
	if c.Parent == nil || c.Parent.Title != "Work" {
		t.Fatalf("c should be lifted to a child of Work, got parent %v", c.Parent)
	}
	if got := titles(find(m.doc, "Work").Children); got[0] != "a" || got[1] != "c" {
		t.Errorf("Work's children = %v, want [a c]", got)
	}
}

func TestMoveModeOutdentTopTaskIsBlocked(t *testing.T) {
	m, _ := newTestModel(t, "# Work\n\n- [ ] a\n")
	m = press(m, "down")      // task a
	m = press(m, "m", "left") // grab a, try to outdent out of Work
	a := find(m.doc, "a")
	if a.Parent == nil || a.Parent.Title != "Work" {
		t.Error("a top-level task must stay under its category")
	}
	if !m.tree.grabbed {
		t.Error("a blocked move should keep the item grabbed")
	}
}

func TestMoveModeCategoryReorder(t *testing.T) {
	m, _ := newTestModel(t, "# A\n\n# B\n")
	a := m.tree.selected().ID // A is the first row
	m = press(m, "m", "down")
	if got := titles(m.doc.Roots); got[0] != "B" || got[1] != "A" {
		t.Errorf("root order = %v, want [B A]", got)
	}
	if m.tree.selected().ID != a {
		t.Error("the cursor should follow the moved category")
	}
}

func TestMoveModeBlockedWhileFiltering(t *testing.T) {
	m, _ := newTestModel(t, "# Work\n\n- [ ] a\n- [ ] b\n")
	m = press(m, "/")
	m = typeText(m, "a")
	m = press(m, "enter") // filter applied, search bar closed
	m = press(m, "m")
	if m.tree.grabbed {
		t.Error("move mode should not start while a filter is active")
	}
	if m.status == "" {
		t.Error("expected a status hint explaining the filter blocks move mode")
	}
}

func TestMoveModeFooterHints(t *testing.T) {
	m, _ := newTestModel(t, "# Work\n\n- [ ] a\n")
	m = press(m, "down", "m")
	got := m.footer(120)
	for _, want := range []string{"Child", "Sibling", "Drop"} {
		if !strings.Contains(got, want) {
			t.Errorf("move-mode footer missing %q:\n%s", want, got)
		}
	}
}

func TestMoveModeGrabbedRowIsMarked(t *testing.T) {
	m, _ := newTestModel(t, "# Work\n\n- [ ] a\n")
	m = press(m, "down", "m")
	if !strings.Contains(m.View(), "⇕") {
		t.Errorf("the grabbed row should carry a distinct marker:\n%s", m.View())
	}
}

func TestMoveModeCrossesCategoriesPersists(t *testing.T) {
	m, path := newTestModel(t, "# Work\n\n- [ ] b\n\n# Personal\n\n- [ ] p1\n")
	m = press(m, "down") // task b (last in Work)
	b := m.tree.selected().ID
	m = press(m, "m", "down") // grab b, move it down across into Personal
	if got := titles(find(m.doc, "Personal").Children); len(got) != 2 || got[0] != "b" || got[1] != "p1" {
		t.Errorf("in-memory Personal children = %v, want [b p1]", got)
	}
	if m.tree.selected().ID != b {
		t.Error("the cursor should follow the item across categories")
	}
	reloaded := todotest.Parse(t, todotest.Dump(t, path))
	if got := titles(find(reloaded, "Personal").Children); len(got) != 2 || got[0] != "b" || got[1] != "p1" {
		t.Errorf("persisted Personal children = %v, want [b p1]", got)
	}
}

// storedChildren loads the store at path through a fresh handle and returns the
// titles of the children of the item titled title.
func storedChildren(t *testing.T, path, title string) []string {
	t.Helper()
	return titles(storedItem(t, path, title).Children)
}

func TestGrabMoveStepsOverHiddenTasks(t *testing.T) {
	m, path := newTestModelOn(t, "# C\n- [/] w1\n- [ ] b\n- [/] w2\n", todo.ColInProgress)
	m = selectTitle(t, m, "w2")
	m = press(m, "m", "up", "m")
	if got, want := storedChildren(t, path, "C"), []string{"w2", "w1", "b"}; !equalStrings(got, want) {
		t.Errorf("stored order under C = %v, want %v", got, want)
	}
	if got := m.tree.selected(); got == nil || got.Title != "w2" {
		t.Errorf("the cursor should follow w2, got %v", got)
	}
	// One down passes w1; a second has no visible row left to pass, so it steps
	// over the hidden b until the move runs out.
	m = press(m, "m", "down")
	if got, want := storedChildren(t, path, "C"), []string{"w1", "w2", "b"}; !equalStrings(got, want) {
		t.Errorf("stored order under C after down = %v, want %v", got, want)
	}
	m = press(m, "down", "m")
	if got, want := storedChildren(t, path, "C"), []string{"w1", "b", "w2"}; !equalStrings(got, want) {
		t.Errorf("stored order under C after a second down = %v, want %v", got, want)
	}
}

func TestGrabMoveCrossesHiddenCategory(t *testing.T) {
	m, path := newTestModelOn(t, "# A\n- [/] w\n# B\n- [ ] x\n# C\n- [/] v\n", todo.ColInProgress)
	m = selectTitle(t, m, "w")
	m = press(m, "m", "down")
	if got, want := storedChildren(t, path, "C"), []string{"w", "v"}; !equalStrings(got, want) {
		t.Errorf("w must step over the hidden B into C, C = %v, want %v", got, want)
	}
	if got := storedChildren(t, path, "B"); !equalStrings(got, []string{"x"}) {
		t.Errorf("B = %v, want [x]", got)
	}
	if got := m.tree.selected(); got == nil || got.Title != "w" || !m.tree.grabbed {
		t.Errorf("w stays selected and grabbed, got %v (grabbed %v)", got, m.tree.grabbed)
	}
}

func TestIndentUnderSubtaskRefusedInTUI(t *testing.T) {
	m, path := newTestModel(t, "# W\n- [ ] a\n  - [ ] s1\n  - [ ] s2\n- [ ] b\n  - [ ] t1\n")
	before := storeRev(t, m)
	for _, title := range []string{"s2", "b"} {
		got := selectTitle(t, m, title)
		got = press(got, "m", "right")
		if got.status != "subtasks can't have subtasks" {
			t.Errorf("indenting %s: status = %q", title, got.status)
		}
		if !strings.Contains(ansi.Strip(got.footer(200)), "subtasks can't have subtasks") {
			t.Errorf("indenting %s: the refusal must show in the footer, got %q", title, got.footer(200))
		}
		if !got.tree.grabbed {
			t.Errorf("indenting %s: a refused move keeps the item grabbed", title)
		}
		if storeRev(t, got) != before {
			t.Errorf("indenting %s wrote to the store", title)
		}
	}
	if got := storedChildren(t, path, "a"); !equalStrings(got, []string{"s1", "s2"}) {
		t.Errorf("a's subtasks = %v, want [s1 s2]", got)
	}
}

func TestMoveModeCrossIntoCollapsedCategoryFollows(t *testing.T) {
	// Descending into a collapsed subcategory reveals it so the cursor keeps up.
	m, _ := newTestModel(t, "# Work\n\n- [ ] b\n\n## Sub\n\n- [ ] w1\n")
	m = press(m, "down", "down") // Sub
	m = press(m, "left")         // collapse Sub
	m = press(m, "up")           // back to task b
	b := m.tree.selected()
	if b == nil || b.Title != "b" {
		t.Fatalf("expected to be on task b, got %v", b)
	}
	m = press(m, "m", "down") // grab b, descend into (collapsed) Sub
	if b = find(m.doc, "b"); b.Parent == nil || b.Parent.Title != "Sub" {
		t.Fatalf("b should have descended into Sub, parent=%v", b.Parent)
	}
	if !m.tree.grabbed {
		t.Error("b should stay grabbed across the move's reload")
	}
	if m.tree.selected() != b {
		t.Error("the cursor should follow b into the revealed subcategory")
	}
}
