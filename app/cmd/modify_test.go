package cmd

import (
	"bytes"
	"strings"
	"testing"

	"github.com/candy-tools/todo/internal/todo"
	"github.com/candy-tools/todo/internal/todo/todotest"
)

func TestRmRemovesSubtree(t *testing.T) {
	path := todotest.Seed(t, "# Work\n\n- [ ] parent\n  - [ ] child\n- [ ] keep\n")
	if _, err := runCmd(t, "rm", "--file", path, "--title", "parent"); err != nil {
		t.Fatalf("rm: %v", err)
	}
	got := todotest.Dump(t, path)
	if strings.Contains(got, "parent") || strings.Contains(got, "child") {
		t.Errorf("parent and its child should be gone:\n%s", got)
	}
	if !strings.Contains(got, "- [ ] keep") {
		t.Errorf("sibling should remain:\n%s", got)
	}
}

func TestPruneRemovesCompleted(t *testing.T) {
	path := todotest.Seed(t, "# Work\n\n- [x] done1\n- [ ] open1\n- [x] done2\n")
	out, err := runCmd(t, "prune", "--file", path)
	if err != nil {
		t.Fatalf("prune: %v", err)
	}
	got := todotest.Dump(t, path)
	if strings.Contains(got, "done1") || strings.Contains(got, "done2") {
		t.Errorf("completed tasks should be pruned:\n%s", got)
	}
	if !strings.Contains(got, "- [ ] open1") {
		t.Errorf("open task should remain:\n%s", got)
	}
	if !strings.Contains(out, "2") {
		t.Errorf("prune should report the count removed, got %q", out)
	}
}

func TestEditTitleAndDesc(t *testing.T) {
	path := todotest.Seed(t, "# Work\n\n- [ ] old\n")
	if _, err := runCmd(t, "edit", "--file", path, "--title", "old", "--set-title", "new", "--set-desc", "why"); err != nil {
		t.Fatalf("edit: %v", err)
	}
	got := todotest.Dump(t, path)
	if !strings.Contains(got, "- [ ] new") || strings.Contains(got, "old") {
		t.Errorf("title should be renamed:\n%s", got)
	}
	if !strings.Contains(got, "why") {
		t.Errorf("description should be set:\n%s", got)
	}
}

func TestEditRequiresAChange(t *testing.T) {
	path := todotest.Seed(t, "# Work\n\n- [ ] a\n")
	if _, err := runCmd(t, "edit", "--file", path, "--title", "a"); codeOf(t, err) != exitUsage {
		t.Error("edit with no --set-* should be a usage error")
	}
}

func TestEditSetPlan(t *testing.T) {
	path := todotest.Seed(t, "# Work\n\n- [ ] a\n")
	if _, err := runCmd(t, "edit", "--file", path, "--title", "a", "--set-plan", "x.md"); err != nil {
		t.Fatal(err)
	}
	out, _ := runCmd(t, "list", "--file", path, "--json")
	if !strings.Contains(out, `"plan": "x.md"`) {
		t.Errorf("plan should be set:\n%s", out)
	}
	if _, err := runCmd(t, "edit", "--file", path, "--title", "a", "--set-plan", ""); err != nil {
		t.Fatal(err)
	}
	out, _ = runCmd(t, "list", "--file", path, "--json")
	if strings.Contains(out, `"plan"`) {
		t.Errorf("an empty --set-plan should clear the plan:\n%s", out)
	}
}

func TestEditTitleKeepsPlan(t *testing.T) {
	path := todotest.Seed(t, "# Work\n\n- [ ] a\n")
	if _, err := runCmd(t, "edit", "--file", path, "--title", "a", "--set-plan", "x.md"); err != nil {
		t.Fatal(err)
	}
	if _, err := runCmd(t, "edit", "--file", path, "--title", "a", "--set-title", "b"); err != nil {
		t.Fatal(err)
	}
	out, _ := runCmd(t, "list", "--file", path, "--json")
	if !strings.Contains(out, `"plan": "x.md"`) {
		t.Errorf("retitling must keep the plan:\n%s", out)
	}
}

func TestEditBlankTitleIsUsageError(t *testing.T) {
	path := todotest.Seed(t, "# Work\n\n- [ ] a\n")
	for _, title := range []string{"", "   "} {
		if _, err := runCmd(t, "edit", "--file", path, "--title", "a", "--set-title", title); codeOf(t, err) != exitUsage {
			t.Errorf("edit --set-title %q: want a usage error, got %v", title, err)
		}
	}
	if got := todotest.Dump(t, path); got != "# Work\n\n- [ ] a\n" {
		t.Errorf("nothing should have changed:\n%s", got)
	}
}

func TestEditInvalidTitleIsUsageError(t *testing.T) {
	path := todotest.Seed(t, "# Work\n\n- [ ] a\n")
	for _, title := range []string{"a\nb", "\x1b]0;pwned\x07", strings.Repeat("x", 201)} {
		if _, err := runCmd(t, "edit", "--file", path, "--title", "a", "--set-title", title); codeOf(t, err) != exitUsage {
			t.Errorf("edit --set-title %q: want a usage error, got %v", title, err)
		}
	}
	if got := todotest.Dump(t, path); got != "# Work\n\n- [ ] a\n" {
		t.Errorf("nothing should have changed:\n%s", got)
	}
}

// The store decides that only a task has a description or a plan: edit gives
// either to a category with the wrong-kind exit, naming the item, and writes
// nothing, also when a new title comes along.
func TestEditDescOrPlanOnCategoryIsWrongKind(t *testing.T) {
	path := todotest.Seed(t, "# Work\n\n- [ ] a\n")
	before := bytesOf(t, path)
	for _, args := range [][]string{{"--set-desc", "x"}, {"--set-plan", "x"}, {"--set-title", "W2", "--set-plan", "x"}} {
		_, err := runCmd(t, append([]string{"edit", "--file", path, "--title", "Work"}, args...)...)
		if codeOf(t, err) != exitWrongKind {
			t.Errorf("edit %v on a category = %v, want wrong-kind", args, err)
			continue
		}
		if want := `cannot edit "Work": ` + todo.ErrNotTask.Error(); err.Error() != want {
			t.Errorf("edit %v: message = %q, want %q", args, err, want)
		}
	}
	if !bytes.Equal(bytesOf(t, path), before) {
		t.Error("nothing should have been written")
	}
}
