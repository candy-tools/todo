package todo_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/candy-tools/todo/internal/todo"
)

// listIn returns the path of a list file in a folder named folder under a
// fresh temporary directory, the folder made.
func listIn(t *testing.T, folder string) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), folder)
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	return filepath.Join(dir, ".todo.json")
}

// v7List is a version 7 list: two sprints without references, the second
// with the task reference prefix sprints had then.
const v7List = `{
  "version": 7,
  "refs": {"last": {"S13": 4}},
  "sprints": [
    {"id": "aaaaaaa", "name": "One", "status": "open"},
    {"id": "bbbbbbb", "name": "Two", "prefix": "S13", "status": "open"}
  ],
  "categories": [],
  "tasks": []
}
`

func TestStoreNumbersOldSprints(t *testing.T) {
	ctx := context.Background()
	path := listIn(t, "todo")
	if err := os.WriteFile(path, []byte(v7List), 0o644); err != nil {
		t.Fatal(err)
	}
	s := todo.NewStore(path)
	doc, _, err := s.Load(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got := []string{doc.Sprints[0].Ref, doc.Sprints[1].Ref}; got[0] != "TODO-001" || got[1] != "TODO-002" {
		t.Errorf("refs on load = %v, want TODO-001, TODO-002", got)
	}
	if b, _ := os.ReadFile(path); string(b) != v7List {
		t.Error("loading wrote the file")
	}

	id, res, err := s.AddSprint(ctx, "Three", "")
	if err != nil {
		t.Fatal(err)
	}
	if got := sprintRef(t, res.Doc, id); got != "TODO-003" {
		t.Errorf("new sprint ref = %q, want TODO-003", got)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(b)
	for _, want := range []string{`"version": 9`, `"ref": "TODO-001"`, `"ref": "TODO-002"`, `"ref": "TODO-003"`} {
		if !strings.Contains(text, want) {
			t.Errorf("written file lacks %s:\n%s", want, text)
		}
	}
	if strings.Contains(text, `"prefix": "S13"`) {
		t.Errorf("written file keeps the old sprint prefix:\n%s", text)
	}
}

func TestStoreUpdateSettings(t *testing.T) {
	ctx := context.Background()
	s := todo.NewStore(listIn(t, "todo"))
	res, err := s.UpdateSettings(ctx, todo.Settings{RefPrefix: new("work"), SprintPrefix: new("sp")})
	if err != nil || !res.Written {
		t.Fatalf("UpdateSettings = %v, %v; want a write", res.Written, err)
	}
	doc, _, err := s.Load(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if doc.RefPrefix != "WORK" || doc.SprintPrefix != "SP" || doc.FolderPrefix != "TODO" {
		t.Errorf("settings = %q, %q, folder %q", doc.RefPrefix, doc.SprintPrefix, doc.FolderPrefix)
	}
	if res, err = s.UpdateSettings(ctx, todo.Settings{SprintPrefix: new("SP")}); err != nil || res.Written {
		t.Errorf("unchanged UpdateSettings = %v, %v; want no write", res.Written, err)
	}
}

func TestImportGivesImportedSprintARef(t *testing.T) {
	path := listIn(t, "todo")
	md := filepath.Join(filepath.Dir(path), "TODO.md")
	if err := os.WriteFile(md, []byte("# Work\n- [/] busy\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := todo.ImportFile(path, md, false); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `"ref": "TODO-001"`) {
		t.Errorf("imported sprint has no reference in the file:\n%s", b)
	}
}
