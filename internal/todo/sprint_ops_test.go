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
	sub := f.sub(id, Open)
	if it := f.item(sub); it.Sprint != "" || it.SprintID() != s {
		t.Errorf("subtask Sprint = %q, SprintID %q; want none of its own, its task's %q", it.Sprint, it.SprintID(), s)
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
	if _, err := f.d.EditSprint("zzzzzzz", SprintChanges{}); !errors.Is(err, ErrSprintNotFound) {
		t.Errorf("err = %v, want ErrSprintNotFound", err)
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
	// A done sprint's task leaves it as any other does.
	doneTask := f.task("d", Done, s)
	if changed, err := f.d.Unassign(doneTask); err != nil || !changed {
		t.Errorf("done sprint: Unassign = %v, %v", changed, err)
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

func TestAssignSameDoneSprintIsNoChange(t *testing.T) {
	f := newSprintFixture(t)
	s := f.sprint("S")
	task := f.task("t", Done, s)
	if changed, err := f.d.Assign(task, s); err != nil || changed {
		t.Errorf("Assign = %v, %v; want no change, no error", changed, err)
	}
}

func TestSprintGoalIsOneLine(t *testing.T) {
	f := newSprintFixture(t)
	id, err := f.d.AddSprint("S", "  ship\nsprints\r\nnow\t!  \n")
	if err != nil {
		t.Fatal(err)
	}
	s, _ := f.d.SprintByID(id)
	if want := "ship sprints now !"; s.Goal != want {
		t.Errorf("AddSprint goal = %q, want %q", s.Goal, want)
	}
	goal := "\tfirst\nsecond "
	if _, err := f.d.EditSprint(id, SprintChanges{Goal: &goal}); err != nil {
		t.Fatal(err)
	}
	if want := "first second"; s.Goal != want {
		t.Errorf("EditSprint goal = %q, want %q", s.Goal, want)
	}
	// A goal that cleans to the one it holds is no change.
	same := " first second\n"
	if changed, err := f.d.EditSprint(id, SprintChanges{Goal: &same}); err != nil || changed {
		t.Errorf("EditSprint to the same goal = %v, %v; want no change", changed, err)
	}
}

func TestDeleteSprintRefusedWhileNotDone(t *testing.T) {
	f := newSprintFixture(t)
	s := f.sprint("S")
	task := f.task("t", Done, s)
	f.sub(task, Open) // the task is done, but a subtask is not
	f.task("u", Working, s)
	err := f.d.DeleteSprint(s)
	var nd *SprintNotDoneError
	if !errors.Is(err, ErrSprintNotDone) || !errors.Is(err, ErrBadRequest) || !errors.As(err, &nd) || nd.Remaining != 2 {
		t.Fatalf("err = %v, want SprintNotDoneError{2}", err)
	}
	if _, ok := f.d.SprintByID(s); !ok {
		t.Error("sprint removed on error")
	}
	if it := f.item(task); it.Sprint != s || it.Status != Done {
		t.Errorf("task changed on error: %q %v", it.Sprint, it.Status)
	}
}

func TestDeleteDoneSprintRemovesItsTasks(t *testing.T) {
	f := newSprintFixture(t)
	s := f.sprint("S")
	keep := f.sprint("Keep")
	task := f.task("t", Done, s)
	sub := f.sub(task, Done)
	other := f.task("other", Working, keep)
	backlog := f.task("backlog", Open, "")
	if err := f.d.DeleteSprint(s); err != nil {
		t.Fatal(err)
	}
	if _, ok := f.d.SprintByID(s); ok {
		t.Error("sprint still there")
	}
	for _, id := range []string{task, sub} {
		if _, err := f.d.lookup(id); !errors.Is(err, ErrNotFound) {
			t.Errorf("%s still there: %v", id, err)
		}
	}
	if _, ok := f.d.SprintByID(keep); !ok {
		t.Error("other sprint removed")
	}
	if it := f.item(other); it.Sprint != keep || it.Status != Working {
		t.Errorf("other sprint's task = %q %v", it.Sprint, it.Status)
	}
	if it := f.item(backlog); it.Sprint != "" || it.Status != Open {
		t.Errorf("backlog task = %q %v", it.Sprint, it.Status)
	}
	if _, err := f.d.lookup(f.cat); err != nil {
		t.Errorf("the category went with the tasks: %v", err)
	}
}

func TestDeleteEmptySprint(t *testing.T) {
	f := newSprintFixture(t)
	s := f.sprint("S")
	if got := f.d.SprintStatus(s); got != SprintOpen {
		t.Errorf("empty sprint status = %v, want open", got)
	}
	if err := f.d.DeleteSprint(s); err != nil {
		t.Fatal(err)
	}
	if len(f.d.Sprints) != 0 {
		t.Errorf("sprints = %v, want none", f.d.Sprints)
	}
}

func TestDeleteMissingSprint(t *testing.T) {
	f := newSprintFixture(t)
	if err := f.d.DeleteSprint("zzzzzzz"); !errors.Is(err, ErrSprintNotFound) {
		t.Errorf("err = %v, want ErrSprintNotFound", err)
	}
}

func TestSprintStatusIsLowestTaskLevel(t *testing.T) {
	tests := []struct {
		name  string
		tasks []Status
		want  SprintStatus
	}{
		{"empty", nil, SprintOpen},
		{"new and ready", []Status{New, Ready}, SprintOpen},
		{"all ready", []Status{Ready, Ready}, SprintReady},
		{"ready and done", []Status{Ready, Done}, SprintReady},
		{"working and done", []Status{Working, Done}, SprintInProgress},
		{"review and followup", []Status{Review, Followup}, SprintInProgress},
		{"working, review, followup, done", []Status{Working, Review, Followup, Done}, SprintInProgress},
		{"all done", []Status{Done, Done}, SprintDone},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := newSprintFixture(t)
			s := f.sprint("S")
			for _, st := range tc.tasks {
				f.task("t", st, s)
			}
			f.task("backlog", Open, "") // the backlog never counts
			if got := f.d.SprintStatus(s); got != tc.want {
				t.Errorf("SprintStatus = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestSprintStatusDoneNeedsDoneSubtasks(t *testing.T) {
	f := newSprintFixture(t)
	s := f.sprint("S")
	task := f.task("t", Done, s)
	sub := f.sub(task, Open)
	if got := f.d.SprintStatus(s); got != SprintInProgress {
		t.Errorf("done task with an open subtask: SprintStatus = %v, want progress", got)
	}
	f.item(sub).Status = Done
	if got := f.d.SprintStatus(s); got != SprintDone {
		t.Errorf("all done: SprintStatus = %v, want done", got)
	}
	// The other sprints and the open list follow their own tasks.
	other := f.sprint("Other")
	if got := f.d.SprintStatus(other); got != SprintOpen {
		t.Errorf("other sprint = %v, want open", got)
	}
	if open := f.d.OpenSprints(); len(open) != 1 || open[0].ID != other {
		t.Errorf("OpenSprints = %v, want only %s", open, other)
	}
}

func TestSprintStatusNames(t *testing.T) {
	for st, want := range map[SprintStatus][2]string{
		SprintOpen: {"open", "Open"}, SprintReady: {"ready", "Ready"},
		SprintInProgress: {"progress", "In Progress"}, SprintDone: {"done", "Done"},
	} {
		if st.String() != want[0] || st.Title() != want[1] {
			t.Errorf("%d: %q %q, want %q %q", int(st), st.String(), st.Title(), want[0], want[1])
		}
	}
}
