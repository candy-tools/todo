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

func TestStatusTo(t *testing.T) {
	path := todotest.Seed(t, "# Work\n\n- [ ] a\n")
	if _, err := runCmd(t, "status", "--file", path, "--title", "a", "--to", "ready"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(todotest.Dump(t, path), "- [ready] a") {
		t.Errorf("a should be ready:\n%s", todotest.Dump(t, path))
	}
}

func TestStatusToConfirmationLine(t *testing.T) {
	path := todotest.Seed(t, "# Work\n\n- [ ] a\n")
	out, err := runCmd(t, "status", "--file", path, "--title", "a", "--to", "review")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "status: [review] a  (Work)") {
		t.Errorf("unexpected confirmation: %q", out)
	}
}

func TestStatusToInvalid(t *testing.T) {
	path := todotest.Seed(t, "# Work\n\n- [ ] a\n")
	_, err := runCmd(t, "status", "--file", path, "--title", "a", "--to", "bogus")
	if codeOf(t, err) != exitUsage {
		t.Errorf("bad state should be a usage error, got %v", err)
	}
	if !strings.Contains(err.Error(), "followup") {
		t.Errorf("error should list the states: %v", err)
	}
	if _, err := runCmd(t, "status", "--file", path, "--title", "a"); codeOf(t, err) != exitUsage {
		t.Error("missing --to should be a usage error")
	}
}

func TestStatusToProgressAlias(t *testing.T) {
	path := todotest.Seed(t, "# Work\n\n- [ ] a\n")
	if _, err := runCmd(t, "status", "--file", path, "--title", "a", "--to", "progress"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(todotest.Dump(t, path), "- [/] a") {
		t.Errorf("progress should mean working:\n%s", todotest.Dump(t, path))
	}
}

func TestReopenGoesToWorking(t *testing.T) {
	path := todotest.Seed(t, "# Work\n\n- [x] a\n")
	if _, err := runCmd(t, "reopen", "--file", path, "--title", "a"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(todotest.Dump(t, path), "- [/] a") {
		t.Errorf("reopen should return the task to working:\n%s", todotest.Dump(t, path))
	}
}

func TestReopenCascadeOpensSubtasks(t *testing.T) {
	path := todotest.Seed(t, "# Work\n\n- [x] parent\n  - [x] child\n")
	if _, err := runCmd(t, "reopen", "--file", path, "--title", "parent", "--cascade"); err != nil {
		t.Fatal(err)
	}
	got := todotest.Dump(t, path)
	if !strings.Contains(got, "- [/] parent") || !strings.Contains(got, "  - [ ] child") {
		t.Errorf("want working parent and open child:\n%s", got)
	}
}

func TestReopenSubtaskGoesToOpen(t *testing.T) {
	path := todotest.Seed(t, "# Work\n\n- [ ] parent\n  - [x] child\n")
	if _, err := runCmd(t, "reopen", "--file", path, "--title", "child"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(todotest.Dump(t, path), "  - [ ] child") {
		t.Errorf("a reopened subtask should be open:\n%s", todotest.Dump(t, path))
	}
}

func TestSubtaskBoardStateIsUsageError(t *testing.T) {
	path := todotest.Seed(t, "# Work\n\n- [ ] parent\n  - [ ] child\n")
	for _, args := range [][]string{
		{"progress"}, {"defer"}, {"status", "--to", "review"},
	} {
		full := append(append([]string{}, args...), "--file", path, "--title", "child")
		_, err := runCmd(t, full...)
		if codeOf(t, err) != exitUsage {
			t.Errorf("%v on a subtask should be a usage error, got %v", args, err)
			continue
		}
		if !strings.Contains(err.Error(), "subtask can only be open or done") {
			t.Errorf("%v: unclear message %q", args, err)
		}
	}
	if !strings.Contains(todotest.Dump(t, path), "  - [ ] child") {
		t.Error("the subtask must be unchanged")
	}
}

func TestStatusToDoneDoesNotCascade(t *testing.T) {
	path := todotest.Seed(t, "# Work\n\n- [ ] parent\n  - [ ] child\n")
	if _, err := runCmd(t, "status", "--file", path, "--title", "parent", "--to", "done"); err != nil {
		t.Fatal(err)
	}
	got := todotest.Dump(t, path)
	if !strings.Contains(got, "- [x] parent") || !strings.Contains(got, "  - [ ] child") {
		t.Errorf("status --to done must complete only the task:\n%s", got)
	}
}

func TestReopenWithoutCascadeKeepsSubtasks(t *testing.T) {
	path := todotest.Seed(t, "# Work\n\n- [x] parent\n  - [x] child\n")
	if _, err := runCmd(t, "reopen", "--file", path, "--title", "parent"); err != nil {
		t.Fatal(err)
	}
	got := todotest.Dump(t, path)
	if !strings.Contains(got, "- [/] parent") || !strings.Contains(got, "  - [x] child") {
		t.Errorf("reopen without --cascade must leave the subtask done:\n%s", got)
	}
}

func TestReopenNotDoneTaskGoesToWorking(t *testing.T) {
	path := todotest.Seed(t, "# Work\n\n- [ ] a\n")
	if _, err := runCmd(t, "status", "--file", path, "--title", "a", "--to", "new"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(todotest.Dump(t, path), "- [new] a") {
		t.Fatalf("a should be new:\n%s", todotest.Dump(t, path))
	}
	if _, err := runCmd(t, "reopen", "--file", path, "--title", "a"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(todotest.Dump(t, path), "- [/] a") {
		t.Errorf("reopen on a task that is not done should still set working:\n%s", todotest.Dump(t, path))
	}
}

func TestStatusToOnDoneParentKeepsDoneSubtasks(t *testing.T) {
	path := todotest.Seed(t, "# Work\n\n- [x] parent\n  - [x] child\n")
	out, err := runCmd(t, "status", "--file", path, "--title", "parent", "--to", "review")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "status: [review] parent  (Work)") {
		t.Errorf("unexpected confirmation: %q", out)
	}
	got := todotest.Dump(t, path)
	if !strings.Contains(got, "- [review] parent") || !strings.Contains(got, "  - [x] child") {
		t.Errorf("status --to review on a done parent must leave its subtasks done:\n%s", got)
	}
}

func TestStatusToOnDoneSubtask(t *testing.T) {
	path := todotest.Seed(t, "# Work\n\n- [ ] parent\n  - [x] child\n")
	_, err := runCmd(t, "status", "--file", path, "--title", "child", "--to", "review")
	if codeOf(t, err) != exitUsage {
		t.Errorf("a board state on a done subtask should be a usage error, got %v", err)
	}
	if !strings.Contains(todotest.Dump(t, path), "  - [x] child") {
		t.Fatalf("the subtask must stay done:\n%s", todotest.Dump(t, path))
	}
	if _, err := runCmd(t, "status", "--file", path, "--title", "child", "--to", "open"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(todotest.Dump(t, path), "  - [ ] child") {
		t.Errorf("status --to open should reopen the subtask:\n%s", todotest.Dump(t, path))
	}
}

func TestStatusToDoneOnDoneTaskKeepsSubtasks(t *testing.T) {
	path := todotest.Seed(t, "# Work\n\n- [x] parent\n  - [ ] child\n")
	if _, err := runCmd(t, "status", "--file", path, "--title", "parent", "--to", "done"); err != nil {
		t.Fatal(err)
	}
	got := todotest.Dump(t, path)
	if !strings.Contains(got, "- [x] parent") || !strings.Contains(got, "  - [ ] child") {
		t.Errorf("status --to done on a done task must not touch its subtasks:\n%s", got)
	}
}
