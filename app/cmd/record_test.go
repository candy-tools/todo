package cmd

import (
	"context"
	"reflect"
	"testing"

	"github.com/candy-tools/todo/internal/todo"
	"github.com/candy-tools/todo/internal/todo/todotest"
)

// loadDoc loads the list at path.
func loadDoc(t *testing.T, path string) *todo.Document {
	t.Helper()
	doc, _, err := todo.NewStore(path).Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return doc
}

func TestRecordTask(t *testing.T) {
	path := todotest.Seed(t, "# Work\n\n## Sub\n\n- [ ] a\n  - [x] b\n  - [ ] c\n")
	if _, err := runCmd(t, "edit", "--file", path, "--title", "a", "--set-ref", "APP-001", "--set-plan", "docs/p.md"); err != nil {
		t.Fatal(err)
	}
	doc := loadDoc(t, path)
	nums := numberMap(doc)
	a := doc.ByTitle("a")[0]
	b, c := doc.ByTitle("b")[0], doc.ByTitle("c")[0]
	got := toDetail(doc, a, nums)
	want := record{
		Number: nums[a], ID: a.ID, Kind: "task", Ref: "APP-001", Title: "a", Status: "open", Column: "backlog",
		Path: []string{"Work", "Sub"}, ParentID: doc.ByTitle("Sub")[0].ID, ParentTitle: "Sub", Plan: "docs/p.md",
		DoneCount: 1, TotalCount: 2,
		Subtasks: []brief{{nums[b], b.ID, "b", "done"}, {nums[c], c.ID, "c", "open"}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v\nwant %+v", got, want)
	}
}

func TestRecordCategory(t *testing.T) {
	path := todotest.Seed(t, "# Work\n\n## Sub\n\n- [ ] a\n")
	doc := loadDoc(t, path)
	sub := doc.ByTitle("Sub")[0]
	got := toDetail(doc, sub, numberMap(doc))
	if got.Kind != "category" || got.Level != 2 || got.Status != "" || got.Column != "" || got.Subtasks != nil {
		t.Errorf("category record: %+v", got)
	}
	if _, err := runCmd(t, "add-category", "--file", path, "--title", "Pfx", "--prefix", "WRK"); err != nil {
		t.Fatal(err)
	}
	doc = loadDoc(t, path)
	if got := toRecord(doc, doc.ByTitle("Pfx")[0], numberMap(doc)); got.Prefix != "WRK" {
		t.Errorf("category prefix = %q, want WRK", got.Prefix)
	}
}

func TestRecordSubtaskFollowsParent(t *testing.T) {
	path := todotest.Seed(t, "# Work\n")
	sid := addSprint(t, path, "S1")
	sprintTask(t, path, "a", sid, todo.Working)
	if _, err := runCmd(t, "add", "--file", path, "--title", "b", "--parent-title", "a"); err != nil {
		t.Fatal(err)
	}
	doc := loadDoc(t, path)
	a, b := doc.ByTitle("a")[0], doc.ByTitle("b")[0]
	got := toRecord(doc, b, numberMap(doc))
	if got.Column != "progress" || got.Sprint != sid || got.SprintName != "S1" || got.ParentID != a.ID || got.ParentTitle != "a" {
		t.Errorf("subtask record: %+v", got)
	}
}

func TestRecordImportedSprintName(t *testing.T) {
	path := todotest.Seed(t, "# Work\n\n- [x] b\n")
	doc := loadDoc(t, path)
	got := toRecord(doc, doc.ByTitle("b")[0], numberMap(doc))
	if got.Sprint != "0000000" || got.SprintName != todo.ImportedSprintName {
		t.Errorf("imported record: %+v", got)
	}
}
