package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/candy-tools/todo/internal/todo"
)

// listInTodo returns the path of a list, with one category Work, in a folder
// named todo, so its sprints' references default to TODO-<n>.
func listInTodo(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "todo")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, ".todo.json")
	const list = `{"version": 8, "sprints": [], "categories": [{"id": "aaaaaaa", "title": "Work"}], "tasks": []}`
	if err := os.WriteFile(path, []byte(list), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestSprintAddPrintsRefFromFolder(t *testing.T) {
	path := listInTodo(t)
	if got := sprintAddRef(t, path, "Alpha"); got != "TODO-001" {
		t.Errorf("first sprint add printed %q, want TODO-001", got)
	}
	if got := sprintAddRef(t, path, "Alpha"); got != "TODO-002" {
		t.Errorf("second sprint with the same name printed %q, want TODO-002", got)
	}
}

func TestSprintFlagTakesRef(t *testing.T) {
	path := listInTodo(t)
	addSprint(t, path, "Alpha")
	addSprint(t, path, "Beta")
	if _, err := runCmd(t, "sprint", "edit", "--file", path, "--sprint", "todo-002", "--set-goal", "g"); err != nil {
		t.Fatal(err)
	}
	if _, err := runCmd(t, "add", "--file", path, "--parent-title", "Work", "--title", "t", "--sprint", "TODO-002"); err != nil {
		t.Fatal(err)
	}
	doc, _, err := todo.NewStore(path).Load(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	beta := doc.Sprints[1]
	if beta.Goal != "g" {
		t.Errorf("goal of TODO-002 = %q, want g", beta.Goal)
	}
	if got := doc.ByTitle("t")[0].Sprint; got != beta.ID {
		t.Errorf("task sprint = %q, want TODO-002's id %q", got, beta.ID)
	}
}

func TestSprintSharedNameIsAmbiguous(t *testing.T) {
	path := listInTodo(t)
	addSprint(t, path, "Same")
	addSprint(t, path, "same")
	_, err := runCmd(t, "sprint", "close", "--file", path, "--sprint", "SAME")
	if err == nil || codeOf(t, err) != exitAmbiguous {
		t.Fatalf("err = %v, want exit %d", err, exitAmbiguous)
	}
	for _, want := range []string{"TODO-001", "TODO-002"} {
		if !strings.Contains(err.Error(), "--sprint "+want) {
			t.Errorf("message lacks --sprint %s:\n%s", want, err)
		}
	}
}

func TestSprintPrefixFlagsGone(t *testing.T) {
	path := listInTodo(t)
	if _, err := runCmd(t, "sprint", "add", "--file", path, "--name", "S", "--prefix", "SP"); err == nil {
		t.Error("sprint add --prefix accepted")
	}
	addSprint(t, path, "S")
	if _, err := runCmd(t, "sprint", "edit", "--file", path, "--sprint", "S", "--set-prefix", "SP"); err == nil {
		t.Error("sprint edit --set-prefix accepted")
	}
}

func TestSprintOutputsShowRef(t *testing.T) {
	path := listInTodo(t)
	addSprint(t, path, "Alpha", "--goal", "g")
	out, err := runCmd(t, "sprint", "list", "--file", path)
	if err != nil || !strings.Contains(out, "TODO-001") {
		t.Errorf("table = %q, %v", out, err)
	}
	out, err = runCmd(t, "sprint", "list", "--file", path, "--json")
	if err != nil {
		t.Fatal(err)
	}
	var got []map[string]any
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if got[0]["ref"] != "TODO-001" || got[0]["prefix"] != nil {
		t.Errorf("json sprint = %v, want ref TODO-001 and no prefix", got[0])
	}
	out, err = runCmd(t, "sprint", "reopen", "--file", path, "--sprint", "Alpha")
	if err != nil || strings.TrimSpace(out) != "reopen: TODO-001 Alpha" {
		t.Errorf("reopen = %q, %v", out, err)
	}
	out, err = runCmd(t, "next", "--file", path)
	if err != nil || !strings.HasPrefix(out, "TODO-001 Alpha — g") {
		t.Errorf("next = %q, %v", out, err)
	}
}

func TestTaskRecordShowsSprintRef(t *testing.T) {
	path := listInTodo(t)
	addSprint(t, path, "Alpha")
	if _, err := runCmd(t, "add", "--file", path, "--parent-title", "Work", "--title", "t", "--sprint", "TODO-001"); err != nil {
		t.Fatal(err)
	}
	out, err := runCmd(t, "show", "--file", path, "--title", "t", "--json")
	if err != nil || !strings.Contains(out, `"sprintRef": "TODO-001"`) {
		t.Errorf("show json = %s, %v", out, err)
	}
	out, err = runCmd(t, "show", "--file", path, "--title", "t")
	if err != nil || !strings.Contains(out, "sprint TODO-001 Alpha") {
		t.Errorf("show = %s, %v", out, err)
	}
}

func TestSettingsSprintPrefix(t *testing.T) {
	path := listInTodo(t)
	out, err := runCmd(t, "settings", "--file", path)
	if err != nil || !strings.Contains(out, "sprint-prefix: TODO (folder name)") {
		t.Fatalf("show = %q, %v", out, err)
	}
	out, err = runCmd(t, "settings", "--file", path, "--sprint-prefix", "sp")
	if err != nil || !strings.Contains(out, "sprint-prefix: SP\n") {
		t.Fatalf("set = %q, %v", out, err)
	}
	if got := sprintAddRef(t, path, "Alpha"); got != "SP-001" {
		t.Errorf("sprint add printed %q, want SP-001", got)
	}
	out, err = runCmd(t, "settings", "--file", path, "--json")
	if err != nil || strings.TrimSpace(out) != `{"refPrefix":"","sprintPrefix":"SP","sprintRefPrefix":"SP"}` {
		t.Errorf("json = %q, %v", out, err)
	}
	if _, err = runCmd(t, "settings", "--file", path, "--sprint-prefix", ""); err != nil {
		t.Fatal(err)
	}
	out, err = runCmd(t, "settings", "--file", path, "--json")
	if err != nil || strings.TrimSpace(out) != `{"refPrefix":"","sprintPrefix":"","sprintRefPrefix":"TODO"}` {
		t.Errorf("json after clear = %q, %v", out, err)
	}
	_, err = runCmd(t, "settings", "--file", path, "--ref-prefix", "X", "--sprint-prefix", "x")
	if err == nil || codeOf(t, err) != exitUsage || !strings.Contains(err.Error(), todo.ErrPrefixClash.Error()) {
		t.Errorf("clash: %v", err)
	}
}
