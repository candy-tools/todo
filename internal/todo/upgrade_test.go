package todo_test

import (
	"context"
	"strings"
	"testing"

	"github.com/candy-tools/todo/internal/todo"
	"github.com/candy-tools/todo/internal/todo/todotest"
)

const v5Sprinted = `{"version": 5, "categories": [{"id": "c000000", "title": "C"}], "tasks": [
	{"id": "t000001", "category": "c000000", "title": "open", "status": "open"},
	{"id": "t000002", "category": "c000000", "title": "new", "status": "new"},
	{"id": "t000003", "category": "c000000", "title": "working", "status": "working"},
	{"id": "t000004", "category": "c000000", "title": "done", "status": "done",
		"subtasks": [{"id": "s000001", "title": "sub", "done": true}]},
	{"id": "t000005", "category": "c000000", "title": "deferred", "status": "deferred"}]}`

func TestUpgradeV5MovesSprintStatesToImported(t *testing.T) {
	d := loadList(t, todotest.SeedJSON(t, v5Sprinted))
	if len(d.Sprints) != 1 || *d.Sprints[0] != (todo.Sprint{ID: "0000000", Name: "Imported", Status: todo.SprintOpen}) {
		t.Fatalf("Sprints = %+v, want the one Imported sprint", d.Sprints)
	}
	for title, want := range map[string]string{"open": "", "new": "0000000", "working": "0000000", "done": "0000000", "deferred": ""} {
		if got := find(d, title).Sprint; got != want {
			t.Errorf("task %q Sprint = %q, want %q", title, got, want)
		}
	}
	if st := find(d, "working").Status; st != todo.Working {
		t.Errorf("working task status = %v, want kept", st)
	}
}

func TestUpgradeV5NoSprintStatesNoSprint(t *testing.T) {
	d := loadList(t, todotest.SeedJSON(t, `{"version": 5, "categories": [{"id": "c000000", "title": "C"}], "tasks": [
		{"id": "t000001", "category": "c000000", "title": "a", "status": "open"}]}`))
	if len(d.Sprints) != 0 {
		t.Errorf("Sprints = %+v, want none", d.Sprints)
	}
}

// Two stores that each load the same v5 file, before either writes, make the
// same Imported sprint, so the two writes leave one.
func TestUpgradeTwoWritersOneImportedSprint(t *testing.T) {
	path := todotest.SeedJSON(t, `{"version": 5, "categories": [{"id": "c000000", "title": "C"}], "tasks": [
		{"id": "t000001", "category": "c000000", "title": "w", "status": "working"}]}`)
	a, b := loadList(t, path), loadList(t, path)
	for _, d := range []*todo.Document{a, b} {
		if len(d.Sprints) != 1 || d.Sprints[0].ID != "0000000" {
			t.Fatalf("Sprints = %+v, want the Imported sprint 0000000", d.Sprints)
		}
	}
	for range 2 {
		if _, err := todo.NewStore(path).Edit(context.Background(), "t000001", todo.Changes{Title: new("w2")}); err != nil {
			t.Fatal(err)
		}
	}
	if n := len(loadList(t, path).Sprints); n != 1 {
		t.Errorf("%d sprints, want 1", n)
	}
	if f := readFile(t, path); !strings.Contains(f, `"version": 7`) {
		t.Errorf("file is not version 7:\n%s", f)
	}
}

func TestV6RoundTrip(t *testing.T) {
	path := todotest.SeedJSON(t, `{"version": 6, "view": {"split": true, "screen": "sprint", "sprint": "sssssss"},
		"sprints": [{"id": "sssssss", "name": "One", "goal": "ship", "status": "done"},
			{"id": "rrrrrrr", "name": "Two", "status": "open"}],
		"categories": [{"id": "c000000", "title": "C"}], "tasks": [
		{"id": "t000001", "category": "c000000", "sprint": "sssssss", "title": "a", "status": "done"},
		{"id": "t000002", "category": "c000000", "sprint": "rrrrrrr", "title": "b", "status": "working"}]}`)
	d := loadList(t, path)
	if _, err := todo.NewStore(path).SetView(context.Background(), d.View); err != nil {
		t.Fatal(err)
	}
	if _, err := todo.NewStore(path).Edit(context.Background(), "t000002", todo.Changes{Title: new("b")}); err != nil {
		t.Fatal(err)
	}
	got := loadList(t, path)
	if want := (todo.View{Split: true, Screen: todo.ScreenSprint, Sprint: "sssssss"}); got.View != want {
		t.Errorf("View = %+v, want %+v", got.View, want)
	}
	if len(got.Sprints) != 2 || *got.Sprints[0] != (todo.Sprint{ID: "sssssss", Name: "One", Goal: "ship", Status: todo.SprintDone}) ||
		*got.Sprints[1] != (todo.Sprint{ID: "rrrrrrr", Name: "Two"}) {
		t.Errorf("Sprints = %+v %+v", got.Sprints[0], got.Sprints[1])
	}
	if find(got, "a").Sprint != "sssssss" || find(got, "b").Sprint != "rrrrrrr" {
		t.Error("tasks lost their sprints")
	}
}
