package cmd

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/candy-tools/todo/internal/todo"
	"github.com/candy-tools/todo/internal/todo/todotest"
)

// nextFixture builds sprint S1 (goal g) holding n1 (new), r1 (ready),
// w1 (working), v1 (review) and d1 (done), in that tree order. A new task goes
// first among its siblings, so they are added in reverse.
func nextFixture(t *testing.T) string {
	t.Helper()
	path := todotest.Seed(t, "# Work\n\n- [ ] seed\n")
	id := addSprint(t, path, "S1", "--goal", "g")
	for _, s := range []struct {
		title string
		st    todo.Status
	}{{"d1", todo.Done}, {"v1", todo.Review}, {"w1", todo.Working}, {"r1", todo.Ready}, {"n1", todo.New}} {
		sprintTask(t, path, s.title, id, s.st)
	}
	return path
}

func titles(rs []record) []string {
	out := []string{}
	for _, r := range rs {
		out = append(out, r.Title)
	}
	return out
}

func TestNextJSON(t *testing.T) {
	path := nextFixture(t)
	out, err := runCmd(t, "next", "--file", path, "--json")
	if err != nil {
		t.Fatal(err)
	}
	var got nextReport
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("bad JSON: %v\n%s", err, out)
	}
	if len(got.Sprints) != 1 {
		t.Fatalf("sprints = %d", len(got.Sprints))
	}
	s := got.Sprints[0]
	if s.Name != "S1" || s.Goal != "g" || s.Status != "open" {
		t.Errorf("sprint = %+v", s)
	}
	if !reflect.DeepEqual(titles(s.InProgress), []string{"w1", "v1"}) {
		t.Errorf("inProgress = %v", titles(s.InProgress))
	}
	if !reflect.DeepEqual(titles(s.Next), []string{"r1", "n1"}) {
		t.Errorf("next = %v", titles(s.Next))
	}
	want := map[string]int{"next": 2, "progress": 2, "done": 1}
	if !reflect.DeepEqual(s.Counts, want) {
		t.Errorf("counts = %v", s.Counts)
	}
}

func TestNextJSONSprintPrefix(t *testing.T) {
	path := todotest.Seed(t, "# Work\n\n- [ ] seed\n")
	addSprint(t, path, "S1", "--prefix", "S13")
	out, err := runCmd(t, "next", "--file", path, "--json")
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		Sprints []map[string]any `json:"sprints"`
	}
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("bad JSON: %v\n%s", err, out)
	}
	if len(got.Sprints) != 1 || got.Sprints[0]["prefix"] != "S13" {
		t.Errorf("next --json sprint lacks prefix S13:\n%s", out)
	}
}

func TestNextAllOpenSprints(t *testing.T) {
	path := todotest.Seed(t, "# Work\n\n- [ ] seed\n")
	s1 := addSprint(t, path, "S1")
	s0 := addSprint(t, path, "S0")
	s2 := addSprint(t, path, "S2")
	sprintTask(t, path, "a", s1, todo.New)
	sprintTask(t, path, "b", s0, todo.Done)
	sprintTask(t, path, "c", s2, todo.Ready)
	if _, err := runCmd(t, "sprint", "close", "--file", path, "--sprint", "S0"); err != nil {
		t.Fatal(err)
	}
	out, err := runCmd(t, "next", "--file", path, "--json")
	if err != nil {
		t.Fatal(err)
	}
	var got nextReport
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, s := range got.Sprints {
		names = append(names, s.Name)
	}
	if !reflect.DeepEqual(names, []string{"S1", "S2"}) {
		t.Errorf("names = %v", names)
	}
}

func TestNextOneSprint(t *testing.T) {
	path := todotest.Seed(t, "# Work\n\n- [ ] seed\n")
	s1 := addSprint(t, path, "S1")
	s2 := addSprint(t, path, "S2")
	s0 := addSprint(t, path, "S0")
	sprintTask(t, path, "a", s1, todo.New)
	sprintTask(t, path, "c", s2, todo.Ready)
	sprintTask(t, path, "b", s0, todo.Done)
	if _, err := runCmd(t, "sprint", "close", "--file", path, "--sprint", "S0"); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"S2", "S0"} {
		out, err := runCmd(t, "next", "--file", path, "--sprint", name, "--json")
		if err != nil {
			t.Fatal(err)
		}
		var got nextReport
		if err := json.Unmarshal([]byte(out), &got); err != nil {
			t.Fatal(err)
		}
		if len(got.Sprints) != 1 || got.Sprints[0].Name != name {
			t.Errorf("--sprint %s -> %+v", name, got.Sprints)
		}
		wantStatus := "open"
		if name == "S0" {
			wantStatus = "done"
		}
		if got.Sprints[0].Status != wantStatus {
			t.Errorf("--sprint %s status = %q, want %q", name, got.Sprints[0].Status, wantStatus)
		}
		text, err := runCmd(t, "next", "--file", path, "--sprint", name)
		if err != nil {
			t.Fatal(err)
		}
		if closed := strings.Contains(strings.SplitN(text, "\n", 2)[0], "(closed)"); closed != (name == "S0") {
			t.Errorf("--sprint %s header closed marker = %v: %q", name, closed, text)
		}
	}
	if _, err := runCmd(t, "next", "--file", path, "--sprint", "nope"); codeOf(t, err) != exitNotFound {
		t.Errorf("want not found, got %v", err)
	}
}

func TestNextNoOpenSprint(t *testing.T) {
	path := todotest.Seed(t, "# Work\n\n- [ ] seed\n")
	out, err := runCmd(t, "next", "--file", path, "--json")
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]json.RawMessage
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatal(err)
	}
	if string(got["sprints"]) != "[]" {
		t.Errorf("sprints = %s", got["sprints"])
	}
	out, err = runCmd(t, "next", "--file", path)
	if err != nil {
		t.Fatal(err)
	}
	if out != "no open sprint\n" {
		t.Errorf("text = %q", out)
	}
}

func TestNextText(t *testing.T) {
	path := nextFixture(t)
	out, err := runCmd(t, "next", "--file", path)
	if err != nil {
		t.Fatal(err)
	}
	doc := loadDoc(t, path)
	nums := numberMap(doc)
	n := func(title string) int { return nums[doc.ByTitle(title)[0]] }
	want := strings.Join([]string{
		"S1 — g",
		"  in progress:",
		fmt.Sprintf("    #%d [working] w1", n("w1")),
		fmt.Sprintf("    #%d [review] v1", n("v1")),
		"  next:",
		fmt.Sprintf("    #%d [ready] r1", n("r1")),
		fmt.Sprintf("    #%d [new] n1", n("n1")),
		"",
	}, "\n")
	if out != want {
		t.Errorf("got:\n%s\nwant:\n%s", out, want)
	}
}

func TestNextTextEmptyListsAndSeparator(t *testing.T) {
	path := todotest.Seed(t, "# Work\n\n- [ ] seed\n")
	addSprint(t, path, "S1")
	addSprint(t, path, "S2", "--goal", "x")
	out, err := runCmd(t, "next", "--file", path)
	if err != nil {
		t.Fatal(err)
	}
	want := "S1\n  in progress:\n    (none)\n  next:\n    (none)\n\nS2 — x\n  in progress:\n    (none)\n  next:\n    (none)\n"
	if out != want {
		t.Errorf("got:\n%q\nwant:\n%q", out, want)
	}
}
