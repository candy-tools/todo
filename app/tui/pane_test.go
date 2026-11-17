package tui

import (
	"strings"
	"testing"
)

const paneDoc = "# Work\n- [ ] a\n## Backend\n- [ ] b\n# Home\n- [ ] c\n# Empty\n"

// rowDesc describes each row as "kind:text" for compact assertions: item rows
// by title, headers by label, and "all" / "new" for the synthetic rows.
func rowDesc(tr tree) []string {
	var out []string
	for _, r := range tr.rows {
		switch r.kind {
		case rowAll:
			out = append(out, "all")
		case rowNewCategory:
			out = append(out, "new")
		case rowHeader:
			out = append(out, "h:"+r.label)
		default:
			out = append(out, r.item.Title)
		}
	}
	return out
}

func newPaneTree(t *testing.T, p paneKind) (tree, func(string) string) {
	t.Helper()
	d := parseDoc(t, paneDoc)
	tr := newTree(d, newViewState())
	tr.viewHeight = 20
	tr.setPane(p)
	return tr, func(title string) string { return find(d, title).ID }
}

func TestSidebarRows(t *testing.T) {
	tr, _ := newPaneTree(t, paneCategories)
	want := []string{"all", "Work", "Backend", "Home", "Empty", "new"}
	if got := rowDesc(tr); !equalStrings(got, want) {
		t.Fatalf("sidebar rows = %v, want %v", got, want)
	}
	tr.selectTop()
	if !tr.onAll() || tr.selected() != nil || tr.selectedID() != "" {
		t.Error("the sidebar should start on All, which selects no item")
	}
}

func TestTasksPaneScopeIncludesSubcategories(t *testing.T) {
	tr, id := newPaneTree(t, paneTasks)
	tr.setScope(id("Work"))
	if got := rowDesc(tr); !equalStrings(got, []string{"a", "h:Backend", "b"}) {
		t.Fatalf("Work scope rows = %v", got)
	}
	for it := range tr.shown {
		if it.Title == "c" {
			t.Error("a task outside the scope must not count as shown")
		}
	}
}

func TestTasksPaneAllScope(t *testing.T) {
	tr, _ := newPaneTree(t, paneTasks)
	want := []string{"h:Work", "a", "h:Work › Backend", "b", "h:Home", "c"}
	if got := rowDesc(tr); !equalStrings(got, want) {
		t.Fatalf("All scope rows = %v, want %v", got, want)
	}
}

func TestTasksPaneCursorSkipsHeaders(t *testing.T) {
	tr, _ := newPaneTree(t, paneTasks)
	tr.selectTop()
	steps := []struct {
		move func()
		want string
	}{
		{func() {}, "a"},
		{tr.moveDown, "b"},
		{tr.moveDown, "c"},
		{tr.moveDown, "c"},
		{tr.moveUp, "b"},
		{tr.moveUp, "a"},
		{tr.moveUp, "a"},
		{tr.pageDown, "c"},
		{tr.pageUp, "a"},
	}
	for i, s := range steps {
		s.move()
		if got := tr.selected(); got == nil || got.Title != s.want {
			t.Fatalf("step %d: selected %v (cursor %d), want %s", i, got, tr.cursor, s.want)
		}
	}
	tr.collapse() // on a top-level task: must not jump onto the header
	if got := tr.selected(); got == nil || got.Title != "a" {
		t.Errorf("left on a top-level task moved to %v", got)
	}
}

func TestTasksPaneEmptyHint(t *testing.T) {
	tr, id := newPaneTree(t, paneTasks)
	tr.setScope(id("Empty"))
	if len(tr.rows) != 0 {
		t.Fatalf("Empty scope rows = %v", rowDesc(tr))
	}
	if got := tr.view(40, 5); !strings.Contains(got, "no tasks here") {
		t.Errorf("empty pane view = %q", got)
	}
}

func TestSidebarFiltered(t *testing.T) {
	tr, _ := newPaneTree(t, paneCategories)
	tr.vs.filter = "b"
	tr.refreshSidebar()
	if got := rowDesc(tr); !equalStrings(got, []string{"all", "Work", "Backend"}) {
		t.Errorf("filtered sidebar rows = %v", got)
	}
}

func TestRefreshSidebarFallsBackToAll(t *testing.T) {
	tr, id := newPaneTree(t, paneCategories)
	if !tr.selectID(id("Home")) {
		t.Fatal("Home not in the sidebar")
	}
	tr.vs.filter = "b"
	tr.refreshSidebar()
	if !tr.onAll() {
		t.Errorf("the sidebar should fall back to All, selected %v", tr.selected())
	}
	tr.vs.filter = ""
	tr.selectID(id("Backend"))
	tr.refreshSidebar()
	if got := tr.selected(); got == nil || got.Title != "Backend" {
		t.Errorf("a still-shown category stays selected, got %v", got)
	}
}
