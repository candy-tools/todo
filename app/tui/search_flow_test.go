package tui

import (
	"errors"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

func TestSlashOpensSearch(t *testing.T) {
	m, _ := newTestModel(t, "# Work\n\n- [ ] a\n")
	m = press(m, "/")
	if m.mode != modeSearch {
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
	if m.mode == modeSearch {
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
	if m.mode == modeSearch {
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

// quits reports whether cmd ends the program: it yields tea.QuitMsg, directly
// or inside a batch. Commands that do not answer at once (the text input's
// cursor blink waits on a timer) are taken as not quitting; tea.Quit answers
// at once.
func quits(cmd tea.Cmd) bool {
	if cmd == nil {
		return false
	}
	out := make(chan tea.Msg, 1)
	go func() { out <- cmd() }()
	select {
	case msg := <-out:
		switch msg := msg.(type) {
		case tea.QuitMsg:
			return true
		case tea.BatchMsg:
			for _, c := range msg {
				if quits(c) {
					return true
				}
			}
		}
		return false
	case <-time.After(50 * time.Millisecond):
		return false
	}
}

func TestCtrlCQuitsWhileSearching(t *testing.T) {
	m, _ := newTestModel(t, "# Work\n\n- [ ] a\n")
	m = press(m, "/")
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if cmd == nil {
		t.Fatal("ctrl+c in the search bar must quit, got no command")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Error("ctrl+c in the search bar must quit")
	}
}

func TestResizeWhileSearchingSizesInput(t *testing.T) {
	m, _ := newTestModel(t, "# Work\n\n- [ ] a\n")
	m = press(m, "/")
	m = send(m, tea.WindowSizeMsg{Width: 60, Height: 30})
	if m.search.Width != 54 {
		t.Errorf("a resize must size the open search input to the footer: width %d, want 54", m.search.Width)
	}
}

func TestSearchCapturesMainKeys(t *testing.T) {
	m, _ := newTestModel(t, "# Work\n\n- [ ] a\n")
	m = press(m, "/")
	for _, k := range []string{"q", "?", "v", "m"} {
		nm, cmd := m.Update(keyMsg(k))
		m = nm.(model)
		if quits(cmd) {
			t.Fatalf("%q in the search bar must be typed, not quit", k)
		}
	}
	if m.mode != modeSearch {
		t.Errorf("the search bar must stay open: mode %v, want modeSearch", m.mode)
	}
	if m.tree.grabbed || m.split {
		t.Errorf("keys typed into the search bar must not act: grabbed %v, split %v", m.tree.grabbed, m.split)
	}
	if m.view.filter != "q?vm" {
		t.Errorf("filter = %q, want q?vm", m.view.filter)
	}
}

func TestSearchBarWinsFooterOverError(t *testing.T) {
	m, _ := newTestModel(t, "# Work\n\n- [ ] a\n")
	m.err = errors.New("save failed: boom")
	m = press(m, "/")
	m = typeText(m, "xy")
	got := ansi.Strip(m.footer(200))
	if !strings.HasPrefix(got, " / xy") {
		t.Errorf("the open search bar must replace the footer, got %q", got)
	}
	if strings.Contains(got, "boom") {
		t.Errorf("the error must not hide the open search bar, got %q", got)
	}
}
