package tui

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/candy-tools/todo/internal/todo"
)

func TestSwitcherMarksWorkspaceAndCrumb(t *testing.T) {
	m, _ := threeSprints(t)
	h := ansi.Strip(m.header(120))
	if !strings.Contains(h, "[b Backlog]") || !strings.Contains(h, "s Sprints") || !strings.Contains(h, "p Planning") ||
		!strings.Contains(h, "Backlog › C · 1 tasks waiting") {
		t.Fatalf("backlog header %q", h)
	}
	m = press(m, "s", "tab", "]")
	h = ansi.Strip(m.header(120))
	if !strings.Contains(h, "[s Sprints]") || !strings.Contains(h, "Sprints › A") || strings.Contains(h, "Sprints › A ›") {
		t.Fatalf("sprints header %q", h)
	}
}

func TestCrumbs(t *testing.T) {
	m, _ := threeSprints(t)
	m = press(m, "u")
	if h := ansi.Strip(m.header(120)); !strings.Contains(h, "[b Backlog]") || !strings.Contains(h, "All states · 5 tasks") {
		t.Errorf("unified header %q", h)
	}
	m = press(m, "u", "s", "down", "down") // C, then the done B
	if h := ansi.Strip(m.header(120)); !strings.Contains(h, "Sprints › B ✓") {
		t.Errorf("done sprint header %q", h)
	}

	empty, _ := newJSONModel(t, sprintJSON(t, nil, nil))
	empty = press(empty, "s")
	if h := ansi.Strip(empty.header(120)); !strings.Contains(h, "Sprints › no sprints yet") {
		t.Errorf("no sprints header %q", h)
	}
}

func TestSwitcherBarTruncates(t *testing.T) {
	line := switcherBar(screenSprint, strings.Repeat("crumb ", 40), 50)
	if w := ansi.StringWidth(line); w > 50 {
		t.Errorf("switcher is %d wide, want at most 50", w)
	}
}

func TestBoardHeadReadiness(t *testing.T) {
	open := &todo.Sprint{Name: "A", Goal: "ship it", Status: todo.SprintOpen}
	lines := boardHead(open, map[todo.Column]int{todo.ColNext: 1, todo.ColDone: 3}, 80, boardChrome)
	if len(lines) != boardChrome {
		t.Fatalf("%d lines, want %d", len(lines), boardChrome)
	}
	if l := ansi.Strip(lines[0]); !strings.Contains(l, "ship it") {
		t.Errorf("goal line %q", l)
	}
	if l := ansi.Strip(lines[1]); !strings.Contains(l, "3/4") || !strings.Contains(l, "close blocked: 1 not done") {
		t.Errorf("progress line %q", l)
	}
	l := ansi.Strip(boardHead(open, map[todo.Column]int{todo.ColDone: 2}, 80, boardChrome)[1])
	if !strings.Contains(l, "ready to close (c in the sidebar)") {
		t.Errorf("all done: %q", l)
	}
	l = ansi.Strip(boardHead(open, map[todo.Column]int{}, 80, boardChrome)[1])
	if !strings.Contains(l, "no tasks yet") {
		t.Errorf("empty: %q", l)
	}
	done := &todo.Sprint{Name: "B", Status: todo.SprintDone}
	l = ansi.Strip(boardHead(done, map[todo.Column]int{todo.ColDone: 2}, 80, boardChrome)[1])
	if !strings.Contains(l, "closed — c in the sidebar reopens it") {
		t.Errorf("done: %q", l)
	}
	if l := ansi.Strip(boardHead(&todo.Sprint{Name: "G"}, nil, 80, boardChrome)[0]); !strings.Contains(l, "—") {
		t.Errorf("no goal: %q", l)
	}
}

// longBoard is a list with one open sprint A holding n tasks, task00 to
// task<n-1>, sized to a width x height terminal.
func longBoard(t *testing.T, n, width, height int) model {
	t.Helper()
	var tasks []seedTask
	for i := range n {
		tasks = append(tasks, seedTask{ID: fmt.Sprintf("t%06d", i+1), Title: fmt.Sprintf("task%02d", i), Status: "new", Sprint: "s00000a"})
	}
	m, _ := newJSONModel(t, sprintJSON(t, nil, []seedSprint{{ID: "s00000a", Name: "A", Goal: "goal a", Status: "open"}}, tasks...))
	return send(m, tea.WindowSizeMsg{Width: width, Height: height})
}

// checkTreeHeight fails unless the trees' stored view height is the one
// mainView draws them at.
func checkTreeHeight(t *testing.T, m model, after string) {
	t.Helper()
	if got, want := m.tree.viewHeight, m.treeViewHeight(); got != want {
		t.Errorf("after %s: tree viewHeight %d, want %d", after, got, want)
	}
	if got, want := m.side.viewHeight, m.treeViewHeight(); got != want {
		t.Errorf("after %s: sidebar viewHeight %d, want %d", after, got, want)
	}
}

func TestBoardCursorVisibleOnShortTerminal(t *testing.T) {
	m := longBoard(t, 30, 100, 14)
	m = press(m, "s", "tab")
	checkTreeHeight(t, m, "s tab")
	for range 30 { // the category row first, then the 30 tasks
		m = press(m, "down")
	}
	if v := m.View(); !strings.Contains(v, "task29") {
		t.Fatalf("the selected last task is off screen:\n%s", v)
	}
	if n := len(strings.Split(m.View(), "\n")); n > 14 {
		t.Errorf("view is %d lines on a 14-line terminal", n)
	}
}

// Switching to the Sprints workspace, whose board head takes rows, must keep
// the cursor visible even when it was scrolled to the bottom on the Backlog.
func TestCursorVisibleAfterSwitchingWorkspace(t *testing.T) {
	m := longBoard(t, 30, 100, 14)
	m = press(m, "s", "tab")
	for range 30 { // the category row first, then the 30 tasks
		m = press(m, "down")
	}
	m = press(m, "b", "s", "tab")
	checkTreeHeight(t, m, "b s tab")
	if v := m.View(); !strings.Contains(v, "task29") {
		t.Fatalf("the selected task is off screen after switching back:\n%s", v)
	}
}

// Every change of the view on screen re-fits the trees, not only a resize.
func TestTreeHeightFollowsTheView(t *testing.T) {
	m := longBoard(t, 30, 100, 14)
	for _, k := range []string{"s", "u", "u", "b", "u"} {
		m = press(m, k)
		checkTreeHeight(t, m, k)
	}
}

// visibleTasks lists the task titles view shows, top to bottom.
func visibleTasks(view string) []string {
	var titles []string
	for _, l := range strings.Split(ansi.Strip(view), "\n") {
		if i := strings.Index(l, "task"); i >= 0 && i+6 <= len(l) {
			titles = append(titles, l[i:i+6])
		}
	}
	return titles
}

// On a board reached after a resize, up from the bottom row moves the cursor
// alone: the page stays put, as it does on any other tree.
func TestBoardUpFromBottomKeepsPage(t *testing.T) {
	m := longBoard(t, 30, 100, 14)
	m = press(m, "s", "tab")
	for range 30 {
		m = press(m, "down")
	}
	before := visibleTasks(m.View())
	m = press(m, "up")
	if sel := m.tree.selected(); sel == nil || sel.Title != "task28" {
		t.Fatalf("up selected %v, want task28", sel)
	}
	if after := visibleTasks(m.View()); !equalStrings(after, before) {
		t.Errorf("up scrolled the page: %v, was %v", after, before)
	}
}

// On a short terminal the board head gives up its lines, goal first, then
// the rule, then progress, so the cursor's task row stays on screen.
func TestBoardHeadYieldsOnShortTerminal(t *testing.T) {
	rule := "││" + strings.Repeat("─", 20)
	for _, tc := range []struct {
		height               int
		goal, progress, rule bool
	}{
		{9, true, true, true},
		{8, false, true, true},
		{7, false, true, false},
		{6, false, false, false},
		{5, false, false, false},
	} {
		m := longBoard(t, 5, 100, tc.height)
		m = press(m, "s", "tab", "down") // task01
		checkTreeHeight(t, m, fmt.Sprintf("height %d", tc.height))
		v := ansi.Strip(m.View())
		if !strings.Contains(v, "task01") {
			t.Errorf("height %d: the cursor's task is off screen:\n%s", tc.height, v)
		}
		if got := strings.Contains(v, "Goal  goal a"); got != tc.goal {
			t.Errorf("height %d: goal shown %v, want %v:\n%s", tc.height, got, tc.goal, v)
		}
		if got := strings.Contains(v, "Done  ░"); got != tc.progress {
			t.Errorf("height %d: progress shown %v, want %v:\n%s", tc.height, got, tc.progress, v)
		}
		if got := strings.Contains(v, rule); got != tc.rule {
			t.Errorf("height %d: rule shown %v, want %v:\n%s", tc.height, got, tc.rule, v)
		}
	}
}

// At 80 columns the bar shrinks so the readiness reads in full; a pane too
// narrow even for the smallest bar cuts it with an ellipsis.
func TestBoardHeadFitsReadiness(t *testing.T) {
	m, _ := threeSprints(t)
	m = send(m, tea.WindowSizeMsg{Width: 80, Height: 24})
	openBoard(&m, "s00000a")
	if v := ansi.Strip(m.View()); !strings.Contains(v, "close blocked: 1 not done") {
		t.Errorf("80 columns: readiness cut:\n%s", v)
	}
	open := &todo.Sprint{Name: "A", Status: todo.SprintOpen}
	ready := map[todo.Column]int{todo.ColDone: 2}
	l := ansi.Strip(boardHead(open, ready, 56, boardChrome)[1])
	if !strings.Contains(l, "ready to close (c in the sidebar)") || ansi.StringWidth(l) > 56 {
		t.Errorf("56 cells: %q", l)
	}
	if !strings.Contains(l, strings.Repeat("█", 8)) || strings.Contains(l, strings.Repeat("█", 9)) {
		t.Errorf("56 cells: want an 8-cell bar, got %q", l)
	}
	for _, w := range []int{40, 24, 23, 10} {
		l := ansi.Strip(boardHead(open, ready, w, boardChrome)[1])
		if !strings.HasSuffix(l, "…") || ansi.StringWidth(l) > w {
			t.Errorf("%d cells: want the line cut with …, got %q", w, l)
		}
	}
}

func TestDoneSprintBoardTitle(t *testing.T) {
	m, _ := threeSprints(t)
	openBoard(&m, "s00000b")
	if got := m.boardTitle(); got != "B ✓ closed" {
		t.Errorf("done sprint title %q, want %q", got, "B ✓ closed")
	}
	openBoard(&m, "s00000a")
	if got := m.boardTitle(); got != "A" {
		t.Errorf("open sprint title %q, want %q", got, "A")
	}
}

// In the Backlog split the crumb follows the sidebar's category, whichever
// pane has the keys; the single tree's follows the selected item.
func TestBacklogCrumbFollowsSidebarCategory(t *testing.T) {
	m, _ := newTestModel(t, splitDoc)
	m = press(m, "down", "down", "down") // Backend, b, then a in Work
	if c := m.crumb(); !strings.HasPrefix(c, "Backlog › Work ·") {
		t.Errorf("single tree on a: crumb %q", c)
	}
	m = press(m, "up") // b, in Work › Backend
	if c := m.crumb(); !strings.HasPrefix(c, "Backlog › Work › Backend ·") {
		t.Errorf("single tree on b: crumb %q", c)
	}

	m, _ = newTestModel(t, splitDoc)
	m = press(splitAll(m), "down", "tab") // Work in the sidebar, b (Work › Backend) in the tasks
	if sel := m.tree.selected(); sel == nil || sel.Title != "b" {
		t.Fatalf("precondition: task list on %v, want b", sel)
	}
	if c := m.crumb(); !strings.HasPrefix(c, "Backlog › Work ·") {
		t.Errorf("split, tasks focused: crumb %q, want the sidebar's Work", c)
	}
	m = press(m, "tab") // back to the sidebar, on Work
	if c := m.crumb(); !strings.HasPrefix(c, "Backlog › Work ·") {
		t.Errorf("split, sidebar focused: crumb %q", c)
	}
	m = press(m, "up", "tab") // All, then the tasks
	if c := m.crumb(); strings.Contains(c, "›") {
		t.Errorf("split on All: crumb %q, want no category", c)
	}
}

// A sprint the board cannot find makes the crumb just "Sprints".
func TestCrumbWithoutShownSprint(t *testing.T) {
	m, _ := threeSprints(t)
	m = press(m, "s")
	m.view.sprint = "gone"
	if c := m.crumb(); c != "Sprints" {
		t.Errorf("crumb %q, want %q", c, "Sprints")
	}
}

// Only the board gives rows to its head: the sidebar draws its rows down to
// the pane's last line.
func TestSidebarKeepsFullHeight(t *testing.T) {
	var sprints []seedSprint
	for i := range 15 {
		sprints = append(sprints, seedSprint{ID: fmt.Sprintf("s%06d", i), Name: fmt.Sprintf("S%02d", i), Goal: fmt.Sprintf("goal%02d", i), Status: "open"})
	}
	m, _ := newJSONModel(t, sprintJSON(t, nil, sprints))
	m = send(m, tea.WindowSizeMsg{Width: 100, Height: 25}) // 20 pane rows: the Open rule, 9 sprint rows and a line
	m = press(m, "s")
	lines := strings.Split(ansi.Strip(m.View()), "\n")
	last := lines[headerHeight+m.sprintListHeight()] // under the top border
	side := []rune(last)[:sidebarWidth(100)]
	if s := strings.Trim(string(side), "│ "); !strings.HasPrefix(s, "● S09") {
		t.Errorf("sidebar's last line %q, want the 10th sprint's name", s)
	}
}

func TestBoardHeadOnEmptySprint(t *testing.T) {
	m, _ := newJSONModel(t, sprintJSON(t, nil, []seedSprint{{ID: "s00000a", Name: "A", Status: "open"}}))
	m = press(m, "s")
	if v := ansi.Strip(m.View()); !strings.Contains(v, "no tasks yet") || !strings.Contains(v, "nothing in this sprint") {
		t.Errorf("empty sprint board:\n%s", v)
	}
}
