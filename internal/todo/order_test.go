package todo_test

import (
	"context"
	"reflect"
	"testing"

	"github.com/candy-tools/todo/internal/todo"
	"github.com/candy-tools/todo/internal/todo/todotest"
)

// docTitles returns the titles of d in document order. Render cannot show where
// a category's tasks sit against its subcategories (markdown puts tasks first),
// so the order tests read Enumerate.
func docTitles(d *todo.Document) []string {
	var out []string
	for _, it := range d.Enumerate() {
		out = append(out, it.Title)
	}
	return out
}

// wantTitles fails the test unless d holds want in document order.
func wantTitles(t *testing.T, d *todo.Document, want ...string) {
	t.Helper()
	if got := docTitles(d); !reflect.DeepEqual(got, want) {
		t.Errorf("document order = %q, want %q", got, want)
	}
}

const mixed = "# A\n\n- [ ] t1\n\n## B\n\n- [ ] b1\n"

func TestImportPutsCategoriesFirst(t *testing.T) {
	d := opDoc(t, "# A\n\n- [ ] t1\n- [ ] t2\n\n## B\n\n- [ ] b1\n")
	wantTitles(t, d, "A", "B", "b1", "t1", "t2")
}

func TestDiskLoadPutsCategoriesFirst(t *testing.T) {
	path := todotest.SeedJSON(t, `{"version": 4, "categories": [
		{"id": "aaaaaaa", "title": "A"}, {"id": "bbbbbbb", "title": "B", "parent": "aaaaaaa"}], "tasks": [
		{"id": "ddddddd", "category": "aaaaaaa", "title": "tA", "status": "open"},
		{"id": "ccccccc", "category": "bbbbbbb", "title": "tB", "status": "open"}]}`)
	wantTitles(t, loadList(t, path), "A", "B", "tB", "tA")
}

// A list whose category holds both tasks and a subcategory is written back
// byte for byte: only the interleaving, which the file does not hold, moved.
func TestDiskCategoriesBeforeTasksStable(t *testing.T) {
	path := todotest.Seed(t, "# A\n\n- [ ] t1\n- [ ] t2\n\n## B\n\n- [ ] b1\n\n# C\n\n- [ ] c1\n")
	before := readFile(t, path)
	s := todo.NewStore(path)
	id := find(loadList(t, path), "t1").ID
	for _, title := range []string{"changed", "t1"} {
		if _, err := s.Edit(context.Background(), id, todo.Changes{Title: new(title)}); err != nil {
			t.Fatal(err)
		}
	}
	if after := readFile(t, path); after != before {
		t.Errorf("rewritten file =\n%s\nwant the bytes it was loaded from\n%s", after, before)
	}
}

func TestAppendTaskGoesLast(t *testing.T) {
	d := opDoc(t, mixed)
	_, err := d.AppendTask(todo.Fields{Title: "t2"}, todo.Open, idOf(t, d, "A"))
	wantNoErr(t, err)
	wantTitles(t, d, "A", "B", "b1", "t1", "t2")
}

func TestAppendCategoryGoesBeforeTasks(t *testing.T) {
	d := opDoc(t, mixed)
	_, err := d.AppendCategory("C", idOf(t, d, "A"))
	wantNoErr(t, err)
	wantTitles(t, d, "A", "B", "b1", "C", "t1")
}

func TestSlotRules(t *testing.T) {
	d := opDoc(t, mixed)
	a := idOf(t, d, "A")
	_, err := d.AddCategory("C", a, "")
	wantNoErr(t, err)
	wantTitles(t, d, "A", "C", "B", "b1", "t1")
	_, err = d.AddTask(todo.Fields{Title: "t0"}, todo.Open, a, "")
	wantNoErr(t, err)
	wantTitles(t, d, "A", "C", "B", "b1", "t0", "t1")
	// A category anchor places a task first among the tasks.
	_, err = d.AddTask(todo.Fields{Title: "tf"}, todo.Open, a, idOf(t, d, "C"))
	wantNoErr(t, err)
	wantTitles(t, d, "A", "C", "B", "b1", "tf", "t0", "t1")
	// A task anchor for a category does not fit: categories come first.
	_, err = d.AddCategory("D", a, idOf(t, d, "t1"))
	wantErr(t, err, todo.ErrConflict)
}

// MoveLast places an item last among the children of its own kind: a task
// after the last task, a category after the last subcategory, before the tasks.
func TestOpMoveLast(t *testing.T) {
	d := opDoc(t, "# A\n\n- [ ] t1\n- [ ] t2\n\n## B\n\n# O\n\n- [ ] x\n\n## C\n")
	a := idOf(t, d, "A")
	wantNoErr(t, d.MoveLast(idOf(t, d, "x"), a))
	wantTitles(t, d, "A", "B", "t1", "t2", "x", "O", "C")
	wantNoErr(t, d.MoveLast(idOf(t, d, "C"), a))
	wantTitles(t, d, "A", "B", "C", "t1", "t2", "x", "O")
	wantNoErr(t, d.MoveLast(idOf(t, d, "t1"), a))
	wantTitles(t, d, "A", "B", "C", "t2", "x", "t1", "O")
	wantErr(t, d.MoveLast(idOf(t, d, "t1"), ""), todo.ErrWrongParent)
	wantErr(t, d.MoveLast("zzzzzzz", a), todo.ErrNotFound)
}

// A list file an older todo wrote holds a category's own tasks before its
// subcategories' tasks; a write that changes nothing else keeps that tasks
// array order, so the file stays byte for byte as it was.
func TestDiskOldTaskOrderKept(t *testing.T) {
	const old = `{
  "version": 5,
  "categories": [
    {
      "id": "aaaaaaa",
      "title": "A"
    },
    {
      "id": "bbbbbbb",
      "title": "B",
      "parent": "aaaaaaa"
    }
  ],
  "tasks": [
    {
      "id": "ccccccc",
      "category": "aaaaaaa",
      "title": "tA",
      "status": "open"
    },
    {
      "id": "ddddddd",
      "category": "bbbbbbb",
      "title": "tB",
      "status": "open"
    }
  ]
}
`
	path := todotest.SeedJSON(t, old)
	s := todo.NewStore(path)
	for _, title := range []string{"tA2", "tA"} {
		if _, err := s.Edit(context.Background(), "ccccccc", todo.Changes{Title: new(title)}); err != nil {
			t.Fatal(err)
		}
	}
	if got := readFile(t, path); got != old {
		t.Errorf("rewritten file =\n%s\nwant the bytes it was loaded from\n%s", got, old)
	}
}
