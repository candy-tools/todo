package cmd

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/candy-tools/todo/internal/todo"
	"github.com/candy-tools/todo/internal/todo/todotest"
)

// sprintTask adds a task in sprint sprintID (state st) to the list at path.
func sprintTask(t *testing.T, path, title, sprintID string, st todo.Status) {
	t.Helper()
	s := todo.NewStore(path)
	ctx := context.Background()
	doc, _, err := s.Load(ctx)
	if err != nil {
		t.Fatal(err)
	}
	cat := doc.ByTitle("Work")
	var catID string
	if len(cat) == 0 {
		if catID, _, err = s.AddCategory(ctx, "Work", "", ""); err != nil {
			t.Fatal(err)
		}
	} else {
		catID = cat[0].ID
	}
	if _, _, err := s.AddTask(ctx, todo.Fields{Title: title, Sprint: sprintID}, st, catID, ""); err != nil {
		t.Fatal(err)
	}
}

// addSprint creates a sprint through the CLI and returns its printed ID.
func addSprint(t *testing.T, path, name string, extra ...string) string {
	t.Helper()
	args := append([]string{"sprint", "add", "--file", path, "--name", name}, extra...)
	out, err := runCmd(t, args...)
	if err != nil {
		t.Fatalf("sprint add: %v", err)
	}
	return strings.TrimSpace(out)
}

func TestSprintAddPrintsID(t *testing.T) {
	path := todotest.Seed(t, "")
	id := addSprint(t, path, "Alpha", "--goal", "ship it")
	doc, _, _ := todo.NewStore(path).Load(context.Background())
	sp, ok := doc.SprintByID(id)
	if !ok || sp.Name != "Alpha" || sp.Goal != "ship it" || sp.Status != todo.SprintOpen {
		t.Errorf("printed id %q -> %+v", id, sp)
	}
}

func TestSprintListHidesDoneUnlessAll(t *testing.T) {
	path := todotest.Seed(t, "")
	addSprint(t, path, "Alpha", "--goal", "g1")
	addSprint(t, path, "Beta")
	if _, err := runCmd(t, "sprint", "close", "--file", path, "--sprint", "beta"); err != nil {
		t.Fatal(err)
	}
	out, err := runCmd(t, "sprint", "list", "--file", path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "Alpha — g1") || strings.Contains(out, "Beta") {
		t.Errorf("default list:\n%s", out)
	}
	out, _ = runCmd(t, "sprint", "list", "--file", path, "--all")
	if !strings.Contains(out, "Alpha") || !strings.Contains(out, "Beta") || !strings.Contains(out, "done") {
		t.Errorf("--all list:\n%s", out)
	}
}

func TestSprintListJSONCounts(t *testing.T) {
	path := todotest.Seed(t, "")
	id := addSprint(t, path, "Alpha", "--goal", "g")
	sprintTask(t, path, "a", id, todo.New)
	sprintTask(t, path, "b", id, todo.New)
	sprintTask(t, path, "c", id, todo.Working)
	sprintTask(t, path, "d", id, todo.Done)
	out, err := runCmd(t, "sprint", "list", "--file", path, "--json")
	if err != nil {
		t.Fatal(err)
	}
	var got []struct {
		ID, Name, Goal, Status string
		Counts                 map[string]int
	}
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if len(got) != 1 || got[0].ID != id || got[0].Name != "Alpha" || got[0].Goal != "g" || got[0].Status != "open" {
		t.Fatalf("got %+v", got)
	}
	c := got[0].Counts
	if c["next"] != 2 || c["progress"] != 1 || c["done"] != 1 {
		t.Errorf("counts = %v", c)
	}
}

func TestSprintListJSONEmpty(t *testing.T) {
	path := todotest.Seed(t, "")
	out, err := runCmd(t, "sprint", "list", "--file", path, "--json")
	if err != nil || strings.TrimSpace(out) != "[]" {
		t.Errorf("out %q err %v", out, err)
	}
}

func TestSprintEditByName(t *testing.T) {
	path := todotest.Seed(t, "")
	addSprint(t, path, "Alpha", "--goal", "old")
	if _, err := runCmd(t, "sprint", "edit", "--file", path, "--sprint", "ALPHA", "--set-name", "Gamma", "--set-goal", "new"); err != nil {
		t.Fatal(err)
	}
	doc, _, _ := todo.NewStore(path).Load(context.Background())
	sp, ok := doc.SprintByName("Gamma")
	if !ok || sp.Goal != "new" {
		t.Errorf("sprint = %+v", sp)
	}
	_, err := runCmd(t, "sprint", "edit", "--file", path, "--sprint", "Gamma")
	if err == nil || codeOf(t, err) != exitUsage {
		t.Errorf("edit with nothing to set: %v", err)
	}
}

func TestSprintCloseRefusedExit2(t *testing.T) {
	path := todotest.Seed(t, "")
	id := addSprint(t, path, "Alpha")
	for _, n := range []string{"a", "b", "c"} {
		sprintTask(t, path, n, id, todo.New)
	}
	_, err := runCmd(t, "sprint", "close", "--file", path, "--sprint", "Alpha")
	if err == nil || codeOf(t, err) != exitUsage {
		t.Fatalf("err = %v", err)
	}
	if want := `cannot close "Alpha": 3 tasks not done`; err.Error() != want {
		t.Errorf("message = %q, want %q", err.Error(), want)
	}
}

func TestSprintReopen(t *testing.T) {
	path := todotest.Seed(t, "")
	id := addSprint(t, path, "Alpha")
	if _, err := runCmd(t, "sprint", "close", "--file", path, "--sprint", id); err != nil {
		t.Fatal(err)
	}
	if _, err := runCmd(t, "sprint", "reopen", "--file", path, "--sprint", id); err != nil {
		t.Fatal(err)
	}
	doc, _, _ := todo.NewStore(path).Load(context.Background())
	if sp, _ := doc.SprintByID(id); sp.Status != todo.SprintOpen {
		t.Errorf("status = %v", sp.Status)
	}
}

func TestSprintRmReturnsTasks(t *testing.T) {
	path := todotest.Seed(t, "")
	id := addSprint(t, path, "Alpha")
	sprintTask(t, path, "a", id, todo.New)
	if _, err := runCmd(t, "sprint", "rm", "--file", path, "--sprint", "Alpha"); err != nil {
		t.Fatal(err)
	}
	doc, _, _ := todo.NewStore(path).Load(context.Background())
	a := doc.ByTitle("a")[0]
	if len(doc.Sprints) != 0 || a.Sprint != "" || a.Status != todo.Open {
		t.Errorf("sprints %v, task sprint %q status %v", doc.Sprints, a.Sprint, a.Status)
	}
}

func TestSprintRmRefusedWithDone(t *testing.T) {
	path := todotest.Seed(t, "")
	id := addSprint(t, path, "Alpha")
	sprintTask(t, path, "a", id, todo.Done)
	_, err := runCmd(t, "sprint", "rm", "--file", path, "--sprint", "Alpha")
	if err == nil || codeOf(t, err) != exitUsage || err.Error() != todo.ErrSprintHasDone.Error() {
		t.Errorf("err = %v", err)
	}
}

func TestSprintUnknownExit3(t *testing.T) {
	path := todotest.Seed(t, "")
	for _, args := range [][]string{{"edit", "--set-goal", "x"}, {"close"}, {"reopen"}, {"rm"}} {
		sub := args[0]
		_, err := runCmd(t, append([]string{"sprint", args[0], "--file", path, "--sprint", "nope"}, args[1:]...)...)
		if err == nil || codeOf(t, err) != exitNotFound || err.Error() != "no sprint nope" {
			t.Errorf("%s: err = %v", sub, err)
		}
	}
	_, err := runCmd(t, "sprint", "close", "--file", path)
	if err == nil || codeOf(t, err) != exitUsage {
		t.Errorf("missing --sprint: %v", err)
	}
}
