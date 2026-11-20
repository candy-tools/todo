package todo_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/candy-tools/todo/internal/todo"
	"github.com/candy-tools/todo/internal/todo/todotest"
)

// viewList is a one-task list file whose view section is view (a JSON
// object, or "" for none).
func viewList(view string) string {
	v := ""
	if view != "" {
		v = `"view": ` + view + `, `
	}
	return `{"version": 5, ` + v + `"categories": [{"id": "c000000", "title": "C"}],
		"tasks": [{"id": "t000001", "category": "c000000", "title": "one", "status": "open"}]}`
}

// The view section of the file loads into the Document's View.
func TestViewLoads(t *testing.T) {
	path := todotest.SeedJSON(t, viewList(`{"unified": true, "split": true}`))
	got := loadList(t, path).View
	if want := (todo.View{Unified: true, Split: true}); got != want {
		t.Errorf("View = %+v, want %+v", got, want)
	}
}

// A file without a view section, such as one from an older todo, loads with
// the default view: the board, no split.
func TestViewMissingIsDefault(t *testing.T) {
	path := todotest.SeedJSON(t, `{"version": 4, "categories": [], "tasks": []}`)
	if got := loadList(t, path).View; got != (todo.View{}) {
		t.Errorf("View = %+v, want the zero View", got)
	}
}

// SetView stores the view, and a later load returns it.
func TestSetViewStoresView(t *testing.T) {
	path := todotest.SeedJSON(t, viewList(""))
	want := todo.View{Unified: true, Split: true}
	res, err := todo.NewStore(path).SetView(context.Background(), want)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Written || res.Doc.View != want {
		t.Errorf("SetView result: Written=%v View=%+v, want written %+v", res.Written, res.Doc.View, want)
	}
	if got := loadList(t, path).View; got != want {
		t.Errorf("loaded View = %+v, want %+v", got, want)
	}
	if f := readFile(t, path); !strings.Contains(f, `"view": {`) {
		t.Errorf("file has no view section:\n%s", f)
	}
}

// SetView with the view the file already holds writes nothing.
func TestSetViewUnchangedWritesNothing(t *testing.T) {
	path := todotest.SeedJSON(t, viewList(`{"split": true}`))
	before := readFile(t, path)
	res, err := todo.NewStore(path).SetView(context.Background(), todo.View{Split: true})
	if err != nil {
		t.Fatal(err)
	}
	if res.Written || readFile(t, path) != before {
		t.Errorf("an unchanged view was written (Written=%v)", res.Written)
	}
}

// Setting the default view back drops the view section from the file, so a
// list that never left the default has none.
func TestSetViewDefaultDropsSection(t *testing.T) {
	path := todotest.SeedJSON(t, viewList(`{"unified": true}`))
	if _, err := todo.NewStore(path).SetView(context.Background(), todo.View{}); err != nil {
		t.Fatal(err)
	}
	if f := readFile(t, path); strings.Contains(f, `"view"`) {
		t.Errorf("the default view should leave no view section:\n%s", f)
	}
}

// A write to the items keeps the view the file holds.
func TestItemWriteKeepsView(t *testing.T) {
	path := todotest.SeedJSON(t, viewList(`{"unified": true, "split": true}`))
	if _, err := todo.NewStore(path).Edit(context.Background(), "t000001", todo.Changes{Title: new("two")}); err != nil {
		t.Fatal(err)
	}
	if got, want := loadList(t, path).View, (todo.View{Unified: true, Split: true}); got != want {
		t.Errorf("View after an edit = %+v, want %+v", got, want)
	}
}

// An unknown field in the view section is refused, naming the section.
func TestViewUnknownField(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".todo.json")
	if err := os.WriteFile(path, []byte(viewList(`{"kanban": true}`)), 0o600); err != nil {
		t.Fatal(err)
	}
	_, _, err := todo.NewStore(path).Load(context.Background())
	if err == nil || !strings.Contains(err.Error(), `view: unknown field "kanban"`) {
		t.Errorf("err = %v, want the unknown view field named", err)
	}
}

// Clone copies the view.
func TestCloneKeepsView(t *testing.T) {
	d := &todo.Document{View: todo.View{Unified: true}}
	if got := d.Clone().View; got != d.View {
		t.Errorf("clone View = %+v, want %+v", got, d.View)
	}
}

// A view that names a sprint the list no longer has still loads: the TUI
// falls back to the list.
func TestViewMissingSprintAccepted(t *testing.T) {
	path := todotest.SeedJSON(t, `{"version": 6, "view": {"screen": "sprint", "sprint": "zzzzzzz"}, "sprints": [],
		"categories": [], "tasks": []}`)
	got := loadList(t, path).View
	if want := (todo.View{Screen: todo.ScreenSprint, Sprint: "zzzzzzz"}); got != want {
		t.Errorf("View = %+v, want %+v", got, want)
	}
}

// An unknown field in a sprint is refused, naming the sprint.
func TestUnknownFieldInSprint(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".todo.json")
	content := `{"version": 6, "sprints": [{"id": "sssssss", "name": "S", "status": "open", "x": 1}], "categories": [], "tasks": []}`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	_, _, err := todo.NewStore(path).Load(context.Background())
	if err == nil || !strings.Contains(err.Error(), `sprint sssssss: unknown field "x"`) {
		t.Errorf("err = %v, want the unknown sprint field named", err)
	}
}
