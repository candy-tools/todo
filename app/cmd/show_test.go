package cmd

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/candy-tools/todo/internal/todo/todotest"
)

// showFixture builds the list the show tests read: task a (APP-001) in sprint
// S1, working, with a plan, a description and two subtasks.
func showFixture(t *testing.T) string {
	t.Helper()
	path := todotest.Seed(t, "# Work\n\n## Sub\n\n- [ ] a\n  - [x] b\n  - [ ] c\n")
	addSprint(t, path, "S1")
	steps := [][]string{
		{"assign", "--sprint", "S1", "--title", "a"},
		{"status", "--to", "working", "--title", "a"},
		{"edit", "--title", "a", "--set-ref", "APP-001", "--set-plan", "docs/p.md", "--set-desc", "line1\nline2"},
	}
	for _, s := range steps {
		if _, err := runCmd(t, append(s, "--file", path)...); err != nil {
			t.Fatalf("%v: %v", s, err)
		}
	}
	return path
}

func TestShowJSONByRef(t *testing.T) {
	path := showFixture(t)
	out, err := runCmd(t, "show", "--file", path, "--id", "app-001", "--json")
	if err != nil {
		t.Fatal(err)
	}
	var got record
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("not one object: %v\n%s", err, out)
	}
	doc := loadDoc(t, path)
	want := toDetail(doc, doc.ByTitle("a")[0], numberMap(doc))
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v\nwant %+v", got, want)
	}
}

func TestShowText(t *testing.T) {
	path := showFixture(t)
	out, err := runCmd(t, "show", "--file", path, "--title", "a")
	if err != nil {
		t.Fatal(err)
	}
	doc := loadDoc(t, path)
	nums := numberMap(doc)
	a := doc.ByTitle("a")[0]
	want := strings.Join([]string{
		a.DisplayTitle(), // "[APP-001] a"
		fmt.Sprintf("  id %s · #%d · task · working · column progress · sprint S1", a.ID, nums[a]),
		"  path: Work / Sub",
		"  plan: docs/p.md",
		"  subtasks: 1/2 done",
		"    [x] b",
		"    [ ] c",
		"  description:",
		"    line1",
		"    line2",
		"",
	}, "\n")
	if out != want {
		t.Errorf("got:\n%s\nwant:\n%s", out, want)
	}
}

func TestShowTextCategory(t *testing.T) {
	path := showFixture(t)
	out, err := runCmd(t, "show", "--file", path, "--title", "Sub")
	if err != nil {
		t.Fatal(err)
	}
	doc := loadDoc(t, path)
	sub := doc.ByTitle("Sub")[0]
	want := fmt.Sprintf("Sub\n  id %s · #%d · category\n  path: Work\n", sub.ID, numberMap(doc)[sub])
	if out != want {
		t.Errorf("got:\n%s\nwant:\n%s", out, want)
	}
}

func TestShowTextCategoryPrefix(t *testing.T) {
	path := showFixture(t)
	if _, err := runCmd(t, "add-category", "--file", path, "--title", "Pfx", "--prefix", "WRK"); err != nil {
		t.Fatal(err)
	}
	out, err := runCmd(t, "show", "--file", path, "--title", "Pfx")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, " · category · prefix WRK\n") {
		t.Errorf("got:\n%s", out)
	}
}

func TestShowAmbiguousTitle(t *testing.T) {
	path := todotest.Seed(t, "# Work\n\n- [ ] x\n- [ ] x\n")
	_, err := runCmd(t, "show", "--file", path, "--title", "x")
	if codeOf(t, err) != 4 {
		t.Errorf("want exit 4, got %v", err)
	}
}

func TestShowNotFound(t *testing.T) {
	path := todotest.Seed(t, "# Work\n\n- [ ] x\n")
	_, err := runCmd(t, "show", "--file", path, "--id", "zzzzzzz")
	if codeOf(t, err) != 3 {
		t.Errorf("want exit 3, got %v", err)
	}
}
