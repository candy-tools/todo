package todo_test

import (
	"strconv"
	"testing"

	"github.com/candy-tools/todo/internal/todo"
	"github.com/candy-tools/todo/internal/todo/todotest"
)

// newTask returns a task titled title, with description desc, in state st,
// not yet placed in a tree.
func newTask(title, desc string, st todo.Status) *todo.Item {
	return &todo.Item{Kind: todo.Task, Title: title, Description: desc, Status: st}
}

// buildTree returns a small task tree: parent → [c1 → [g1], c2], all not done.
func buildTree() (parent, c1, c2, g1 *todo.Item) {
	parent = newTask("parent", "", todo.Open)
	c1 = newTask("c1", "", todo.Open)
	c2 = newTask("c2", "", todo.Open)
	g1 = newTask("g1", "", todo.Open)
	parent.AppendChild(c1)
	parent.AppendChild(c2)
	c1.AppendChild(g1)
	return
}

// TestCloneIsDeep checks that a cloned document shares no item with the
// original and that its Parent links point into the copy.
func TestCloneIsDeep(t *testing.T) {
	parent, c1, _, g1 := buildTree()
	d := &todo.Document{}
	d.Roots = append(d.Roots, parent)
	c := d.Clone()
	cp := c.Roots[0]
	if cp == parent || cp.Title != "parent" || cp.Parent != nil || len(cp.Children) != 2 {
		t.Fatalf("clone root = %+v, want a copy of parent", cp)
	}
	cc1 := cp.Children[0]
	if cc1 == c1 || cc1.Parent != cp || cc1.Children[0] == g1 || cc1.Children[0].Parent != cc1 {
		t.Errorf("clone children must be copies linked to their cloned parents")
	}
	cp.Children = cp.Children[1:]
	cp.Title = "changed"
	if len(parent.Children) != 2 || parent.Title != "parent" {
		t.Errorf("changing the clone changed the original: %+v", parent)
	}
}

func TestDescendants(t *testing.T) {
	parent, c1, c2, g1 := buildTree()
	got := parent.Descendants()
	want := []*todo.Item{c1, g1, c2} // depth-first
	if len(got) != len(want) {
		t.Fatalf("got %d descendants, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("descendant %d = %s, want %s", i, got[i].Title, want[i].Title)
		}
	}
}

func TestEnclosingCategory(t *testing.T) {
	work := &todo.Item{Kind: todo.Category, Title: "Work", Level: 1}
	sub := &todo.Item{Kind: todo.Category, Title: "Backend", Level: 2}
	task := newTask("a", "", todo.Open)
	subtask := newTask("sub", "", todo.Open)
	work.AppendChild(sub)
	sub.AppendChild(task)
	task.AppendChild(subtask)

	cases := []struct {
		name string
		in   *todo.Item
		want *todo.Item
	}{
		{"category returns itself", work, work},
		{"task returns its category", task, sub},
		{"subtask returns nearest category ancestor", subtask, sub},
	}
	for _, tc := range cases {
		if got := tc.in.EnclosingCategory(); got != tc.want {
			t.Errorf("%s: EnclosingCategory() = %v, want %v", tc.name, got, tc.want)
		}
	}

	// A task with no category ancestor yields nil (defensive; not a parsed tree).
	if got := newTask("orphan", "", todo.Open).EnclosingCategory(); got != nil {
		t.Errorf("orphan task: EnclosingCategory() = %v, want nil", got)
	}
}

func TestTaskCounts(t *testing.T) {
	parent, _, c2, _ := buildTree()
	c2.Status = todo.Done
	done, total := parent.TaskCounts()
	if done != 1 || total != 3 {
		t.Errorf("counts = %d/%d, want 1/3", done, total)
	}
}

func TestSnapshotDone(t *testing.T) {
	parent, c1, c2, g1 := buildTree()
	for i, it := range []*todo.Item{parent, c1, c2, g1} {
		it.ID = strconv.Itoa(i + 1)
	}
	// A mixed prior state: c2 already done, the rest not.
	c2.Status = todo.Done

	snap := todo.SnapshotDone(parent)
	want := todo.DoneStates{"1": todo.Open, "2": todo.Open, "3": todo.Done, "4": todo.Open}
	if len(snap) != len(want) {
		t.Fatalf("snapshot = %v, want %v", snap, want)
	}
	for id, st := range want {
		if snap[id] != st {
			t.Errorf("snapshot[%s] = %v, want %v", id, snap[id], st)
		}
	}
}

func TestRemovableDoneCountsDoneLeaves(t *testing.T) {
	d := &todo.Document{}
	cat := &todo.Item{Kind: todo.Category, Level: 1, Title: "Cat"}
	d.Roots = append(d.Roots, cat)
	cat.AppendChild(newTask("a", "", todo.Done)) // done
	cat.AppendChild(newTask("b", "", todo.Open)) // open
	cat.AppendChild(newTask("c", "", todo.Done)) // done

	if n := d.RemovableDone(); n != 2 {
		t.Fatalf("RemovableDone = %d, want 2", n)
	}
}

func TestRemovableDoneKeepsUnfinishedSubtree(t *testing.T) {
	d := &todo.Document{}
	cat := &todo.Item{Kind: todo.Category, Level: 1, Title: "Cat"}
	d.Roots = append(d.Roots, cat)

	// p1 is done and its whole subtree is done -> the subtree is removed.
	p1 := newTask("p1", "", todo.Done)
	p1.AppendChild(newTask("p1c", "", todo.Done))
	// p2 is done but still holds an unfinished child -> it must be kept, so no
	// unfinished work is discarded.
	p2 := newTask("p2", "", todo.Done)
	p2.AppendChild(newTask("p2c", "", todo.Open))
	cat.AppendChild(p1)
	cat.AppendChild(p2)

	if n := d.RemovableDone(); n != 2 {
		t.Fatalf("RemovableDone = %d, want 2 (only p1's subtree)", n)
	}
	if got := titles(d.DoneSubtrees()); len(got) != 1 || got[0] != "p1" {
		t.Fatalf("only p1 should be removable, got %v", got)
	}
}

func TestRemovableDoneCountsNestedSubtree(t *testing.T) {
	d := &todo.Document{}
	cat := &todo.Item{Kind: todo.Category, Level: 1, Title: "Cat"}
	d.Roots = append(d.Roots, cat)
	cat.AppendChild(newTask("x", "", todo.Done)) // done leaf
	p := newTask("p", "", todo.Done)             // done parent, done child
	p.AppendChild(newTask("pc", "", todo.Done))
	cat.AppendChild(p)
	cat.AppendChild(newTask("y", "", todo.Open)) // open

	if got := d.RemovableDone(); got != 3 {
		t.Fatalf("RemovableDone = %d, want 3 (x, p, pc)", got)
	}
}

func titles(items []*todo.Item) []string {
	out := make([]string, len(items))
	for i, it := range items {
		out[i] = it.Title
	}
	return out
}

func TestPlacement(t *testing.T) {
	doc := todotest.Parse(t, "# A\n\n- [ ] t1\n- [ ] t2\n\n## B\n")
	a := doc.Roots[0]
	t1, t2, b := a.Children[0], a.Children[1], a.Children[2]
	a.ID, t1.ID, t2.ID, b.ID = "1", "2", "3", "4"
	if p, af := doc.Placement(t1); p != "1" || af != "" {
		t.Errorf("t1: %s,%s", p, af)
	}
	if p, af := doc.Placement(t2); p != "1" || af != "2" {
		t.Errorf("t2: %s,%s", p, af)
	}
	if p, af := doc.Placement(b); p != "1" || af != "3" {
		t.Errorf("b: %s,%s", p, af)
	}
	if p, af := doc.Placement(a); p != "" || af != "" {
		t.Errorf("a: %s,%s", p, af)
	}
}

func TestDoneSubtrees(t *testing.T) {
	doc := todotest.Parse(t, "# A\n\n- [x] done\n  - [x] d1\n- [x] mixed\n  - [ ] open\n  - [x] inner\n")
	got := doc.DoneSubtrees()
	// "done" (whole subtree) and "inner" (inside the kept "mixed"); never "d1" separately.
	if len(got) != 2 || got[0].Title != "done" || got[1].Title != "inner" {
		t.Fatalf("got %v", titles(got))
	}
}

// A task sits in its status's column and a subtask in its task's.
func TestItemColumn(t *testing.T) {
	d := todotest.Parse(t, "# A\n\n- [/] x\n  - [x] s\n- [x] y\n  - [ ] t\n")
	for title, want := range map[string]todo.Column{
		"x": todo.ColInProgress, "s": todo.ColInProgress, "y": todo.ColDone, "t": todo.ColDone,
	} {
		if got := find(d, title).Column(); got != want {
			t.Errorf("%s.Column() = %v, want %v", title, got, want)
		}
	}
}
