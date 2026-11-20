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
	if !strings.Contains(h, "[s Sprints]") || !strings.Contains(h, "Sprints › A › In Progress") {
		t.Fatalf("sprints header %q", h)
	}
}

func TestCrumbs(t *testing.T) {
	m, _ := threeSprints(t)
	m = press(m, "u")
	if h := ansi.Strip(m.header(120)); !strings.Contains(h, "[b Backlog]") || !strings.Contains(h, "All states · 5 tasks") {
		t.Errorf("unified header %q", h)
	}
	m = press(m, "u", "s", "down", "down") // C, then the folded Closed row
	if h := ansi.Strip(m.header(120)); !strings.Contains(h, "Sprints › Closed") {
		t.Errorf("Closed row header %q", h)
	}
	openBoard(&m, "s00000b")
	if h := ansi.Strip(m.header(120)); !strings.Contains(h, "Sprints › B ✓ › Next") {
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
	lines := boardHead(open, map[todo.Column]int{todo.ColNext: 1, todo.ColDone: 3}, todo.ColNext, 80)
	if len(lines) != boardChrome {
		t.Fatalf("%d lines, want %d", len(lines), boardChrome)
	}
	if l := ansi.Strip(lines[0]); !strings.Contains(l, "ship it") {
		t.Errorf("goal line %q", l)
	}
	if l := ansi.Strip(lines[1]); !strings.Contains(l, "3/4") || !strings.Contains(l, "close blocked: 1 not done") {
		t.Errorf("progress line %q", l)
	}
	l := ansi.Strip(boardHead(open, map[todo.Column]int{todo.ColDone: 2}, todo.ColNext, 80)[1])
	if !strings.Contains(l, "ready to close (c in the sidebar)") {
		t.Errorf("all done: %q", l)
	}
	l = ansi.Strip(boardHead(open, map[todo.Column]int{}, todo.ColNext, 80)[1])
	if !strings.Contains(l, "no tasks yet") {
		t.Errorf("empty: %q", l)
	}
	done := &todo.Sprint{Name: "B", Status: todo.SprintDone}
	l = ansi.Strip(boardHead(done, map[todo.Column]int{todo.ColDone: 2}, todo.ColDone, 80)[1])
	if !strings.Contains(l, "closed — c in the sidebar reopens it") {
		t.Errorf("done: %q", l)
	}
	if l := ansi.Strip(boardHead(&todo.Sprint{Name: "G"}, nil, todo.ColNext, 80)[0]); !strings.Contains(l, "—") {
		t.Errorf("no goal: %q", l)
	}
}

func TestBoardCursorVisibleOnShortTerminal(t *testing.T) {
	var tasks []seedTask
	for i := range 30 {
		tasks = append(tasks, seedTask{ID: fmt.Sprintf("t%06d", i+1), Title: fmt.Sprintf("task%02d", i), Status: "new", Sprint: "s00000a"})
	}
	m, _ := newJSONModel(t, sprintJSON(t, nil, []seedSprint{{ID: "s00000a", Name: "A", Status: "open"}}, tasks...))
	m = send(m, tea.WindowSizeMsg{Width: 100, Height: 14})
	m = press(m, "s", "tab")
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
	var tasks []seedTask
	for i := range 30 {
		tasks = append(tasks, seedTask{ID: fmt.Sprintf("t%06d", i+1), Title: fmt.Sprintf("task%02d", i), Status: "new", Sprint: "s00000a"})
	}
	m, _ := newJSONModel(t, sprintJSON(t, nil, []seedSprint{{ID: "s00000a", Name: "A", Status: "open"}}, tasks...))
	m = send(m, tea.WindowSizeMsg{Width: 100, Height: 14})
	m = press(m, "s", "tab")
	for range 30 { // the category row first, then the 30 tasks
		m = press(m, "down")
	}
	m = press(m, "b", "s", "tab")
	if v := m.View(); !strings.Contains(v, "task29") {
		t.Fatalf("the selected task is off screen after switching back:\n%s", v)
	}
}

// Only the board gives rows to its head: the sidebar keeps the pane's height.
func TestSidebarKeepsFullHeight(t *testing.T) {
	m, _ := threeSprints(t)
	m = press(m, "s")
	if got, want := m.treeViewHeight(), m.sprintListHeight()-boardChrome; got != want {
		t.Errorf("board tree height %d, want the sidebar's %d less the head", got, m.sprintListHeight())
	}
}

func TestBoardHeadOnEmptySprint(t *testing.T) {
	m, _ := newJSONModel(t, sprintJSON(t, nil, []seedSprint{{ID: "s00000a", Name: "A", Status: "open"}}))
	m = press(m, "s")
	if v := ansi.Strip(m.View()); !strings.Contains(v, "no tasks yet") || !strings.Contains(v, "[Next 0]") {
		t.Errorf("empty sprint board:\n%s", v)
	}
}
