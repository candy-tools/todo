package tui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/candy-tools/todo/internal/todo"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

// sectionsBoard is a board of sprint A with n tasks in each of its three
// sections, under one category, at width x height.
func sectionsBoard(t *testing.T, n, width, height int) model {
	t.Helper()
	var tasks []seedTask
	for i, st := range []string{"new", "working", "done"} {
		for j := range n {
			tasks = append(tasks, seedTask{ID: fmt.Sprintf("t%03d%03d", i, j), Title: fmt.Sprintf("%s%02d", st, j), Status: st, Sprint: "s00000a"})
		}
	}
	m, _ := newJSONModel(t, sprintJSON(t, nil, []seedSprint{{ID: "s00000a", Name: "A", Goal: "goal a"}}, tasks...))
	m = send(m, tea.WindowSizeMsg{Width: width, Height: height})
	return press(m, "s", "tab")
}

// boardLines is the board pane's text, one string per line, without the
// sidebar and the borders.
func boardLines(m model) []string {
	var out []string
	sw := sidebarWidth(m.width)
	for _, l := range strings.Split(ansi.Strip(m.View()), "\n") {
		r := []rune(l)
		if len(r) > sw+1 {
			out = append(out, strings.TrimRight(string(r[sw+1:len(r)-1]), " "))
		}
	}
	return out
}

func TestSectionRowFillsWidth(t *testing.T) {
	r := treeRow{kind: rowSection, col: todo.ColInProgress, count: 3}
	l := ansi.Strip(sectionRow(r, 30))
	if !strings.HasPrefix(l, "━━ In Progress 3 ━") || ansi.StringWidth(l) != 30 {
		t.Errorf("section row = %q (%d wide), want the title and count, ruled to 30", l, ansi.StringWidth(l))
	}
	if l := ansi.Strip(sectionRow(r, 8)); ansi.StringWidth(l) > 8 {
		t.Errorf("a narrow section row is %d wide, want at most 8", ansi.StringWidth(l))
	}
}

func TestBoardHeadRuleSetsTasksApart(t *testing.T) {
	m := sectionsBoard(t, 1, 100, 30)
	lines := boardLines(m)
	var at int
	for i, l := range lines {
		if strings.Contains(l, "Done  ") && strings.Contains(l, "1/3") {
			at = i
		}
	}
	if at == 0 || at+2 >= len(lines) {
		t.Fatalf("no progress line in:\n%s", strings.Join(lines, "\n"))
	}
	if rule := lines[at+1]; strings.Trim(rule, "─") != "" || rule == "" {
		t.Errorf("the line under the progress = %q, want a rule", rule)
	}
	if next := lines[at+2]; !strings.HasPrefix(next, "━━ Next 1") {
		t.Errorf("the line under the rule = %q, want the Next section", next)
	}
}

// A jump scrolls its section to the top of the pane, headings included.
func TestSectionJumpShowsSectionFromTop(t *testing.T) {
	m := sectionsBoard(t, 8, 100, 16)
	m = press(m, "2")
	if sel := m.tree.selected(); sel == nil || sel.Title != "working00" {
		t.Fatalf("2 selected %v, want working00", sel)
	}
	lines := boardLines(m)
	top := headerHeight + 1 + boardChrome // under the header, the top border and the head
	if !strings.HasPrefix(lines[top], "━━ In Progress 8") {
		t.Errorf("the In Progress section must open the tree:\n%s", strings.Join(lines, "\n"))
	}
	m = press(m, "3")
	if !strings.Contains(strings.Join(boardLines(m), "\n"), "━━ Done 8") {
		t.Errorf("3 must show the Done section:\n%s", strings.Join(boardLines(m), "\n"))
	}
}

// Moving up onto a section's first task brings its headings back into view.
func TestUpShowsHeadingsAboveFirstTask(t *testing.T) {
	m := sectionsBoard(t, 8, 100, 16)
	m = press(m, "2", "down", "down")
	m = press(m, "up", "up", "up") // onto new07, the last task of Next
	if sel := m.tree.selected(); sel == nil || sel.Title != "new07" {
		t.Fatalf("selected %v, want new07", sel)
	}
	m = press(m, "1")
	text := strings.Join(boardLines(m), "\n")
	if !strings.Contains(text, "━━ Next 8") || !strings.Contains(text, "── C") {
		t.Errorf("the Next section and its category header must show above new00:\n%s", text)
	}
}

// A grabbed task steps only past the tasks of its own section.
func TestGrabStepsWithinSection(t *testing.T) {
	m, path := newJSONModel(t, sprintJSON(t, nil, []seedSprint{{ID: "s00000a", Name: "A"}},
		seedTask{ID: "t000001", Title: "n1", Status: "new", Sprint: "s00000a"},
		seedTask{ID: "t000002", Title: "w1", Status: "working", Sprint: "s00000a"},
		seedTask{ID: "t000003", Title: "n2", Status: "new", Sprint: "s00000a"},
	))
	m = press(m, "s", "tab")
	m = selectTitle(t, m, "n2")
	m = press(m, "m", "up", "m")
	if got, want := storedChildren(t, path, "C"), []string{"n2", "n1", "w1"}; !equalStrings(got, want) {
		t.Errorf("stored order = %v, want %v: n2 passes n1, skipping w1 of In Progress", got, want)
	}
	if got := sectionTitles(m.tree, todo.ColNext); !equalStrings(got, []string{"n2", "n1"}) {
		t.Errorf("Next rows = %v, want [n2 n1]", got)
	}
}
