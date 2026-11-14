package cmd

import (
	"errors"
	"strconv"
	"strings"
	"testing"

	"github.com/candy-tools/todo/internal/todo/todotest"
)

func codeOf(t *testing.T, err error) int {
	t.Helper()
	var ce *cliError
	if !errors.As(err, &ce) {
		t.Fatalf("expected a *cliError, got %v", err)
	}
	return ce.code
}

func TestDoneByTitle(t *testing.T) {
	path := todotest.Seed(t, "# Work\n\n- [ ] a\n- [ ] b\n")
	if _, err := runCmd(t, "done", "--file", path, "--title", "a"); err != nil {
		t.Fatalf("done: %v", err)
	}
	got := todotest.Dump(t, path)
	if !strings.Contains(got, "- [x] a") || !strings.Contains(got, "- [ ] b") {
		t.Errorf("only task a should be done:\n%s", got)
	}
}

func TestDoneByNumberAndID(t *testing.T) {
	path := todotest.Seed(t, "# Work\n\n- [ ] a\n")
	if _, err := runCmd(t, "done", "--file", path, "--number", "2"); err != nil {
		t.Fatalf("done --number: %v", err)
	}
	if !strings.Contains(todotest.Dump(t, path), "- [x] a") {
		t.Error("done --number 2 should complete task a")
	}
	path2 := todotest.Seed(t, "# Work\n\n- [ ] a\n")
	id := strconv.Itoa(int(idOf(t, path2, "a")))
	if _, err := runCmd(t, "done", "--file", path2, "--id", id); err != nil {
		t.Fatalf("done --id: %v", err)
	}
	if !strings.Contains(todotest.Dump(t, path2), "- [x] a") {
		t.Error("done --id should complete task a")
	}
}

func TestDoneSelectorErrors(t *testing.T) {
	path := todotest.Seed(t, "# Work\n\n- [ ] a\n\n# Other\n\n- [ ] a\n")
	if _, err := runCmd(t, "done", "--file", path); codeOf(t, err) != exitUsage {
		t.Error("no selector should be a usage error")
	}
	if _, err := runCmd(t, "done", "--file", path, "--number", "2", "--id", "3"); codeOf(t, err) != exitUsage {
		t.Error("two selectors should be a usage error")
	}
	if _, err := runCmd(t, "done", "--file", path, "--title", "a"); codeOf(t, err) != exitAmbiguous {
		t.Error("duplicate title should be ambiguous")
	}
	if _, err := runCmd(t, "done", "--file", path, "--number", "99"); codeOf(t, err) != exitNotFound {
		t.Error("out-of-range number should be not-found")
	}
	if _, err := runCmd(t, "done", "--file", path, "--title", "Work"); codeOf(t, err) != exitWrongKind {
		t.Error("done on a category should be wrong-kind")
	}
}

func TestDoneCascade(t *testing.T) {
	path := todotest.Seed(t, "# Work\n\n- [ ] parent\n  - [ ] child\n")
	if _, err := runCmd(t, "done", "--file", path, "--title", "parent", "--cascade"); err != nil {
		t.Fatal(err)
	}
	got := todotest.Dump(t, path)
	if !strings.Contains(got, "- [x] parent") || !strings.Contains(got, "- [x] child") {
		t.Errorf("--cascade should complete the subtree:\n%s", got)
	}
}

func TestDoneNoCascadeLeavesChildren(t *testing.T) {
	path := todotest.Seed(t, "# Work\n\n- [ ] parent\n  - [ ] child\n")
	if _, err := runCmd(t, "done", "--file", path, "--title", "parent"); err != nil {
		t.Fatal(err)
	}
	got := todotest.Dump(t, path)
	if !strings.Contains(got, "- [x] parent") || !strings.Contains(got, "  - [ ] child") {
		t.Errorf("target-only done must leave the child open:\n%s", got)
	}
}

func TestProgressAndDefer(t *testing.T) {
	path := todotest.Seed(t, "# Work\n\n- [ ] a\n- [ ] b\n")
	if _, err := runCmd(t, "progress", "--file", path, "--title", "a"); err != nil {
		t.Fatal(err)
	}
	if _, err := runCmd(t, "defer", "--file", path, "--title", "b"); err != nil {
		t.Fatal(err)
	}
	got := todotest.Dump(t, path)
	if !strings.Contains(got, "- [/] a") || !strings.Contains(got, "- [>] b") {
		t.Errorf("want a in-progress and b deferred:\n%s", got)
	}
}

func TestReopen(t *testing.T) {
	path := todotest.Seed(t, "# Work\n\n- [x] a\n")
	if _, err := runCmd(t, "reopen", "--file", path, "--title", "a"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(todotest.Dump(t, path), "- [ ] a") {
		t.Error("reopen should return the task to open")
	}
}

func TestReopenCascade(t *testing.T) {
	path := todotest.Seed(t, "# Work\n\n- [x] parent\n  - [x] child\n")
	if _, err := runCmd(t, "reopen", "--file", path, "--title", "parent", "--cascade"); err != nil {
		t.Fatal(err)
	}
	got := todotest.Dump(t, path)
	if !strings.Contains(got, "- [ ] parent") || !strings.Contains(got, "  - [ ] child") {
		t.Errorf("--cascade reopen should open the subtree:\n%s", got)
	}
}

func TestSelectByID(t *testing.T) {
	path := todotest.Seed(t, "# Work\n\n- [ ] a\n- [ ] b\n")
	id := strconv.Itoa(int(idOf(t, path, "b")))
	if _, err := runCmd(t, "done", "--file", path, "--id", id); err != nil {
		t.Fatalf("done --id: %v", err)
	}
	got := todotest.Dump(t, path)
	if !strings.Contains(got, "- [x] b") || !strings.Contains(got, "- [ ] a") {
		t.Errorf("only b should be done:\n%s", got)
	}
	if _, err := runCmd(t, "done", "--file", path, "--id", "999"); codeOf(t, err) != exitNotFound {
		t.Error("an unknown --id should be not-found")
	}
}
