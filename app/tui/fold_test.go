package tui

import (
	"maps"
	"slices"
	"strings"
	"testing"
)

const foldDoc = "# A\n\n## A1\n\n- [ ] x\n\n## A2\n\n- [ ] y\n\n# B\n\n- [ ] z\n"

// hasRows reports whether every title has a row in the main tree.
func hasRows(m model, titles ...string) bool {
	rows := rowTitles(m.tree)
	for _, t := range titles {
		if !slices.Contains(rows, t) {
			return false
		}
	}
	return true
}

func TestFoldLevelCollapsesSiblings(t *testing.T) {
	m, _ := newTestModel(t, foldDoc)
	m = press(selectTitle(t, m, "A1"), "z")
	rows := rowTitles(m.tree)
	if slices.Contains(rows, "x") || slices.Contains(rows, "y") {
		t.Errorf("A1 and A2 should be folded, rows %v", rows)
	}
	if !hasRows(m, "A", "A1", "A2", "B", "z") {
		t.Errorf("A and B should stay unfolded, rows %v", rows)
	}
}

func TestFoldLevelExpandsWhenAllCollapsed(t *testing.T) {
	m, _ := newTestModel(t, foldDoc)
	m = press(selectTitle(t, m, "A1"), "z", "z")
	if !hasRows(m, "x", "y") {
		t.Errorf("a second z should unfold the level, rows %v", rowTitles(m.tree))
	}
}

// With one of the level folded and one open, z folds them all.
func TestFoldLevelMixedCollapsesAll(t *testing.T) {
	m, _ := newTestModel(t, foldDoc)
	m = press(selectTitle(t, m, "A2"), "left") // fold A2 only
	m = press(selectTitle(t, m, "A1"), "z")
	if hasRows(m, "x") || hasRows(m, "y") {
		t.Errorf("z with A1 open should fold A1 and A2, rows %v", rowTitles(m.tree))
	}
}

func TestFoldLevelFromTask(t *testing.T) {
	m, _ := newTestModel(t, foldDoc)
	m = press(selectTitle(t, m, "x"), "z")
	if hasRows(m, "x") || hasRows(m, "y") {
		t.Errorf("z on a task should fold its category's level, rows %v", rowTitles(m.tree))
	}
}

func TestFoldLevelKeepsSelectionOnCategory(t *testing.T) {
	m, _ := newTestModel(t, foldDoc)
	m = press(selectTitle(t, m, "x"), "z")
	if got := m.tree.selected(); got == nil || got.Title != "A1" {
		t.Errorf("selected %v after folding x away, want A1", got)
	}
}

func TestFoldLevelRoots(t *testing.T) {
	m, _ := newTestModel(t, foldDoc)
	m = press(selectTitle(t, m, "A"), "z")
	if got, want := rowTitles(m.tree), []string{"A", "B"}; !equalStrings(got, want) {
		t.Errorf("rows = %v, want %v", got, want)
	}
}

func TestFoldLevelWhileSearchingRefused(t *testing.T) {
	m, _ := newTestModel(t, foldDoc)
	m = press(m, "/")
	m = typeText(m, "x")
	m = press(m, "enter")
	m = selectTitle(t, m, "A1")
	before := maps.Clone(m.tree.collapsed)
	m = press(m, "z")
	if !maps.Equal(m.tree.collapsed, before) {
		t.Errorf("z while searching changed the folds: %v, was %v", m.tree.collapsed, before)
	}
	if got := m.footer(200); !strings.Contains(got, "folds are off while searching") {
		t.Errorf("footer = %q", got)
	}
}

// In the split view's task list categories are headers that do not fold, so
// z changes nothing (not even the tree's hidden folds) and points at the
// sidebar.
func TestFoldLevelInTaskListRefused(t *testing.T) {
	m, _ := newTestModel(t, foldDoc)
	m = press(m, "v", "x", "down")
	before := maps.Clone(m.tree.collapsed)
	m = press(m, "z")
	if !maps.Equal(m.tree.collapsed, before) {
		t.Errorf("z in the task list changed the folds: %v, was %v", m.tree.collapsed, before)
	}
	if got := m.footer(200); !strings.Contains(got, "fold categories in the sidebar") {
		t.Errorf("footer = %q", got)
	}
}
