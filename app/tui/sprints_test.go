package tui

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/candy-tools/todo/internal/todo"
	"github.com/charmbracelet/x/ansi"
)

// loadSprintDoc reads the list file at path back.
func loadSprintDoc(t *testing.T, path string) *todo.Document {
	t.Helper()
	doc, _, err := todo.NewStore(path).Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return doc
}

func TestSprintListNewCreatesSprint(t *testing.T) {
	m, path := threeSprints(t)
	m = press(m, "s", "n")
	if m.mode != modeForm || m.form.kind != formSprint {
		t.Fatalf("n: mode %v kind %v, want the sprint form", m.mode, m.form.kind)
	}
	m = typeText(m, "Delta")
	m = press(m, "tab")
	m = typeText(m, "ship it")
	m = press(m, "enter")
	if m.mode != modeMain || m.err != nil {
		t.Fatalf("after enter: mode %v err %v", m.mode, m.err)
	}
	s, ok := loadSprintDoc(t, path).SprintByName("Delta")
	if !ok || s.Goal != "ship it" || s.Status != todo.SprintOpen {
		t.Fatalf("stored sprint = %+v (found %v), want open Delta with its goal", s, ok)
	}
	if sel := m.sprints.selected(m.doc); sel == nil || sel.ID != s.ID {
		t.Errorf("selected = %v, want the new sprint", sel)
	}
}

func TestSprintListNewDuplicateKeepsForm(t *testing.T) {
	m, _ := threeSprints(t)
	m = typeText(press(m, "s", "n"), "a")
	m = press(m, "enter")
	// A refused write is no failed save: the reason is the footer status.
	if m.mode != modeForm || m.err != nil || !strings.Contains(m.status, todo.ErrDuplicateSprint.Error()) {
		t.Errorf("a duplicate name: mode %v err %v status %q, want the form open with the refusal", m.mode, m.err, m.status)
	}
}

func TestSprintListEditRenames(t *testing.T) {
	m, path := threeSprints(t)
	m = press(m, "s", "e")
	if m.mode != modeForm || m.form.title.Value() != "A" || m.form.desc.Value() != "goal a" {
		t.Fatalf("e: mode %v name %q goal %q, want A's fields", m.mode, m.form.title.Value(), m.form.desc.Value())
	}
	m = typeText(m, "lpha")
	m = press(m, "enter")
	if m.mode != modeMain || m.err != nil {
		t.Fatalf("after enter: mode %v err %v", m.mode, m.err)
	}
	s, _ := loadSprintDoc(t, path).SprintByID("s00000a")
	if s.Name != "Alpha" || s.Goal != "goal a" {
		t.Errorf("stored sprint = %+v, want Alpha with the goal untouched", s)
	}
}

func TestSprintListCloseRefusedShowsCount(t *testing.T) {
	m, path := threeSprints(t) // A holds a task in Next
	m = press(m, "s", "c")
	if m.err != nil {
		t.Errorf("err = %v, want none: a refused close is a status", m.err)
	}
	if m.status != "can't close: 1 task not done" {
		t.Errorf("status = %q", m.status)
	}
	if s, _ := loadSprintDoc(t, path).SprintByID("s00000a"); s.Status != todo.SprintOpen {
		t.Errorf("A is %v, want still open", s.Status)
	}
	if f := m.footer(100); !strings.Contains(f, "can't close: 1 task not done") {
		t.Errorf("footer = %q", f)
	}
}

func TestSprintListCloseThenReopen(t *testing.T) {
	m, path := newJSONModel(t, sprintJSON(t, nil,
		[]seedSprint{{ID: "s00000a", Name: "A", Status: "open"}, {ID: "s00000b", Name: "B", Status: "open"}},
		seedTask{ID: "t000001", Title: "x", Status: "done", Sprint: "s00000a"}))
	m = press(m, "s", "c")
	if m.err != nil || m.status != "" {
		t.Fatalf("close: err %v status %q", m.err, m.status)
	}
	if s, _ := loadSprintDoc(t, path).SprintByID("s00000a"); s.Status != todo.SprintDone {
		t.Fatalf("A is %v, want done", s.Status)
	}
	// A moved below the open B; the cursor stays on A, so c reopens it.
	if sel := m.sprints.selected(m.doc); sel == nil || sel.ID != "s00000a" {
		t.Fatalf("selected = %v, want A after it moved", sel)
	}
	m = press(m, "c")
	if s, _ := loadSprintDoc(t, path).SprintByID("s00000a"); s.Status != todo.SprintOpen {
		t.Errorf("A is %v, want reopened", s.Status)
	}
}

func TestSprintListDeleteConfirm(t *testing.T) {
	m, path := newJSONModel(t, sprintJSON(t, nil,
		[]seedSprint{{ID: "s00000a", Name: "A", Status: "open"}},
		seedTask{ID: "t000001", Title: "w", Status: "working", Sprint: "s00000a"}))
	m = press(m, "s", "d")
	if m.mode != modeConfirm {
		t.Fatalf("d: mode %v, want the confirm dialog", m.mode)
	}
	if q, want := m.confirmQuestion(), `Delete sprint "A"? Its unfinished tasks go back to the Backlog.`; q != want {
		t.Errorf("question = %q, want %q", q, want)
	}
	m = press(m, "y")
	doc := loadSprintDoc(t, path)
	if len(doc.Sprints) != 0 {
		t.Errorf("sprints = %v, want none", doc.Sprints)
	}
	if it := find(doc, "w"); it == nil || it.Status != todo.Open || it.SprintID() != "" {
		t.Errorf("task = %+v, want open in the backlog", it)
	}
	if m.err != nil {
		t.Errorf("err = %v", m.err)
	}
}

func TestSprintListDeleteFallsBackByIndex(t *testing.T) {
	m, path := threeSprints(t) // rows: A, C (open), Closed (B)
	m = press(m, "s", "down", "d", "y")
	if len(loadSprintDoc(t, path).Sprints) != 2 {
		t.Fatalf("C was not deleted")
	}
	if !m.sprints.onGroup(m.doc) {
		t.Errorf("after deleting C: the cursor should sit on the Closed row now at C's index")
	}
	if m.view.sprint != "s00000a" {
		t.Errorf("board = %q, want A kept: the Closed row keeps the last sprint shown", m.view.sprint)
	}
	m = press(m, "right", "down")
	if s := m.sprints.selected(m.doc); s == nil || s.ID != "s00000b" {
		t.Errorf("after unfolding: selected %v, want B", s)
	}
	if !m.listFocus {
		t.Error("the sidebar should keep the focus")
	}
}

func TestSprintListDeleteCancelKeepsSprint(t *testing.T) {
	m, path := threeSprints(t)
	m = press(m, "s", "d", "esc")
	if len(loadSprintDoc(t, path).Sprints) != 3 || m.mode != modeMain {
		t.Errorf("esc must cancel the delete")
	}
}

func TestSprintListDeleteRefusedWithDone(t *testing.T) {
	m, path := threeSprints(t) // A holds a done task
	m = press(m, "s", "d", "y")
	if m.err != nil {
		t.Errorf("err = %v, want none: a refused delete is a status", m.err)
	}
	if m.status != todo.ErrSprintHasDone.Error() {
		t.Errorf("status = %q, want %q", m.status, todo.ErrSprintHasDone.Error())
	}
	if _, ok := loadSprintDoc(t, path).SprintByID("s00000a"); !ok {
		t.Error("A must still exist")
	}
}

func TestSprintListFooterShowsOwnHints(t *testing.T) {
	m, _ := threeSprints(t)
	m = press(m, "s")
	m.view.filter = "needle" // carried over from a board
	f := ansi.Strip(m.footer(120))
	if strings.Contains(f, "filter") || strings.Contains(f, "Clear") {
		t.Errorf("footer = %q, must not show the search filter", f)
	}
	for _, want := range []string{"New", "Edit", "Close/reopen", "Delete", "Board"} {
		if !strings.Contains(f, want) {
			t.Errorf("footer = %q, misses %q", f, want)
		}
	}
}

// manySprints is a list of n open sprints named S00, S01, ….
func manySprints(t *testing.T, n int) model {
	t.Helper()
	var ss []seedSprint
	for i := 0; i < n; i++ {
		ss = append(ss, seedSprint{ID: fmt.Sprintf("s0000%02d", i), Name: fmt.Sprintf("S%02d", i), Status: "open"})
	}
	m, _ := newJSONModel(t, sprintJSON(t, nil, ss))
	return press(m, "s")
}

func TestSprintListScrollsOnlyWhenCursorLeavesWindow(t *testing.T) {
	m := manySprints(t, 60)
	h := m.sprintListHeight() / rowLines // rows shown
	for i := 0; i < h+3; i++ {
		m = press(m, "down")
	}
	want := m.sprints.offset
	if want == 0 {
		t.Fatalf("offset = 0 after %d downs with %d rows shown", h+3, h)
	}
	firstRow := fmt.Sprintf("S%02d", want)
	for i := 0; i < 2; i++ {
		m = press(m, "up")
		if m.sprints.offset != want {
			t.Fatalf("up %d: offset = %d, want it to stay %d", i+1, m.sprints.offset, want)
		}
		if got := ansi.Strip(strings.Split(m.sprints.view(m.doc, 100, h*rowLines), "\n")[0]); !strings.Contains(got, firstRow) {
			t.Fatalf("up %d: first row %v, want %s", i+1, got, firstRow)
		}
	}
	// Going above the window moves it by one row.
	for m.sprints.cursor >= want {
		m = press(m, "up")
	}
	if m.sprints.offset != m.sprints.cursor {
		t.Errorf("offset = %d, cursor = %d, want the window to follow the cursor up", m.sprints.offset, m.sprints.cursor)
	}
}

func TestSprintListCursorFollowsSprintOnReload(t *testing.T) {
	m, path := newJSONModel(t, sprintJSON(t, nil, []seedSprint{
		{ID: "s00000a", Name: "A", Status: "open"},
		{ID: "s00000b", Name: "B", Status: "open"},
		{ID: "s00000c", Name: "C", Status: "open"},
	}))
	m = press(m, "s", "down", "down") // on C
	// Another session closes A and B: they move below C.
	st := openSecond(t, path)
	for _, id := range []string{"s00000a", "s00000b"} {
		if _, err := st.CloseSprint(context.Background(), id); err != nil {
			t.Fatal(err)
		}
	}
	m = pollOnce(t, m)
	if sel := m.sprints.selected(m.doc); sel == nil || sel.ID != "s00000c" {
		t.Errorf("selected = %v, want C (by ID, not index 2)", sel)
	}
}

func TestClosedGroupFoldsDoneSprints(t *testing.T) {
	m, _ := threeSprints(t) // A open, B done, C open
	m = press(m, "s")
	view := ansi.Strip(m.sprints.view(m.doc, 30, 20))
	if strings.Contains(view, " B") || !strings.Contains(view, "Closed") {
		t.Fatalf("folded: %q, want the Closed row and no B", view)
	}
	m = press(m, "down", "down") // A, C, Closed
	if !m.sprints.onGroup(m.doc) || m.view.sprint != "s00000c" {
		t.Fatalf("on the group row: onGroup %v board %q, want C kept", m.sprints.onGroup(m.doc), m.view.sprint)
	}
	m = press(m, "right", "down")
	if s := m.sprints.selected(m.doc); s == nil || s.ID != "s00000b" || m.view.sprint != "s00000b" {
		t.Fatalf("after unfolding and down: %v / %q, want B shown", s, m.view.sprint)
	}
	m = press(m, "left")
	if !m.sprints.onGroup(m.doc) {
		t.Fatal("left on a done sprint goes to the group row")
	}
	m = press(m, "left")
	if m.sprints.closedOpen {
		t.Error("left on the group row folds it")
	}
	m = press(m, "enter")
	if !m.sprints.closedOpen || !m.listFocus {
		t.Errorf("enter on the group row: closedOpen %v listFocus %v, want it unfolded and the list focused", m.sprints.closedOpen, m.listFocus)
	}
	m = press(m, "enter")
	if m.sprints.closedOpen {
		t.Error("enter on the group row toggles it again")
	}
	if m = press(m, "tab", "e", "c", "d"); !m.listFocus || m.mode != modeMain || m.view.sprint != "s00000b" {
		t.Errorf("tab/e/c/d on the group row: listFocus %v mode %v board %q, want nothing", m.listFocus, m.mode, m.view.sprint)
	}
}

func TestSidebarRowsAreTwoLinesAndCut(t *testing.T) {
	long := strings.Repeat("n", 80)
	m, _ := newJSONModel(t, sprintJSON(t, nil,
		[]seedSprint{{ID: "s00000a", Name: long, Goal: strings.Repeat("g", 80), Status: "open"}},
		seedTask{ID: "t000001", Title: "x", Status: "new", Sprint: "s00000a"}))
	lines := strings.Split(ansi.Strip(m.sprints.view(m.doc, 30, 10)), "\n")
	if len(lines) != 2 {
		t.Fatalf("one sprint renders %d lines, want 2: %q", len(lines), lines)
	}
	for _, l := range lines {
		if ansi.StringWidth(l) > 30 {
			t.Errorf("line %q is wider than 30", l)
		}
	}
	if !strings.HasSuffix(lines[0], "1·0·0") || ansi.StringWidth(lines[0]) != 30 {
		t.Errorf("first line %q, want the counts ending at the right edge (width 30)", lines[0])
	}
	if !strings.Contains(lines[0], "…") || !strings.Contains(lines[0], "1·0·0") || !strings.Contains(lines[1], "…") {
		t.Errorf("lines = %q, want the name and goal cut and the counts kept", lines)
	}
}

func TestSidebarScrollCountsLines(t *testing.T) {
	m := manySprints(t, 20)
	if got := len(strings.Split(m.sprints.view(m.doc, 30, 9), "\n")); got != 8 {
		t.Errorf("9 lines hold 4 two-line rows, got %d lines", got)
	}
	if got := len(strings.Split(m.sprints.view(m.doc, 30, 1), "\n")); got != 2 {
		t.Errorf("a one-line height still shows one row (2 lines), got %d", got)
	}
}

func TestKToDoneSprintUnfoldsClosed(t *testing.T) {
	m, _ := threeSprints(t)
	m = press(m, "K", "down", "down", "enter") // A, C, ✓ B
	if m.view.sprint != "s00000b" || !m.sprints.closedOpen {
		t.Fatalf("K to B: board %q closedOpen %v", m.view.sprint, m.sprints.closedOpen)
	}
}

func TestClosingSelectedSprintUnfoldsClosed(t *testing.T) {
	m, _ := newJSONModel(t, sprintJSON(t, nil,
		[]seedSprint{{ID: "s00000a", Name: "A", Status: "open"}, {ID: "s00000b", Name: "B", Status: "open"}},
		seedTask{ID: "t000001", Title: "x", Status: "done", Sprint: "s00000a"}))
	m = press(m, "s", "c")
	if s := m.sprints.selected(m.doc); s == nil || s.ID != "s00000a" || m.view.sprint != "s00000a" || !m.sprints.closedOpen {
		t.Errorf("after closing A: selected %v board %q closedOpen %v, want A kept, shown, under the unfolded Closed", s, m.view.sprint, m.sprints.closedOpen)
	}
}

func TestRefusedCloseKeepsClosedFolded(t *testing.T) {
	m, _ := threeSprints(t) // A holds a task in Next
	if m = press(m, "s", "c"); m.sprints.closedOpen {
		t.Error("a refused close must leave Closed folded")
	}
	if m.status != "can't close: 1 task not done" {
		t.Errorf("status = %q, want the refusal", m.status)
	}
}

func TestReloadCursorStaysOnSprintClosedElsewhere(t *testing.T) {
	m, path := newJSONModel(t, sprintJSON(t, nil,
		[]seedSprint{
			{ID: "s00000a", Name: "A", Status: "open"},
			{ID: "s00000b", Name: "B", Status: "open"},
			{ID: "s00000d", Name: "D", Status: "done"},
		}))
	m = press(m, "s") // on A
	if _, err := openSecond(t, path).CloseSprint(context.Background(), "s00000a"); err != nil {
		t.Fatal(err)
	}
	m = pollOnce(t, m)
	if s := m.sprints.selected(m.doc); s == nil || s.ID != "s00000a" || m.view.sprint != "s00000a" || !m.sprints.closedOpen {
		t.Errorf("selected %v board %q closedOpen %v, want A kept and shown under the unfolded Closed", s, m.view.sprint, m.sprints.closedOpen)
	}
}

func TestReloadClosedRowGoneMovesCursorToShownSprint(t *testing.T) {
	m, path := newJSONModel(t, sprintJSON(t, nil,
		[]seedSprint{
			{ID: "s00000a", Name: "A", Status: "open"},
			{ID: "s00000b", Name: "B", Status: "open"},
			{ID: "s00000d", Name: "D", Status: "done"},
		}))
	m = press(m, "s", "down", "down") // A, B, Closed: the board keeps B
	if !m.sprints.onGroup(m.doc) || m.view.sprint != "s00000b" {
		t.Fatalf("setup: onGroup %v board %q", m.sprints.onGroup(m.doc), m.view.sprint)
	}
	if _, err := openSecond(t, path).ReopenSprint(context.Background(), "s00000d"); err != nil {
		t.Fatal(err)
	}
	m = pollOnce(t, m)
	if s := m.sprints.selected(m.doc); s == nil || s.ID != "s00000b" || m.view.sprint != "s00000b" {
		t.Errorf("selected %v board %q, want B on both", s, m.view.sprint)
	}
}

func TestDeletingLastOpenSprintFoldsClosed(t *testing.T) {
	m, _ := newJSONModel(t, sprintJSON(t, nil,
		[]seedSprint{{ID: "s00000a", Name: "A", Status: "open"}, {ID: "s00000d", Name: "D", Status: "done"}}))
	m = press(m, "s", "down", "right", "up", "d", "y") // Closed unfolded, back on A
	if !m.sprints.onGroup(m.doc) || m.sprints.closedOpen || m.view.sprint != "" {
		t.Errorf("onGroup %v closedOpen %v board %q, want the folded Closed row and no sprint", m.sprints.onGroup(m.doc), m.sprints.closedOpen, m.view.sprint)
	}
	if out := m.View(); !strings.Contains(out, "no open sprints") {
		t.Errorf("view = %q, want the board to say no open sprints", out)
	}
}

func TestDoneSprintsAreNestedUnderClosed(t *testing.T) {
	m, _ := threeSprints(t)
	m = press(m, "s", "down", "down", "right") // A, C, Closed unfolded, B
	lines := strings.Split(ansi.Strip(m.sprints.view(m.doc, 30, 20)), "\n")
	if len(lines) != 8 {
		t.Fatalf("want 4 rows, got %d lines", len(lines))
	}
	if !strings.HasPrefix(lines[4], "  ▾") && !strings.HasPrefix(lines[4], "❯ ▾") {
		t.Errorf("group row = %q", lines[4])
	}
	if !strings.HasPrefix(lines[6], "    ✓ B") || ansi.StringWidth(lines[6]) > 30 {
		t.Errorf("done row = %q, want it indented under Closed", lines[6])
	}
}
