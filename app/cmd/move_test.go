package cmd

import (
	"context"
	"strings"
	"testing"

	"github.com/candy-tools/todo/internal/todo"
	"github.com/candy-tools/todo/internal/todo/todotest"
)

// childTitles returns the titles of the children of the item titled title
// (the roots for "") in the list file at path.
func childTitles(t *testing.T, path, title string) []string {
	t.Helper()
	doc, _, err := todo.NewStore(path).Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	items := doc.Roots
	if title != "" {
		items = doc.ByTitle(title)[0].Children
	}
	var out []string
	for _, it := range items {
		out = append(out, it.Title)
	}
	return out
}

func wantChildren(t *testing.T, path, title string, want ...string) {
	t.Helper()
	if got := childTitles(t, path, title); strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("%q's children = %v, want %v", title, got, want)
	}
}

const moveDoc = "# Work\n\n- [ ] a\n- [ ] b\n\n## Sub\n\n- [ ] s1\n  - [ ] st\n\n# Other\n\n- [ ] o1\n- [ ] o2\n"

func TestMoveTaskToCategory(t *testing.T) {
	path := todotest.Seed(t, moveDoc)
	out, err := runCmd(t, "move", "--file", path, "--title", "a", "--parent-title", "Other")
	if err != nil {
		t.Fatalf("move: %v", err)
	}
	if !strings.Contains(out, "move: a  (Other)") {
		t.Errorf("confirmation = %q", out)
	}
	wantChildren(t, path, "Other", "o1", "o2", "a")
	wantChildren(t, path, "Work", "Sub", "b")
}

func TestMoveFirstAndAfter(t *testing.T) {
	path := todotest.Seed(t, moveDoc)
	if _, err := runCmd(t, "move", "--file", path, "--title", "a", "--parent-title", "Other", "--first"); err != nil {
		t.Fatal(err)
	}
	wantChildren(t, path, "Other", "a", "o1", "o2")
	if _, err := runCmd(t, "move", "--file", path, "--title", "b", "--parent-title", "Other", "--after-title", "o1"); err != nil {
		t.Fatal(err)
	}
	wantChildren(t, path, "Other", "a", "o1", "b", "o2")
}

func TestMoveCategoryToRoot(t *testing.T) {
	path := todotest.Seed(t, moveDoc)
	out, err := runCmd(t, "move", "--file", path, "--title", "Sub", "--root")
	if err != nil {
		t.Fatalf("move: %v", err)
	}
	if !strings.Contains(out, "(top level)") {
		t.Errorf("confirmation = %q", out)
	}
	wantChildren(t, path, "", "Work", "Other", "Sub")
}

// A subtask moved to a category becomes a task there, taking its status from
// the move rules (an open subtask of an open task stays open).
func TestMoveSubtaskOut(t *testing.T) {
	path := todotest.Seed(t, moveDoc)
	if _, err := runCmd(t, "move", "--file", path, "--title", "st", "--parent-title", "Other"); err != nil {
		t.Fatal(err)
	}
	wantChildren(t, path, "Other", "o1", "o2", "st")
	wantChildren(t, path, "s1")
}

func TestMoveFlagErrors(t *testing.T) {
	for name, args := range map[string][]string{
		"no parent":        {"--title", "a"},
		"two parents":      {"--title", "a", "--parent-title", "Other", "--root"},
		"first and after":  {"--title", "a", "--parent-title", "Other", "--first", "--after-title", "o1"},
		"two after flags":  {"--title", "a", "--parent-title", "Other", "--after-title", "o1", "--after-number", "2"},
		"no item selector": {"--parent-title", "Other"},
	} {
		path := todotest.Seed(t, moveDoc)
		_, err := runCmd(t, append([]string{"move", "--file", path}, args...)...)
		if codeOf(t, err) != exitUsage {
			t.Errorf("%s: err = %v, want a usage error", name, err)
		}
	}
}

// An --after item that is not a child of the target parent is a usage error
// naming both, not the conflict a stale read would be.
func TestMoveAfterNotChildIsUsage(t *testing.T) {
	path := todotest.Seed(t, moveDoc)
	_, err := runCmd(t, "move", "--file", path, "--title", "a", "--parent-title", "Other", "--after-title", "b")
	if codeOf(t, err) != exitUsage || !strings.Contains(err.Error(), `"b"`) || !strings.Contains(err.Error(), `"Other"`) {
		t.Errorf("err = %v, want a usage error naming b and Other", err)
	}
}

func TestMoveWrongKind(t *testing.T) {
	path := todotest.Seed(t, moveDoc)
	if _, err := runCmd(t, "move", "--file", path, "--title", "a", "--root"); codeOf(t, err) != exitWrongKind {
		t.Errorf("task to the root: err = %v, want wrong kind", err)
	}
	if _, err := runCmd(t, "move", "--file", path, "--title", "Sub", "--parent-title", "a"); codeOf(t, err) != exitWrongKind {
		t.Errorf("category under a task: err = %v, want wrong kind", err)
	}
}

func TestMoveByRef(t *testing.T) {
	path := todotest.Seed(t, moveDoc)
	if _, err := runCmd(t, "edit", "--file", path, "--title", "a", "--set-ref", "APP-001"); err != nil {
		t.Fatal(err)
	}
	out, err := runCmd(t, "move", "--file", path, "--id", "app-001", "--parent-title", "Other")
	if err != nil {
		t.Fatalf("move by ref: %v", err)
	}
	if !strings.Contains(out, "move: [APP-001] a  (Other)") {
		t.Errorf("confirmation = %q", out)
	}
	wantChildren(t, path, "Other", "o1", "o2", "a")
}

func TestMoveRefTaskUnderTask(t *testing.T) {
	path := todotest.Seed(t, moveDoc)
	if _, err := runCmd(t, "edit", "--file", path, "--title", "a", "--set-ref", "APP-001"); err != nil {
		t.Fatal(err)
	}
	_, err := runCmd(t, "move", "--file", path, "--title", "a", "--parent-title", "b")
	if codeOf(t, err) != exitUsage {
		t.Errorf("err = %v, want a usage error", err)
	}
}

// An --after sibling the item cannot follow is a usage error, not the
// conflict a stale read would be: itself, or a task for a category.
func TestMoveAfterUnfitSiblingIsUsage(t *testing.T) {
	for name, args := range map[string][]string{
		"after itself":          {"--title", "a", "--parent-title", "Work", "--after-title", "a"},
		"category after a task": {"--title", "Sub", "--parent-title", "Work", "--after-title", "a"},
	} {
		path := todotest.Seed(t, moveDoc)
		_, err := runCmd(t, append([]string{"move", "--file", path}, args...)...)
		if codeOf(t, err) != exitUsage {
			t.Errorf("%s: err = %v, want a usage error", name, err)
		}
	}
}
