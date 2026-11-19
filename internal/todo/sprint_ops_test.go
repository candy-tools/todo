package todo

import (
	"errors"
	"testing"
)

// sprintDoc returns a document with a category and the helpers to add tasks.
type sprintFixture struct {
	t   *testing.T
	d   *Document
	cat string
}

func newSprintFixture(t *testing.T) *sprintFixture {
	t.Helper()
	d := &Document{}
	cat, err := d.AddCategory("C", "", "")
	if err != nil {
		t.Fatal(err)
	}
	return &sprintFixture{t: t, d: d, cat: cat}
}

func (f *sprintFixture) sprint(name string) string {
	f.t.Helper()
	id, err := f.d.AddSprint(name, "")
	if err != nil {
		f.t.Fatal(err)
	}
	return id
}

func (f *sprintFixture) task(title string, st Status, sprint string) string {
	f.t.Helper()
	id, err := f.d.AppendTask(Fields{Title: title, Sprint: sprint}, st, f.cat)
	if err != nil {
		f.t.Fatal(err)
	}
	return id
}

func (f *sprintFixture) sub(parent string, st Status) string {
	f.t.Helper()
	id, err := f.d.AppendTask(Fields{Title: "sub"}, st, parent)
	if err != nil {
		f.t.Fatal(err)
	}
	return id
}

func (f *sprintFixture) item(id string) *Item {
	f.t.Helper()
	it, err := f.d.lookup(id)
	if err != nil {
		f.t.Fatal(err)
	}
	return it
}

func TestAddSprintDuplicateNameIgnoringCase(t *testing.T) {
	f := newSprintFixture(t)
	f.sprint("Sprint 1")
	if _, err := f.d.AddSprint("sprint 1 ", ""); !errors.Is(err, ErrDuplicateSprint) {
		t.Errorf("err = %v, want ErrDuplicateSprint", err)
	}
	if len(f.d.Sprints) != 1 {
		t.Errorf("%d sprints, want 1", len(f.d.Sprints))
	}
}

func TestAddSprintBadName(t *testing.T) {
	d := &Document{}
	if _, err := d.AddSprint("", ""); !errors.Is(err, ErrInvalidTitle) {
		t.Errorf("err = %v, want ErrInvalidTitle", err)
	}
}

func TestAddTaskJoinsSprintOnlyAtTopLevel(t *testing.T) {
	f := newSprintFixture(t)
	s := f.sprint("S")
	id := f.task("t", New, s)
	if got := f.item(id).Sprint; got != s {
		t.Errorf("Sprint = %q, want %q", got, s)
	}
	sub, err := f.d.AppendTask(Fields{Title: "x", Sprint: s}, Open, id)
	if err != nil {
		t.Fatal(err)
	}
	if got := f.item(sub).Sprint; got != "" {
		t.Errorf("subtask Sprint = %q, want empty", got)
	}
}

func TestEditSprintRenameKeepsTasks(t *testing.T) {
	f := newSprintFixture(t)
	s := f.sprint("Old")
	task := f.task("t", New, s)
	name, goal := "New", "ship it"
	changed, err := f.d.EditSprint(s, SprintChanges{Name: &name, Goal: &goal})
	if err != nil || !changed {
		t.Fatalf("EditSprint = %v, %v", changed, err)
	}
	sp, _ := f.d.SprintByID(s)
	if sp.Name != "New" || sp.Goal != "ship it" {
		t.Errorf("sprint = %+v", sp)
	}
	if got := f.item(task).Sprint; got != s {
		t.Errorf("task sprint = %q, want %q", got, s)
	}
	if changed, err := f.d.EditSprint(s, SprintChanges{Name: &name}); err != nil || changed {
		t.Errorf("same edit = %v, %v; want no change", changed, err)
	}
	other := f.sprint("Other")
	dup := "NEW"
	if _, err := f.d.EditSprint(other, SprintChanges{Name: &dup}); !errors.Is(err, ErrDuplicateSprint) {
		t.Errorf("err = %v, want ErrDuplicateSprint", err)
	}
	if _, err := f.d.EditSprint("zzzzzzz", SprintChanges{}); !errors.Is(err, ErrSprintNotFound) {
		t.Errorf("err = %v, want ErrSprintNotFound", err)
	}
}

func TestCloseSprintCountsSubtasks(t *testing.T) {
	f := newSprintFixture(t)
	s := f.sprint("S")
	task := f.task("t", Done, s)
	sub := f.sub(task, Open)
	err := f.d.CloseSprint(s)
	var nd *SprintNotDoneError
	if !errors.As(err, &nd) || nd.Remaining != 1 {
		t.Fatalf("err = %v, want SprintNotDoneError{1}", err)
	}
	if sp, _ := f.d.SprintByID(s); sp.Status != SprintOpen {
		t.Error("sprint closed despite the error")
	}
	f.item(sub).Status = Done
	if err := f.d.CloseSprint(s); err != nil {
		t.Fatal(err)
	}
	if sp, _ := f.d.SprintByID(s); sp.Status != SprintDone {
		t.Error("sprint not done")
	}
}

func TestReopenSprint(t *testing.T) {
	f := newSprintFixture(t)
	s := f.sprint("S")
	if err := f.d.CloseSprint(s); err != nil {
		t.Fatal(err)
	}
	if err := f.d.ReopenSprint(s); err != nil {
		t.Fatal(err)
	}
	if sp, _ := f.d.SprintByID(s); sp.Status != SprintOpen {
		t.Error("sprint still done")
	}
	if err := f.d.ReopenSprint("zzzzzzz"); !errors.Is(err, ErrSprintNotFound) {
		t.Errorf("err = %v", err)
	}
}

func TestDeleteSprintReturnsUnfinishedToBacklog(t *testing.T) {
	f := newSprintFixture(t)
	s := f.sprint("S")
	keep := f.sprint("Keep")
	task := f.task("t", Working, s)
	sub := f.sub(task, Done)
	if err := f.d.DeleteSprint(s); err != nil {
		t.Fatal(err)
	}
	it := f.item(task)
	if it.Sprint != "" || it.Status != Open {
		t.Errorf("task = sprint %q status %v, want backlog Open", it.Sprint, it.Status)
	}
	if f.item(sub).Status != Done {
		t.Error("subtask state changed")
	}
	if _, ok := f.d.SprintByID(s); ok {
		t.Error("sprint still there")
	}
	if _, ok := f.d.SprintByID(keep); !ok {
		t.Error("other sprint removed")
	}
}

func TestDeleteSprintRefusedWithDone(t *testing.T) {
	f := newSprintFixture(t)
	s := f.sprint("S")
	f.task("done", Done, s)
	open := f.task("open", Working, s)
	if err := f.d.DeleteSprint(s); !errors.Is(err, ErrSprintHasDone) {
		t.Fatalf("err = %v, want ErrSprintHasDone", err)
	}
	if it := f.item(open); it.Sprint != s || it.Status != Working {
		t.Error("document changed on error")
	}
	if got := f.d.PruneSprint(s); got != 1 {
		t.Fatalf("PruneSprint = %d, want 1", got)
	}
	if err := f.d.DeleteSprint(s); err != nil {
		t.Fatal(err)
	}
}

func TestAssignBacklogTaskBecomesNew(t *testing.T) {
	f := newSprintFixture(t)
	s := f.sprint("S")
	task := f.task("t", Open, "")
	changed, err := f.d.Assign(task, s)
	if err != nil || !changed {
		t.Fatalf("Assign = %v, %v", changed, err)
	}
	if it := f.item(task); it.Sprint != s || it.Status != New {
		t.Errorf("task = %q %v", it.Sprint, it.Status)
	}
	if changed, err := f.d.Assign(task, s); err != nil || changed {
		t.Errorf("repeat = %v, %v; want no change", changed, err)
	}
}

func TestAssignAcrossSprintsKeepsState(t *testing.T) {
	f := newSprintFixture(t)
	a, b := f.sprint("A"), f.sprint("B")
	task := f.task("t", Working, a)
	if changed, err := f.d.Assign(task, b); err != nil || !changed {
		t.Fatalf("Assign = %v, %v", changed, err)
	}
	if it := f.item(task); it.Sprint != b || it.Status != Working {
		t.Errorf("task = %q %v", it.Sprint, it.Status)
	}
}

func TestAssignClosedSprintRefused(t *testing.T) {
	f := newSprintFixture(t)
	open, closed := f.sprint("Open"), f.sprint("Closed")
	if err := f.d.CloseSprint(closed); err != nil {
		t.Fatal(err)
	}
	backlog := f.task("b", Open, "")
	if _, err := f.d.Assign(backlog, closed); !errors.Is(err, ErrSprintClosed) {
		t.Errorf("target closed: err = %v", err)
	}
	doneTask := f.task("d", Done, closed)
	if _, err := f.d.Assign(doneTask, open); !errors.Is(err, ErrSprintClosed) {
		t.Errorf("source closed: err = %v", err)
	}
	if it := f.item(doneTask); it.Sprint != closed {
		t.Error("task moved despite the error")
	}
}

func TestAssignSubtaskRefused(t *testing.T) {
	f := newSprintFixture(t)
	s := f.sprint("S")
	sub := f.sub(f.task("t", Open, ""), Open)
	if _, err := f.d.Assign(sub, s); !errors.Is(err, ErrTooDeep) {
		t.Errorf("err = %v, want ErrTooDeep", err)
	}
}

func TestAssignRefusals(t *testing.T) {
	f := newSprintFixture(t)
	s := f.sprint("S")
	task := f.task("t", Open, "")
	if _, err := f.d.Assign(f.cat, s); !errors.Is(err, ErrNotTask) {
		t.Errorf("category: err = %v, want ErrNotTask", err)
	}
	if _, err := f.d.Assign(task, "zzzzzzz"); !errors.Is(err, ErrSprintNotFound) {
		t.Errorf("sprint: err = %v, want ErrSprintNotFound", err)
	}
	if _, err := f.d.Assign("zzzzzzz", s); !errors.Is(err, ErrNotFound) || errors.Is(err, ErrSprintNotFound) {
		t.Errorf("task: err = %v, want the item ErrNotFound", err)
	}
}

func TestUnassignBecomesOpen(t *testing.T) {
	f := newSprintFixture(t)
	s := f.sprint("S")
	task := f.task("t", Working, s)
	if changed, err := f.d.Unassign(task); err != nil || !changed {
		t.Fatalf("Unassign = %v, %v", changed, err)
	}
	if it := f.item(task); it.Sprint != "" || it.Status != Open {
		t.Errorf("task = %q %v", it.Sprint, it.Status)
	}
	if changed, err := f.d.Unassign(task); err != nil || changed {
		t.Errorf("backlog Unassign = %v, %v; want no change", changed, err)
	}
	if err := f.d.CloseSprint(s); err != nil {
		t.Fatal(err)
	}
	doneTask := f.task("d", Done, s)
	if _, err := f.d.Unassign(doneTask); !errors.Is(err, ErrSprintClosed) {
		t.Errorf("closed: err = %v", err)
	}
}

func TestPruneSprintLeavesOtherSprints(t *testing.T) {
	f := newSprintFixture(t)
	a, b := f.sprint("A"), f.sprint("B")
	f.task("a1", Done, a)
	keepA := f.task("a2", Working, a)
	b1 := f.task("b1", Done, b)
	if got := f.d.RemovableDoneIn(a); got != 1 {
		t.Errorf("RemovableDoneIn = %d, want 1", got)
	}
	if got := f.d.PruneSprint(a); got != 1 {
		t.Errorf("PruneSprint = %d, want 1", got)
	}
	if _, err := f.d.lookup(b1); err != nil {
		t.Error("other sprint's done task removed")
	}
	if _, err := f.d.lookup(keepA); err != nil {
		t.Error("unfinished task removed")
	}
	if got := f.d.RemovableDoneIn(a); got != 0 {
		t.Errorf("RemovableDoneIn after prune = %d", got)
	}
}

func TestAssignSameClosedSprintIsNoChange(t *testing.T) {
	f := newSprintFixture(t)
	s := f.sprint("S")
	task := f.task("t", Done, s)
	if err := f.d.CloseSprint(s); err != nil {
		t.Fatal(err)
	}
	if changed, err := f.d.Assign(task, s); err != nil || changed {
		t.Errorf("Assign = %v, %v; want no change, no error", changed, err)
	}
}

func TestDeleteSprintRefusedWithDoneTaskHoldingOpenSubtask(t *testing.T) {
	f := newSprintFixture(t)
	s := f.sprint("S")
	task := f.task("t", Done, s)
	f.sub(task, Open)
	if err := f.d.DeleteSprint(s); !errors.Is(err, ErrSprintHasDone) {
		t.Fatalf("err = %v, want ErrSprintHasDone", err)
	}
	if it := f.item(task); it.Sprint != s || it.Status != Done {
		t.Errorf("task changed on error: %q %v", it.Sprint, it.Status)
	}
}
