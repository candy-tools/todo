package cmd

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/candy-tools/todo/internal/todo/todotest"
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
	path := todotest.Seed(t, "# Work\n\n- [ ] a\n- [x] b\n")
	out, err := runCmd(t, "list", "--file", path)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	for _, want := range []string{"Work", "open", "a", "done", "b"} {
		if !strings.Contains(out, want) {
			t.Errorf("list output missing %q:\n%s", want, out)
		}
	}
}

func TestListJSON(t *testing.T) {
	path := todotest.Seed(t, "# Work\n\n- [ ] a\n  - [x] b\n  - [ ] c\n")
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
	if a.Title != "a" || a.Status != "open" || a.Number != 2 || a.ID == "" {
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
	path := todotest.Seed(t, "# Work\n\n- [ ] alpha\n- [ ] beta\n")
	out, err := runCmd(t, "list", "--file", path, "--filter", "alpha")
	if err != nil {
		t.Fatalf("list --filter: %v", err)
	}
	if !strings.Contains(out, "alpha") || strings.Contains(out, "beta") {
		t.Errorf("filter should show alpha and hide beta:\n%s", out)
	}
}

func TestListJSONFilter(t *testing.T) {
	path := todotest.Seed(t, "# Work\n\n- [ ] alpha\n- [ ] beta\n")
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

// boardFixture builds a store with one task per column state of interest.
func boardFixture(t *testing.T) string {
	t.Helper()
	path := todotest.Seed(t, "# Work\n\n- [ ] backlogged\n- [ ] busy\n  - [ ] sub\n- [ ] finished\n\n# Other\n\n- [ ] elsewhere\n")
	for title, to := range map[string]string{"busy": "working", "finished": "done"} {
		if _, err := runCmd(t, "status", "--file", path, "--title", title, "--to", to); err != nil {
			t.Fatal(err)
		}
	}
	return path
}

func TestListColumnFilter(t *testing.T) {
	path := boardFixture(t)
	out, err := runCmd(t, "list", "--file", path, "--column", "progress")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Work", "busy", "sub"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q:\n%s", want, out)
		}
	}
	for _, not := range []string{"backlogged", "finished", "Other", "elsewhere"} {
		if strings.Contains(out, not) {
			t.Errorf("unexpected %q:\n%s", not, out)
		}
	}
}

func TestListColumnWithFilter(t *testing.T) {
	path := boardFixture(t)
	out, err := runCmd(t, "list", "--file", path, "--column", "backlog", "--filter", "backlogged")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "backlogged") || strings.Contains(out, "elsewhere") {
		t.Errorf("column and filter should intersect:\n%s", out)
	}
}

func TestListColumnInvalid(t *testing.T) {
	path := boardFixture(t)
	_, err := runCmd(t, "list", "--file", path, "--column", "bogus")
	if codeOf(t, err) != exitUsage {
		t.Errorf("bad column should be a usage error, got %v", err)
	}
}

func TestListShowsStateNames(t *testing.T) {
	path := boardFixture(t)
	out, err := runCmd(t, "list", "--file", path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "working") || strings.Contains(out, "[/]") {
		t.Errorf("table should show state names, not markers:\n%s", out)
	}
}

func TestListJSONColumnAndStatus(t *testing.T) {
	path := boardFixture(t)
	out, err := runCmd(t, "list", "--file", path, "--json")
	if err != nil {
		t.Fatal(err)
	}
	var items []jsonItem
	if err := json.Unmarshal([]byte(out), &items); err != nil {
		t.Fatal(err)
	}
	var busy *jsonItem
	for i := range items[0].Children {
		if items[0].Children[i].Title == "busy" {
			busy = &items[0].Children[i]
		}
	}
	if busy == nil || busy.Status != "working" || busy.Column != "progress" {
		t.Fatalf("busy wrong: %+v", busy)
	}
	if sub := busy.Children[0]; sub.Status != "open" || sub.Column != "progress" {
		t.Errorf("an open subtask under a working task is in the progress column: %+v", sub)
	}
	if items[0].Column != "" {
		t.Errorf("a category has no column, got %q", items[0].Column)
	}
}

func TestListColumnFilterMatchesWithinColumn(t *testing.T) {
	path := todotest.Seed(t, "# C\n\n- [/] w\n- [ ] beta\n")
	out, err := runCmd(t, "list", "--file", path, "--column", "progress", "--filter", "beta")
	if err != nil {
		t.Fatal(err)
	}
	if rows := strings.SplitN(out, "\n", 2)[1]; strings.TrimSpace(rows) != "" {
		t.Errorf("a match outside the column must show no rows, not even its category:\n%s", out)
	}
}

// A hand-edited title with terminal escapes makes the file invalid, so list
// fails instead of printing them.
func TestListRefusesStoredControlCharacters(t *testing.T) {
	path := todotest.Seed(t, "# Work\n\n- [ ] a\n")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	edited := strings.Replace(string(raw), `"title": "a"`, `"title": "a\u001b[31m\u001b]0;pwn\u0007"`, 1)
	if edited == string(raw) {
		t.Fatalf("no title replaced in:\n%s", raw)
	}
	if err := os.WriteFile(path, []byte(edited), 0o600); err != nil {
		t.Fatal(err)
	}
	out, err := runCmd(t, "list", "--file", path)
	if codeOf(t, err) != exitGeneric {
		t.Fatalf("list on a list with control characters = %v, want exit 1", err)
	}
	if strings.ContainsAny(out, "\x1b\x07") {
		t.Errorf("list output carries stored control characters:\n%q", out)
	}
}
