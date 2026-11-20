package tui

import (
	"strings"
	"testing"

	"github.com/candy-tools/todo/internal/todo"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

// onBoard switches m to sprint's board, in section c, and selects the task
// titled title, failing the test when the board doesn't show it.
func onBoard(t *testing.T, m model, sprint string, c todo.Column, title string) model {
	t.Helper()
	m.switchScreen(screenSprint, sprint)
	m.tree.rebuild() // a copy of m may have switched the shared view already
	toSection(&m, c)
	return selectTitle(t, m, title)
}

func TestNOnBoardAddsToSprintTab(t *testing.T) {
	m, path := twoSprints(t)
	m = onBoard(t, m, "s000001", todo.ColInProgress, "working")
	m = press(m, "n")
	m = typeText(m, "idea")
	m = press(m, "enter")
	if m.err != nil || m.mode != modeMain {
		t.Fatalf("after enter: mode %v err %v", m.mode, m.err)
	}
	it := find(loadSprintDoc(t, path), "idea")
	if it == nil || it.Sprint != "s000001" || it.Status != todo.Working {
		t.Fatalf("stored idea = %+v, want Sprint 1 and working", it)
	}
	if got := m.tree.selected(); got == nil || got.Title != "idea" {
		t.Errorf("the new task should be selected on the board, got %v", got)
	}
}

func TestNOnBacklogAddsOpen(t *testing.T) {
	m, path := twoSprints(t)
	m = selectTitle(t, m, "backlogTask")
	m = typeText(press(m, "n"), "idea")
	m = press(m, "enter")
	if it := find(loadSprintDoc(t, path), "idea"); it == nil || it.Sprint != "" || it.Status != todo.Open {
		t.Fatalf("stored idea = %+v, want the Backlog and open", it)
	}
	// The unified view adds to the Backlog too, even next to a sprint task.
	m = press(m, "u")
	m = selectTitle(t, m, "working")
	m = typeText(press(m, "n"), "other")
	_ = press(m, "enter")
	if it := find(loadSprintDoc(t, path), "other"); it == nil || it.Sprint != "" || it.Status != todo.Open {
		t.Errorf("stored other = %+v, want the Backlog and open", it)
	}
}

func TestAANextBackToBacklog(t *testing.T) {
	m, path := twoSprints(t)
	m = onBoard(t, m, "s000001", todo.ColInProgress, "working")
	m = press(m, "A")
	if it := find(loadSprintDoc(t, path), "working"); it.Sprint != "s000001" || it.Status != todo.New {
		t.Fatalf("A from In Progress stored sprint %q state %v, want Sprint 1, new", it.Sprint, it.Status)
	}
	m = onBoard(t, m, "s000001", todo.ColNext, "working")
	m = press(m, "A")
	if m.err != nil {
		t.Fatalf("A from Next: %v", m.err)
	}
	if it := find(loadSprintDoc(t, path), "working"); it.Sprint != "" || it.Status != todo.Open {
		t.Errorf("A from Next stored sprint %q state %v, want the Backlog, open", it.Sprint, it.Status)
	}
	if m.status != "moved to Backlog" {
		t.Errorf("status = %q, want %q", m.status, "moved to Backlog")
	}

	// On the Backlog A only hints.
	m.switchScreen(screenBacklog, "")
	m = selectTitle(t, m, "working")
	before := storeFP(t, m)
	if got := press(m, "A"); got.status != "already in the Backlog" || storeFP(t, got) != before {
		t.Errorf("A on a backlog task: status %q, want the hint and no write", got.status)
	}
}

func TestAMovesAlongSprintColumns(t *testing.T) {
	m, path := newJSONModel(t, sprintJSON(t, nil,
		[]seedSprint{{ID: "s000001", Name: "Sprint 1", Status: "open"}},
		seedTask{ID: "t000001", Title: "x", Status: "ready", Sprint: "s000001"}))
	m = onBoard(t, m, "s000001", todo.ColNext, "x")
	for _, step := range []struct {
		key    string
		col    todo.Column
		want   todo.Status
		footer string
	}{
		{"a", todo.ColNext, todo.Working, "moved to In Progress — working"},
		{"a", todo.ColInProgress, todo.Done, "moved to Done — done"},
		{"A", todo.ColDone, todo.Working, "moved to In Progress — working"},
		{"A", todo.ColInProgress, todo.New, "moved to Next — new"},
	} {
		m = onBoard(t, m, "s000001", step.col, "x")
		m = press(m, step.key)
		it := find(loadSprintDoc(t, path), "x")
		if m.err != nil || it.Sprint != "s000001" || it.Status != step.want {
			t.Fatalf("%s from %v: sprint %q state %v err %v, want Sprint 1, %v", step.key, step.col, it.Sprint, it.Status, m.err, step.want)
		}
		if m.status != step.footer {
			t.Errorf("%s from %v: status %q, want %q", step.key, step.col, m.status, step.footer)
		}
	}
	// Back in Next after the steps; two a reach Done, the board's edge.
	m = press(onBoard(t, m, "s000001", todo.ColNext, "x"), "a")
	m = press(onBoard(t, m, "s000001", todo.ColInProgress, "x"), "a")
	m = onBoard(t, m, "s000001", todo.ColDone, "x")
	before := storeFP(t, m)
	if got := press(m, "a"); got.status != "no column beyond Done" || storeFP(t, got) != before {
		t.Errorf("a on Done: status %q, want the edge hint and no write", got.status)
	}
}

// pickStates lists the selectable states of the open state dialog, and a
// heading as "#Title".
func pickStates(m model) []string {
	var out []string
	for _, r := range m.pick.rows {
		if r.header != "" {
			out = append(out, "#"+r.header)
		} else {
			out = append(out, r.state.String())
		}
	}
	return out
}

func TestTOffersMembershipStates(t *testing.T) {
	m, _ := twoSprints(t)
	m = selectTitle(t, m, "backlogTask")
	m = press(m, "t")
	if got, want := strings.Join(pickStates(m), ","), "open,deferred,wontdo"; got != want {
		t.Errorf("t on a backlog task offers %q, want %q", got, want)
	}
	m = press(m, "esc")
	m = onBoard(t, m, "s000001", todo.ColInProgress, "working")
	m = press(m, "t")
	want := "#Next,new,ready,#In Progress,working,review,followup,#Done,done"
	if got := strings.Join(pickStates(m), ","); got != want {
		t.Errorf("t on a sprint task offers %q, want %q", got, want)
	}
	// In the unified view each task's dialog follows its own membership.
	m = press(press(m, "esc"), "u")
	m = selectTitle(t, m, "backlogTask")
	if got := strings.Join(pickStates(press(m, "t")), ","); got != "open,deferred,wontdo" {
		t.Errorf("t on a backlog task in the unified view offers %q", got)
	}
}

func TestDScopedToSprint(t *testing.T) {
	m, path := newJSONModel(t, sprintJSON(t, nil,
		[]seedSprint{{ID: "s000001", Name: "Sprint 1", Status: "open"}, {ID: "s000002", Name: "Sprint 2", Status: "open"}},
		seedTask{ID: "t000001", Title: "done1", Status: "done", Sprint: "s000001"},
		seedTask{ID: "t000002", Title: "done2", Status: "done", Sprint: "s000002"},
		seedTask{ID: "t000003", Title: "open1", Status: "new", Sprint: "s000001"}))
	m.switchScreen(screenSprint, "s000001")
	m = press(m, "D")
	if m.mode != modeConfirm {
		t.Fatalf("D on a board with done tasks: mode %v, want the confirm", m.mode)
	}
	if out := ansi.Strip(m.View()); !strings.Contains(out, "Remove 1 completed task(s)?") {
		t.Errorf("the confirm must count the board's done tasks only:\n%s", out)
	}
	m = press(m, "y")
	if m.err != nil || m.status != "Removed 1 completed task(s)." {
		t.Errorf("after y: status %q err %v", m.status, m.err)
	}
	doc := loadSprintDoc(t, path)
	if find(doc, "done1") != nil {
		t.Error("Sprint 1's done task must be removed")
	}
	if find(doc, "done2") == nil || find(doc, "open1") == nil {
		t.Error("Sprint 2's done task and Sprint 1's open task must stay")
	}
	// Nothing left to remove on this board.
	if m = press(m, "D"); m.mode != modeMain || m.status != "No completed tasks to remove." {
		t.Errorf("D again: mode %v status %q", m.mode, m.status)
	}
}

func TestDOutsideBoardHints(t *testing.T) {
	m, _ := twoSprints(t)
	before := storeFP(t, m)
	if got := press(m, "D"); got.mode != modeMain || got.status != "no done tasks in the Backlog" || storeFP(t, got) != before {
		t.Errorf("D on the Backlog: mode %v status %q", got.mode, got.status)
	}
	m.switchScreen(screenSprint, "s000003") // Old, with a done task
	m = press(m, "u")
	before = storeFP(t, m) // a stored the view
	if got := press(m, "D"); got.mode != modeMain || got.status != "open a sprint to remove its done tasks" || storeFP(t, got) != before {
		t.Errorf("D in the unified view: mode %v status %q", got.mode, got.status)
	}
}

func TestWSNoLongerNavigate(t *testing.T) {
	m, _ := twoSprints(t)
	m = press(m, "down")
	cursor := m.tree.cursor
	if got := press(m, "w"); got.tree.cursor != cursor || got.mode != modeMain {
		t.Errorf("w in the main view moved the cursor %d -> %d", cursor, got.tree.cursor)
	}
	if got := press(m, "s"); got.view.screen != screenSprint || !got.listFocus {
		t.Errorf("s in the main view: screen %v, want the Sprints workspace", got.view.screen)
	}

	// The help dialog, scrolled part way.
	h := send(m, tea.WindowSizeMsg{Width: 80, Height: 24})
	h = press(h, "?", "down", "down")
	off := h.helpOffset
	if h = press(h, "w", "s", "s"); h.helpOffset != off || h.mode != modeHelp {
		t.Errorf("w/s in the help dialog scrolled %d -> %d", off, h.helpOffset)
	}

	// The t and f dialogs, with the cursor on a middle row.
	m = onBoard(t, m, "s000001", todo.ColInProgress, "working")
	for _, key := range []string{"t", "f"} {
		d := press(m, key, "down")
		cur := d.pick.cursor
		for _, k := range []string{"w", "s"} {
			if got := press(d, k); got.pick.cursor != cur {
				t.Errorf("%s in the %s dialog moved the cursor %d -> %d", k, key, cur, got.pick.cursor)
			}
		}
	}

	// Grab mode: w/s move nothing.
	g, path := newTestModel(t, "# C\n- [ ] a\n- [ ] b\n- [ ] c\n")
	g = press(selectTitle(t, g, "b"), "m")
	for _, k := range []string{"w", "s"} {
		if got := press(g, k); !got.tree.grabbed || !equalStrings(storedChildren(t, path, "C"), []string{"a", "b", "c"}) {
			t.Errorf("%s while grabbed moved the item: order %v", k, storedChildren(t, path, "C"))
		}
	}
}

func TestHelpListsSprintKeys(t *testing.T) {
	desc := map[string][]string{}
	for _, s := range helpSections {
		for _, b := range s.bindings {
			desc[b.keys] = append(desc[b.keys], b.desc)
		}
	}
	for _, keys := range []string{"b", "s", "p", "k", "K", "q", "u", "a A", "a", "A", "o", "t", "1–3", "[  ]", "tab", "n", "e", "c", "d", "enter  e", "enter  tab", "D", "→ a", "← A", "r", "k K"} {
		if len(desc[keys]) == 0 {
			t.Errorf("help has no row for %q", keys)
		}
	}
	for _, gone := range []string{"↑ ↓  w s", "1–4", "x", "p P", "P"} {
		if len(desc[gone]) > 0 {
			t.Errorf("help still lists %q", gone)
		}
	}
	all := ansi.Strip(helpModal(200, 80, 0))
	for _, want := range []string{"The Sprints workspace", "else focus the sidebar", "Unified view", "Close / reopen the sprint", "assign it to a sprint", "Send the task to another sprint or the Backlog"} {
		if !strings.Contains(all, want) {
			t.Errorf("help should say %q:\n%s", want, all)
		}
	}
	if len(desc["p"]) == 0 {
		t.Error("help has no row for p")
	}
	for _, d := range desc["p"] {
		if !strings.Contains(d, "Planning") {
			t.Errorf("p is the Planning screen: %q", d)
		}
	}
	for _, k := range desc["k"] {
		if strings.Contains(strings.ToLower(k), "all states") || strings.Contains(strings.ToLower(k), "unified") {
			t.Errorf("k is no longer the unified view: %q", k)
		}
	}
}

func TestUnifiedRowsNameTheSprint(t *testing.T) {
	long := strings.Repeat("L", 40)
	m, _ := newJSONModel(t, sprintJSON(t, nil,
		[]seedSprint{{ID: "s000001", Name: "Sprint 1", Status: "open"}, {ID: "s000002", Name: long, Status: "open"}},
		seedTask{ID: "t000001", Title: "backlogTask", Status: "open"},
		seedTask{ID: "t000002", Title: "working", Status: "working", Sprint: "s000001"},
		seedTask{ID: "t000003", Title: "longOne", Status: "new", Sprint: "s000002"}))
	m = press(m, "u")
	if got := ansi.Strip(rowOf(t, m.tree, "working")); !strings.Contains(got, "working · SPRINT-001 Sprint 1") {
		t.Errorf("a sprint task's unified row = %q, want its sprint next to its state", got)
	}
	m = selectTitle(t, m, "working")
	if got := ansi.Strip(m.tree.rowString(m.tree.rows[m.tree.cursor], true)); !strings.Contains(got, "working · SPRINT-001 Sprint 1") {
		t.Errorf("the selected row = %q, want its sprint too", got)
	}
	if got := ansi.Strip(rowOf(t, m.tree, "backlogTask")); strings.Contains(got, "·") {
		t.Errorf("a backlog task's unified row = %q, want no sprint", got)
	}
	got := ansi.Strip(rowOf(t, m.tree, "longOne"))
	label := "SPRINT-002 " + strings.Repeat("L", maxNameWidth-len("SPRINT-002 ")-1) + "…"
	if strings.Contains(got, long) || !strings.Contains(got, "new · "+label) {
		t.Errorf("a long sprint label must be cut to %d columns, row = %q", maxNameWidth, got)
	}
	// Boards and the Backlog don't repeat it.
	m = press(m, "u")
	if got := ansi.Strip(rowOf(t, m.tree, "backlogTask")); strings.Contains(got, "·") {
		t.Errorf("Backlog row = %q", got)
	}
	m = onBoard(t, m, "s000001", todo.ColInProgress, "working")
	if got := ansi.Strip(rowOf(t, m.tree, "working")); strings.Contains(got, "Sprint 1") {
		t.Errorf("board row = %q, want no sprint name", got)
	}
}

func TestUnifiedMovesSayWhereTheTaskWent(t *testing.T) {
	m, _ := twoSprints(t)
	m = selectTitle(t, press(m, "u"), "working")
	m = press(m, "o", "down", "enter") // to Sprint 2
	if m.err != nil || m.status != "moved to SPRINT-002 Sprint 2 — "+todo.Working.Label() {
		t.Errorf("o in the unified view: status %q err %v", m.status, m.err)
	}
	m = selectTitle(t, m, "working")
	m = press(m, "A", "A") // In Progress -> Next -> Backlog
	if m.err != nil || m.status != "moved to Backlog" {
		t.Errorf("A from Next in the unified view: status %q err %v", m.status, m.err)
	}
}
