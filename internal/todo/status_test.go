package todo_test

import (
	"slices"
	"testing"

	"github.com/candy-tools/todo/internal/todo"
)

func TestStatusNumbersArePinned(t *testing.T) {
	want := map[todo.Status]int{
		todo.Open: 0, todo.Working: 1, todo.Deferred: 2, todo.Done: 3,
		todo.New: 4, todo.Ready: 5, todo.Review: 6, todo.Followup: 7, todo.WontDo: 8,
	}
	for s, n := range want {
		if int(s) != n {
			t.Errorf("%v = %d, want %d", s, int(s), n)
		}
	}
}

func TestStatusNamesRoundTrip(t *testing.T) {
	for _, c := range todo.Columns() {
		for _, s := range c.States() {
			got, ok := todo.StatusFromName(s.String())
			if !ok || got != s {
				t.Errorf("StatusFromName(%q) = %v ok=%v, want %v", s.String(), got, ok, s)
			}
		}
	}
}

func TestStatusFromNameProgressAlias(t *testing.T) {
	if got, ok := todo.StatusFromName("progress"); !ok || got != todo.Working {
		t.Errorf("StatusFromName(progress) = %v ok=%v, want Working", got, ok)
	}
	if _, ok := todo.StatusFromName("bogus"); ok {
		t.Error("StatusFromName(bogus) should be ok=false")
	}
}

func TestUnknownStatusIsVisible(t *testing.T) {
	if got := todo.Status(9).String(); got != "status(9)" {
		t.Errorf("String() = %q, want status(9)", got)
	}
	if got := todo.Status(9).Column(); got != todo.ColBacklog {
		t.Errorf("Column() = %v, want ColBacklog", got)
	}
}

func TestStatusMarker(t *testing.T) {
	cases := map[todo.Status]string{
		todo.Open: " ", todo.Working: "/", todo.Deferred: ">", todo.Done: "x",
		todo.New: "", todo.Ready: "", todo.Review: "", todo.Followup: "",
	}
	for s, want := range cases {
		if got := s.Marker(); got != want {
			t.Errorf("Status(%d).Marker() = %q, want %q", s, got, want)
		}
	}
}

func TestStatusColumn(t *testing.T) {
	cases := map[todo.Status]todo.Column{
		todo.Open: todo.ColBacklog, todo.Deferred: todo.ColBacklog,
		todo.New: todo.ColNext, todo.Ready: todo.ColNext,
		todo.Working: todo.ColInProgress, todo.Review: todo.ColInProgress, todo.Followup: todo.ColInProgress,
		todo.Done: todo.ColDone,
	}
	for s, want := range cases {
		if got := s.Column(); got != want {
			t.Errorf("%v.Column() = %v, want %v", s, got, want)
		}
	}
}

func TestColumnNames(t *testing.T) {
	cases := []struct {
		c           todo.Column
		name, title string
		first       todo.Status
	}{
		{todo.ColBacklog, "backlog", "Backlog", todo.Open},
		{todo.ColNext, "next", "Next", todo.New},
		{todo.ColInProgress, "progress", "In Progress", todo.Working},
		{todo.ColDone, "done", "Done", todo.Done},
	}
	for _, c := range cases {
		if c.c.String() != c.name || c.c.Title() != c.title || c.c.FirstState() != c.first {
			t.Errorf("column %v: got %q/%q/%v", c.c, c.c.String(), c.c.Title(), c.c.FirstState())
		}
		if got, ok := todo.ColumnFromName(c.name); !ok || got != c.c {
			t.Errorf("ColumnFromName(%q) = %v ok=%v", c.name, got, ok)
		}
	}
	if _, ok := todo.ColumnFromName("x"); ok {
		t.Error("ColumnFromName(x) should be ok=false")
	}
	if len(todo.Columns()) != 4 {
		t.Errorf("Columns() = %v, want 4", todo.Columns())
	}
}

func TestSprintColumns(t *testing.T) {
	want := []todo.Column{todo.ColNext, todo.ColInProgress, todo.ColDone}
	if got := todo.SprintColumns(); !slices.Equal(got, want) {
		t.Errorf("SprintColumns() = %v, want %v", got, want)
	}
}

func TestSubtaskAllowed(t *testing.T) {
	for _, s := range []todo.Status{todo.Open, todo.Working, todo.Deferred, todo.Done, todo.New, todo.Ready, todo.Review, todo.Followup} {
		want := s == todo.Open || s == todo.Done
		if got := s.SubtaskAllowed(); got != want {
			t.Errorf("%v.SubtaskAllowed() = %v, want %v", s, got, want)
		}
	}
}

func TestNeedsPlan(t *testing.T) {
	cases := []struct {
		st   todo.Status
		plan string
		want bool
	}{
		{todo.Ready, "", true}, {todo.Ready, "  ", true},
		{todo.Ready, "docs/p.md", false}, {todo.New, "", false},
	}
	for _, c := range cases {
		it := newTask("t", "", todo.Open)
		it.Status, it.Plan = c.st, c.plan
		if got := it.NeedsPlan(); got != c.want {
			t.Errorf("NeedsPlan(%v,%q) = %v, want %v", c.st, c.plan, got, c.want)
		}
	}
	cat := &todo.Item{Kind: todo.Category, Status: todo.Ready}
	if cat.NeedsPlan() {
		t.Error("a category never needs a plan")
	}
}

func TestIsSubtask(t *testing.T) {
	cat := &todo.Item{Kind: todo.Category, Title: "c", Level: 1}
	parent := newTask("p", "", todo.Open)
	child := newTask("c", "", todo.Open)
	cat.AppendChild(parent)
	parent.AppendChild(child)
	if cat.IsSubtask() || parent.IsSubtask() {
		t.Error("category and task under a category are not subtasks")
	}
	if !child.IsSubtask() {
		t.Error("task under a task is a subtask")
	}
}

func TestStatusAfterMove(t *testing.T) {
	cases := []struct {
		cur       todo.Status
		toSubtask bool
		oldParent todo.Status
		want      todo.Status
	}{
		{todo.Review, true, todo.Open, todo.Open},
		{todo.Done, true, todo.Open, todo.Done},
		{todo.Open, false, todo.Ready, todo.Ready},
		{todo.Done, false, todo.Ready, todo.Done},
		{todo.Open, false, todo.Done, todo.Working},
	}
	for _, c := range cases {
		if got := todo.StatusAfterMove(c.cur, c.toSubtask, c.oldParent); got != c.want {
			t.Errorf("StatusAfterMove(%v,%v,%v) = %v, want %v", c.cur, c.toSubtask, c.oldParent, got, c.want)
		}
	}
}

func TestWontDo(t *testing.T) {
	if got := todo.WontDo.Column(); got != todo.ColBacklog {
		t.Errorf("wontdo column = %v, want backlog", got)
	}
	if got, _ := todo.StatusFromName("wontdo"); got != todo.WontDo {
		t.Errorf("StatusFromName(wontdo) = %v", got)
	}
	if got := todo.WontDo.Label(); got != "won't do" {
		t.Errorf("label = %q", got)
	}
	if got := todo.Open.Label(); got != "open" {
		t.Errorf("open label = %q", got)
	}
}
