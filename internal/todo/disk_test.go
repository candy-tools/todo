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

// commentLine is the line todo's "_comment" note takes, first in the file.
const commentLine = `  "_comment": "` + todo.FileComment + "\",\n"

// The store writes the list in one fixed form: two-space indented, todo's
// note first (the seed has none), keys in field order, categories before
// tasks, subtasks inline, empty optional fields left out and a trailing
// newline.
func TestDiskFileFormat(t *testing.T) {
	path := todotest.SeedJSON(t, `{"version":4,"categories":[{"id":"c000000","title":"A"},`+
		`{"id":"c000001","title":"B","parent":"c000000"}],"tasks":[{"id":"t000001","category":"c000000","ref":"APP-001",`+
		`"title":"one","status":"review","plan":"docs/p.md","description":"d","subtasks":[{"id":"s000001","title":"sub","done":true}]}]}`)
	s := todo.NewStore(path)
	if _, err := s.Edit(context.Background(), "c000001", todo.Changes{Title: new("B2")}); err != nil {
		t.Fatal(err)
	}
	const want = "{\n" + commentLine + `  "version": 9,
  "refs": {
    "last": {
      "SPRINT": 1
    }
  },
  "sprints": [
    {
      "id": "0000000",
      "ref": "SPRINT-001",
      "name": "Imported"
    }
  ],
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
      "sprint": "0000000",
      "ref": "APP-001",
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
	if got, want := readFile(t, path), "{\n"+commentLine+"  \"version\": 9,\n  \"sprints\": [],\n  \"categories\": [],\n  \"tasks\": []\n}\n"; got != want {
		t.Errorf("file = %q, want %q", got, want)
	}
}

// noteFile is a version 7 list file with note (a whole "_comment" line, or
// "") and one category, aaaaaaa, in the form the store writes.
func noteFile(note string) string {
	return "{\n" + note + `  "version": 9,
  "sprints": [],
  "categories": [
    {
      "id": "aaaaaaa",
      "title": "A"
    }
  ],
  "tasks": []
}
`
}

// A save puts todo's note in a file that has none, and keeps a different note
// the file holds; the rest of the file is written as it was.
func TestDiskSaveNote(t *testing.T) {
	edited := `  "_comment": "edited",` + "\n"
	for name, tc := range map[string]struct{ note, want string }{
		"missing": {"", commentLine},
		"changed": {edited, edited},
		"todo's":  {commentLine, commentLine},
	} {
		t.Run(name, func(t *testing.T) {
			path := todotest.SeedJSON(t, noteFile(tc.note))
			s := todo.NewStore(path)
			for _, title := range []string{"B", "A"} {
				if _, err := s.Edit(context.Background(), "aaaaaaa", todo.Changes{Title: new(title)}); err != nil {
					t.Fatal(err)
				}
			}
			if got := readFile(t, path); got != noteFile(tc.want) {
				t.Errorf("file =\n%s\nwant\n%s", got, noteFile(tc.want))
			}
		})
	}
}

// Loading a list writes nothing: a file without todo's note, in an older
// version, keeps its bytes, and no lock file is made beside it.
func TestDiskLoadLeavesFileAlone(t *testing.T) {
	const v6 = `{"version": 6, "sprints": [], "categories": [{"id": "aaaaaaa", "title": "A"}], "tasks": []}`
	path := todotest.SeedJSON(t, v6)
	loadList(t, path)
	if got := readFile(t, path); got != v6 {
		t.Errorf("file =\n%s\nwant it as it was\n%s", got, v6)
	}
	if entries, _ := os.ReadDir(filepath.Dir(path)); len(entries) != 1 {
		t.Errorf("the directory holds %d files after a load, want only the list", len(entries))
	}
}

// A file that a save creates starts with todo's note.
func TestDiskNewFileGetsNote(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".todo.json")
	if _, _, err := todo.NewStore(path).AppendCategory(context.Background(), "A", ""); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, path); !strings.HasPrefix(got, "{\n"+commentLine) {
		t.Errorf("file does not start with todo's note:\n%s", got)
	}
}

// A note that is not a string is refused like any field of the wrong type.
func TestDiskCommentNotAString(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".todo.json")
	if err := os.WriteFile(path, []byte(`{"_comment": 1, "version": 7, "sprints": [], "categories": [], "tasks": []}`), 0o600); err != nil {
		t.Fatal(err)
	}
	_, _, err := todo.NewStore(path).Load(context.Background())
	if !errors.Is(err, todo.ErrInvalidList) || !strings.Contains(err.Error(), "_comment must be a string") {
		t.Errorf("Load = %v, want an ErrInvalidList saying _comment must be a string", err)
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
	err := json.Unmarshal([]byte(`{"version":10,"extra":1}`), &l)
	if !errors.Is(err, todo.ErrNewerFormat) {
		t.Errorf("err = %v, want ErrNewerFormat", err)
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

// A list file that starts with a UTF-8 byte order mark, as some Windows
// editors write, loads.
func TestListFileWithByteOrderMarkLoads(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".todo.json")
	content := "\ufeff" + `{"version": 3, "categories": [{"id": "aaaaaaa", "title": "C"}], "tasks": []}`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	if doc := loadList(t, path); len(doc.Roots) != 1 || doc.Roots[0].Title != "C" {
		t.Fatalf("roots = %v, want category C", doc.Roots)
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
			"invalid list file: line 1, column 201: tasks[0].subtasks[0].done must be true or false, not a string"},
		{"wrong type, a string with an escaped quote", strings.Replace(good, `"title": "S"`, `"title": "S", "done": "x\"y"`, 1),
			"invalid list file: line 1, column 201: tasks[0].subtasks[0].done must be true or false, not a string"},
		{"wrong type, a number", `{"version": 3, "categories": [{"id": "aaaaaaa", "title": 12.5}], "tasks": []}`,
			"invalid list file: line 1, column 58: categories[0].title must be a string, not a number"},
		{"wrong type, a boolean", `{"version": 3, "categories": [{"id": "aaaaaaa", "title": true}], "tasks": []}`,
			"invalid list file: line 1, column 58: categories[0].title must be a string, not true or false"},
		{"non-ASCII invalid character", `{"version": 3, é}`,
			"invalid list file: line 1, column 16: invalid character 'é' looking for beginning of object key string"},
		{"syntax error after a byte order mark", "\ufeff{\"version\": 3,\n  \"categories\": [,\n",
			"invalid list file: line 2, column 18: invalid character ',' looking for beginning of value"},
		{"truncated in an escape", `{"version": 3, "categories": [{"id": "a\`,
			"invalid list file: line 1, column 41: unexpected end of JSON input"},
		{"wrong type on a later line", "{\n  \"version\": 3,\n  \"categories\": {},\n  \"tasks\": []\n}",
			"invalid list file: line 3, column 17: categories must be an array, not an object"},
		{"wrong type after a blank first line", "\n{\"version\": 3, \"categories\": {}, \"tasks\": []}",
			"invalid list file: line 2, column 30: categories must be an array, not an object"},
		{"not an object", `[]`,
			"invalid list file: line 1, column 1: the file must hold a JSON object, not an array"},
		{"version a string", `{"version": "3", "categories": [], "tasks": []}`,
			`invalid list file: "version" must be a whole number, not "3"`},
		{"version a fraction", `{"version": 3.5, "categories": [], "tasks": []}`,
			`invalid list file: "version" must be a whole number, not 3.5`},
		{"version missing", `{"categories": [], "tasks": []}`,
			`invalid list file: no "version" field; this todo reads format version 9`},
		{"version null", `{"version": null, "categories": [], "tasks": []}`,
			`invalid list file: no "version" field; this todo reads format version 9`},
		{"older version", `{"version": 2, "categories": [], "tasks": []}`,
			"invalid list file: unsupported format version 2"},
		{"older version with other fields", `{"version": 2, "items": []}`,
			"invalid list file: unsupported format version 2"},
		{"newer version", `{"version": 10, "categories": [], "tasks": [], "tags": []}`,
			"list was written by a newer todo (format version 10); upgrade todo"},
		{"unknown top-level field", strings.Replace(good, `{"version": 3,`, `{"version": 3, "tags": [],`, 1),
			`invalid list file: unknown field "tags"`},
		{"unknown category field", strings.Replace(good, `"title": "C"`, `"title": "C", "color": "red"`, 1),
			`invalid list file: category aaaaaaa: unknown field "color"`},
		{"unknown task field", strings.Replace(good, `"status": "open"`, `"status": "open", "due": "today"`, 1),
			`invalid list file: task bbbbbbb: unknown field "due"`},
		{"unknown subtask field", strings.Replace(good, `"title": "S"`, `"title": "S", "due": "today"`, 1),
			`invalid list file: subtask ccccccc of task bbbbbbb: unknown field "due"`},
		{"unknown field in tasks written before categories",
			`{"tasks": [{"id": "bbbbbbb", "x": 1}], "version": 3, "categories": [{"id": "aaaaaaa", "title": "C", "x": 1}]}`,
			`invalid list file: task bbbbbbb: unknown field "x"`},
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

// refsFile is a list file in the form the store writes, with the reference
// settings: a list prefix, the sprint prefix and counters, a category prefix
// and a sprint reference.
const refsFile = "{\n" + commentLine + `  "version": 9,
  "refs": {
    "prefix": "TODO",
    "sprintPrefix": "SPR",
    "last": {
      "SPR": 1,
      "TODO": 3,
      "WORK": 5
    }
  },
  "sprints": [
    {
      "id": "sssssss",
      "ref": "SPR-001",
      "name": "S1",
      "goal": "g"
    }
  ],
  "categories": [
    {
      "id": "aaaaaaa",
      "title": "A",
      "prefix": "WORK"
    }
  ],
  "tasks": []
}
`

// The reference settings survive a load and a write unchanged.
func TestDiskRefsRoundTrip(t *testing.T) {
	path := todotest.SeedJSON(t, refsFile)
	s := todo.NewStore(path)
	before := readFile(t, path)
	for _, title := range []string{"B", "A"} {
		if _, err := s.Edit(context.Background(), "aaaaaaa", todo.Changes{Title: new(title)}); err != nil {
			t.Fatal(err)
		}
	}
	if after := readFile(t, path); after != before {
		t.Errorf("rewritten file =\n%s\nwant\n%s", after, before)
	}
	doc := loadList(t, path)
	if doc.RefPrefix != "TODO" || doc.SprintPrefix != "SPR" || doc.RefLast["WORK"] != 5 || doc.Roots[0].Prefix != "WORK" || doc.Sprints[0].Ref != "SPR-001" {
		t.Errorf("loaded refs = %q %q %v, category %q, sprint %q", doc.RefPrefix, doc.SprintPrefix, doc.RefLast, doc.Roots[0].Prefix, doc.Sprints[0].Ref)
	}
}

// A list with no prefixes and no counters has no "refs" and no "prefix" keys.
func TestDiskRefsLeftOut(t *testing.T) {
	path := todotest.Seed(t, "# A\n\n- [ ] one\n")
	got := readFile(t, path)
	if strings.Contains(got, `"refs"`) || strings.Contains(got, `"prefix"`) {
		t.Errorf("file =\n%s\nwant no refs and no prefix keys", got)
	}
}

// A refs section holds only a prefix and counters.
func TestUnknownFieldInRefs(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".todo.json")
	if err := os.WriteFile(path, []byte(`{"version": 7, "refs": {"prefix": "A", "x": 1}, "categories": [], "tasks": []}`), 0o600); err != nil {
		t.Fatal(err)
	}
	_, _, err := todo.NewStore(path).Load(context.Background())
	if !errors.Is(err, todo.ErrInvalidList) || !strings.HasSuffix(err.Error(), `refs: unknown field "x"`) {
		t.Errorf("Load = %v, want an invalid list ending refs: unknown field \"x\"", err)
	}
}
