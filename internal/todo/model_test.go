package todo_test

import (
	"testing"

	"github.com/candy-tools/todo/internal/todo"
)

// buildTree returns a small task tree: parent → [c1 → [g1], c2], all not done.
func buildTree() (parent, c1, c2, g1 *todo.Item) {
	parent = todo.NewTask("parent", "", false)
	c1 = todo.NewTask("c1", "", false)
	c2 = todo.NewTask("c2", "", false)
	g1 = todo.NewTask("g1", "", false)
	parent.AppendChild(c1)
	parent.AppendChild(c2)
	c1.AppendChild(g1)
	return
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
	task := todo.NewTask("a", "", false)
	subtask := todo.NewTask("sub", "", false)
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
	if got := todo.NewTask("orphan", "", false).EnclosingCategory(); got != nil {
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

func TestCascadeSetDone(t *testing.T) {
	parent, c1, c2, g1 := buildTree()
	todo.CascadeSetDone(parent, true)
	for _, it := range []*todo.Item{parent, c1, c2, g1} {
		if !it.IsDone() {
			t.Errorf("%s should be done after cascade", it.Title)
		}
	}
	todo.CascadeSetDone(parent, false)
	for _, it := range []*todo.Item{parent, c1, c2, g1} {
		if it.IsDone() {
			t.Errorf("%s should be undone after reverse cascade", it.Title)
		}
	}
}

func TestSnapshotRestore(t *testing.T) {
	parent, c1, c2, g1 := buildTree()
	// A mixed prior state: c2 already done, the rest not.
	c2.Status = todo.Done

	snap := todo.SnapshotDone(parent)
	// Accidental complete: everything forced done.
	todo.CascadeSetDone(parent, true)
	if !c1.IsDone() || !g1.IsDone() {
		t.Fatalf("cascade should have completed all children")
	}

	// Undo: restore exactly the prior mixed state.
	todo.RestoreDone(snap)
	if parent.IsDone() || c1.IsDone() || g1.IsDone() {
		t.Errorf("restore should have undone parent/c1/g1")
	}
	if !c2.IsDone() {
		t.Errorf("restore should have kept c2 done (its prior state)")
	}
}

func TestAppendTaskBeforeSubcategory(t *testing.T) {
	cat := &todo.Item{Kind: todo.Category, Level: 1, Title: "Cat"}
	sub := &todo.Item{Kind: todo.Category, Level: 2, Title: "Sub"}
	t1 := todo.NewTask("t1", "", false)
	cat.AppendChild(t1)
	cat.AppendChild(sub) // Cat.Children = [t1, Sub]

	t2 := todo.NewTask("t2", "", false)
	cat.AppendTask(t2)
	// t2 must be placed before the Sub subcategory: [t1, t2, Sub].
	if got := titles(cat.Children); len(got) != 3 || got[0] != "t1" || got[1] != "t2" || got[2] != "Sub" {
		t.Errorf("AppendTask order = %v, want [t1 t2 Sub]", got)
	}
	if t2.Parent != cat {
		t.Errorf("AppendTask should set the parent")
	}
}

func TestAppendTaskToTaskAppends(t *testing.T) {
	parent := todo.NewTask("p", "", false)
	parent.AppendTask(todo.NewTask("s1", "", false))
	parent.AppendTask(todo.NewTask("s2", "", false))
	if got := titles(parent.Children); len(got) != 2 || got[0] != "s1" || got[1] != "s2" {
		t.Errorf("subtasks should append in order, got %v", got)
	}
}

func TestInsertAfterSibling(t *testing.T) {
	d := &todo.Document{}
	cat := &todo.Item{Kind: todo.Category, Level: 1, Title: "Cat"}
	d.AppendRoot(cat)
	a := todo.NewTask("a", "", false)
	b := todo.NewTask("b", "", false)
	cat.AppendChild(a)
	cat.AppendChild(b)

	mid := todo.NewTask("mid", "", false)
	if !d.InsertAfter(a, mid) {
		t.Fatal("InsertAfter returned false")
	}
	if len(cat.Children) != 3 || cat.Children[1] != mid {
		t.Fatalf("mid not inserted after a: %v", titles(cat.Children))
	}
	if mid.Parent != cat {
		t.Errorf("inserted sibling should share the parent")
	}
}

func TestInsertAfterRoot(t *testing.T) {
	d := &todo.Document{}
	a := todo.NewTask("a", "", false)
	d.AppendRoot(a)
	b := todo.NewTask("b", "", false)
	if !d.InsertAfter(a, b) {
		t.Fatal("InsertAfter among roots returned false")
	}
	if len(d.Roots) != 2 || d.Roots[1] != b || b.Parent != nil {
		t.Errorf("root sibling insert wrong: %v", titles(d.Roots))
	}
}

func TestRemove(t *testing.T) {
	d := &todo.Document{}
	cat := &todo.Item{Kind: todo.Category, Level: 1, Title: "Cat"}
	d.AppendRoot(cat)
	a := todo.NewTask("a", "", false)
	b := todo.NewTask("b", "", false)
	cat.AppendChild(a)
	cat.AppendChild(b)

	if !d.Remove(a) {
		t.Fatal("Remove returned false")
	}
	if len(cat.Children) != 1 || cat.Children[0] != b {
		t.Errorf("remove wrong: %v", titles(cat.Children))
	}
	if !d.Remove(cat) {
		t.Fatal("Remove of a root returned false")
	}
	if len(d.Roots) != 0 {
		t.Errorf("root not removed")
	}
}

func TestRemoveDone(t *testing.T) {
	d := &todo.Document{}
	cat := &todo.Item{Kind: todo.Category, Level: 1, Title: "Cat"}
	d.AppendRoot(cat)
	cat.AppendChild(todo.NewTask("a", "", true))  // done
	cat.AppendChild(todo.NewTask("b", "", false)) // open
	cat.AppendChild(todo.NewTask("c", "", true))  // done

	if n := d.RemoveDone(); n != 2 {
		t.Fatalf("RemoveDone removed %d, want 2", n)
	}
	if got := titles(cat.Children); len(got) != 1 || got[0] != "b" {
		t.Errorf("only the open task should remain, got %v", got)
	}
}

func TestRemoveDoneKeepsUnfinishedSubtree(t *testing.T) {
	d := &todo.Document{}
	cat := &todo.Item{Kind: todo.Category, Level: 1, Title: "Cat"}
	d.AppendRoot(cat)

	// p1 is done and its whole subtree is done -> the subtree is removed.
	p1 := todo.NewTask("p1", "", true)
	p1.AppendChild(todo.NewTask("p1c", "", true))
	// p2 is done but still holds an unfinished child -> it must be kept, so no
	// unfinished work is discarded.
	p2 := todo.NewTask("p2", "", true)
	p2c := todo.NewTask("p2c", "", false)
	p2.AppendChild(p2c)
	cat.AppendChild(p1)
	cat.AppendChild(p2)

	if n := d.RemoveDone(); n != 2 {
		t.Fatalf("RemoveDone removed %d, want 2 (only p1's subtree)", n)
	}
	if got := titles(cat.Children); len(got) != 1 || got[0] != "p2" {
		t.Fatalf("cat should keep only p2, got %v", got)
	}
	if len(p2.Children) != 1 || p2.Children[0] != p2c {
		t.Errorf("p2 must still hold its unfinished child p2c")
	}
}

func TestRemovableDoneMatchesRemoved(t *testing.T) {
	d := &todo.Document{}
	cat := &todo.Item{Kind: todo.Category, Level: 1, Title: "Cat"}
	d.AppendRoot(cat)
	cat.AppendChild(todo.NewTask("x", "", true)) // done leaf
	p := todo.NewTask("p", "", true)             // done parent, done child
	p.AppendChild(todo.NewTask("pc", "", true))
	cat.AppendChild(p)
	cat.AppendChild(todo.NewTask("y", "", false)) // open

	want := d.RemovableDone()
	if want != 3 {
		t.Fatalf("RemovableDone = %d, want 3 (x, p, pc)", want)
	}
	if got := d.RemoveDone(); got != want {
		t.Errorf("RemoveDone removed %d, RemovableDone predicted %d", got, want)
	}
}

func titles(items []*todo.Item) []string {
	out := make([]string, len(items))
	for i, it := range items {
		out[i] = it.Title
	}
	return out
}
