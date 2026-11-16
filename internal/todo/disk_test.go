package todo_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/candy-tools/todo/internal/todo"
	"github.com/candy-tools/todo/internal/todo/todotest"
)

// encodeDisk encodes l the way the store does.
func encodeDisk(t *testing.T, l todo.DiskList) []byte {
	t.Helper()
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(l); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// roundTrip encodes d, decodes and validates it, and returns the rebuilt tree.
func roundTrip(t *testing.T, d *todo.Document) *todo.Document {
	t.Helper()
	b := encodeDisk(t, todo.ToDisk(d))
	var l todo.DiskList
	if err := json.Unmarshal(b, &l); err != nil {
		t.Fatalf("unmarshal: %v\n%s", err, b)
	}
	if err := todo.ValidateDisk(&l); err != nil {
		t.Fatalf("validate: %v\n%s", err, b)
	}
	return todo.FromDisk(l)
}

func TestDiskRoundTripExample(t *testing.T) {
	md, err := os.ReadFile("../../example.md")
	if err != nil {
		t.Fatal(err)
	}
	d, _, err := todo.ImportMarkdown(md)
	if err != nil {
		t.Fatal(err)
	}
	d.FlattenSubtasks()
	d.AssignIDs()
	got := roundTrip(t, d)
	if todotest.Render(got) != todotest.Render(d) {
		t.Errorf("render differs:\n%s\nwant:\n%s", todotest.Render(got), todotest.Render(d))
	}
	for _, it := range got.Enumerate() {
		if it.Parent != nil && it.Parent.Children[indexOf(it.Parent.Children, it)] != it {
			t.Errorf("bad parent for %q", it.Title)
		}
	}
}

func indexOf(list []*todo.Item, it *todo.Item) int {
	for i, x := range list {
		if x == it {
			return i
		}
	}
	return -1
}

func TestDiskEncodingStable(t *testing.T) {
	d := todotest.Parse(t, "# A\n\n- [ ] one\n  - [x] sub\n")
	d.FlattenSubtasks()
	d.AssignIDs()
	a := encodeDisk(t, todo.ToDisk(d))
	b := encodeDisk(t, todo.ToDisk(d))
	if !bytes.Equal(a, b) {
		t.Error("encoding is not stable")
	}
	s := string(a)
	ci, ti := strings.Index(s, `"categories": [`), strings.Index(s, `"tasks": [`)
	if ci < 0 || ti < 0 || ci > ti {
		t.Errorf("categories must come before tasks:\n%s", s)
	}
}

func TestDiskEmptyListArrays(t *testing.T) {
	b := encodeDisk(t, todo.ToDisk(&todo.Document{}))
	var c bytes.Buffer
	if err := json.Compact(&c, b); err != nil {
		t.Fatal(err)
	}
	if want := `{"version":3,"categories":[],"tasks":[]}`; c.String() != want {
		t.Errorf("got %s, want %s", c.String(), want)
	}
}

func TestDiskSpecialCharactersRoundTrip(t *testing.T) {
	for _, s := range []string{`a<b>&"c"`, "ñandú 🚀"} {
		d := &todo.Document{}
		cat := &todo.Item{Kind: todo.Category, Level: 1, Title: s}
		task := todo.NewTask(s, s, false)
		task.Plan = s
		sub := todo.NewTask(s, s, true)
		sub.Plan = s
		task.AppendChild(sub)
		cat.AppendChild(task)
		d.AppendRoot(cat)
		d.AssignIDs()
		b := encodeDisk(t, todo.ToDisk(d))
		if !bytes.Contains(b, []byte(s[:1])) || (strings.Contains(s, "<") && !bytes.Contains(b, []byte("a<b>&"))) {
			t.Errorf("not written literally: %s", b)
		}
		got := roundTrip(t, d)
		gt := got.Roots[0].Children[0]
		if got.Roots[0].Title != s || gt.Title != s || gt.Description != s || gt.Plan != s ||
			gt.Children[0].Title != s || gt.Children[0].Description != s || gt.Children[0].Plan != s {
			t.Errorf("round trip changed %q", s)
		}
	}
	// A description with a tab round-trips (titles may not hold one).
	d := &todo.Document{}
	cat := &todo.Item{Kind: todo.Category, Level: 1, Title: "C"}
	cat.AppendChild(todo.NewTask("T", "tab\tsep", false))
	d.AppendRoot(cat)
	d.AssignIDs()
	if got := roundTrip(t, d).Roots[0].Children[0].Description; got != "tab\tsep" {
		t.Errorf("description = %q", got)
	}
}

func TestDiskTaskOrderBeforeSubcategories(t *testing.T) {
	l := todo.DiskList{
		Version: 3,
		Categories: []todo.DiskCategory{
			{ID: "aaaaaaa", Title: "A"},
			{ID: "bbbbbbb", Title: "B", Parent: "aaaaaaa"},
		},
		Tasks: []todo.DiskTask{
			{ID: "ccccccc", Category: "bbbbbbb", Title: "tB", Status: "open"},
			{ID: "ddddddd", Category: "aaaaaaa", Title: "tA", Status: "open"},
		},
	}
	if err := todo.ValidateDisk(&l); err != nil {
		t.Fatal(err)
	}
	got := todotest.Render(todo.FromDisk(l))
	want := "# A\n\n- [ ] tA\n\n## B\n\n- [ ] tB\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestUnmarshalNewerVersion(t *testing.T) {
	var l todo.DiskList
	err := json.Unmarshal([]byte(`{"version":4,"extra":1}`), &l)
	if !errors.Is(err, todo.ErrNewerSchema) {
		t.Errorf("err = %v, want ErrNewerSchema", err)
	}
}

func TestUnmarshalUnknownField(t *testing.T) {
	var l todo.DiskList
	if err := json.Unmarshal([]byte(`{"version":3,"categories":[],"tasks":[],"x":1}`), &l); err == nil {
		t.Error("want an error for an unknown field")
	}
	if err := json.Unmarshal([]byte(`{"version":3,"categories":[],"tasks":[{"id":"aaaaaaa","bogus":1}]}`), &l); err == nil {
		t.Error("want an error for a nested unknown field")
	}
}

// A hand-edited list file that does not load fails with a message a person
// can act on: where the problem is (line and column, or the item), in JSON
// terms rather than Go's, and with the "invalid list file" prefix once.
func TestListFileErrorMessages(t *testing.T) {
	const good = `{"version": 3, "categories": [{"id": "aaaaaaa", "title": "C"}], "tasks": [` +
		`{"id": "bbbbbbb", "category": "aaaaaaa", "title": "T", "status": "open", "subtasks": [{"id": "ccccccc", "title": "S"}]}]}`
	cases := []struct{ name, content, want string }{
		{"syntax error", "{\"version\": 3,\n  \"categories\": [,\n",
			"invalid list file: line 2, column 18: invalid character ',' looking for beginning of value"},
		{"truncated", `{"version": 3, "categories": [`,
			"invalid list file: line 1, column 31: unexpected end of JSON input"},
		{"wrong type", `{"version": 3, "categories": [{"id": "aaaaaaa", "title": 5}], "tasks": []}`,
			"invalid list file: line 1, column 58: categories[0].title must be a string, not a number"},
		{"wrong type in a subtask", strings.Replace(good, `"title": "S"`, `"title": "S", "done": "yes"`, 1),
			"invalid list file: line 1, column 205: tasks[0].subtasks[0].done must be true or false, not a string"},
		{"wrong type on a later line", "{\n  \"version\": 3,\n  \"categories\": {},\n  \"tasks\": []\n}",
			"invalid list file: line 3, column 17: categories must be an array, not an object"},
		{"not an object", `[]`,
			"invalid list file: line 1, column 1: the file must hold a JSON object, not an array"},
		{"version a string", `{"version": "3", "categories": [], "tasks": []}`,
			`invalid list file: "version" must be a whole number, not "3"`},
		{"version a fraction", `{"version": 3.5, "categories": [], "tasks": []}`,
			`invalid list file: "version" must be a whole number, not 3.5`},
		{"version missing", `{"categories": [], "tasks": []}`,
			`invalid list file: no "version" field; this todo reads format version 3`},
		{"version null", `{"version": null, "categories": [], "tasks": []}`,
			`invalid list file: no "version" field; this todo reads format version 3`},
		{"older version", `{"version": 2, "categories": [], "tasks": []}`,
			"invalid list file: unsupported format version 2"},
		{"newer version", `{"version": 4, "categories": [], "tasks": [], "tags": []}`,
			"list was written by a newer todo (format version 4); upgrade todo"},
		{"unknown top-level field", strings.Replace(good, `{"version": 3,`, `{"version": 3, "tags": [],`, 1),
			`invalid list file: unknown field "tags"`},
		{"unknown category field", strings.Replace(good, `"title": "C"`, `"title": "C", "color": "red"`, 1),
			`invalid list file: category aaaaaaa: unknown field "color"`},
		{"unknown task field", strings.Replace(good, `"status": "open"`, `"status": "open", "due": "today"`, 1),
			`invalid list file: task bbbbbbb: unknown field "due"`},
		{"unknown subtask field", strings.Replace(good, `"title": "S"`, `"title": "S", "due": "today"`, 1),
			`invalid list file: subtask ccccccc of task bbbbbbb: unknown field "due"`},
		{"unknown field in a task without an id", `{"version": 3, "categories": [], "tasks": [{"due": 1}]}`,
			`invalid list file: tasks[0]: unknown field "due"`},
		{"malformed task id", strings.Replace(good, `"bbbbbbb", "category"`, `"BB-1", "category"`, 1),
			`invalid list file: task "T": malformed id "BB-1": an id is 7 characters from 0-9 and a-z except i, l, o and u`},
		{"malformed subtask id", strings.Replace(good, `"ccccccc"`, `"cccccccc"`, 1),
			`invalid list file: subtask "S" of task "T": malformed id "cccccccc": an id is 7 characters from 0-9 and a-z except i, l, o and u`},
		{"empty title", strings.Replace(good, `"title": "T"`, `"title": ""`, 1),
			"invalid list file: task bbbbbbb: invalid title: it must not be empty"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), ".todo.json")
			if err := os.WriteFile(path, []byte(tc.content), 0o600); err != nil {
				t.Fatal(err)
			}
			_, _, err := todo.NewStore(path).Load(context.Background())
			if !errors.Is(err, todo.ErrInvalidList) {
				t.Fatalf("Load = %v, want an ErrInvalidList", err)
			}
			if want := path + ": " + tc.want; err.Error() != want {
				t.Errorf("Load error\n got %q\nwant %q", err, want)
			}
		})
	}
}
