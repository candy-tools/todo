package tui

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/candy-tools/todo/internal/todo"
	"github.com/candy-tools/todo/internal/todo/todotest"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

// storedSprint is the sprint ID of the task with id in the list file at path,
// read with a fresh load ("" for the Backlog).
func storedSprint(t *testing.T, path, id string) string {
	t.Helper()
	it, ok := loadSprintDoc(t, path).ByID(id)
	if !ok {
		t.Fatalf("task %s not stored", id)
	}
	return it.Sprint
}

// finishAndCloseOnDisk completes every task of sprint id and closes it,
// through a second store on path, standing in for another session.
func finishAndCloseOnDisk(t *testing.T, path, id string) {
	t.Helper()
	ctx := context.Background()
	st := openSecond(t, path)
	doc, _, err := st.Load(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, it := range doc.SprintTasks(id) {
		if _, err := st.SetState(ctx, it.ID, todo.Done, todo.StateOptions{Cascade: true}); err != nil {
			t.Fatalf("complete %s: %v", it.ID, err)
		}
	}
	if _, err := st.CloseSprint(ctx, id); err != nil {
		t.Fatalf("close %s: %v", id, err)
	}
}

// chooseSprint chooses the row of the open sprint picker that names sprint id.
func chooseSprint(t *testing.T, m model, id string) model {
	t.Helper()
	if m.mode != modeSprintPick {
		t.Fatalf("no sprint picker open, mode %v", m.mode)
	}
	i := slices.IndexFunc(m.sprintPick.rows, func(r sprintPickRow) bool { return r.sprintID == id })
	if i < 0 {
		t.Fatalf("picker %q has no row for %s", m.sprintPick.title, id)
	}
	m.sprintPick.cursor = i
	return press(m, "enter")
}

func TestPOpensPlanningOnShownSprint(t *testing.T) {
	m, _ := threeSprints(t)
	m = press(m, "s", "down", "p") // C
	if m.view.screen != screenPlan || m.plan.sprintID != "s00000c" || m.plan.right {
		t.Fatalf("p: screen %v sprint %q right %v", m.view.screen, m.plan.sprintID, m.plan.right)
	}
	if h := ansi.Strip(m.header(120)); !strings.Contains(h, "[p Planning]") || !strings.Contains(h, "C · 0 in Next · 1 in the Backlog") ||
		strings.Contains(h, "[s Sprints]") {
		t.Errorf("header %q", h)
	}
}

func TestPFromBacklogPlansLastOrFirstOpenSprint(t *testing.T) {
	m, _ := threeSprints(t)
	m = press(m, "p")
	if m.view.screen != screenPlan || m.plan.sprintID != "s00000a" {
		t.Fatalf("p from the Backlog: screen %v sprint %q, want A", m.view.screen, m.plan.sprintID)
	}
	m = press(m, "esc", "k", "b", "p") // the last viewed sprint is C
	if m.plan.sprintID != "s00000c" {
		t.Fatalf("p after viewing C: sprint %q, want C", m.plan.sprintID)
	}
	m = press(m, "b", "u", "p") // from the unified view too
	if m.view.screen != screenPlan || m.view.all {
		t.Fatalf("p from the unified view: screen %v all %v", m.view.screen, m.view.all)
	}
}

func TestPlanningPullsAndSendsBack(t *testing.T) {
	m, path := threeSprints(t)
	m = press(m, "s", "p", "right") // A; pull backlogTask
	if got := storedSprint(t, path, "t000001"); got != "s00000a" {
		t.Fatalf("→ stored sprint %q, want A", got)
	}
	if got := storedStatus(t, path, "t000001"); got != todo.New {
		t.Fatalf("→ stored %v, want new", got)
	}
	if m.plan.rightSel != "t000001" {
		t.Errorf("after a pull the sprint side selects the task, got %q", m.plan.rightSel)
	}
	m = press(m, "tab")
	m.plan.rightSel = "t000001"
	m = press(m, "r")
	if got := storedStatus(t, path, "t000001"); got != todo.Ready {
		t.Fatalf("r stored %v, want ready", got)
	}
	m = press(m, "r")
	if got := storedStatus(t, path, "t000001"); got != todo.New {
		t.Fatalf("r again stored %v, want new", got)
	}
	m = press(m, "left")
	if got := storedSprint(t, path, "t000001"); got != "" {
		t.Fatalf("← left it in %q, want the Backlog", got)
	}
	if m.plan.leftSel != "t000001" {
		t.Errorf("after a return the Backlog side selects the task, got %q", m.plan.leftSel)
	}
	m.plan.rightSel = "t000003" // aDone
	m = press(m, "A")
	if !strings.Contains(m.footer(200), "only tasks in Next go back to the Backlog") {
		t.Errorf("A on a done task: footer %q", m.footer(200))
	}
	m.plan.rightSel = "t000003"
	m = press(m, "r")
	if !strings.Contains(m.footer(200), "only new and ready tasks toggle with r") {
		t.Errorf("r on a done task: footer %q", m.footer(200))
	}
	if got := storedStatus(t, path, "t000003"); got != todo.Done {
		t.Errorf("r on a done task stored %v", got)
	}
}

func TestPlanningKeysActOnTheirSideOnly(t *testing.T) {
	m, path := threeSprints(t)
	m = press(m, "s", "p", "left", "A", "r") // the Backlog side: no return, no toggle
	if got := storedSprint(t, path, "t000002"); got != "s00000a" {
		t.Fatalf("← on the Backlog side moved aNext to %q", got)
	}
	m = press(m, "tab", "right", "a") // the sprint side: no pull
	if got := storedSprint(t, path, "t000001"); got != "" {
		t.Fatalf("→ on the sprint side pulled backlogTask into %q", got)
	}
	if !m.plan.right {
		t.Fatal("tab should focus the sprint side")
	}
	if m = press(m, "shift+tab"); m.plan.right {
		t.Fatal("shift+tab should go back to the Backlog side")
	}
}

// fourBacklog is a list with one open sprint A and the backlog tasks b1..b4
// in category C.
func fourBacklog(t *testing.T) (model, string) {
	t.Helper()
	return newJSONModel(t, sprintJSON(t, nil,
		[]seedSprint{{ID: "s00000a", Name: "A", Status: "open"}},
		seedTask{ID: "t000001", Title: "b1", Status: "open"},
		seedTask{ID: "t000002", Title: "b2", Status: "open"},
		seedTask{ID: "t000003", Title: "b3", Status: "open"},
		seedTask{ID: "t000004", Title: "b4", Status: "open"},
	))
}

func TestPlanningPullKeepsTheBacklogIndex(t *testing.T) {
	m, _ := fourBacklog(t)
	m = press(m, "p", "down", "a") // pull b2
	if m.plan.leftSel != "t000003" || m.plan.rightSel != "t000002" {
		t.Fatalf("after pulling b2: left %q right %q, want b3 and b2", m.plan.leftSel, m.plan.rightSel)
	}
	m = press(m, "down", "a") // pull b4, the last: the selection goes to b3
	if m.plan.leftSel != "t000003" || m.plan.rightSel != "t000004" {
		t.Fatalf("after pulling the last: left %q right %q, want b3 and b4", m.plan.leftSel, m.plan.rightSel)
	}
	m = press(m, "tab", "up", "A") // send b2 (Next's first) back: the sprint side keeps index 1, now b4
	if m.plan.leftSel != "t000002" || m.plan.rightSel != "t000004" {
		t.Fatalf("after a return: left %q right %q, want b2 and b4", m.plan.leftSel, m.plan.rightSel)
	}
}

func TestPlanningGroupsBacklogByCategoryPath(t *testing.T) {
	path := todotest.Seed(t, "# Work\n- [ ] w1\n## Backend\n- [ ] b1\n  - [ ] sub\n# Empty\n# Home\n- [ ] h1\n")
	if _, _, err := openSecond(t, path).AddSprint(context.Background(), "S", ""); err != nil {
		t.Fatal(err)
	}
	m := modelOn(t, path)
	var got []string
	for _, r := range m.plan.leftRows(m.doc) {
		if r.task != nil {
			got = append(got, r.task.Title)
		} else {
			got = append(got, "## "+r.header)
		}
	}
	want := []string{"## Work", "w1", "## Work › Backend", "b1", "## Home", "h1"}
	if !equalStrings(got, want) {
		t.Fatalf("left rows %v, want %v", got, want)
	}
	m = press(m, "p")
	if v := ansi.Strip(m.View()); !strings.Contains(v, "b1 open (0/1)") {
		t.Errorf("a task row shows glyph, title, state and subtask progress:\n%s", v)
	}
}

func TestPlanningSprintSideByColumn(t *testing.T) {
	m, _ := threeSprints(t)
	m = press(m, "s", "p")
	var got []string
	for _, r := range m.plan.rightRows(m.doc) {
		if r.task != nil {
			got = append(got, r.task.Title)
		} else {
			got = append(got, r.header)
		}
	}
	want := []string{"Next (1)", "aNext", "In Progress (0)", "Done (1)", "aDone"}
	if !equalStrings(got, want) {
		t.Fatalf("right rows %v, want %v", got, want)
	}
	v := ansi.Strip(m.View())
	if !strings.Contains(v, "A — goal a") || !strings.Contains(v, "Backlog") {
		t.Errorf("pane titles missing:\n%s", v)
	}
	if !strings.Contains(m.footer(200), "Pull in") || !strings.Contains(m.footer(200), "Sprint side") {
		t.Errorf("Backlog-side footer %q", m.footer(200))
	}
	m = press(m, "tab")
	if f := m.footer(200); !strings.Contains(f, "Back") || !strings.Contains(f, "Ready") || !strings.Contains(f, "Backlog side") {
		t.Errorf("sprint-side footer %q", f)
	}
}

func TestPlanningKRotatesOpenSprints(t *testing.T) {
	m, _ := threeSprints(t)
	m = press(m, "s", "p", "k")
	if m.plan.sprintID != "s00000c" {
		t.Fatalf("k: %q, want C", m.plan.sprintID)
	}
	m = press(m, "k")
	if m.plan.sprintID != "s00000a" {
		t.Fatalf("k again: %q, want A (wraps, skips done B)", m.plan.sprintID)
	}
}

func TestPlanningBigKListsEverySprint(t *testing.T) {
	m, _ := threeSprints(t)
	m = press(m, "s", "p", "K")
	if m.mode != modeSprintPick || m.sprintPick.title != "Plan sprint" || len(m.sprintPick.rows) != 3 {
		t.Fatalf("K: mode %v title %q rows %d, want every sprint", m.mode, m.sprintPick.title, len(m.sprintPick.rows))
	}
	if r := m.sprintPick.rows[m.sprintPick.cursor]; r.sprintID != "s00000a" {
		t.Errorf("K opens on the planned sprint, cursor on %q", r.sprintID)
	}
	m = chooseSprint(t, m, "s00000c")
	if m.view.screen != screenPlan || m.plan.sprintID != "s00000c" {
		t.Fatalf("picking open C: screen %v sprint %q", m.view.screen, m.plan.sprintID)
	}
	m = chooseSprint(t, press(m, "K"), "s00000b")
	if m.view.screen != screenSprint || m.view.sprint != "s00000b" {
		t.Fatalf("picking closed B: screen %v sprint %q, want B's board", m.view.screen, m.view.sprint)
	}
}

func TestEscLeavesPlanningToBoard(t *testing.T) {
	m, _ := threeSprints(t)
	m = press(m, "s", "p", "k", "esc")
	if m.view.screen != screenSprint || m.view.sprint != "s00000c" || m.listFocus || m.boardColumn() != todo.ColInProgress {
		t.Fatalf("esc: screen %v sprint %q listFocus %v section %v, want C's board on cWorking (Next is empty)",
			m.view.screen, m.view.sprint, m.listFocus, m.boardColumn())
	}
	m = press(m, "p", "a", "p") // pull backlogTask into C's Next; p leaves too, in the Next section
	if m.view.screen != screenSprint || m.view.sprint != "s00000c" || m.boardColumn() != todo.ColNext {
		t.Fatalf("p: screen %v sprint %q section %v", m.view.screen, m.view.sprint, m.boardColumn())
	}
	if sel := m.tree.selected(); sel == nil || sel.Title != "backlogTask" {
		t.Errorf("p: selected %v, want the pulled-in backlogTask", sel)
	}
}

func TestPlanningScreenKeysWork(t *testing.T) {
	m, _ := threeSprints(t)
	if m = press(m, "p", "b"); m.view.screen != screenBacklog {
		t.Fatalf("b from Planning: screen %v", m.view.screen)
	}
	if m = press(m, "p", "s"); m.view.screen != screenSprint {
		t.Fatalf("s from Planning: screen %v", m.view.screen)
	}
	if m = press(m, "p", "?"); m.mode != modeHelp {
		t.Fatalf("? from Planning: mode %v", m.mode)
	}
}

func TestPlanningNeverStoresTheView(t *testing.T) {
	m, _ := threeSprints(t)
	m = press(m, "s", "p")
	before := storeFP(t, m)
	_, cmd := press(m, "q").Update(keyMsg("enter"))
	if !quits(cmd) || storeFP(t, m) != before {
		t.Fatal("q from Planning must quit without storing the view")
	}
}

func TestPWithoutOpenSprintHints(t *testing.T) {
	m, _ := newTestModel(t, "# Work\n- [ ] a\n")
	m = press(m, "p")
	if m.view.screen == screenPlan || !strings.Contains(m.footer(200), msgNoOpenSprints) {
		t.Fatalf("p without sprints: screen %v footer %q", m.view.screen, m.footer(200))
	}
}

func TestPlannedSprintClosedElsewhereLeaves(t *testing.T) {
	m, path := threeSprints(t)
	m = press(m, "s", "down", "p") // C
	finishAndCloseOnDisk(t, path, "s00000c")
	m = pollOnce(t, m)
	if m.view.screen == screenPlan {
		t.Fatal("Planning should leave a sprint closed by another session")
	}
	if m.view.screen != screenSprint || !strings.Contains(m.footer(200), "the planned sprint is gone or closed") {
		t.Fatalf("after the close: screen %v footer %q", m.view.screen, m.footer(200))
	}
}

func TestPlannedSprintDeletedElsewhereLeaves(t *testing.T) {
	m, path := threeSprints(t)
	m = press(m, "s", "down", "p") // C
	if _, err := openSecond(t, path).DeleteSprint(context.Background(), "s00000c"); err != nil {
		t.Fatal(err)
	}
	before := storeFP(t, m)
	m = pollOnce(t, m)
	if m.view.screen != screenSprint || m.view.sprint == "s00000c" {
		t.Fatalf("after the delete: screen %v sprint %q", m.view.screen, m.view.sprint)
	}
	if m.status != msgPlanGone || m.err != nil {
		t.Errorf("after the delete: status %q err %v, want %q", m.status, m.err, msgPlanGone)
	}
	if storeFP(t, m) != before {
		t.Error("leaving Planning for a gone sprint must not store the view")
	}
}

func TestPlanningEmptyBacklog(t *testing.T) {
	m, _ := newJSONModel(t, sprintJSON(t, nil, []seedSprint{{ID: "s00000a", Name: "A", Status: "open"}}))
	m = press(m, "s", "p", "right", "a")
	if !strings.Contains(ansi.Strip(m.View()), "nothing in the Backlog") {
		t.Errorf("empty backlog view:\n%s", ansi.Strip(m.View()))
	}
}

func TestPlanningEmptySprintShowsItsColumns(t *testing.T) {
	m, _ := newJSONModel(t, sprintJSON(t, nil, []seedSprint{{ID: "s00000a", Name: "A", Status: "open"}},
		seedTask{ID: "t000001", Title: "b1", Status: "open"}))
	m = press(m, "s", "p")
	v := ansi.Strip(m.View())
	for _, h := range []string{"Next (0)", "In Progress (0)", "Done (0)"} {
		if !strings.Contains(v, h) {
			t.Errorf("empty sprint view lacks %q:\n%s", h, v)
		}
	}
	m, _ = newJSONModel(t, sprintJSON(t, nil, []seedSprint{{ID: "s00000a", Name: "A", Status: "open"}}))
	m = press(m, "s", "p")
	v = ansi.Strip(m.View())
	if !strings.Contains(v, "nothing in the Backlog") || !strings.Contains(v, "Next (0)") {
		t.Errorf("empty sprint and Backlog view:\n%s", v)
	}
}

func TestPlanningWritesOnAGoneSprintSayGone(t *testing.T) {
	deleteC := func(t *testing.T, path string) {
		if _, err := openSecond(t, path).DeleteSprint(context.Background(), "s00000c"); err != nil {
			t.Fatal(err)
		}
	}
	for _, tc := range []struct {
		name  string
		setup []string // before the sprint goes: pull the Backlog's task into C
		keys  []string
		gone  func(t *testing.T, path string)
	}{
		{"pull after a delete", nil, []string{"a"}, deleteC},
		{"return after a delete", []string{"a", "tab"}, []string{"A"}, deleteC},
		{"ready after a delete", []string{"a", "tab"}, []string{"r"}, deleteC},
		{"pull after a close", nil, []string{"a"}, func(t *testing.T, path string) { finishAndCloseOnDisk(t, path, "s00000c") }},
		{"ready after a close", []string{"a", "tab"}, []string{"r"}, func(t *testing.T, path string) { finishAndCloseOnDisk(t, path, "s00000c") }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m, path := threeSprints(t)
			m = press(m, "s", "down", "p") // C
			m = press(m, tc.setup...)
			tc.gone(t, path)
			m = press(m, tc.keys...)
			if m.view.screen != screenSprint || m.status != msgPlanGone || m.err != nil {
				t.Fatalf("screen %v status %q err %v, want the Sprints screen and %q", m.view.screen, m.status, m.err, msgPlanGone)
			}
		})
	}
}

func TestPlanningRIgnoresTheBacklogSide(t *testing.T) {
	m, path := threeSprints(t)
	press(m, "s", "p", "r")
	if got := storedStatus(t, path, "t000001"); got != todo.Open {
		t.Fatalf("r on the Backlog side stored %v, want open", got)
	}
}

func TestPlanningSprintChangeResetsTheSprintSelection(t *testing.T) {
	for _, keys := range [][]string{{"k"}, {"K"}} {
		m, _ := threeSprints(t)
		m = press(m, "s", "p", "tab", "down") // A's Done task
		if m.plan.rightSel != "t000003" {
			t.Fatalf("setup: selected %q", m.plan.rightSel)
		}
		if keys[0] == "K" {
			m = press(m, "K")
			m = chooseSprint(t, m, "s00000c")
		} else {
			m = press(m, keys...)
		}
		if m.plan.sprintID != "s00000c" {
			t.Fatalf("%v: sprint %q", keys, m.plan.sprintID)
		}
		if got := m.plan.selected(m.doc, true); got == nil || got.ID != "t000005" {
			t.Fatalf("%v: sprint side selects %v, want C's first task", keys, got)
		}
		if m.plan.rightOff != 0 {
			t.Errorf("%v: window offset %d, want 0", keys, m.plan.rightOff)
		}
	}
}

func TestPlanningReloadKeepsTheCursorOnTheNearestTask(t *testing.T) {
	for _, tc := range []struct {
		name string
		keys []string
		move func(t *testing.T, st *todo.Store)
		want string
	}{
		{"deleted", []string{"p", "down", "down"}, func(t *testing.T, st *todo.Store) { // b3
			if _, err := st.Delete(context.Background(), "t000003"); err != nil {
				t.Fatal(err)
			}
		}, "t000004"},
		{"deleted last", []string{"p", "down", "down", "down"}, func(t *testing.T, st *todo.Store) { // b4
			if _, err := st.Delete(context.Background(), "t000004"); err != nil {
				t.Fatal(err)
			}
		}, "t000003"},
		{"moved to the sprint", []string{"p", "down"}, func(t *testing.T, st *todo.Store) { // b2
			if _, err := st.Assign(context.Background(), "t000002", "s00000a"); err != nil {
				t.Fatal(err)
			}
		}, "t000003"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m, path := fourBacklog(t)
			m = press(m, tc.keys...)
			tc.move(t, openSecond(t, path))
			m = pollOnce(t, m)
			if got := m.plan.selected(m.doc, false); got == nil || got.ID != tc.want {
				t.Fatalf("cursor on %v, want %s", got, tc.want)
			}
		})
	}
}

func TestPlanningWriteOnAChangedListWithGoneSprintSaysGone(t *testing.T) {
	m, path := threeSprints(t)
	m = press(m, "s", "down", "p") // C
	if _, err := openSecond(t, path).DeleteSprint(context.Background(), "s00000c"); err != nil {
		t.Fatal(err)
	}
	m = press(m, "right") // a write on the list another session changed
	if m.view.screen != screenSprint || m.status != msgPlanGone {
		t.Fatalf("screen %v status %q, want the Sprints screen and %q", m.view.screen, m.status, msgPlanGone)
	}
}

func TestQuitFromPlanningStoresTheBrowsedSprint(t *testing.T) {
	m, path := threeSprints(t)
	m = press(m, "s", "down", "p") // C, browsed to but not stored
	_, cmd := press(m, "q").Update(keyMsg("enter"))
	if !quits(cmd) {
		t.Fatal("q must quit")
	}
	if got := loadSprintDoc(t, path).View.Sprint; got != "s00000c" {
		t.Fatalf("stored sprint %q, want C", got)
	}
}

func TestPlanningFitsShortTerminal(t *testing.T) {
	tasks := make([]seedTask, 0, 20)
	for i := range 20 {
		tasks = append(tasks, seedTask{ID: fmt.Sprintf("t%06d", i), Title: fmt.Sprintf("task%02d", i), Status: "open"})
	}
	m, _ := newJSONModel(t, sprintJSON(t, nil, []seedSprint{{ID: "s00000a", Name: "A", Status: "open"}}, tasks...))
	m = press(m, "p")
	for range 15 {
		m = press(m, "down")
	}
	if m.plan.leftSel != "t000015" {
		t.Fatalf("after 15 downs the selection is %q, want task15", m.plan.leftSel)
	}
	for _, h := range []int{30, 8, 5} {
		m = send(m, tea.WindowSizeMsg{Width: 60, Height: h})
		v := ansi.Strip(m.View())
		lines := strings.Split(v, "\n")
		if len(lines) > max(h, 6) {
			t.Errorf("height %d: %d lines", h, len(lines))
		}
		for _, l := range lines {
			if ansi.StringWidth(l) > 60 {
				t.Errorf("height %d: line wider than the terminal: %q", h, l)
			}
		}
		if !strings.Contains(v, "❯ ○ task15") {
			t.Errorf("height %d: the selected task is not on screen:\n%s", h, v)
		}
	}
}

// Entering Planning starts the footer afresh, as b and s do: an earlier
// failure is not left over Planning, while a failure of the view save made on
// the way in shows there.
func TestEnteringPlanningLeavesNoStaleError(t *testing.T) {
	m, path := threeSprints(t)
	m = press(m, "s") // A's board, stored
	m.err = errors.New("save failed: boom")
	if m = press(m, "p"); m.view.screen != screenPlan || m.err != nil {
		t.Errorf("p with the view already stored: screen %v err %v, want Planning and no old error", m.view.screen, m.err)
	}

	m = press(m, "esc", "esc", "down") // A's board, its sidebar, C: the stored view is A's board
	todotest.HoldWriteLock(t, path)
	m = press(m, "p")
	if m.view.screen != screenPlan || m.plan.sprintID != "s00000c" {
		t.Fatalf("p after browsing: screen %v planned %q, want Planning on C", m.view.screen, m.plan.sprintID)
	}
	if got := m.footer(200); !strings.Contains(got, "save failed: list busy") {
		t.Errorf("footer = %q, want the failed save of the browsed view", got)
	}
}
