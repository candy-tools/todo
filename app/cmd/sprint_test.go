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

// addSprint creates a sprint through the CLI and returns the ID of the sprint
// whose reference it printed.
func addSprint(t *testing.T, path, name string, extra ...string) string {
	t.Helper()
	return sprintIDOf(t, path, sprintAddRef(t, path, name, extra...))
}

// sprintAddRef creates a sprint through the CLI and returns the reference it
// printed.
func sprintAddRef(t *testing.T, path, name string, extra ...string) string {
	t.Helper()
	args := append([]string{"sprint", "add", "--file", path, "--name", name}, extra...)
	out, err := runCmd(t, args...)
	if err != nil {
		t.Fatalf("sprint add: %v", err)
	}
	return strings.TrimSpace(out)
}

// sprintIDOf returns the ID of the sprint with reference ref in the list at
// path.
func sprintIDOf(t *testing.T, path, ref string) string {
	t.Helper()
	doc, _, err := todo.NewStore(path).Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	m := doc.ResolveSprint(ref)
	if len(m) != 1 || m[0].Ref != ref {
		t.Fatalf("no sprint with reference %q: %v", ref, m)
	}
	return m[0].ID
}

func TestSprintAddPrintsRef(t *testing.T) {
	path := todotest.Seed(t, "")
	ref := sprintAddRef(t, path, "Alpha", "--goal", "ship it")
	doc, _, _ := todo.NewStore(path).Load(context.Background())
	sp, ok := doc.SprintByID(sprintIDOf(t, path, ref))
	if !ok || sp.Name != "Alpha" || sp.Goal != "ship it" || sp.Status != todo.SprintOpen {
		t.Errorf("printed ref %q -> %+v", ref, sp)
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
	m := doc.ResolveSprint("Gamma")
	if len(m) != 1 || m[0].Goal != "new" {
		t.Errorf("sprints named Gamma = %+v", m)
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
	if want := `cannot close "SPRINT-001 Alpha": 3 tasks not done`; err.Error() != want {
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

func TestSprintRmDeletesClosedSprintAndTasks(t *testing.T) {
	path := todotest.Seed(t, "")
	id := addSprint(t, path, "Alpha")
	keep := addSprint(t, path, "Keep")
	sprintTask(t, path, "a", id, todo.Done)
	sprintTask(t, path, "k", keep, todo.New)
	if _, err := runCmd(t, "sprint", "close", "--file", path, "--sprint", "Alpha"); err != nil {
		t.Fatal(err)
	}
	if _, err := runCmd(t, "sprint", "rm", "--file", path, "--sprint", "Alpha"); err != nil {
		t.Fatal(err)
	}
	doc, _, _ := todo.NewStore(path).Load(context.Background())
	if _, ok := doc.SprintByID(id); ok || len(doc.Sprints) != 1 {
		t.Errorf("sprints = %v, want only Keep", doc.Sprints)
	}
	if got := doc.ByTitle("a"); len(got) != 0 {
		t.Errorf("the sprint's task is still there: %v", got)
	}
	if got := doc.ByTitle("k"); len(got) != 1 || got[0].Sprint != keep {
		t.Errorf("the other sprint's task = %v", got)
	}
}

func TestSprintRmRefusedWhileOpen(t *testing.T) {
	path := todotest.Seed(t, "")
	id := addSprint(t, path, "Alpha")
	sprintTask(t, path, "a", id, todo.Done)
	before := bytesOf(t, path)
	_, err := runCmd(t, "sprint", "rm", "--file", path, "--sprint", "Alpha")
	if err == nil || codeOf(t, err) != exitUsage || err.Error() != todo.ErrSprintOpen.Error() {
		t.Errorf("err = %v", err)
	}
	if string(bytesOf(t, path)) != string(before) {
		t.Error("a refused rm must leave the list unchanged")
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

func TestSprintAddRequiresNameExit2(t *testing.T) {
	path := todotest.Seed(t, "")
	_, err := runCmd(t, "sprint", "add", "--file", path)
	if err == nil || codeOf(t, err) != exitUsage {
		t.Errorf("err = %v", err)
	}
}

func TestSprintEditPrintsNewName(t *testing.T) {
	path := todotest.Seed(t, "")
	addSprint(t, path, "Alpha")
	out, err := runCmd(t, "sprint", "edit", "--file", path, "--sprint", "Alpha", "--set-name", "Gamma")
	if err != nil || strings.TrimSpace(out) != "edit: SPRINT-001 Gamma" {
		t.Errorf("out %q err %v", out, err)
	}
}
