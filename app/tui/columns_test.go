package tui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/candy-tools/todo/internal/todo"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

func TestLShowsThreeColumns(t *testing.T) {
	m, _ := twoSprints(t) // Sprint 1 has "working" In Progress
	m = send(m, tea.WindowSizeMsg{Width: 140, Height: 30})
	m = press(m, "s", "tab")
	if m.view.column != todo.ColNext {
		t.Fatalf("board opens on %v, want Next", m.view.column)
	}
	// On the Next tab, the In Progress task is not on screen.
	if v := ansi.Strip(m.View()); strings.Contains(v, "working") {
		t.Fatalf("tabs layout on Next shows the In Progress task:\n%s", v)
	}
	m = press(m, "L")
	if !m.columns {
		t.Fatal("L should turn the columns layout on")
	}
	v := ansi.Strip(m.View())
	// "working" is in a column that is not the active one: only the columns
	// layout draws it.
	for _, want := range []string{"Next 0", "In Progress 1", "Done 0", "◐ working", " │ "} {
		if !strings.Contains(v, want) {
			t.Errorf("view lacks %q:\n%s", want, v)
		}
	}
	// The column titles replace the tab bar.
	if strings.Contains(v, "[Next 0]") {
		t.Errorf("columns layout still shows the tab bar:\n%s", v)
	}
	m = press(m, "L")
	if m.columns {
		t.Error("L again goes back to tabs")
	}
	if v := ansi.Strip(m.View()); !strings.Contains(v, "[Next 0]") || strings.Contains(v, "working") {
		t.Errorf("tabs layout not back:\n%s", v)
	}
}

func TestLRefusedWhenNarrow(t *testing.T) {
	m, _ := twoSprints(t)
	m = send(m, tea.WindowSizeMsg{Width: 80, Height: 30})
	m = press(m, "s", "tab", "L")
	if m.columns || !strings.Contains(m.footer(200), "too narrow for columns") {
		t.Fatalf("narrow L: columns %v footer %q", m.columns, m.footer(200))
	}
}

func TestColumnsFallBackToTabsWhenNarrowed(t *testing.T) {
	m, _ := twoSprints(t)
	m = send(m, tea.WindowSizeMsg{Width: 140, Height: 30})
	m = press(m, "s", "tab", "L")
	m = send(m, tea.WindowSizeMsg{Width: 80, Height: 30})
	if !m.columns {
		t.Fatal("a resize must keep the columns flag")
	}
	if v := ansi.Strip(m.View()); !strings.Contains(v, "[Next 0]") || strings.Contains(v, "working") {
		t.Fatalf("narrow board should show tabs:\n%s", v)
	}
	m = send(m, tea.WindowSizeMsg{Width: 140, Height: 30})
	if v := ansi.Strip(m.View()); !strings.Contains(v, "working") {
		t.Fatalf("wide again, columns should be back:\n%s", v)
	}
}

func TestLTogglesFromSidebar(t *testing.T) {
	m, _ := twoSprints(t)
	m = send(m, tea.WindowSizeMsg{Width: 140, Height: 30})
	m = press(m, "s")
	if !m.listFocus {
		t.Fatal("s should focus the sprint list")
	}
	m = press(m, "L")
	if !m.columns || !m.listFocus {
		t.Fatalf("L on the sidebar: columns %v listFocus %v", m.columns, m.listFocus)
	}
	if v := ansi.Strip(m.View()); !strings.Contains(v, "◐ working") {
		t.Errorf("board beside the sidebar should show columns:\n%s", v)
	}
}

func TestLDoesNothingOffTheBoard(t *testing.T) {
	m, _ := twoSprints(t)
	m = send(m, tea.WindowSizeMsg{Width: 140, Height: 30})
	if m = press(m, "L"); m.columns || m.status != "" {
		t.Errorf("L on the Backlog: columns %v status %q", m.columns, m.status)
	}
	if m = press(m, "s", "u", "L"); m.columns || m.status != "" {
		t.Errorf("L in the unified view: columns %v status %q", m.columns, m.status)
	}
}

func TestColumnsKeepColumnKeysAndFooter(t *testing.T) {
	m, _ := twoSprints(t)
	m = send(m, tea.WindowSizeMsg{Width: 140, Height: 30})
	m = press(m, "s", "tab")
	if f := ansi.Strip(m.footer(200)); !strings.Contains(f, "L: Columns") {
		t.Errorf("tabs footer = %q, want L: Columns", f)
	}
	m = press(m, "L", "2")
	if m.view.column != todo.ColInProgress {
		t.Fatalf("2 in columns: column %v, want In Progress", m.view.column)
	}
	if f := ansi.Strip(m.footer(200)); !strings.Contains(f, "L: Tabs") {
		t.Errorf("columns footer = %q, want L: Tabs", f)
	}
	// The active column is the real tree: its task has the cursor.
	m = press(m, "down") // past the category row, onto the task
	v := ansi.Strip(m.View())
	if !strings.Contains(v, "working") {
		t.Fatalf("active column should be the tree:\n%s", v)
	}
	// The cursor sits in the middle (active) column's cells, under its title.
	var start, cursor = -1, -1
	for _, line := range strings.Split(v, "\n") {
		if i := strings.Index(line, "In Progress 1"); i >= 0 && start < 0 {
			start = ansi.StringWidth(line[:i])
		}
		if i := strings.LastIndex(line, "❯"); i >= 0 && cursor < 0 && strings.Contains(line, "working") {
			cursor = ansi.StringWidth(line[:i])
		}
	}
	if cw := columnWidth(m.boardWidth()); start < 0 || cursor < start || cursor >= start+cw {
		t.Errorf("cursor at cell %d, want within the middle column [%d, %d):\n%s", cursor, start, start+cw, v)
	}
}

func TestColumnPreviewCutsAndCounts(t *testing.T) {
	var tasks []seedTask
	for i := range 6 {
		tasks = append(tasks, seedTask{ID: fmt.Sprintf("t%06d", i+1), Title: fmt.Sprintf("a very long task title %d", i), Status: "done", Sprint: "s00000a"})
	}
	m, _ := newJSONModel(t, sprintJSON(t, nil, []seedSprint{{ID: "s00000a", Name: "A", Status: "open"}}, tasks...))
	lines := strings.Split(ansi.Strip(columnPreview(m.doc, m.view, "s00000a", todo.ColDone, 12, 3)), "\n")
	if len(lines) != 3 || !strings.Contains(lines[2], "4 more") {
		t.Fatalf("preview = %q, want 2 rows then '… 4 more'", lines)
	}
	for _, l := range lines {
		if ansi.StringWidth(l) > 12 {
			t.Errorf("%q is wider than 12", l)
		}
	}
}

func TestColumnPreviewFiltersAndEmpty(t *testing.T) {
	m, _ := newJSONModel(t, sprintJSON(t, nil, []seedSprint{{ID: "s00000a", Name: "A", Status: "open"}},
		seedTask{ID: "t000001", Title: "alpha", Status: "working", Sprint: "s00000a"},
		seedTask{ID: "t000002", Title: "beta", Status: "review", Sprint: "s00000a"},
	))
	got := func() string {
		return ansi.Strip(columnPreview(m.doc, m.view, "s00000a", todo.ColInProgress, 30, 5))
	}
	if p := got(); !strings.Contains(p, "alpha") || !strings.Contains(p, "beta") {
		t.Fatalf("unfiltered preview = %q", p)
	}
	m.view.filter = "alp"
	if p := got(); !strings.Contains(p, "alpha") || strings.Contains(p, "beta") {
		t.Errorf("searched preview = %q, want alpha only", p)
	}
	m.view.filter = ""
	m.view.states[todo.Review] = true
	if p := got(); strings.Contains(p, "alpha") || !strings.Contains(p, "beta") {
		t.Errorf("state-filtered preview = %q, want beta only", p)
	}
	if p := ansi.Strip(columnPreview(m.doc, m.view, "s00000a", todo.ColDone, 30, 5)); p != "—" {
		t.Errorf("empty preview = %q, want —", p)
	}
}

func TestColumnsCursorVisibleOnShortTerminal(t *testing.T) {
	var tasks []seedTask
	for i := range 30 {
		tasks = append(tasks, seedTask{ID: fmt.Sprintf("t%06d", i+1), Title: fmt.Sprintf("task%02d", i), Status: "new", Sprint: "s00000a"})
	}
	m, _ := newJSONModel(t, sprintJSON(t, nil, []seedSprint{{ID: "s00000a", Name: "A", Status: "open"}}, tasks...))
	m = send(m, tea.WindowSizeMsg{Width: 140, Height: 14})
	m = press(m, "s", "tab", "L")
	for range 30 { // the category row first, then the 30 tasks
		m = press(m, "down")
	}
	if it := m.tree.selected(); it == nil || it.Title != "task29" {
		t.Fatalf("cursor on %v, want task29", it)
	}
	if v := m.View(); !strings.Contains(v, "task29") || len(strings.Split(v, "\n")) > 14 {
		t.Fatalf("last task not visible or view too tall:\n%s", v)
	}
}

func TestColumnsTitlesStayOnAVeryShortPane(t *testing.T) {
	m, _ := twoSprints(t)
	m = send(m, tea.WindowSizeMsg{Width: 140, Height: 7}) // two pane rows: one head line, one task row
	m = press(m, "s", "tab", "L", "2", "down")            // past the category row, onto the task
	v := ansi.Strip(m.View())
	if !strings.Contains(v, "In Progress 1") || !strings.Contains(v, "working") || strings.Contains(v, "Goal") {
		t.Fatalf("short columns pane should keep the titles and the task, not the goal:\n%s", v)
	}
	if len(strings.Split(v, "\n")) > 7 {
		t.Fatalf("view taller than the terminal:\n%s", v)
	}
}

// The active column with nothing in it draws the same dim dash as a preview.
func TestEmptyActiveColumnShowsDash(t *testing.T) {
	m, _ := twoSprints(t) // Sprint 1 has nothing in Next
	m = send(m, tea.WindowSizeMsg{Width: 140, Height: 30})
	m = press(m, "s", "tab", "L")
	if m.view.column != todo.ColNext {
		t.Fatalf("board opens on %v, want Next", m.view.column)
	}
	v := ansi.Strip(m.View())
	if strings.Contains(v, "nothing in") || !strings.Contains(v, "—") {
		t.Errorf("empty active column should show a dash, not 'nothing in':\n%s", v)
	}
}

// [ and ] move the active column in the columns layout, as on tabs.
func TestColumnsBracketsMoveTheActiveColumn(t *testing.T) {
	m, _ := twoSprints(t)
	m = send(m, tea.WindowSizeMsg{Width: 140, Height: 30})
	m = press(m, "s", "tab", "L")
	for _, tc := range []struct {
		key  string
		want todo.Column
	}{{"]", todo.ColInProgress}, {"]", todo.ColDone}, {"[", todo.ColInProgress}, {"[", todo.ColNext}} {
		m = press(m, tc.key)
		if m.view.column != tc.want || !m.columns {
			t.Fatalf("after %s: column %v columns %v, want %v in columns", tc.key, m.view.column, m.columns, tc.want)
		}
	}
}

// a and A move a task between columns: it then shows in the neighbouring
// column's preview with its new state.
func TestColumnsMoveShowsInNeighbourPreview(t *testing.T) {
	m, _ := twoSprints(t)
	m = send(m, tea.WindowSizeMsg{Width: 140, Height: 30})
	m = press(m, "s", "tab", "L", "2", "down") // on "working", In Progress
	if it := m.tree.selected(); it == nil || it.Title != "working" {
		t.Fatalf("cursor on %v, want working", it)
	}
	preview := func(c todo.Column) string {
		return ansi.Strip(columnPreview(m.doc, m.view, "s000001", c, 30, 5))
	}
	glyph := func(it *todo.Item) string {
		g, _ := statusGlyph(it.Status, false)
		return g + " working"
	}
	m = press(m, "a")
	it := find(m.doc, "working")
	if it == nil || it.Column() != todo.ColDone {
		t.Fatalf("after a: %v, want the task in Done", it)
	}
	if p := preview(todo.ColDone); !strings.Contains(p, glyph(it)) {
		t.Errorf("Done preview = %q, want %q", p, glyph(it))
	}
	if v := ansi.Strip(m.View()); !strings.Contains(v, "Done 1") || !strings.Contains(v, glyph(it)) {
		t.Errorf("columns view should show the task in the Done column:\n%s", v)
	}
	m = press(m, "]", "down", "A") // follow it to Done, then send it back
	it = find(m.doc, "working")
	if it == nil || it.Column() != todo.ColInProgress {
		t.Fatalf("after A: %v, want the task back in In Progress", it)
	}
	if p := preview(todo.ColInProgress); !strings.Contains(p, glyph(it)) {
		t.Errorf("In Progress preview = %q, want %q", p, glyph(it))
	}
}
