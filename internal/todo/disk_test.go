package todo_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
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
