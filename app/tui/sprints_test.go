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
	if m.mode != modeForm || m.err == nil || !strings.Contains(m.err.Error(), "save failed") {
		t.Errorf("a duplicate name: mode %v err %v, want the form open with a save error", m.mode, m.err)
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
	for _, want := range []string{"New", "Edit", "Close/reopen", "Delete", "Open"} {
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
	h := m.treeViewHeight()
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
		if got := ansi.Strip(strings.Split(m.sprints.view(m.doc, 100, h), "\n")[0]); !strings.Contains(got, firstRow) {
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

func TestSprintListCapsLongNames(t *testing.T) {
	long := strings.Repeat("n", 80)
	m, _ := newJSONModel(t, sprintJSON(t, nil,
		[]seedSprint{{ID: "s00000a", Name: long, Goal: "goal", Status: "open"}},
		seedTask{ID: "t000001", Title: "x", Status: "new", Sprint: "s00000a"}))
	line := ansi.Strip(m.sprints.view(m.doc, 100, 5))
	if strings.Contains(line, long) || !strings.Contains(line, "…") || !strings.Contains(line, "1 · 0 · 0") {
		t.Errorf("row = %q, want the name cut and the counts kept", line)
	}
}
