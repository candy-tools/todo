package tui

import (
	"strings"
	"testing"
)

func TestSlashOpensSearch(t *testing.T) {
	m, _ := newTestModel(t, "# Work\n\n- [ ] a\n")
	m = press(m, "/")
	if !m.searching {
		t.Fatal("/ should open the search input")
	}
}

func TestSearchFiltersLive(t *testing.T) {
	m, _ := newTestModel(t, "# Work\n\n- [ ] Task A\n  - [ ] Explore catacombs\n  - [ ] Buy supplies\n- [ ] Task B\n")
	m = press(m, "/")
	m = typeText(m, "cat")
	got := rowTitles(m.tree)
	want := []string{"Work", "Task A", "Explore catacombs"}
	if !equalStrings(got, want) {
		t.Errorf("live-filtered rows = %v, want %v", got, want)
	}
}

func TestSearchEscClearsFilter(t *testing.T) {
	m, _ := newTestModel(t, "# Work\n\n- [ ] Task A\n  - [ ] Explore catacombs\n")
	before := len(m.tree.rows)
	m = press(m, "/")
	m = typeText(m, "cat")
	m = press(m, "esc")
	if m.searching {
		t.Error("esc should close the search input")
	}
	if m.view.filter != "" {
		t.Errorf("esc should clear the filter, got %q", m.view.filter)
	}
	if len(m.tree.rows) != before {
		t.Errorf("esc should restore the tree: got %d rows, want %d", len(m.tree.rows), before)
	}
}

func TestSearchEnterKeepsFilter(t *testing.T) {
	m, _ := newTestModel(t, "# Work\n\n- [ ] Task A\n  - [ ] Explore catacombs\n  - [ ] Buy supplies\n")
	m = press(m, "/")
	m = typeText(m, "cat")
	m = press(m, "enter")
	if m.searching {
		t.Error("enter should stop editing the query")
	}
	if m.view.filter != "cat" {
		t.Errorf("enter should keep the filter active, got %q", m.view.filter)
	}
	got := rowTitles(m.tree)
	want := []string{"Work", "Task A", "Explore catacombs"}
	if !equalStrings(got, want) {
		t.Errorf("filtered rows after enter = %v, want %v", got, want)
	}
}

func TestEscInMainClearsActiveFilter(t *testing.T) {
	m, _ := newTestModel(t, "# Work\n\n- [ ] Task A\n  - [ ] Explore catacombs\n")
	before := len(m.tree.rows)
	m = press(m, "/")
	m = typeText(m, "cat")
	m = press(m, "enter") // filter active, no longer editing
	m = press(m, "esc")   // esc in the main view clears the active filter (not quit)
	if m.view.filter != "" {
		t.Errorf("esc in the main view should clear the active filter, got %q", m.view.filter)
	}
	if len(m.tree.rows) != before {
		t.Errorf("clearing the filter should restore the tree: got %d rows, want %d", len(m.tree.rows), before)
	}
}

func TestClearFilterKeepsFocusOnNavigatedItem(t *testing.T) {
	m, _ := newTestModel(t, "# Work\n\n- [ ] catalog\n- [ ] category\n- [ ] other\n")
	m = press(m, "/")
	m = typeText(m, "cat") // filtered rows: Work, catalog, category
	m = press(m, "down", "down")
	if sel := m.tree.selected(); sel == nil || sel.Title != "category" {
		t.Fatalf("precondition: cursor should be on \"category\" while filtering, got %v", sel)
	}
	m = press(m, "esc") // clear the filter
	if m.view.filter != "" {
		t.Fatalf("esc should clear the filter, got %q", m.view.filter)
	}
	if sel := m.tree.selected(); sel == nil || sel.Title != "category" {
		t.Errorf("clearing the filter should keep focus on \"category\", got %v", sel)
	}
}

func TestClearFilterFromMainViewKeepsFocus(t *testing.T) {
	m, _ := newTestModel(t, "# Work\n\n- [ ] catalog\n- [ ] category\n- [ ] other\n")
	m = press(m, "/")
	m = typeText(m, "cat")
	m = press(m, "enter")        // keep the filter, leave the search bar
	m = press(m, "down", "down") // navigate in the main view to "category"
	if sel := m.tree.selected(); sel == nil || sel.Title != "category" {
		t.Fatalf("precondition: cursor should be on \"category\", got %v", sel)
	}
	m = press(m, "esc") // esc in the main view clears the active filter
	if sel := m.tree.selected(); sel == nil || sel.Title != "category" {
		t.Errorf("clearing the filter from the main view should keep focus on \"category\", got %v", sel)
	}
}

func TestClearFilterRevealsItemInsideCollapsedParent(t *testing.T) {
	m, _ := newTestModel(t, "# Work\n\n- [ ] Task A\n  - [ ] Explore catacombs\n")
	m.tree.collapsed[find(m.doc, "Task A").ID] = true // the match is hidden in the unfiltered tree
	m = press(m, "/")
	m = typeText(m, "cat") // filtering reveals the match regardless of the fold
	m = press(m, "down", "down")
	if sel := m.tree.selected(); sel == nil || sel.Title != "Explore catacombs" {
		t.Fatalf("precondition: cursor should be on the match while filtering, got %v", sel)
	}
	m = press(m, "esc") // clear the filter
	if sel := m.tree.selected(); sel == nil || sel.Title != "Explore catacombs" {
		t.Errorf("clearing the filter should reveal and keep focus on the match, got %v", sel)
	}
}

func TestSearchMatchesDescription(t *testing.T) {
	m, _ := newTestModel(t, "# Work\n\n- [ ] groceries\n    remember cat food\n- [ ] laundry\n")
	m = press(m, "/")
	m = typeText(m, "cat food")
	got := rowTitles(m.tree)
	if !contains(got, "groceries") {
		t.Errorf("a description match should keep the item visible, got %v", got)
	}
	if contains(got, "laundry") {
		t.Errorf("a non-matching item should be hidden, got %v", got)
	}
}

func TestSearchBarVisibleWhileTyping(t *testing.T) {
	m, _ := newTestModel(t, "# Work\n\n- [ ] a\n")
	m = press(m, "/")
	m = typeText(m, "xy")
	if out := m.View(); !strings.Contains(out, "xy") {
		t.Errorf("the search query should be visible in the view while typing")
	}
}

func TestFooterAdvertisesSearch(t *testing.T) {
	m, _ := newTestModel(t, "# Work\n")
	if got := m.footer(200); !strings.Contains(got, "Search") {
		t.Errorf("the footer should advertise the / search shortcut:\n%s", got)
	}
}
