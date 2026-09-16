package cmd

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func runCmd(t *testing.T, args ...string) (string, error) {
	t.Helper()
	c := newRootCommand()
	var buf bytes.Buffer
	c.SetOut(&buf)
	c.SetErr(&buf)
	c.SetArgs(args)
	err := c.Execute()
	return buf.String(), err
}

func TestListTable(t *testing.T) {
	path := writeTemp(t, "# Work\n\n- [ ] a\n- [x] b\n")
	out, err := runCmd(t, "list", "--file", path)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	for _, want := range []string{"Work", "[ ]", "a", "[x]", "b"} {
		if !strings.Contains(out, want) {
			t.Errorf("list output missing %q:\n%s", want, out)
		}
	}
}

func TestListJSON(t *testing.T) {
	path := writeTemp(t, "# Work\n\n- [ ] a\n  - [x] b\n  - [ ] c\n")
	out, err := runCmd(t, "list", "--file", path, "--json")
	if err != nil {
		t.Fatalf("list --json: %v", err)
	}
	var items []jsonItem
	if err := json.Unmarshal([]byte(out), &items); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out)
	}
	if len(items) != 1 || items[0].Kind != "category" || items[0].Title != "Work" {
		t.Fatalf("root should be the Work category: %+v", items)
	}
	a := items[0].Children[0]
	if a.Title != "a" || a.Status != "open" || a.Number != 2 {
		t.Errorf("task a wrong: %+v", a)
	}
	if len(a.Path) != 1 || a.Path[0] != "Work" {
		t.Errorf("a.Path = %v, want [Work]", a.Path)
	}
	if a.DoneCount != 1 || a.TotalCount != 2 {
		t.Errorf("a doneCount/totalCount = %d/%d, want 1/2", a.DoneCount, a.TotalCount)
	}
	b := a.Children[0]
	if b.Title != "b" || b.Status != "done" {
		t.Errorf("subtask b wrong: %+v", b)
	}
	if len(b.Path) != 2 || b.Path[0] != "Work" || b.Path[1] != "a" {
		t.Errorf("b.Path = %v, want [Work a]", b.Path)
	}
}

func TestListFilter(t *testing.T) {
	path := writeTemp(t, "# Work\n\n- [ ] alpha\n- [ ] beta\n")
	out, err := runCmd(t, "list", "--file", path, "--filter", "alpha")
	if err != nil {
		t.Fatalf("list --filter: %v", err)
	}
	if !strings.Contains(out, "alpha") || strings.Contains(out, "beta") {
		t.Errorf("filter should show alpha and hide beta:\n%s", out)
	}
}

func TestListJSONFilter(t *testing.T) {
	path := writeTemp(t, "# Work\n\n- [ ] alpha\n- [ ] beta\n")
	out, err := runCmd(t, "list", "--file", path, "--json", "--filter", "alpha")
	if err != nil {
		t.Fatalf("list --json --filter: %v", err)
	}
	var items []jsonItem
	if err := json.Unmarshal([]byte(out), &items); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out)
	}
	if len(items) != 1 || items[0].Title != "Work" {
		t.Fatalf("root should be the Work category: %+v", items)
	}
	if !containsTitle(items, "alpha") {
		t.Errorf("filtered JSON tree should contain alpha:\n%s", out)
	}
	if containsTitle(items, "beta") {
		t.Errorf("filtered JSON tree should not contain beta:\n%s", out)
	}
}

// containsTitle reports whether title appears anywhere in the JSON tree.
func containsTitle(items []jsonItem, title string) bool {
	for _, it := range items {
		if it.Title == title || containsTitle(it.Children, title) {
			return true
		}
	}
	return false
}
