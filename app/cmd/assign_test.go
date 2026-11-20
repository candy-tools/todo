package cmd

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/candy-tools/todo/internal/todo"
	"github.com/candy-tools/todo/internal/todo/todotest"
)

// taskNamed loads the list at path and returns the task titled title.
func taskNamed(t *testing.T, path, title string) *todo.Item {
	t.Helper()
	doc, _, err := todo.NewStore(path).Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	m := doc.ByTitle(title)
	if len(m) != 1 {
		t.Fatalf("want one item titled %q, got %d", title, len(m))
	}
	return m[0]
}

func TestAssignBacklogTaskNew(t *testing.T) {
	path := todotest.Seed(t, "# Work\n\n- [ ] a\n")
	id := addSprint(t, path, "Alpha")
	out, err := runCmd(t, "assign", "--file", path, "--sprint", "Alpha", "--title", "a")
	if err != nil {
		t.Fatalf("assign: %v", err)
	}
	if got := strings.TrimSpace(out); got != "assign: [new] a  (Alpha)" {
		t.Errorf("confirmation %q", got)
	}
	it := taskNamed(t, path, "a")
	if it.Sprint != id || it.Status != todo.New {
		t.Errorf("sprint %q status %v", it.Sprint, it.Status)
	}
}

func TestAssignAcrossSprintsKeepsState(t *testing.T) {
	path := todotest.Seed(t, "")
	a := addSprint(t, path, "Alpha")
	b := addSprint(t, path, "Beta")
	sprintTask(t, path, "t", a, todo.Working)
	if _, err := runCmd(t, "assign", "--file", path, "--sprint", b, "--title", "t"); err != nil {
		t.Fatalf("assign: %v", err)
	}
	it := taskNamed(t, path, "t")
	if it.Sprint != b || it.Status != todo.Working {
		t.Errorf("sprint %q status %v", it.Sprint, it.Status)
	}
}

func TestAssignNeedsKnownSprint(t *testing.T) {
	path := todotest.Seed(t, "# Work\n\n- [ ] a\n")
	if _, err := runCmd(t, "assign", "--file", path, "--title", "a"); codeOf(t, err) != exitUsage {
		t.Errorf("no --sprint: %v", err)
	}
	if _, err := runCmd(t, "assign", "--file", path, "--sprint", "nope", "--title", "a"); codeOf(t, err) != exitNotFound {
		t.Errorf("unknown sprint: %v", err)
	}
}

func TestUnassignOpen(t *testing.T) {
	path := todotest.Seed(t, "")
	a := addSprint(t, path, "Alpha")
	sprintTask(t, path, "t", a, todo.Ready)
	out, err := runCmd(t, "unassign", "--file", path, "--title", "t")
	if err != nil {
		t.Fatalf("unassign: %v", err)
	}
	if got := strings.TrimSpace(out); got != "unassign: [open] t" {
		t.Errorf("confirmation %q", got)
	}
	it := taskNamed(t, path, "t")
	if it.Sprint != "" || it.Status != todo.Open {
		t.Errorf("sprint %q status %v", it.Sprint, it.Status)
	}
}

func TestAddWithSprintDefaultsNew(t *testing.T) {
	path := todotest.Seed(t, "# Work\n")
	id := addSprint(t, path, "Alpha")
	out, err := runCmd(t, "add", "--file", path, "--title", "x", "--parent-title", "Work", "--sprint", "alpha")
	if err != nil {
		t.Fatalf("add: %v", err)
	}
	if !strings.Contains(out, "[new]") {
		t.Errorf("confirmation %q", out)
	}
	it := taskNamed(t, path, "x")
	if it.Sprint != id || it.Status != todo.New {
		t.Errorf("sprint %q status %v", it.Sprint, it.Status)
	}
	if _, err := runCmd(t, "add", "--file", path, "--title", "y", "--parent-title", "Work", "--sprint", "alpha", "--status", "ready"); err != nil {
		t.Fatalf("add ready: %v", err)
	}
	if taskNamed(t, path, "y").Status != todo.Ready {
		t.Error("an explicit --status is kept")
	}
}

func TestAddWithSprintOnSubtaskIsUsageError(t *testing.T) {
	path := todotest.Seed(t, "# Work\n\n- [ ] parent\n")
	addSprint(t, path, "Alpha")
	_, err := runCmd(t, "add", "--file", path, "--title", "x", "--parent-title", "parent", "--sprint", "Alpha")
	if codeOf(t, err) != exitUsage || !strings.Contains(err.Error(), "--sprint applies to top-level tasks") {
		t.Fatalf("got %v", err)
	}
	if strings.Contains(todotest.Dump(t, path), "x") {
		t.Error("nothing should have been added")
	}
}

func TestAddSprintStateNeedsSprintFlag(t *testing.T) {
	path := todotest.Seed(t, "# Work\n")
	addSprint(t, path, "Alpha")
	before := bytesOf(t, path)
	_, err := runCmd(t, "add", "--file", path, "--title", "x", "--parent-title", "Work", "--status", "working")
	if codeOf(t, err) != exitUsage || err.Error() != "--status working needs --sprint <id|name>" {
		t.Fatalf("--status working without --sprint: %v", err)
	}
	if string(bytesOf(t, path)) != string(before) {
		t.Error("the list must be left unchanged")
	}
}

func TestAddSprintWithBacklogStateIsUsageError(t *testing.T) {
	path := todotest.Seed(t, "# Work\n")
	addSprint(t, path, "Alpha")
	before := bytesOf(t, path)
	for _, st := range []string{"open", "deferred", "wontdo"} {
		_, err := runCmd(t, "add", "--file", path, "--title", "x", "--parent-title", "Work", "--sprint", "Alpha", "--status", st)
		if want := "--sprint needs a sprint state (new|ready|working|review|followup|done)"; codeOf(t, err) != exitUsage || err.Error() != want {
			t.Errorf("--sprint with --status %s: %v, want %q", st, err, want)
		}
	}
	if string(bytesOf(t, path)) != string(before) {
		t.Error("the list must be left unchanged")
	}
}

func TestListSprintWithBacklogColumnIsUsageError(t *testing.T) {
	path, _ := sprintListFixture(t)
	if _, err := runCmd(t, "list", "--file", path, "--sprint", "Alpha", "--column", "backlog"); codeOf(t, err) != exitUsage {
		t.Errorf("--sprint with --column backlog: %v", err)
	}
}

// sprintListFixture has a backlog task and one task per sprint column.
func sprintListFixture(t *testing.T) (path, id string) {
	t.Helper()
	path = todotest.Seed(t, "# Work\n\n- [ ] backlogged\n")
	id = addSprint(t, path, "Alpha")
	other := addSprint(t, path, "Beta")
	sprintTask(t, path, "nexty", id, todo.New)
	sprintTask(t, path, "busy", id, todo.Working)
	sprintTask(t, path, "finished", id, todo.Done)
	sprintTask(t, path, "elsewhere", other, todo.New)
	return path, id
}

func TestListSprintScope(t *testing.T) {
	path, _ := sprintListFixture(t)
	out, err := runCmd(t, "list", "--file", path, "--sprint", "Alpha")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"nexty", "busy", "finished"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q:\n%s", want, out)
		}
	}
	for _, not := range []string{"backlogged", "elsewhere"} {
		if strings.Contains(out, not) {
			t.Errorf("unexpected %q:\n%s", not, out)
		}
	}
	out, err = runCmd(t, "list", "--file", path, "--sprint", "Alpha", "--column", "done")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "finished") || strings.Contains(out, "busy") {
		t.Errorf("--column within a sprint:\n%s", out)
	}
	if _, err := runCmd(t, "list", "--file", path, "--sprint", "nope"); codeOf(t, err) != exitNotFound {
		t.Errorf("unknown sprint: %v", err)
	}
}

func TestListBacklogFlag(t *testing.T) {
	path, _ := sprintListFixture(t)
	out, err := runCmd(t, "list", "--file", path, "--backlog")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "backlogged") || strings.Contains(out, "nexty") {
		t.Errorf("--backlog:\n%s", out)
	}
	if _, err := runCmd(t, "list", "--file", path, "--backlog", "--sprint", "Alpha"); codeOf(t, err) != exitUsage {
		t.Errorf("--backlog with --sprint: %v", err)
	}
	if _, err := runCmd(t, "list", "--file", path, "--backlog", "--column", "done"); codeOf(t, err) != exitUsage {
		t.Errorf("--backlog with --column: %v", err)
	}
}

func TestListColumnWithoutSprintSpansSprints(t *testing.T) {
	path, _ := sprintListFixture(t)
	out, err := runCmd(t, "list", "--file", path, "--column", "next")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "nexty") || !strings.Contains(out, "elsewhere") || strings.Contains(out, "busy") {
		t.Errorf("--column next:\n%s", out)
	}
}

func TestListJSONHasSprint(t *testing.T) {
	path, id := sprintListFixture(t)
	sprintTask(t, path, "parent", id, todo.New)
	if _, err := runCmd(t, "add", "--file", path, "--title", "kid", "--parent-title", "parent"); err != nil {
		t.Fatal(err)
	}
	out, err := runCmd(t, "list", "--file", path, "--json")
	if err != nil {
		t.Fatal(err)
	}
	var roots []jsonItem
	if err := json.Unmarshal([]byte(out), &roots); err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	var walk func(items []jsonItem)
	walk = func(items []jsonItem) {
		for _, it := range items {
			got[it.Title] = it.Sprint
			walk(it.Children)
		}
	}
	walk(roots)
	if got["nexty"] != id || got["kid"] != id || got["backlogged"] != "" {
		t.Errorf("sprints %v", got)
	}
}

func TestPruneNeedsScope(t *testing.T) {
	path := todotest.Seed(t, "# Work\n\n- [x] gone\n")
	_, err := runCmd(t, "prune", "--file", path)
	if codeOf(t, err) != exitUsage || !strings.Contains(err.Error(), "specify --sprint <id|name> or --all") {
		t.Fatalf("got %v", err)
	}
	addSprint(t, path, "Alpha")
	if _, err := runCmd(t, "prune", "--file", path, "--all", "--sprint", "Alpha"); codeOf(t, err) != exitUsage {
		t.Errorf("--all with --sprint: %v", err)
	}
	if !strings.Contains(todotest.Dump(t, path), "gone") {
		t.Error("nothing should have been pruned")
	}
}

func TestPruneSprintOnly(t *testing.T) {
	path := todotest.Seed(t, "# Work\n\n- [x] backlog done\n")
	a := addSprint(t, path, "Alpha")
	b := addSprint(t, path, "Beta")
	sprintTask(t, path, "a done", a, todo.Done)
	sprintTask(t, path, "a open", a, todo.New)
	sprintTask(t, path, "b done", b, todo.Done)
	out, err := runCmd(t, "prune", "--file", path, "--sprint", "Alpha")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "1") {
		t.Errorf("count %q", out)
	}
	got := todotest.Dump(t, path)
	if strings.Contains(got, "a done") || !strings.Contains(got, "a open") || !strings.Contains(got, "b done") || !strings.Contains(got, "backlog done") {
		t.Errorf("only Alpha's done task goes:\n%s", got)
	}
	if _, err := runCmd(t, "prune", "--file", path, "--all"); err != nil {
		t.Fatal(err)
	}
	if got := todotest.Dump(t, path); strings.Contains(got, "b done") || strings.Contains(got, "backlog done") {
		t.Errorf("--all prunes everything done:\n%s", got)
	}
}

func TestStatusWorkingOnBacklogTaskExit2(t *testing.T) {
	path := todotest.Seed(t, "# Work\n\n- [ ] a\n")
	_, err := runCmd(t, "status", "--file", path, "--to", "working", "--title", "a")
	if codeOf(t, err) != exitUsage || !strings.Contains(err.Error(), "task is not in a sprint (use todo assign)") {
		t.Fatalf("got %v", err)
	}
}
