package cmd

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/candy-tools/todo/internal/todo"
	"github.com/candy-tools/todo/internal/todo/todotest"
)

// refOf returns the reference of the item titled title in the list file at
// path.
func refOf(t *testing.T, path, title string) string {
	t.Helper()
	doc, _, err := todo.NewStore(path).Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return doc.ByTitle(title)[0].Ref
}

func TestAddWithRef(t *testing.T) {
	path := todotest.Seed(t, "# Work\n")
	out, err := runCmd(t, "add", "--file", path, "--title", "ship", "--ref", "App-001", "--parent-title", "Work")
	if err != nil {
		t.Fatalf("add: %v", err)
	}
	if !strings.Contains(out, "[App-001] ship") {
		t.Errorf("confirmation = %q, want the ref before the title", out)
	}
	if got := refOf(t, path, "ship"); got != "App-001" {
		t.Errorf("ref = %q", got)
	}
	// A second task with the same reference, in another case, is refused.
	_, err = runCmd(t, "add", "--file", path, "--title", "other", "--ref", "APP-001", "--parent-title", "Work")
	if codeOf(t, err) != exitUsage || !strings.Contains(err.Error(), "duplicate reference") {
		t.Errorf("duplicate ref: err = %v, want a usage error naming the duplicate", err)
	}
	// A subtask cannot have one.
	_, err = runCmd(t, "add", "--file", path, "--title", "sub", "--ref", "APP-002", "--parent-title", "ship")
	if codeOf(t, err) != exitUsage {
		t.Errorf("subtask ref: err = %v, want a usage error", err)
	}
}

func TestEditSetRef(t *testing.T) {
	path := todotest.Seed(t, "# Work\n\n- [ ] a\n")
	out, err := runCmd(t, "edit", "--file", path, "--title", "a", "--set-ref", "APP-7")
	if err != nil {
		t.Fatalf("edit: %v", err)
	}
	if !strings.Contains(out, "edit: [APP-7] a") {
		t.Errorf("confirmation = %q", out)
	}
	if _, err := runCmd(t, "edit", "--file", path, "--title", "a", "--set-ref", ""); err != nil {
		t.Fatalf("clear: %v", err)
	}
	if got := refOf(t, path, "a"); got != "" {
		t.Errorf("ref after clearing = %q", got)
	}
	_, err = runCmd(t, "edit", "--file", path, "--title", "Work", "--set-ref", "X")
	if codeOf(t, err) != exitWrongKind {
		t.Errorf("category ref: err = %v, want the wrong-kind exit", err)
	}
}

// --id takes a task's reference, ignoring case, wherever it takes an ID.
func TestIDSelectorTakesRef(t *testing.T) {
	path := todotest.Seed(t, "# Work\n\n- [ ] a\n")
	if _, err := runCmd(t, "edit", "--file", path, "--title", "a", "--set-ref", "APP-001"); err != nil {
		t.Fatal(err)
	}
	out, err := runCmd(t, "status", "--file", path, "--id", "app-001", "--to", "review")
	if err != nil {
		t.Fatalf("status by ref: %v", err)
	}
	if !strings.Contains(out, "status: [review] [APP-001] a") {
		t.Errorf("confirmation = %q", out)
	}
	if _, err := runCmd(t, "add", "--file", path, "--title", "sub", "--parent-id", "APP-001"); err != nil {
		t.Errorf("--parent-id by ref: %v", err)
	}
	_, err = runCmd(t, "done", "--file", path, "--id", "APP-404")
	if codeOf(t, err) != exitNotFound {
		t.Errorf("unknown ref: err = %v, want not found", err)
	}
}

func TestListShowsRef(t *testing.T) {
	path := todotest.Seed(t, "# Work\n\n- [ ] a\n")
	if _, err := runCmd(t, "edit", "--file", path, "--title", "a", "--set-ref", "APP-001"); err != nil {
		t.Fatal(err)
	}
	out, err := runCmd(t, "list", "--file", path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "[APP-001] a") {
		t.Errorf("table = %q, want the ref before the title", out)
	}
	if out, err = runCmd(t, "list", "--file", path, "--json"); err != nil {
		t.Fatal(err)
	}
	var roots []jsonItem
	if err := json.Unmarshal([]byte(out), &roots); err != nil {
		t.Fatal(err)
	}
	if got := roots[0].Children[0]; got.Ref != "APP-001" || got.Title != "a" {
		t.Errorf("json task = ref %q title %q, want ref APP-001 and the bare title", got.Ref, got.Title)
	}
	if out, err = runCmd(t, "list", "--file", path, "--filter", "app-0"); err != nil || !strings.Contains(out, "[APP-001] a") {
		t.Errorf("filter by ref = %q, %v", out, err)
	}
}
