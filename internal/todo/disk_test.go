package todo_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/candy-tools/todo/internal/todo"
	"github.com/candy-tools/todo/internal/todo/todotest"
)

// readFile returns the content of path, failing the test on error.
func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// loadList loads the list at path, failing the test on error.
func loadList(t *testing.T, path string) *todo.Document {
	t.Helper()
	doc, _, err := todo.NewStore(path).Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return doc
}

// The example list written by the store loads back as it was imported, with
// every item linked to its parent and listed among its parent's children.
func TestDiskRoundTripExample(t *testing.T) {
	md, err := os.ReadFile("../../example.md")
	if err != nil {
		t.Fatal(err)
	}
	want, _, err := todo.ImportMarkdown(md)
	if err != nil {
		t.Fatal(err)
	}
	want.FlattenSubtasks()
	path := filepath.Join(t.TempDir(), ".todo.json")
	if _, _, err := todo.ImportFile(path, "../../example.md", false); err != nil {
		t.Fatal(err)
	}
	got := loadList(t, path)
	if todotest.Render(got) != todotest.Render(want) {
		t.Errorf("render differs:\n%s\nwant:\n%s", todotest.Render(got), todotest.Render(want))
	}
	for _, it := range got.Enumerate() {
		siblings := got.Roots
		if it.Parent != nil {
			siblings = it.Parent.Children
		}
		if !slices.Contains(siblings, it) {
			t.Errorf("%q is not among its parent's children", it.Title)
		}
	}
}

// The store writes the list in one fixed form: two-space indented, keys in
// field order, categories before tasks, subtasks inline, empty optional
// fields left out and a trailing newline.
func TestDiskFileFormat(t *testing.T) {
	path := todotest.SeedJSON(t, `{"version":3,"categories":[{"id":"c000000","title":"A"},`+
		`{"id":"c000001","title":"B","parent":"c000000"}],"tasks":[{"id":"t000001","category":"c000000",`+
		`"title":"one","status":"review","plan":"docs/p.md","description":"d","subtasks":[{"id":"s000001","title":"sub","done":true}]}]}`)
	s := todo.NewStore(path)
	if _, err := s.Edit(context.Background(), "c000001", todo.Changes{Title: new("B2")}); err != nil {
		t.Fatal(err)
	}
	const want = `{
  "version": 3,
  "categories": [
    {
      "id": "c000000",
      "title": "A"
    },
    {
      "id": "c000001",
      "title": "B2",
      "parent": "c000000"
    }
  ],
  "tasks": [
    {
      "id": "t000001",
      "category": "c000000",
      "title": "one",
      "status": "review",
      "plan": "docs/p.md",
      "description": "d",
      "subtasks": [
        {
          "id": "s000001",
          "title": "sub",
          "done": true
        }
      ]
    }
  ]
}
`
	if got := readFile(t, path); got != want {
		t.Errorf("file =\n%s\nwant\n%s", got, want)
	}
}

// Loading a list the store wrote and writing it again gives the same bytes.
func TestDiskEncodingStable(t *testing.T) {
	path := todotest.Seed(t, "# A\n\n- [ ] one\n  desc\n  - [x] sub\n\n## B\n\n- [/] two\n")
	before := readFile(t, path)
	s := todo.NewStore(path)
	id := find(loadList(t, path), "one").ID
	for _, title := range []string{"changed", "one"} {
		if _, err := s.Edit(context.Background(), id, todo.Changes{Title: new(title)}); err != nil {
			t.Fatal(err)
		}
	}
	if after := readFile(t, path); after != before {
		t.Errorf("rewritten file =\n%s\nwant the bytes it was loaded from\n%s", after, before)
	}
}

// A list left empty is written with empty arrays, not nulls.
func TestDiskEmptyListArrays(t *testing.T) {
	path := todotest.Seed(t, "# A\n")
	s := todo.NewStore(path)
	if _, err := s.Delete(context.Background(), find(loadList(t, path), "A").ID); err != nil {
		t.Fatal(err)
	}
	if got, want := readFile(t, path), "{\n  \"version\": 3,\n  \"categories\": [],\n  \"tasks\": []\n}\n"; got != want {
		t.Errorf("file = %q, want %q", got, want)
	}
}

// Special characters are written literally (no HTML escaping, no \u escapes
// for non-ASCII) and load back unchanged; a tab in a description is written
// as JSON's \t.
func TestDiskSpecialCharactersRoundTrip(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct{ text, written string }{
		{`a<b>&"c"`, `"a<b>&\"c\""`},
		{"ñandú 🚀", `"ñandú 🚀"`},
		{"tab\tsep", `"tab\tsep"`},
	} {
		title := strings.ReplaceAll(tc.text, "\t", " ") // titles may not hold a tab
		s := todo.NewStore(filepath.Join(t.TempDir(), ".todo.json"))
		c, _, err := s.AppendCategory(ctx, title, "")
		if err != nil {
			t.Fatal(err)
		}
		f := todo.Fields{Title: title, Description: tc.text, Plan: tc.text}
		task, _, err := s.AppendTask(ctx, f, todo.Open, c)
		if err != nil {
			t.Fatal(err)
		}
		sub, _, err := s.AppendTask(ctx, f, todo.Done, task)
		if err != nil {
			t.Fatal(err)
		}
		file := readFile(t, s.Path())
		if want := `"description": ` + tc.written; strings.Count(file, want) != 2 {
			t.Errorf("file holds %q %d times, want twice (task and subtask):\n%s", want, strings.Count(file, want), file)
		}
		doc := loadList(t, s.Path())
		for _, id := range []string{task, sub} {
			it, _ := doc.ByID(id)
			if it.Title != title || it.Description != tc.text || it.Plan != tc.text {
				t.Errorf("%q loaded back as title %q, description %q, plan %q", tc.text, it.Title, it.Description, it.Plan)
			}
		}
		if cat, _ := doc.ByID(c); cat.Title != title {
			t.Errorf("category title %q loaded back as %q", title, cat.Title)
		}
	}
}

// A category's tasks load before its subcategories, whatever the file order.
func TestDiskTaskOrderBeforeSubcategories(t *testing.T) {
	path := todotest.SeedJSON(t, `{"version": 3, "categories": [
		{"id": "aaaaaaa", "title": "A"}, {"id": "bbbbbbb", "title": "B", "parent": "aaaaaaa"}], "tasks": [
		{"id": "ccccccc", "category": "bbbbbbb", "title": "tB", "status": "open"},
		{"id": "ddddddd", "category": "aaaaaaa", "title": "tA", "status": "open"}]}`)
	if got, want := todotest.Render(loadList(t, path)), "# A\n\n- [ ] tA\n\n## B\n\n- [ ] tB\n"; got != want {
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
