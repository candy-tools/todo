package todo

import (
	"errors"
	"reflect"
	"testing"
)

// unchanged fails the test when d no longer renders as before.
func (f *sprintFixture) unchanged(before *Document) {
	f.t.Helper()
	if got, want := toDisk(f.d), toDisk(before); !reflect.DeepEqual(got, want) {
		f.t.Errorf("the document changed on error:\n%+v\nwant\n%+v", got, want)
	}
}

func TestAddTopLevelSprintStateWithoutSprint(t *testing.T) {
	f := newSprintFixture(t)
	before := f.d.Clone()
	if _, err := f.d.AppendTask(Fields{Title: "t"}, Working, f.cat); !errors.Is(err, ErrNeedsSprint) {
		t.Fatalf("err = %v, want ErrNeedsSprint", err)
	}
	f.unchanged(before)
}

func TestAddBacklogStateWithSprint(t *testing.T) {
	f := newSprintFixture(t)
	s := f.sprint("S")
	before := f.d.Clone()
	if _, err := f.d.AppendTask(Fields{Title: "t", Sprint: s}, Deferred, f.cat); !errors.Is(err, ErrInSprint) {
		t.Fatalf("err = %v, want ErrInSprint", err)
	}
	f.unchanged(before)
}

func TestAddToMissingSprint(t *testing.T) {
	f := newSprintFixture(t)
	if _, err := f.d.AppendTask(Fields{Title: "t", Sprint: "zzzzzzz"}, New, f.cat); !errors.Is(err, ErrSprintNotFound) {
		t.Fatalf("err = %v, want ErrSprintNotFound", err)
	}
}

func TestAddSubtaskWithSprintRefused(t *testing.T) {
	f := newSprintFixture(t)
	s := f.sprint("S")
	task := f.task("t", New, s)
	if _, err := f.d.AppendTask(Fields{Title: "x", Sprint: s}, Open, task); !errors.Is(err, ErrSprintMembership) {
		t.Fatalf("err = %v, want ErrSprintMembership", err)
	}
}

func TestSetStateWorkingOnBacklogTask(t *testing.T) {
	f := newSprintFixture(t)
	task := f.task("t", Open, "")
	before := f.d.Clone()
	if _, err := f.d.SetState(task, Working, StateOptions{}); !errors.Is(err, ErrNeedsSprint) {
		t.Fatalf("err = %v, want ErrNeedsSprint", err)
	}
	if _, err := f.d.SetState(task, Done, StateOptions{Cascade: true}); !errors.Is(err, ErrNeedsSprint) {
		t.Fatalf("cascade: err = %v, want ErrNeedsSprint", err)
	}
	f.unchanged(before)
}

func TestSetStateDeferredOnSprintTask(t *testing.T) {
	f := newSprintFixture(t)
	task := f.task("t", Working, f.sprint("S"))
	before := f.d.Clone()
	if _, err := f.d.SetState(task, Deferred, StateOptions{}); !errors.Is(err, ErrInSprint) {
		t.Fatalf("err = %v, want ErrInSprint", err)
	}
	f.unchanged(before)
}

func TestMoveCategoryKeepsSprint(t *testing.T) {
	f := newSprintFixture(t)
	other, err := f.d.AddCategory("D", "", f.cat)
	if err != nil {
		t.Fatal(err)
	}
	s := f.sprint("S")
	task := f.task("t", Working, s)
	if err := f.d.Move(task, other, ""); err != nil {
		t.Fatal(err)
	}
	if it := f.item(task); it.Sprint != s || it.Status != Working {
		t.Errorf("task = %q %v, want %q working", it.Sprint, it.Status, s)
	}
}

func TestNestAcrossSprintsRefused(t *testing.T) {
	f := newSprintFixture(t)
	a := f.task("a", New, f.sprint("A"))
	b := f.task("b", New, f.sprint("B"))
	before := f.d.Clone()
	if err := f.d.Move(b, a, ""); !errors.Is(err, ErrSprintMembership) {
		t.Fatalf("err = %v, want ErrSprintMembership", err)
	}
	f.unchanged(before)
}

func TestNestBacklogUnderSprintTaskRefused(t *testing.T) {
	f := newSprintFixture(t)
	a := f.task("a", New, f.sprint("A"))
	b := f.task("b", Open, "")
	if err := f.d.Move(b, a, ""); !errors.Is(err, ErrSprintMembership) {
		t.Errorf("backlog under sprint: err = %v, want ErrSprintMembership", err)
	}
	if err := f.d.Move(a, b, ""); !errors.Is(err, ErrSprintMembership) {
		t.Errorf("sprint under backlog: err = %v, want ErrSprintMembership", err)
	}
}

func TestNestInSameSprintDropsOwnSprint(t *testing.T) {
	f := newSprintFixture(t)
	s := f.sprint("S")
	a, b := f.task("a", New, s), f.task("b", Working, s)
	if err := f.d.Move(b, a, ""); err != nil {
		t.Fatal(err)
	}
	if it := f.item(b); it.Sprint != "" || it.SprintID() != s || it.Status != Open {
		t.Errorf("subtask = own %q, SprintID %q, %v; want none, %q, open", it.Sprint, it.SprintID(), it.Status, s)
	}
}

func TestLiftSubtaskTakesParentSprint(t *testing.T) {
	f := newSprintFixture(t)
	s := f.sprint("S")
	task := f.task("t", Review, s)
	sub := f.sub(task, Open)
	if err := f.d.Move(sub, f.cat, ""); err != nil {
		t.Fatal(err)
	}
	if it := f.item(sub); it.Sprint != s || it.Status != Review {
		t.Errorf("lifted = %q %v, want %q review", it.Sprint, it.Status, s)
	}
}

func TestLiftDoneSubtaskOfBacklogTaskRefused(t *testing.T) {
	f := newSprintFixture(t)
	task := f.task("t", Open, "")
	sub := f.sub(task, Done)
	before := f.d.Clone()
	if err := f.d.Move(sub, f.cat, ""); !errors.Is(err, ErrNeedsSprint) {
		t.Fatalf("err = %v, want ErrNeedsSprint", err)
	}
	f.unchanged(before)
}

func TestStepInOntoHiddenOtherSprintTaskRefused(t *testing.T) {
	f := newSprintFixture(t)
	f.task("other", New, f.sprint("Other"))
	mine := f.task("mine", New, f.sprint("Mine"))
	if _, _, err := f.d.StepPlacement(mine, StepIn, nil); !errors.Is(err, ErrSprintMembership) {
		t.Fatalf("err = %v, want ErrSprintMembership", err)
	}
}

// A sprint whose tasks are all done is not frozen: its tasks reopen, nest and
// lift, and it takes new tasks, which move its state back.
func TestDoneSprintStaysEditable(t *testing.T) {
	f := newSprintFixture(t)
	s := f.sprint("S")
	a, b := f.task("a", Done, s), f.task("b", Done, s)
	sub := f.sub(a, Done)
	if got := f.d.SprintStatus(s); got != SprintDone {
		t.Fatalf("status = %v, want done", got)
	}
	if err := f.d.Move(b, a, ""); err != nil {
		t.Errorf("nest: %v", err)
	}
	if err := f.d.Move(sub, f.cat, ""); err != nil {
		t.Errorf("lift: %v", err)
	}
	if _, err := f.d.SetState(a, Working, StateOptions{}); err != nil {
		t.Errorf("reopen: %v", err)
	}
	if got := f.d.SprintStatus(s); got != SprintInProgress {
		t.Errorf("status after reopen = %v, want progress", got)
	}
	if _, err := f.d.AppendTask(Fields{Title: "new", Sprint: s}, New, f.cat); err != nil {
		t.Errorf("add: %v", err)
	}
	if got := f.d.SprintStatus(s); got != SprintOpen {
		t.Errorf("status after add = %v, want open", got)
	}
}
