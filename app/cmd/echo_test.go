package cmd

import (
	"encoding/json"
	"strings"
	"testing"
)

// decodeRecord runs the CLI and decodes its stdout as one record.
func decodeRecord(t *testing.T, args ...string) record {
	t.Helper()
	out, _, err := runCmdSplit(t, args...)
	if err != nil {
		t.Fatalf("%v: %v", args, err)
	}
	var r record
	if err := json.Unmarshal([]byte(out), &r); err != nil {
		t.Fatalf("not a record: %v\n%s", err, out)
	}
	return r
}

func TestAddJSONEchoesRecord(t *testing.T) {
	path := boardFixture(t)
	r := decodeRecord(t, "add", "--file", path, "--title", "t", "--parent-title", "Work", "--ref", "APP-009", "--json")
	if r.Title != "t" || r.Ref != "APP-009" || r.Status != "open" || len(r.Path) != 1 || r.Path[0] != "Work" {
		t.Errorf("record = %+v", r)
	}
	if r.ID == "" || r.ID != idOf(t, path, "t") {
		t.Errorf("id = %q, want %q", r.ID, idOf(t, path, "t"))
	}
}

func TestAddCategoryJSONShowsPrefix(t *testing.T) {
	path := boardFixture(t)
	r := decodeRecord(t, "add-category", "--file", path, "--title", "X", "--prefix", "WRK", "--json")
	if r.Kind != "category" || r.Prefix != "WRK" {
		t.Errorf("record = %+v", r)
	}
}

func TestDoneCascadeJSONShowsSubtasks(t *testing.T) {
	path := boardFixture(t)
	r := decodeRecord(t, "done", "--file", path, "--title", "busy", "--cascade", "--json")
	if r.Status != "done" || len(r.Subtasks) != 1 {
		t.Fatalf("record = %+v", r)
	}
	for _, s := range r.Subtasks {
		if s.Status != "done" {
			t.Errorf("subtask %+v not done", s)
		}
	}
}

func TestDoneJSONHintStaysOnStderr(t *testing.T) {
	path := boardFixture(t)
	out, errOut, err := runCmdSplit(t, "done", "--file", path, "--title", "busy", "--json")
	if err != nil {
		t.Fatal(err)
	}
	var r record
	if err := json.Unmarshal([]byte(out), &r); err != nil {
		t.Fatalf("stdout is not a record: %v\n%s", err, out)
	}
	if !strings.Contains(errOut, "1 subtask still open") {
		t.Errorf("stderr = %q", errOut)
	}
}

func TestAssignJSON(t *testing.T) {
	path := boardFixture(t)
	addSprint(t, path, "S1")
	r := decodeRecord(t, "assign", "--file", path, "--sprint", "S1", "--title", "backlogged", "--json")
	if r.Status != "new" || r.SprintName != "S1" {
		t.Errorf("record = %+v", r)
	}
}

func TestMoveJSON(t *testing.T) {
	path := boardFixture(t)
	r := decodeRecord(t, "move", "--file", path, "--title", "elsewhere", "--parent-title", "Work", "--json")
	if len(r.Path) != 1 || r.Path[0] != "Work" {
		t.Errorf("record = %+v", r)
	}
}

func TestEditJSON(t *testing.T) {
	path := boardFixture(t)
	r := decodeRecord(t, "edit", "--file", path, "--title", "backlogged", "--set-plan", "x", "--json")
	if r.Plan != "x" {
		t.Errorf("record = %+v", r)
	}
}

func TestRmJSON(t *testing.T) {
	path := boardFixture(t)
	out, _, err := runCmdSplit(t, "rm", "--file", path, "--title", "busy", "--json")
	if err != nil {
		t.Fatal(err)
	}
	var got struct{ Removed record }
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if got.Removed.Title != "busy" || len(got.Removed.Subtasks) != 1 || got.Removed.Subtasks[0].Title != "sub" {
		t.Errorf("removed = %+v", got.Removed)
	}
}

func TestPruneJSON(t *testing.T) {
	path := boardFixture(t)
	out, _, err := runCmdSplit(t, "prune", "--file", path, "--all", "--json")
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]int
	if err := json.Unmarshal([]byte(out), &got); err != nil || got["removed"] != 1 {
		t.Errorf("got %v, %v\n%s", got, err, out)
	}
}

func TestSprintAddJSON(t *testing.T) {
	path := boardFixture(t)
	out, _, err := runCmdSplit(t, "sprint", "add", "--file", path, "--name", "S9", "--json")
	if err != nil {
		t.Fatal(err)
	}
	var sp jsonSprint
	if err := json.Unmarshal([]byte(out), &sp); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if sp.Name != "S9" || sp.Status != "open" || sp.ID == "" {
		t.Errorf("sprint = %+v", sp)
	}
	// S9 has no task, so it may be removed.
	out, _, err = runCmdSplit(t, "sprint", "rm", "--file", path, "--sprint", "S9", "--json")
	if err != nil {
		t.Fatal(err)
	}
	var rm struct{ Removed jsonSprint }
	if err := json.Unmarshal([]byte(out), &rm); err != nil || rm.Removed.Name != "S9" {
		t.Errorf("rm = %+v, %v\n%s", rm, err, out)
	}
}

func TestSprintEditJSON(t *testing.T) {
	path := boardFixture(t)
	addSprint(t, path, "S1")
	out, _, err := runCmdSplit(t, "sprint", "edit", "--file", path, "--sprint", "S1", "--set-goal", "g", "--json")
	if err != nil {
		t.Fatal(err)
	}
	var sp jsonSprint
	if err := json.Unmarshal([]byte(out), &sp); err != nil || sp.Goal != "g" {
		t.Errorf("sprint = %+v, %v", sp, err)
	}
}

func TestMutationTextUnchanged(t *testing.T) {
	path := boardFixture(t)
	out, _, err := runCmdSplit(t, "done", "--file", path, "--title", "busy")
	if err != nil {
		t.Fatal(err)
	}
	if out != "done: [done] busy  (Work)\n" {
		t.Errorf("out = %q", out)
	}
}
