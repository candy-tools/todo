package tui

import (
	"strings"
	"testing"

	"github.com/candy-tools/todo/internal/todo"
)

// twoSprints is a list with the open sprints Sprint 1 and Sprint 2, the done
// Old, a backlog task and a Sprint 1 task in progress.
func twoSprints(t *testing.T) (model, string) {
	t.Helper()
	return newJSONModel(t, sprintJSON(t, nil,
		[]seedSprint{
			{ID: "s000001", Name: "Sprint 1", Status: "open"},
			{ID: "s000002", Name: "Sprint 2", Status: "open"},
			{ID: "s000003", Name: "Old", Status: "done"},
		},
		seedTask{ID: "t000001", Title: "backlogTask", Status: "open"},
		seedTask{ID: "t000002", Title: "working", Status: "working", Sprint: "s000001"},
		seedTask{ID: "t000003", Title: "oldDone", Status: "done", Sprint: "s000003"},
	))
}

// rowLabels lists the labels of the open sprint dialog.
func rowLabels(m model) []string {
	var out []string
	for _, r := range m.sprintPick.rows {
		out = append(out, r.label)
	}
	return out
}

func TestKJumpsToChosenSprintIncludingDone(t *testing.T) {
	m, _ := twoSprints(t)
	m = press(m, "K")
	if m.mode != modeSprintPick || m.sprintPick.title != "Go to sprint" {
		t.Fatalf("K: mode %v title %q, want the jump dialog", m.mode, m.sprintPick.title)
	}
	if got, want := strings.Join(rowLabels(m), ","), "Sprint 1,Sprint 2,✓ Old"; got != want {
		t.Fatalf("rows = %q, want %q (open first, done marked)", got, want)
	}
	m = press(m, "down", "down", "enter")
	if m.mode != modeMain || m.view.screen != screenSprint || m.view.sprint != "s000003" {
		t.Errorf("after enter: mode %v screen %v sprint %q, want Old's board", m.mode, m.view.screen, m.view.sprint)
	}
	// K works from the Sprint list too, and esc cancels.
	m = press(m, "s", "K", "esc")
	if m.mode != modeMain || m.view.screen != screenSprints {
		t.Errorf("K, esc on the Sprint list: mode %v screen %v", m.mode, m.view.screen)
	}
}

func TestPAssignsBacklogTask(t *testing.T) {
	m, path := twoSprints(t)
	if !m.tree.selectID("t000001") {
		t.Fatal("backlog task not shown")
	}
	m = press(m, "p")
	if m.mode != modeSprintPick || m.sprintPick.title != "Assign to sprint" {
		t.Fatalf("p: mode %v title %q, want the assign dialog", m.mode, m.sprintPick.title)
	}
	if got, want := strings.Join(rowLabels(m), ","), "Sprint 1,Sprint 2"; got != want {
		t.Fatalf("rows = %q, want only the open sprints %q", got, want)
	}
	m = press(m, "enter")
	if m.err != nil || m.mode != modeMain {
		t.Fatalf("after enter: mode %v err %v", m.mode, m.err)
	}
	it := find(loadSprintDoc(t, path), "backlogTask")
	if it.Sprint != "s000001" || it.Status != todo.New {
		t.Errorf("stored task = sprint %q state %v, want Sprint 1 and new", it.Sprint, it.Status)
	}
	if m.status != "moved to Sprint 1 — new" {
		t.Errorf("status = %q", m.status)
	}
	if find(m.doc, "backlogTask").Sprint == "" {
		t.Error("the view still holds the task in the Backlog")
	}
}

func TestOSendsToOtherSprintKeepingState(t *testing.T) {
	m, path := twoSprints(t)
	m.switchScreen(screenSprint, "s000001")
	m.setColumn(todo.ColInProgress)
	if !m.tree.selectID("t000002") {
		t.Fatal("working task not shown")
	}
	m = press(m, "o")
	if m.sprintPick.title != "Send to" || strings.Join(rowLabels(m), ",") != "Backlog,Sprint 2" {
		t.Fatalf("o: title %q rows %v", m.sprintPick.title, rowLabels(m))
	}
	m = press(m, "down", "enter")
	it := find(loadSprintDoc(t, path), "working")
	if m.err != nil || it.Sprint != "s000002" || it.Status != todo.Working {
		t.Errorf("task = sprint %q state %v err %v, want Sprint 2, working", it.Sprint, it.Status, m.err)
	}
	if m.status != "moved to Sprint 2 — "+todo.Working.Label() {
		t.Errorf("status = %q", m.status)
	}
}

func TestOSendsToBacklogAsOpen(t *testing.T) {
	m, path := twoSprints(t)
	m.switchScreen(screenSprint, "s000001")
	m.setColumn(todo.ColInProgress)
	m.tree.selectID("t000002")
	m = press(m, "o", "enter")
	it := find(loadSprintDoc(t, path), "working")
	if m.err != nil || it.Sprint != "" || it.Status != todo.Open {
		t.Errorf("task = sprint %q state %v err %v, want the Backlog, open", it.Sprint, it.Status, m.err)
	}
	if m.status != "moved to Backlog" {
		t.Errorf("status = %q", m.status)
	}
}

func TestOListsOnlyOtherOpenSprints(t *testing.T) {
	m, _ := twoSprints(t)
	m = press(m, "a") // the unified view shows every task
	m.tree.selectID("t000002")
	m = press(m, "o")
	if got := strings.Join(rowLabels(m), ","); got != "Backlog,Sprint 2" {
		t.Errorf("rows = %q, want the Backlog and the other open sprint only", got)
	}
	m = press(m, "esc")
	m.tree.selectID("t000001")
	if m = press(m, "o"); m.mode != modeMain || m.status != "use p to assign it to a sprint" {
		t.Errorf("o on a backlog task: mode %v status %q", m.mode, m.status)
	}
}

func TestOOnSubtaskHints(t *testing.T) {
	m, _ := newTestModel(t, "# Todo\n\n## C\n\n- [ ] parent\n  - [ ] child\n")
	m = press(m, "a")
	m.tree.selectID(find(m.doc, "child").ID)
	if m = press(m, "o"); m.mode != modeMain || m.status != "subtasks move with their task" {
		t.Errorf("o on a subtask: mode %v status %q", m.mode, m.status)
	}
}

func TestNoOpenSprintHints(t *testing.T) {
	const hint = "no open sprints — press s, then n to create one"
	m, _ := newJSONModel(t, sprintJSON(t, nil, nil, seedTask{ID: "t000001", Title: "x", Status: "open"}))
	m.tree.selectID("t000001")
	for _, k := range []string{"k", "K", "p"} {
		m2 := press(m, k)
		if m2.mode != modeMain || m2.status != hint {
			t.Errorf("%s: mode %v status %q, want the hint and no dialog", k, m2.mode, m2.status)
		}
	}

	m, _ = newJSONModel(t, sprintJSON(t, nil,
		[]seedSprint{{ID: "s000001", Name: "Only", Status: "open"}},
		seedTask{ID: "t000001", Title: "x", Status: "new", Sprint: "s000001"}))
	m.switchScreen(screenSprint, "s000001")
	m.tree.selectID("t000001")
	m = press(m, "o")
	if got := strings.Join(rowLabels(m), ","); got != "Backlog" {
		t.Errorf("o with one open sprint: rows = %q, want only Backlog", got)
	}
}
