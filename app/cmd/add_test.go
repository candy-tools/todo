package cmd

import (
	"strings"
	"testing"

	"github.com/candy-tools/todo/internal/todo/todotest"
)

func TestAddTaskUnderCategory(t *testing.T) {
	path := todotest.Seed(t, "# Work\n\n- [ ] existing\n")
	if _, err := runCmd(t, "add", "--file", path, "--title", "fresh", "--parent-title", "Work"); err != nil {
		t.Fatalf("add: %v", err)
	}
	got := todotest.Dump(t, path)
	if !strings.Contains(got, "- [ ] fresh") {
		t.Errorf("new task missing:\n%s", got)
	}
}

func TestAddTaskGoesBeforeSubcategories(t *testing.T) {
	// The new task must land among Work's tasks, before its "## Sub" child, so
	// the file round-trips (a task after a subheader would re-parent on reload).
	path := todotest.Seed(t, "# Work\n\n- [ ] existing\n\n## Sub\n\n- [ ] s1\n")
	if _, err := runCmd(t, "add", "--file", path, "--title", "fresh", "--parent-title", "Work"); err != nil {
		t.Fatal(err)
	}
	got := todotest.Dump(t, path)
	if strings.Index(got, "- [ ] fresh") > strings.Index(got, "## Sub") {
		t.Errorf("new task must precede the subcategory:\n%s", got)
	}
}

func TestAddSubtaskUnderTask(t *testing.T) {
	path := todotest.Seed(t, "# Work\n\n- [ ] parent\n")
	if _, err := runCmd(t, "add", "--file", path, "--title", "child", "--parent-title", "parent"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(todotest.Dump(t, path), "  - [ ] child") {
		t.Errorf("child should be nested under parent:\n%s", todotest.Dump(t, path))
	}
}

func TestAddWithStatusAndDesc(t *testing.T) {
	path := todotest.Seed(t, "# Work\n\n- [ ] a\n")
	if _, err := runCmd(t, "add", "--file", path, "--title", "b", "--parent-title", "Work", "--status", "progress", "--desc", "note"); err != nil {
		t.Fatal(err)
	}
	got := todotest.Dump(t, path)
	if !strings.Contains(got, "- [/] b") || !strings.Contains(got, "note") {
		t.Errorf("want in-progress b with a description:\n%s", got)
	}
}

func TestAddErrors(t *testing.T) {
	path := todotest.Seed(t, "# Work\n\n- [ ] a\n")
	if _, err := runCmd(t, "add", "--file", path, "--parent-title", "Work"); codeOf(t, err) != exitUsage {
		t.Error("missing --title should be a usage error")
	}
	if _, err := runCmd(t, "add", "--file", path, "--title", "x", "--parent-title", "Work", "--status", "bogus"); codeOf(t, err) != exitUsage {
		t.Error("bad --status should be a usage error")
	}
	if _, err := runCmd(t, "add", "--file", path, "--title", "x"); codeOf(t, err) != exitUsage {
		t.Error("missing parent selector should be a usage error")
	}
}

func TestAddCategory(t *testing.T) {
	path := todotest.Seed(t, "# Work\n\n- [ ] a\n")
	if _, err := runCmd(t, "add-category", "--file", path, "--title", "Personal"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(todotest.Dump(t, path), "# Personal") {
		t.Errorf("top-level category missing:\n%s", todotest.Dump(t, path))
	}
}

func TestAddNestedCategory(t *testing.T) {
	path := todotest.Seed(t, "# Work\n\n- [ ] a\n")
	if _, err := runCmd(t, "add-category", "--file", path, "--title", "Backend", "--parent-title", "Work"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(todotest.Dump(t, path), "## Backend") {
		t.Errorf("nested category should render as an H2:\n%s", todotest.Dump(t, path))
	}
}

func TestAddUnderSubtaskIsUsageError(t *testing.T) {
	path := todotest.Seed(t, "# Work\n\n- [ ] parent\n  - [ ] child\n")
	_, err := runCmd(t, "add", "--file", path, "--title", "x", "--parent-title", "child")
	if codeOf(t, err) != exitUsage {
		t.Fatalf("add under a subtask should be a usage error, got %v", err)
	}
	if !strings.Contains(err.Error(), "subtasks cannot have subtasks") {
		t.Errorf("unclear message %q", err)
	}
	if strings.Contains(todotest.Dump(t, path), "x") {
		t.Error("nothing should have been added")
	}
}

func TestAddBoardStateUnderTaskIsUsageError(t *testing.T) {
	path := todotest.Seed(t, "# Work\n\n- [ ] parent\n")
	_, err := runCmd(t, "add", "--file", path, "--title", "x", "--parent-title", "parent", "--status", "working")
	if codeOf(t, err) != exitUsage {
		t.Fatalf("a working subtask should be a usage error, got %v", err)
	}
	if !strings.Contains(err.Error(), "subtask can only be open or done") {
		t.Errorf("unclear message %q", err)
	}
}

func TestAddWithBoardState(t *testing.T) {
	path := todotest.Seed(t, "# Work\n\n- [ ] a\n")
	if _, err := runCmd(t, "add", "--file", path, "--title", "b", "--parent-title", "Work", "--status", "ready"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(todotest.Dump(t, path), "- [ready] b") {
		t.Errorf("want ready b:\n%s", todotest.Dump(t, path))
	}
}

func TestAddWithPlan(t *testing.T) {
	path := todotest.Seed(t, "# Work\n\n- [ ] a\n")
	if _, err := runCmd(t, "add", "--file", path, "--title", "b", "--parent-title", "Work", "--plan", "docs/p.md"); err != nil {
		t.Fatal(err)
	}
	out, err := runCmd(t, "list", "--file", path, "--json")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, `"plan": "docs/p.md"`) {
		t.Errorf("plan missing from JSON:\n%s", out)
	}
}
