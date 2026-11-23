package tui

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/candy-tools/todo/internal/todo"
	tea "github.com/charmbracelet/bubbletea"
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
	doc := loadSprintDoc(t, path)
	found := doc.ResolveSprint("Delta")
	if len(found) != 1 || found[0].Goal != "ship it" || doc.SprintStatus(found[0].ID) != todo.SprintOpen {
		t.Fatalf("stored sprints named Delta = %+v, want one open with its goal", found)
	}
	s := found[0]
	if sel := m.sprints.selected(m.doc); sel == nil || sel.ID != s.ID {
		t.Errorf("selected = %v, want the new sprint", sel)
	}
}

// Sprints may share a name: their references tell them apart.
func TestSprintListNewDuplicateNameAdds(t *testing.T) {
	m, path := threeSprints(t)
	m = typeText(press(m, "s", "n"), "a")
	m = press(m, "enter")
	if m.mode != modeMain || m.err != nil || m.status != "" {
		t.Fatalf("a duplicate name: mode %v err %v status %q, want it added", m.mode, m.err, m.status)
	}
	if got := loadSprintDoc(t, path).ResolveSprint("A"); len(got) != 2 {
		t.Errorf("sprints named A = %d, want 2", len(got))
	}
}

// A sprint is shown with its reference before its name: in the sidebar, the
// board's title, the crumb and the sprint picker.
func TestSprintShownWithRef(t *testing.T) {
	m, _ := threeSprints(t)
	m = press(m, "s")
	a, _ := m.doc.SprintByID("s00000a")
	first, _ := sidebarRowLines(m.doc, a, 60, false)
	if !strings.Contains(first, "SPRINT-001 A") {
		t.Errorf("sidebar row = %q, want SPRINT-001 A", first)
	}
	if got := m.boardTitle(); got != "SPRINT-001 A" {
		t.Errorf("board title = %q, want SPRINT-001 A", got)
	}
	if got := m.crumb(); !strings.Contains(got, "SPRINT-001 A") {
		t.Errorf("crumb = %q, want SPRINT-001 A", got)
	}
	if got := sprintRow(m.doc, a).label; got != "SPRINT-001 A" {
		t.Errorf("picker row = %q, want SPRINT-001 A", got)
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

func TestSprintListDeleteConfirm(t *testing.T) {
	m, path := newJSONModel(t, sprintJSON(t, nil,
		[]seedSprint{{ID: "s00000a", Name: "A"}, {ID: "s00000k", Name: "K"}},
		seedTask{ID: "t000001", Title: "d1", Status: "done", Sprint: "s00000a"},
		seedTask{ID: "t000002", Title: "d2", Status: "done", Sprint: "s00000a"},
		seedTask{ID: "t000003", Title: "k", Status: "new", Sprint: "s00000k"},
		seedTask{ID: "t000004", Title: "b", Status: "open"}))
	m = press(m, "s", "down", "d") // rows: K (Open), A (Done)
	if m.mode != modeConfirm {
		t.Fatalf("d: mode %v, want the confirm dialog", m.mode)
	}
	if q, want := m.confirmQuestion(), `Delete sprint "SPRINT-001 A" and its 2 task(s)?`; q != want {
		t.Errorf("question = %q, want %q", q, want)
	}
	m = press(m, "y")
	doc := loadSprintDoc(t, path)
	if _, ok := doc.SprintByID("s00000a"); ok || len(doc.Sprints) != 1 {
		t.Errorf("sprints = %v, want only K", doc.Sprints)
	}
	for _, title := range []string{"d1", "d2"} {
		if it := find(doc, title); it != nil {
			t.Errorf("task %s is still there: %+v", title, it)
		}
	}
	for _, title := range []string{"k", "b"} {
		if find(doc, title) == nil {
			t.Errorf("task %s was deleted", title)
		}
	}
	if m.err != nil {
		t.Errorf("err = %v", m.err)
	}
}

// A sprint without tasks is Open, yet holds nothing to lose: d deletes it.
func TestSprintListDeleteEmptySprintConfirm(t *testing.T) {
	m, _ := newJSONModel(t, sprintJSON(t, nil, []seedSprint{{ID: "s00000a", Name: "A"}}))
	m = press(m, "s", "d")
	if m.mode != modeConfirm {
		t.Fatalf("d on an empty sprint: mode %v status %q, want the confirm dialog", m.mode, m.status)
	}
	if q, want := m.confirmQuestion(), `Delete sprint "SPRINT-001 A"?`; q != want {
		t.Errorf("question = %q, want %q", q, want)
	}
}

func TestSprintListDeleteFallsBackByIndex(t *testing.T) {
	m, path := newJSONModel(t, sprintJSON(t, nil,
		[]seedSprint{
			{ID: "s00000a", Name: "A"},
			{ID: "s00000b", Name: "B"},
			{ID: "s00000d", Name: "D"},
		},
		seedTask{ID: "t000001", Title: "bDone", Status: "done", Sprint: "s00000b"},
		seedTask{ID: "t000002", Title: "dDone", Status: "done", Sprint: "s00000d"},
	)) // rows: A (Open), D, B (Done)
	m = press(m, "s", "down", "d", "y")
	if _, ok := loadSprintDoc(t, path).SprintByID("s00000d"); ok {
		t.Fatalf("D was not deleted")
	}
	if s := m.sprints.selected(m.doc); s == nil || s.ID != "s00000b" || m.view.sprint != "s00000b" {
		t.Errorf("after deleting D: selected %v board %q, want B, now at D's index, and its board", s, m.view.sprint)
	}
	if !m.listFocus {
		t.Error("the sidebar should keep the focus")
	}
}

func TestSprintListDeleteCancelKeepsSprint(t *testing.T) {
	m, path := threeSprints(t) // rows: A (Open), C (In Progress), B (Done)
	m = press(m, "s", "down", "down", "d")
	if m.mode != modeConfirm {
		t.Fatalf("d on the done B: mode %v, want the confirm dialog", m.mode)
	}
	m = press(m, "esc")
	if len(loadSprintDoc(t, path).Sprints) != 3 || m.mode != modeMain {
		t.Errorf("esc must cancel the delete")
	}
}

// d on a sprint with a task not done is refused at once, as a status, without
// a dialog.
func TestSprintListDeleteRefusedWhileNotDone(t *testing.T) {
	m, path := threeSprints(t) // A holds aNext, still new
	m = press(m, "s", "d")
	if m.mode != modeMain {
		t.Errorf("mode = %v, want no dialog", m.mode)
	}
	if m.err != nil {
		t.Errorf("err = %v, want none: a refused delete is a status", m.err)
	}
	if want := "can't delete: 1 task not done"; m.status != want {
		t.Errorf("status = %q, want %q", m.status, want)
	}
	doc := loadSprintDoc(t, path)
	if _, ok := doc.SprintByID("s00000a"); !ok || find(doc, "aNext") == nil {
		t.Error("a refused delete must leave A and its tasks")
	}
}

// A sprint whose task another session reopened after the dialog opened is
// refused at write time, as a status.
func TestSprintListDeleteRefusedWhenReopenedElsewhere(t *testing.T) {
	m, path := threeSprints(t) // rows: A (Open), C (In Progress), B (Done)
	m = press(m, "s", "down", "down", "d")
	if _, err := openSecond(t, path).Reopen(context.Background(), "t000004", false); err != nil {
		t.Fatal(err)
	}
	m = press(m, "y")
	if m.err != nil {
		t.Errorf("err = %v, want none: a refused delete is a status", m.err)
	}
	if want := "can't delete: 1 task not done"; m.status != want {
		t.Errorf("status = %q, want %q", m.status, want)
	}
	if strings.Contains(m.footer(200), "save failed") {
		t.Errorf("footer = %q, want the refusal, not a failed save", m.footer(200))
	}
	if _, ok := loadSprintDoc(t, path).SprintByID("s00000b"); !ok {
		t.Error("B must still exist")
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
	for _, want := range []string{"New", "Edit", "Delete", "Board"} {
		if !strings.Contains(f, want) {
			t.Errorf("footer = %q, misses %q", f, want)
		}
	}
	if strings.Contains(f, "Close") {
		t.Errorf("footer = %q, a sprint is no longer closed by hand", f)
	}
}

// manySprints is a list of n open sprints named S00, S01, ….
func manySprints(t *testing.T, n int) model {
	t.Helper()
	var ss []seedSprint
	for i := 0; i < n; i++ {
		ss = append(ss, seedSprint{ID: fmt.Sprintf("s0000%02d", i), Name: fmt.Sprintf("S%02d", i)})
	}
	m, _ := newJSONModel(t, sprintJSON(t, nil, ss))
	return press(m, "s")
}

// sidebarLines renders m's sidebar, width x height, as plain lines.
func sidebarLines(m model, width, height int) []string {
	return strings.Split(ansi.Strip(m.sprints.view(m.doc, width, height)), "\n")
}

func TestSprintListScrollsOnlyWhenCursorLeavesWindow(t *testing.T) {
	m := manySprints(t, 60)
	h := m.sprintListHeight()
	for i := 0; i < h/rowLines+3; i++ {
		m = press(m, "down")
	}
	top := sidebarLines(m, 100, h)[0]
	if strings.HasPrefix(top, "━━") {
		t.Fatalf("first line %q after %d downs, want the window scrolled off the Open rule", top, h/rowLines+3)
	}
	for i := 0; i < 2; i++ {
		m = press(m, "up")
		if got := sidebarLines(m, 100, h)[0]; got != top {
			t.Fatalf("up %d: first line %q, want it to stay %q", i+1, got, top)
		}
	}
	// Going above the window moves it by one row, the cursor's on top.
	for strings.Contains(sidebarLines(m, 100, h)[0], top) {
		m = press(m, "up")
	}
	if got := sidebarLines(m, 100, h)[0]; !strings.HasPrefix(got, "❯ ") {
		t.Errorf("first line %q, want the window to follow the cursor up", got)
	}
}

// sectionedSprints is Open A (a New task, a goal) and C (no task, no goal)
// beside Done B (older, a goal) and D (newer, no goal), each with a done task.
func sectionedSprints(t *testing.T) (model, string) {
	t.Helper()
	return newJSONModel(t, sprintJSON(t, nil,
		[]seedSprint{
			{ID: "s00000a", Name: "A", Goal: "goal a"},
			{ID: "s00000b", Name: "B", Goal: "goal b"},
			{ID: "s00000c", Name: "C"},
			{ID: "s00000d", Name: "D"},
		},
		seedTask{ID: "t000001", Title: "aNext", Status: "new", Sprint: "s00000a"},
		seedTask{ID: "t000002", Title: "bDone", Status: "done", Sprint: "s00000b"},
		seedTask{ID: "t000003", Title: "dDone", Status: "done", Sprint: "s00000d"}))
}

// sidebarRule and sidebarRow build the plain lines of a 30-cell sidebar: a
// section's rule, and the first line of a sprint's row with its counts at the
// right edge.
func sidebarRule(label string) string {
	return label + strings.Repeat("━", 30-ansi.StringWidth(label))
}

func sidebarRow(lead, counts string) string {
	return lead + strings.Repeat(" ", 30-ansi.StringWidth(lead)-ansi.StringWidth(counts)) + counts
}

// The sidebar lists the sprints under four rules, Open, Ready, In Progress
// and Done, each in creation order but Done newest first; an empty section
// shows as its rule alone.
func TestSidebarSectionsListByState(t *testing.T) {
	m, _ := sectionedSprints(t)
	m = press(m, "s")
	want := []string{
		sidebarRule("━━ Open 2 "),
		sidebarRow("❯ ◌ SPRINT-001 A", "1·0·0"), "    goal a",
		sidebarRow("  ◌ SPRINT-003 C", "0·0·0"), "    —",
		sidebarRule("━━ Ready 0 "),
		sidebarRule("━━ In Progress 0 "),
		sidebarRule("━━ Done 2 "),
		sidebarRow("  ✓ SPRINT-004 D", "0·0·1"), "    —",
		sidebarRow("  ✓ SPRINT-002 B", "0·0·1"), "    goal b",
	}
	if got := sidebarLines(m, 30, 20); !equalStrings(got, want) {
		t.Errorf("sidebar =\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

// A sprint's section is the lowest state all its tasks have reached: one New
// task keeps it Open however far the others are, and it moves to Ready only
// once every task is Ready or further along.
func TestSidebarSprintMovesWhenAllTasksReachState(t *testing.T) {
	m, path := newJSONModel(t, sprintJSON(t, nil,
		[]seedSprint{{ID: "s00000a", Name: "A"}},
		seedTask{ID: "t000001", Title: "r1", Status: "ready", Sprint: "s00000a"},
		seedTask{ID: "t000002", Title: "w", Status: "working", Sprint: "s00000a"},
		seedTask{ID: "t000003", Title: "d", Status: "done", Sprint: "s00000a"},
		seedTask{ID: "t000004", Title: "n", Status: "new", Sprint: "s00000a"}))
	m = press(m, "s")
	if got := sidebarLines(m, 30, 20); got[0] != sidebarRule("━━ Open 1 ") || !strings.Contains(got[1], "◌ SPRINT-001 A") {
		t.Fatalf("with a New task the sidebar is\n%s\nwant A under Open", strings.Join(got, "\n"))
	}
	if _, err := openSecond(t, path).SetState(context.Background(), "t000004", todo.Ready, todo.StateOptions{}); err != nil {
		t.Fatal(err)
	}
	m = pollOnce(t, m)
	want := []string{
		sidebarRule("━━ Open 0 "),
		sidebarRule("━━ Ready 1 "),
		sidebarRow("❯ ◉ SPRINT-001 A", "2·1·1"), "    —",
		sidebarRule("━━ In Progress 0 "),
		sidebarRule("━━ Done 0 "),
	}
	if got := sidebarLines(m, 30, 20); !equalStrings(got, want) {
		t.Errorf("with every task Ready or further the sidebar is\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

// An empty section shows as its rule alone, as on a board.
func TestSidebarEmptySectionShowsItsRuleAlone(t *testing.T) {
	m := manySprints(t, 1)
	want := []string{"━━ Open 1 ", "", "", "━━ Ready 0 ", "━━ In Progress 0 ", "━━ Done 0 "}
	got := sidebarLines(m, 30, 20)
	if len(got) != len(want) {
		t.Fatalf("one open sprint renders %q, want its rule, its row, then three rules alone", got)
	}
	for i, w := range want {
		if w != "" && !strings.HasPrefix(got[i], w) {
			t.Errorf("line %d = %q, want %q", i, got[i], w)
		}
	}
	only, _ := newJSONModel(t, sprintJSON(t, nil, []seedSprint{{ID: "s00000b", Name: "B"}},
		seedTask{ID: "t000001", Title: "x", Status: "done", Sprint: "s00000b"}))
	only = press(only, "s")
	if got := sidebarLines(only, 30, 20); len(got) != 6 || !strings.HasPrefix(got[0], "━━ Open 0 ━") || !strings.HasPrefix(got[3], "━━ Done 1 ━") {
		t.Errorf("one done sprint renders %q, want three rules alone, then the Done rule and its row", got)
	}
}

// The arrows walk the sprints across both sections, never onto a rule, and
// show each one's board at once.
func TestSidebarArrowsSkipRules(t *testing.T) {
	m, _ := sectionedSprints(t)
	m = press(m, "s")
	for _, step := range []struct{ key, want string }{
		{"down", "s00000c"}, {"down", "s00000d"}, {"down", "s00000b"}, {"down", "s00000b"},
		{"up", "s00000d"}, {"up", "s00000c"}, {"up", "s00000a"}, {"up", "s00000a"},
	} {
		m = press(m, step.key)
		if s := m.sprints.selected(m.doc); s == nil || s.ID != step.want || m.view.sprint != step.want {
			t.Fatalf("%s: selected %v board %q, want %s on both", step.key, s, m.view.sprint, step.want)
		}
	}
}

// A done sprint's row shows its board, which is not frozen: A on its task
// moves it back to In Progress, and the sprint follows to that section with
// the cursor kept on it.
func TestSidebarDoneSprintShowsItsBoard(t *testing.T) {
	m, path := sectionedSprints(t)
	m = press(m, "s", "down", "down") // A, C, then D
	for _, want := range []string{"D ✓ done", "✓ Done", "dDone"} {
		if !inBoardPane(m, want) {
			t.Errorf("D's board misses %q:\n%s", want, ansi.Strip(m.View()))
		}
	}
	m = press(m, "tab")
	if f := ansi.Strip(m.footer(200)); !strings.Contains(f, "a/A:") {
		t.Errorf("D's board footer = %q, want a/A", f)
	}
	m = press(m, "A")
	if m.err != nil {
		t.Fatalf("A on dDone: err %v", m.err)
	}
	doc := loadSprintDoc(t, path)
	if st := doc.SprintStatus("s00000d"); st != todo.SprintInProgress {
		t.Fatalf("D is %v, want in progress", st)
	}
	if s := m.sprints.selected(m.doc); s == nil || s.ID != "s00000d" || m.view.sprint != "s00000d" {
		t.Errorf("after reopening dDone: selected %v board %q, want the cursor kept on D", s, m.view.sprint)
	}
	if got := sidebarLines(m, 30, 20); got[6] != sidebarRule("━━ In Progress 1 ") || !strings.Contains(got[7], "◐ SPRINT-004 D") {
		t.Errorf("after reopening dDone the sidebar is\n%s\nwant D under In Progress", strings.Join(got, "\n"))
	}
}

// [ ] step between the sections that hold sprints, wrapping around, and 1 to 4
// jump to Open, Ready, In Progress and Done, onto a section's first sprint,
// showing its board.
func TestSidebarSectionKeys(t *testing.T) {
	m, _ := sectionedSprints(t)
	m = press(m, "s", "down") // on C
	for _, step := range []struct{ key, want string }{
		{"]", "s00000d"}, {"]", "s00000a"}, {"[", "s00000d"}, {"[", "s00000a"},
		{"4", "s00000d"}, {"4", "s00000d"}, {"1", "s00000a"},
	} {
		m = press(m, step.key)
		if s := m.sprints.selected(m.doc); s == nil || s.ID != step.want || m.view.sprint != step.want || m.status != "" {
			t.Fatalf("%s: selected %v board %q footer %q, want %s on both", step.key, s, m.view.sprint, m.status, step.want)
		}
	}
	if !m.listFocus {
		t.Error("the section keys keep the keys on the sidebar")
	}
}

// Each digit jumps to its own section: 2 to Ready and 3 to In Progress too.
func TestSidebarDigitsJumpToEachSection(t *testing.T) {
	m, _ := newJSONModel(t, sprintJSON(t, nil,
		[]seedSprint{{ID: "s00000a", Name: "A"}, {ID: "s00000b", Name: "B"}, {ID: "s00000c", Name: "C"}, {ID: "s00000d", Name: "D"}},
		seedTask{ID: "t000001", Title: "a", Status: "new", Sprint: "s00000a"},
		seedTask{ID: "t000002", Title: "b", Status: "ready", Sprint: "s00000b"},
		seedTask{ID: "t000003", Title: "c", Status: "review", Sprint: "s00000c"},
		seedTask{ID: "t000004", Title: "d", Status: "done", Sprint: "s00000d"}))
	m = press(m, "s")
	for _, step := range []struct{ key, want string }{
		{"3", "s00000c"}, {"2", "s00000b"}, {"4", "s00000d"}, {"1", "s00000a"},
	} {
		m = press(m, step.key)
		if s := m.sprints.selected(m.doc); s == nil || s.ID != step.want || m.view.sprint != step.want || m.status != "" {
			t.Fatalf("%s: selected %v board %q footer %q, want %s", step.key, s, m.view.sprint, m.status, step.want)
		}
	}
	if m = press(m, "5"); m.status != "" || m.view.sprint != "s00000a" {
		t.Errorf("5: footer %q board %q, want nothing to happen", m.status, m.view.sprint)
	}
}

// With every sprint open, 4 says Done is empty and leaves the cursor; [ and ]
// find no other section and go to the first open sprint, as on a board.
func TestSidebarSectionKeysOnEmptySection(t *testing.T) {
	m := manySprints(t, 3)
	m = press(m, "down", "4")
	if s := m.sprints.selected(m.doc); s == nil || s.ID != "s000001" || m.status != "nothing in Done" {
		t.Errorf("4: selected %v footer %q, want S01 kept and %q", s, m.status, "nothing in Done")
	}
	if got := press(m, "2"); got.status != "nothing in Ready" {
		t.Errorf("2: footer %q, want %q", got.status, "nothing in Ready")
	}
	for _, key := range []string{"]", "["} {
		got := press(m, "down", key)
		if s := got.sprints.selected(got.doc); s == nil || s.ID != "s000000" || got.view.sprint != "s000000" || got.status != "" {
			t.Errorf("%s: selected %v board %q footer %q, want S00, the first open sprint", key, s, got.view.sprint, got.status)
		}
	}
	empty, _ := newJSONModel(t, sprintJSON(t, nil, nil))
	if empty = press(empty, "s", "1"); empty.status != "nothing in Open" {
		t.Errorf("1 on an empty list: footer %q, want %q", empty.status, "nothing in Open")
	}
}

// The window counts the one-line rules and the two-line rows: the cursor's
// row is always whole on screen, the pane never overfilled, and the rule over
// a section's first sprint comes into view with it.
func TestSidebarScrollWithRules(t *testing.T) {
	var ss []seedSprint
	var tasks []seedTask
	for i := range 12 {
		id := fmt.Sprintf("s0000%02d", i)
		ss = append(ss, seedSprint{ID: id, Name: fmt.Sprintf("S%02d", i)})
		if i%2 == 1 { // done; the even ones, without tasks, are open
			tasks = append(tasks, seedTask{ID: fmt.Sprintf("t0000%02d", i), Title: fmt.Sprintf("d%02d", i), Status: "done", Sprint: id})
		}
	}
	m, _ := newJSONModel(t, sprintJSON(t, nil, ss, tasks...))
	m = send(m, tea.WindowSizeMsg{Width: 100, Height: 12}) // 7 pane rows
	m = press(m, "s")
	h := m.sprintListHeight()
	check := func(when string) {
		t.Helper()
		lines := sidebarLines(m, 30, h)
		at := slices.IndexFunc(lines, func(l string) bool { return strings.HasPrefix(l, "❯ ") })
		if len(lines) > h || at < 0 || at+1 >= len(lines) {
			t.Fatalf("%s: %d lines (pane %d), cursor row at %d:\n%s", when, len(lines), h, at, strings.Join(lines, "\n"))
		}
		if s := m.sprints.selected(m.doc); !strings.Contains(lines[at], s.Name) {
			t.Fatalf("%s: cursor line %q, want %s", when, lines[at], s.Name)
		}
	}
	for i := 0; i < 12; i++ {
		check(fmt.Sprintf("after %d downs", i))
		m = press(m, "down")
	}
	for i := 0; i < 12; i++ {
		check(fmt.Sprintf("after %d ups", i))
		m = press(m, "up")
	}
	m = press(m, "1", "down", "down", "down", "down", "down", "down") // S00 … S10, then S11
	check("down onto Done")
	if lines := sidebarLines(m, 30, h); !strings.HasPrefix(lines[len(lines)-3], "━━ Done 6 ") {
		t.Errorf("on Done's first sprint the sidebar is\n%s\nwant the Done rule right above it", strings.Join(lines, "\n"))
	}
	m = press(m, "4", "1")
	if lines := sidebarLines(m, 30, h); !strings.HasPrefix(lines[0], "━━ Open 6 ") || !strings.HasPrefix(lines[1], "❯ ◌ SPRINT-001 S00") {
		t.Errorf("1 shows\n%s\nwant the Open rule on top over S00", strings.Join(lines, "\n"))
	}
	m = press(m, "4")
	lines := sidebarLines(m, 30, h)
	if at := slices.IndexFunc(lines, func(l string) bool { return strings.HasPrefix(l, "❯ ✓ SPRINT-012 S11") }); at < 1 || !strings.HasPrefix(lines[at-1], "━━ Done 6 ") {
		t.Errorf("4 shows\n%s\nwant the Done rule right above S11", strings.Join(lines, "\n"))
	}
}

func TestSprintListCursorFollowsSprintOnReload(t *testing.T) {
	m, path := newJSONModel(t, sprintJSON(t, nil, []seedSprint{
		{ID: "s00000a", Name: "A"},
		{ID: "s00000b", Name: "B"},
		{ID: "s00000c", Name: "C"},
	},
		seedTask{ID: "t000001", Title: "a", Status: "new", Sprint: "s00000a"},
		seedTask{ID: "t000002", Title: "b", Status: "new", Sprint: "s00000b"}))
	m = press(m, "s", "down", "down") // on C
	// Another session finishes A's and B's tasks: they move below C, to Done.
	st := openSecond(t, path)
	for _, id := range []string{"t000001", "t000002"} {
		if _, err := st.SetState(context.Background(), id, todo.Done, todo.StateOptions{}); err != nil {
			t.Fatal(err)
		}
	}
	m = pollOnce(t, m)
	if sel := m.sprints.selected(m.doc); sel == nil || sel.ID != "s00000c" {
		t.Errorf("selected = %v, want C (by ID, not index 2)", sel)
	}
}

func TestSidebarRowsAreTwoLinesAndCut(t *testing.T) {
	long := strings.Repeat("n", 80)
	m, _ := newJSONModel(t, sprintJSON(t, nil,
		[]seedSprint{{ID: "s00000a", Name: long, Goal: strings.Repeat("g", 80)}},
		seedTask{ID: "t000001", Title: "x", Status: "new", Sprint: "s00000a"}))
	lines := sidebarLines(m, 30, 10)
	if len(lines) != 6 {
		t.Fatalf("one sprint renders %d lines, want its two under its rule, then the three other rules: %q", len(lines), lines)
	}
	for _, l := range lines {
		if ansi.StringWidth(l) > 30 {
			t.Errorf("line %q is wider than 30", l)
		}
	}
	if !strings.HasSuffix(lines[1], "1·0·0") || ansi.StringWidth(lines[1]) != 30 {
		t.Errorf("first line %q, want the counts ending at the right edge (width 30)", lines[1])
	}
	if !strings.Contains(lines[1], "…") || !strings.Contains(lines[1], "1·0·0") || !strings.Contains(lines[2], "…") {
		t.Errorf("lines = %q, want the name and goal cut and the counts kept", lines)
	}
}

func TestSidebarRowWithoutGoalShowsDash(t *testing.T) {
	m, _ := newJSONModel(t, sprintJSON(t, nil,
		[]seedSprint{{ID: "s00000a", Name: "A"}},
		seedTask{ID: "t000001", Title: "x", Status: "new", Sprint: "s00000a"}))
	lines := sidebarLines(m, 30, 10)
	if len(lines) != 6 || strings.TrimSpace(lines[2]) != "—" {
		t.Errorf("lines = %q, want a dim — as the goal of a sprint without one", lines)
	}
}

func TestSidebarScrollCountsLines(t *testing.T) {
	m := manySprints(t, 20)
	if got := len(sidebarLines(m, 30, 9)); got != 9 {
		t.Errorf("9 lines hold the rule and 4 two-line rows, got %d lines", got)
	}
	if got := len(sidebarLines(m, 30, 1)); got != 2 {
		t.Errorf("a one-line height still shows the cursor's row (2 lines), got %d", got)
	}
}

func TestKToDoneSprintSelectsItInSidebar(t *testing.T) {
	m, _ := threeSprints(t)
	m = press(m, "K", "down", "down", "enter") // A, C, ✓ B
	if s := m.sprints.selected(m.doc); m.view.sprint != "s00000b" || s == nil || s.ID != "s00000b" {
		t.Fatalf("K to B: board %q selected %v, want B on both", m.view.sprint, s)
	}
}

// Completing a sprint's last task on its board moves the sprint to Done, the
// sidebar's cursor following it.
func TestCompletingLastTaskMovesSprintToDone(t *testing.T) {
	m, _ := newJSONModel(t, sprintJSON(t, nil,
		[]seedSprint{{ID: "s00000a", Name: "A"}, {ID: "s00000b", Name: "B"}},
		seedTask{ID: "t000001", Title: "x", Status: "working", Sprint: "s00000a"}))
	m = press(m, "s", "down", "tab", "a") // B (Open) first, then A; x, In Progress to Done
	if m.err != nil {
		t.Fatalf("a: err %v", m.err)
	}
	if s := m.sprints.selected(m.doc); s == nil || s.ID != "s00000a" || m.view.sprint != "s00000a" {
		t.Errorf("after completing x: selected %v board %q, want A kept and shown", s, m.view.sprint)
	}
	if lines := sidebarLines(m, 30, 20); !strings.HasPrefix(lines[5], "━━ Done 1 ") || !strings.Contains(lines[6], "✓ SPRINT-001 A") {
		t.Errorf("after completing x the sidebar is %q, want A under Done", lines)
	}
}

func TestReloadCursorStaysOnSprintDoneElsewhere(t *testing.T) {
	m, path := newJSONModel(t, sprintJSON(t, nil,
		[]seedSprint{{ID: "s00000a", Name: "A"}, {ID: "s00000b", Name: "B"}, {ID: "s00000d", Name: "D"}},
		seedTask{ID: "t000001", Title: "a", Status: "working", Sprint: "s00000a"},
		seedTask{ID: "t000002", Title: "d", Status: "done", Sprint: "s00000d"}))
	m = press(m, "s", "down") // B (Open, no task) first, then A (In Progress)
	if s := m.sprints.selected(m.doc); s == nil || s.ID != "s00000a" {
		t.Fatalf("precondition: selected %v, want A", s)
	}
	if _, err := openSecond(t, path).SetState(context.Background(), "t000001", todo.Done, todo.StateOptions{}); err != nil {
		t.Fatal(err)
	}
	m = pollOnce(t, m)
	if s := m.sprints.selected(m.doc); s == nil || s.ID != "s00000a" || m.view.sprint != "s00000a" {
		t.Errorf("selected %v board %q, want A kept and shown under Done", s, m.view.sprint)
	}
}

// Deleting the last row, the only done sprint, falls back to the open sprint
// above it, across the section boundary.
func TestDeletingLastDoneSprintGoesToOpenAbove(t *testing.T) {
	m, _ := newJSONModel(t, sprintJSON(t, nil,
		[]seedSprint{{ID: "s00000a", Name: "A"}, {ID: "s00000d", Name: "D"}},
		seedTask{ID: "t000001", Title: "d", Status: "done", Sprint: "s00000d"}))
	m = press(m, "s", "down", "d", "y")
	if s := m.sprints.selected(m.doc); s == nil || s.ID != "s00000a" || m.view.sprint != "s00000a" || !m.listFocus {
		t.Errorf("selected %v board %q listFocus %v, want A, now last, shown with the sidebar focused", s, m.view.sprint, m.listFocus)
	}
}

// A reload that deletes the shown sprint, leaving only done ones, shows the
// newest done sprint, the sidebar's first.
func TestReloadGoneSprintFallsBackToNewestDone(t *testing.T) {
	m, path := newJSONModel(t, sprintJSON(t, nil,
		[]seedSprint{
			{ID: "s00000a", Name: "A"},
			{ID: "s00000b", Name: "B"},
			{ID: "s00000d", Name: "D"},
		},
		seedTask{ID: "t000001", Title: "b", Status: "done", Sprint: "s00000b"},
		seedTask{ID: "t000002", Title: "d", Status: "done", Sprint: "s00000d"}))
	m = press(m, "s") // on A
	deleteSprintOnDisk(t, path, "s00000a")
	m = pollOnce(t, m)
	if s := m.sprints.selected(m.doc); s == nil || s.ID != "s00000d" || m.view.sprint != "s00000d" || !m.listFocus {
		t.Errorf("selected %v board %q listFocus %v, want D on both with the sidebar focused", s, m.view.sprint, m.listFocus)
	}
}

// An empty list has no sprint to edit, delete or show, so its footer
// offers only n.
func TestSidebarFooterDropsDeadHints(t *testing.T) {
	empty, _ := newTestModel(t, "# Work\n- [ ] a\n")
	empty = press(empty, "s")
	f := ansi.Strip(empty.footer(120))
	for _, gone := range []string{"e:", "c:", "d:", "tab:"} {
		if strings.Contains(f, gone) {
			t.Errorf("empty list footer = %q, must not offer %q", f, gone)
		}
	}
	for _, want := range []string{"n: New", "?: Help", "q: Quit"} {
		if !strings.Contains(f, want) {
			t.Errorf("empty list footer = %q, misses %q", f, want)
		}
	}
}

// At 60 cells the sidebar's footer drops whole hints, Help first, never
// cutting one.
func TestSidebarFooterFitsWholeHints(t *testing.T) {
	check := func(name, f string, w int) {
		t.Helper()
		if ansi.StringWidth(f) > w || !strings.HasPrefix(f, " ?: Help") {
			t.Errorf("%s footer at %d = %q, want it to fit and start with Help", name, w, f)
		}
		for _, part := range strings.Split(strings.TrimSpace(f), "  ") {
			if !strings.Contains(part, ": ") {
				t.Errorf("%s footer at %d = %q holds a cut hint %q", name, w, f, part)
			}
		}
	}
	m, _ := threeSprints(t)
	m = press(m, "s")
	check("sidebar", ansi.Strip(m.footer(60)), 60)
}

// The board footer leads with Help and the section keys; the columns layout
// and its L are gone.
func TestBoardFooterSectionHints(t *testing.T) {
	m, _ := twoSprints(t)
	m = send(m, tea.WindowSizeMsg{Width: 80, Height: 30})
	m = press(m, "s", "tab")
	f := ansi.Strip(m.footer(200))
	for _, want := range []string{"[ ] 1-3: Section", "a/A: Move task"} {
		if !strings.Contains(f, want) {
			t.Errorf("footer at 80 = %q, misses %q", f, want)
		}
	}
	if strings.Contains(f, "Next/prev column") {
		t.Errorf("footer at 80 = %q, the old a/A label is gone", f)
	}
	if !strings.HasPrefix(f, " ?: Help  tab: Sprints") {
		t.Errorf("footer at 80 = %q, want Help then tab Sprints first", f)
	}
	wide := send(m, tea.WindowSizeMsg{Width: 140, Height: 30})
	if f := ansi.Strip(wide.footer(200)); strings.Contains(f, "L:") {
		t.Errorf("footer at 140 = %q, want no L", f)
	}
}

// A done sprint's board is not frozen: its footer keeps a/A.
func TestDoneSprintBoardFooterKeepsMoveHint(t *testing.T) {
	m, _ := threeSprints(t)
	m = send(m, tea.WindowSizeMsg{Width: 80, Height: 30})
	m = press(m, "K", "down", "down", "enter", "tab") // the done sprint B
	if m.view.sprint != "s00000b" {
		t.Fatalf("board = %q, want the done sprint B", m.view.sprint)
	}
	f := ansi.Strip(m.footer(200))
	if !strings.Contains(f, "a/A:") || !strings.HasPrefix(f, " ?: Help") {
		t.Errorf("done board footer = %q, want the a/A hint and Help first", f)
	}
}
