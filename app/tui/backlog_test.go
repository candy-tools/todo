package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

// paneTitles returns the titles on the top border of m's panes, left to right.
func paneTitles(m model) []string {
	lines := strings.Split(ansi.Strip(m.View()), "\n")
	var titles []string
	for _, box := range strings.Split(lines[headerHeight], "╮") {
		if i := strings.Index(box, "╭─ "); i >= 0 {
			titles = append(titles, strings.TrimSpace(strings.TrimRight(box[i+len("╭─ "):], "─")))
		}
	}
	return titles
}

func TestBacklogPaneTitles(t *testing.T) {
	m, _ := newTestModel(t, splitDoc)
	if got := paneTitles(m); len(got) != 1 || got[0] != "Backlog" {
		t.Errorf("single tree titles = %q, want Backlog", got)
	}
	m = press(splitAll(m), "down", "down") // All, Work, Backend
	if got := paneTitles(m); len(got) != 2 || got[0] != "Categories" || got[1] != "Work › Backend" {
		t.Errorf("split on Backend titles = %q, want Categories and the category's path", got)
	}
	m = press(m, "up", "up") // All
	if got := paneTitles(m); len(got) != 2 || got[1] != "Backlog" {
		t.Errorf("split on All titles = %q, want Backlog for the task list", got)
	}
	m = press(m, "u")
	if got := paneTitles(m); len(got) != 2 || got[1] != "Tasks" {
		t.Errorf("unified split titles = %q, want the task list's title kept", got)
	}
	if got := paneTitles(press(m, "v")); len(got) != 1 || got[0] != "Tasks" {
		t.Errorf("unified single tree titles = %q, want the title kept", got)
	}
}

func TestBacklogFooterHints(t *testing.T) {
	m, _ := newTestModel(t, splitDoc)
	f := ansi.Strip(m.footer(80))
	for _, want := range []string{"?: Help", "a: Assign", "v: Split", "s: Sprints", "p: Plan", "/: Search", "q: Quit"} {
		if !strings.Contains(f, want) {
			t.Errorf("single tree footer = %q, misses %q", f, want)
		}
	}
	m = splitAll(m)
	f = ansi.Strip(m.footer(80))
	for _, want := range []string{"?: Help", "tab: Pane", "a: Assign", "v: Single tree", "s: Sprints", "p: Plan"} {
		if !strings.Contains(f, want) {
			t.Errorf("split footer = %q, misses %q", f, want)
		}
	}
	if strings.Contains(f, "v: Split") {
		t.Errorf("split footer = %q, v goes back to the single tree there", f)
	}
	// A narrow footer drops whole hints from the end, Help first to stay.
	for _, w := range []int{80, 40, 20} {
		f := ansi.Strip(m.footer(w))
		if ansi.StringWidth(f) > w || !strings.HasPrefix(f, " ?: Help") {
			t.Errorf("footer at %d = %q, want it to fit and start with Help", w, f)
		}
		for _, part := range strings.Split(strings.TrimSpace(f), "  ") {
			if !strings.Contains(part, ": ") {
				t.Errorf("footer at %d = %q holds a cut hint %q", w, f, part)
			}
		}
	}
}

func TestEscInSplitTaskListFocusesCategories(t *testing.T) {
	m, _ := newTestModel(t, splitDoc)
	m = press(splitAll(m), "tab")
	if m.sideFocus {
		t.Fatal("setup: tab should focus the task list")
	}
	next, cmd := m.Update(keyMsg("esc"))
	m = next.(model)
	if quits(cmd) || !m.sideFocus {
		t.Fatalf("esc in the task list: quits %v sideFocus %v, want the Categories sidebar focused", quits(cmd), m.sideFocus)
	}
	if !quitsConfirmed(t, m, "esc") {
		t.Error("esc on the Categories sidebar should quit once confirmed")
	}
}

func TestAOnBacklogCategoryHintsToSelectATask(t *testing.T) {
	m, _ := newTestModel(t, splitDoc) // the cursor starts on Work
	if m = press(m, "a"); m.status != "select a task to assign it to a sprint" {
		t.Errorf("a on a Backlog category: status %q", m.status)
	}
	b, _ := threeSprints(t)
	b = press(b, "s", "tab", "u") // the unified view over A's board, on its top category row
	b.tree.selectTop()
	if it := b.selectedItem(); it == nil || it.IsTask() {
		t.Fatalf("setup: selected %v, want a category row", it)
	}
	if b = press(b, "a"); b.status != "only tasks move between columns" {
		t.Errorf("a on a category in the unified view: status %q", b.status)
	}
}
