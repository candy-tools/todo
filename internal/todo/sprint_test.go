package todo

import (
	"errors"
	"testing"
)

func TestStatusInSprint(t *testing.T) {
	for _, s := range []Status{Open, Deferred, WontDo} {
		if s.InSprint() {
			t.Errorf("%v.InSprint() = true, want false", s)
		}
	}
	for _, s := range []Status{Working, Done, New, Ready, Review, Followup} {
		if !s.InSprint() {
			t.Errorf("%v.InSprint() = false, want true", s)
		}
	}
}

func TestResolveSprintIDBeforeName(t *testing.T) {
	a := &Sprint{ID: "aaaaaaa", Name: "bbbbbbb"}
	b := &Sprint{ID: "bbbbbbb", Name: "Sprint 2"}
	d := &Document{Sprints: []*Sprint{a, b}}
	if got, ok := d.ResolveSprint("bbbbbbb"); !ok || got != b {
		t.Errorf("ResolveSprint(id) = %v, %v; want B", got, ok)
	}
	if got, ok := d.ResolveSprint("SPRINT 2"); !ok || got != b {
		t.Errorf("ResolveSprint(name) = %v, %v; want B", got, ok)
	}
	if _, ok := d.ResolveSprint("nope"); ok {
		t.Error("ResolveSprint(nope) found a sprint")
	}
}

func TestSubtaskSprintID(t *testing.T) {
	parent := &Item{Kind: Task, Sprint: "aaaaaaa"}
	sub := &Item{Kind: Task}
	parent.AppendChild(sub)
	if got := sub.SprintID(); got != "aaaaaaa" {
		t.Errorf("subtask SprintID() = %q, want aaaaaaa", got)
	}
	if got := parent.SprintID(); got != "aaaaaaa" {
		t.Errorf("task SprintID() = %q, want aaaaaaa", got)
	}
}

func TestUnfinishedCountsSubtasks(t *testing.T) {
	cat := &Item{Kind: Category, Level: 1}
	done := &Item{Kind: Task, Status: Done, Sprint: "s"}
	done.AppendChild(&Item{Kind: Task, Status: Open})
	working := &Item{Kind: Task, Status: Working, Sprint: "s"}
	other := &Item{Kind: Task, Status: Working, Sprint: "t"}
	cat.AppendChild(done)
	cat.AppendChild(working)
	cat.AppendChild(other)
	d := &Document{Roots: []*Item{cat}}
	if got := d.Unfinished("s"); got != 2 {
		t.Errorf("Unfinished = %d, want 2", got)
	}
	if got := len(d.SprintTasks("s")); got != 2 {
		t.Errorf("SprintTasks = %d, want 2", got)
	}
}

func TestSprintNotDoneErrorText(t *testing.T) {
	for _, c := range []struct {
		n    int
		want string
	}{{1, "1 task not done"}, {3, "3 tasks not done"}} {
		var err error = &SprintNotDoneError{Remaining: c.n}
		if err.Error() != c.want {
			t.Errorf("Error() = %q, want %q", err.Error(), c.want)
		}
		if !errors.Is(err, ErrBadRequest) || !errors.Is(err, ErrSprintNotDone) {
			t.Errorf("errors.Is class/sentinel failed for %v", err)
		}
	}
}

func TestCloneCopiesSprints(t *testing.T) {
	d := &Document{Sprints: []*Sprint{{ID: "a", Name: "one"}}}
	c := d.Clone()
	c.Sprints[0].Name = "changed"
	if d.Sprints[0].Name != "one" {
		t.Errorf("original name = %q, want one", d.Sprints[0].Name)
	}
}
