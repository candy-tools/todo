package todo_test

import (
	"testing"

	"github.com/candy-tools/todo/internal/todo"
)

func TestStatusString(t *testing.T) {
	cases := map[todo.Status]string{
		todo.Open: "open", todo.InProgress: "progress",
		todo.Deferred: "deferred", todo.Done: "done",
	}
	for s, want := range cases {
		if got := s.String(); got != want {
			t.Errorf("Status(%d).String() = %q, want %q", s, got, want)
		}
	}
}

func TestStatusMarker(t *testing.T) {
	cases := map[todo.Status]string{
		todo.Open: " ", todo.InProgress: "/", todo.Deferred: ">", todo.Done: "x",
	}
	for s, want := range cases {
		if got := s.Marker(); got != want {
			t.Errorf("Status(%d).Marker() = %q, want %q", s, got, want)
		}
	}
}

func TestStatusFromName(t *testing.T) {
	for name, want := range map[string]todo.Status{
		"open": todo.Open, "progress": todo.InProgress,
		"deferred": todo.Deferred, "done": todo.Done,
	} {
		got, ok := todo.StatusFromName(name)
		if !ok || got != want {
			t.Errorf("StatusFromName(%q) = %v ok=%v, want %v", name, got, ok, want)
		}
	}
	if _, ok := todo.StatusFromName("bogus"); ok {
		t.Error("StatusFromName(bogus) should be ok=false")
	}
}
