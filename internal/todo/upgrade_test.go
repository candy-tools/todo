package todo_test

import (
	"context"
	"encoding/json"
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
	if len(d.Sprints) != 1 || *d.Sprints[0] != (todo.Sprint{ID: "0000000", Name: "Imported", Ref: "SPRINT-001"}) {
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
	if f := readFile(t, path); !strings.Contains(f, `"version": 9`) {
		t.Errorf("file is not version 9:\n%s", f)
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
	if len(got.Sprints) != 2 || *got.Sprints[0] != (todo.Sprint{ID: "sssssss", Name: "One", Goal: "ship", Ref: "SPRINT-001"}) ||
		*got.Sprints[1] != (todo.Sprint{ID: "rrrrrrr", Name: "Two", Ref: "SPRINT-002"}) {
		t.Errorf("Sprints = %+v %+v", got.Sprints[0], got.Sprints[1])
	}
	if find(got, "a").Sprint != "sssssss" || find(got, "b").Sprint != "rrrrrrr" {
		t.Error("tasks lost their sprints")
	}
	// The stored sprint status was read and ignored: the tasks decide it.
	if a, b := got.SprintStatus("sssssss"), got.SprintStatus("rrrrrrr"); a != todo.SprintDone || b != todo.SprintInProgress {
		t.Errorf("SprintStatus = %v, %v; want done, progress", a, b)
	}
}

// assertNoSprintStatus fails when a sprint of the list file at path still
// holds a "status" field: version 9 no longer writes it.
func assertNoSprintStatus(t *testing.T, path string) {
	t.Helper()
	var l struct {
		Sprints []map[string]any `json:"sprints"`
	}
	if err := json.Unmarshal([]byte(readFile(t, path)), &l); err != nil {
		t.Fatal(err)
	}
	for _, sp := range l.Sprints {
		if _, ok := sp["status"]; ok {
			t.Errorf("sprint %v still writes a status", sp["id"])
		}
	}
}

// A version 8 file's sprint status is read and ignored, even one that does not
// match its tasks (a "done" sprint with a working task, which version 8
// refused), and is not written back.
func TestV8SprintStatusIgnored(t *testing.T) {
	path := todotest.SeedJSON(t, `{"version": 8,
		"sprints": [{"id": "sssssss", "ref": "SPRINT-001", "name": "One", "status": "done"},
			{"id": "rrrrrrr", "ref": "SPRINT-002", "name": "Two", "status": "open"}],
		"categories": [{"id": "c000000", "title": "C"}], "tasks": [
		{"id": "t000001", "category": "c000000", "sprint": "sssssss", "title": "a", "status": "working"},
		{"id": "t000002", "category": "c000000", "sprint": "rrrrrrr", "title": "b", "status": "done"}]}`)
	d := loadList(t, path)
	if a, b := d.SprintStatus("sssssss"), d.SprintStatus("rrrrrrr"); a != todo.SprintInProgress || b != todo.SprintDone {
		t.Errorf("SprintStatus = %v, %v; want progress, done", a, b)
	}
	if _, err := todo.NewStore(path).Edit(context.Background(), "t000001", todo.Changes{Title: new("a2")}); err != nil {
		t.Fatal(err)
	}
	assertNoSprintStatus(t, path)
}
