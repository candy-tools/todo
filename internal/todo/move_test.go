package todo_test

import (
	"testing"

	"github.com/candy-tools/todo/internal/todo"
)

// find returns the first item in the document whose title matches, or nil.
func find(d *todo.Document, title string) *todo.Item {
	var out *todo.Item
	var walk func([]*todo.Item)
	walk = func(items []*todo.Item) {
		for _, it := range items {
			if it.Title == title {
				out = it
				return
			}
			walk(it.Children)
		}
	}
	walk(d.Roots)
	return out
}

func TestMoveDownSwapsWithNextSibling(t *testing.T) {
	d := todo.Parse("# Work\n\n- [ ] a\n- [ ] b\n- [ ] c\n")
	a := find(d, "a")
	if !d.MoveDown(a) {
		t.Fatal("MoveDown should report it moved")
	}
	if got := titles(find(d, "Work").Children); len(got) != 3 || got[0] != "b" || got[1] != "a" || got[2] != "c" {
		t.Errorf("order after MoveDown(a) = %v, want [b a c]", got)
	}
}

func TestMoveUpSwapsWithPrevSibling(t *testing.T) {
	d := todo.Parse("# Work\n\n- [ ] a\n- [ ] b\n- [ ] c\n")
	c := find(d, "c")
	if !d.MoveUp(c) {
		t.Fatal("MoveUp should report it moved")
	}
	if got := titles(find(d, "Work").Children); got[1] != "c" || got[2] != "b" {
		t.Errorf("order after MoveUp(c) = %v, want [a c b]", got)
	}
}

func TestMoveUpClampsAtStart(t *testing.T) {
	d := todo.Parse("# Work\n\n- [ ] a\n- [ ] b\n")
	if d.MoveUp(find(d, "a")) {
		t.Error("MoveUp on the first sibling should be a no-op")
	}
	if got := titles(find(d, "Work").Children); got[0] != "a" || got[1] != "b" {
		t.Errorf("order unchanged expected, got %v", got)
	}
}

func TestMoveDownClampsAtEnd(t *testing.T) {
	d := todo.Parse("# Work\n\n- [ ] a\n- [ ] b\n")
	if d.MoveDown(find(d, "b")) {
		t.Error("MoveDown on the last sibling should be a no-op")
	}
}

func TestMoveDownTaskDescendsIntoSubcategory(t *testing.T) {
	// A task that is the last one in its category flows down into that category's
	// first subcategory, landing as the subcategory's first task (it can't sit
	// after a subheader, so "down" carries it in).
	d := todo.Parse("# Work\n\n- [ ] a\n- [ ] b\n\n## Sub\n\n- [ ] w1\n")
	b := find(d, "b")
	if !d.MoveDown(b) {
		t.Fatal("MoveDown should carry the last task into the subcategory")
	}
	sub := find(d, "Sub")
	if b.Parent != sub {
		t.Errorf("b's parent = %v, want Sub", b.Parent)
	}
	if got := titles(sub.Children); len(got) != 2 || got[0] != "b" || got[1] != "w1" {
		t.Errorf("Sub's children = %v, want [b w1]", got)
	}
	if got := titles(find(d, "Work").Children); len(got) != 2 || got[0] != "a" || got[1] != "Sub" {
		t.Errorf("Work's children = %v, want [a Sub]", got)
	}
}

func TestMoveDownTaskBubblesToNextCategory(t *testing.T) {
	// With no subcategory to descend into, the last task of a category crosses to
	// become the first task of the next category in document order.
	d := todo.Parse("# Work\n\n- [ ] b\n\n# Personal\n\n- [ ] p1\n")
	b := find(d, "b")
	if !d.MoveDown(b) {
		t.Fatal("MoveDown should carry the task across to the next category")
	}
	if b.Parent != find(d, "Personal") {
		t.Errorf("b's parent = %v, want Personal", b.Parent)
	}
	if got := titles(find(d, "Personal").Children); len(got) != 2 || got[0] != "b" || got[1] != "p1" {
		t.Errorf("Personal's children = %v, want [b p1]", got)
	}
	if got := titles(find(d, "Work").Children); len(got) != 0 {
		t.Errorf("Work should be empty, got %v", got)
	}
}

func TestMoveUpTaskAscendsToPreviousCategory(t *testing.T) {
	// The first task of a category crosses up to become the last task of the
	// previous category.
	d := todo.Parse("# Work\n\n- [ ] a\n\n# Personal\n\n- [ ] b\n")
	b := find(d, "b")
	if !d.MoveUp(b) {
		t.Fatal("MoveUp should carry the first task back to the previous category")
	}
	if b.Parent != find(d, "Work") {
		t.Errorf("b's parent = %v, want Work", b.Parent)
	}
	if got := titles(find(d, "Work").Children); len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Errorf("Work's children = %v, want [a b]", got)
	}
}

func TestMoveUpTaskAscendsIntoParentCategory(t *testing.T) {
	// The first task of a subcategory crosses up into its parent category, as that
	// category's last task (before its subcategories).
	d := todo.Parse("# Work\n\n- [ ] a\n\n## Sub\n\n- [ ] b\n")
	b := find(d, "b")
	if !d.MoveUp(b) {
		t.Fatal("MoveUp should lift the first subcategory task into the parent")
	}
	if b.Parent != find(d, "Work") {
		t.Errorf("b's parent = %v, want Work", b.Parent)
	}
	if got := titles(find(d, "Work").Children); len(got) != 3 || got[0] != "a" || got[1] != "b" || got[2] != "Sub" {
		t.Errorf("Work's children = %v, want [a b Sub]", got)
	}
}

func TestMoveUpTaskDescendsIntoPreviousCategorysSubtree(t *testing.T) {
	// Crossing up into a previous category that has its own subcategory lands the
	// task at the end of that subtree (the visible row just above), not at a
	// shallow spot — so up moves one line at a time.
	d := todo.Parse("# Work\n\n## Sub\n\n- [ ] w1\n\n# Personal\n\n- [ ] b\n")
	b := find(d, "b")
	if !d.MoveUp(b) {
		t.Fatal("MoveUp should carry the task into the previous category's subtree")
	}
	sub := find(d, "Sub")
	if b.Parent != sub {
		t.Errorf("b's parent = %v, want Sub", b.Parent)
	}
	if got := titles(sub.Children); len(got) != 2 || got[0] != "w1" || got[1] != "b" {
		t.Errorf("Sub's children = %v, want [w1 b]", got)
	}
	if got := titles(find(d, "Personal").Children); len(got) != 0 {
		t.Errorf("Personal should be empty, got %v", got)
	}
}

func TestMoveTaskAcrossBoundaryIsReversible(t *testing.T) {
	// "Natural" up/down: a ↑ across a boundary is undone by a ↓ — the task returns
	// to where it started, even out of a nested previous category.
	d := todo.Parse("# Work\n\n## Sub\n\n- [ ] w1\n\n# Personal\n\n- [ ] b\n")
	b := find(d, "b")
	if !d.MoveUp(b) || b.Parent.Title != "Sub" { // up: into the end of Work's subtree
		t.Fatalf("MoveUp should land b in Sub, parent=%v", b.Parent)
	}
	if !d.MoveDown(b) || b.Parent.Title != "Personal" { // down: back out to Personal
		t.Fatalf("MoveDown should return b to Personal, parent=%v", b.Parent)
	}
	if got := titles(find(d, "Personal").Children); len(got) != 1 || got[0] != "b" {
		t.Errorf("Personal's children = %v, want [b]", got)
	}
}

func TestMoveSubtaskDoesNotCrossCategories(t *testing.T) {
	// Sub-task nesting is changed with indent/outdent, not up/down: a lone subtask
	// neither bubbles out nor crosses categories on up/down.
	d := todo.Parse("# Work\n\n- [ ] p\n  - [ ] c1\n\n# Personal\n")
	c1 := find(d, "c1")
	if d.MoveDown(c1) {
		t.Error("a subtask should not cross categories on MoveDown")
	}
	if d.MoveUp(c1) {
		t.Error("a subtask should not cross categories on MoveUp")
	}
	if c1.Parent != find(d, "p") {
		t.Errorf("c1 should stay a subtask of p, got parent %v", c1.Parent)
	}
}

func TestMoveTaskAcrossCategoriesRoundTrips(t *testing.T) {
	d := todo.Parse("# Work\n\n- [ ] b\n\n## Sub\n\n- [ ] w1\n")
	d.MoveDown(find(d, "b")) // b descends into Sub
	re := todo.Parse(d.Render())
	b := find(re, "b")
	if b == nil || b.Parent == nil || b.Parent.Title != "Sub" {
		t.Errorf("after round-trip b should nest under Sub:\n%s", d.Render())
	}
}

func TestMoveDownSubcategoryDescendsIntoNextCategory(t *testing.T) {
	// The last subcategory of a category crosses to become the first subcategory of
	// the next category, re-levelled to sit one below its new parent.
	d := todo.Parse("# Work\n\n## Sub\n\n- [ ] x\n\n# Personal\n")
	sub := find(d, "Sub")
	if !d.MoveDown(sub) {
		t.Fatal("MoveDown should carry the subcategory across to the next category")
	}
	personal := find(d, "Personal")
	if sub.Parent != personal || sub.Level != 2 {
		t.Errorf("Sub should be a level-2 child of Personal, got parent=%v level=%d", sub.Parent, sub.Level)
	}
	if got := titles(personal.Children); len(got) != 1 || got[0] != "Sub" {
		t.Errorf("Personal's children = %v, want [Sub]", got)
	}
	if got := titles(find(d, "Work").Children); len(got) != 0 {
		t.Errorf("Work should be empty, got %v", got)
	}
}

func TestMoveUpSubcategoryAscendsIntoPreviousCategory(t *testing.T) {
	d := todo.Parse("# Work\n\n# Personal\n\n## Sub\n\n- [ ] x\n")
	sub := find(d, "Sub")
	if !d.MoveUp(sub) {
		t.Fatal("MoveUp should carry the subcategory back into the previous category")
	}
	work := find(d, "Work")
	if sub.Parent != work || sub.Level != 2 {
		t.Errorf("Sub should be a level-2 child of Work, got parent=%v level=%d", sub.Parent, sub.Level)
	}
	if got := titles(find(d, "Personal").Children); len(got) != 0 {
		t.Errorf("Personal should be empty, got %v", got)
	}
}

func TestMoveCategoryAcrossRelevelsSubtreeAndRoundTrips(t *testing.T) {
	// Crossing a subcategory that has its own subcategory re-levels the whole
	// subtree and still round-trips.
	d := todo.Parse("# Work\n\n## Sub\n\n### Deep\n\n# Personal\n")
	sub, deep := find(d, "Sub"), find(d, "Deep")
	if !d.MoveDown(sub) {
		t.Fatal("MoveDown(Sub) should cross into Personal")
	}
	if sub.Level != 2 || deep.Level != 3 {
		t.Errorf("levels after cross: Sub=%d Deep=%d, want 2 and 3", sub.Level, deep.Level)
	}
	re := todo.Parse(d.Render())
	rs, rd := find(re, "Sub"), find(re, "Deep")
	if rs == nil || rs.Parent == nil || rs.Parent.Title != "Personal" {
		t.Errorf("after round-trip Sub should nest under Personal:\n%s", d.Render())
	}
	if rd == nil || rd.Parent == nil || rd.Parent.Title != "Sub" {
		t.Errorf("after round-trip Deep should nest under Sub:\n%s", d.Render())
	}
}

func TestMoveRootCategoriesReorder(t *testing.T) {
	d := todo.Parse("# A\n\n# B\n")
	if !d.MoveDown(find(d, "A")) {
		t.Fatal("root categories should reorder")
	}
	if got := titles(d.Roots); got[0] != "B" || got[1] != "A" {
		t.Errorf("root order after MoveDown(A) = %v, want [B A]", got)
	}
}

func TestIndentMakesTaskSubtaskOfPrevious(t *testing.T) {
	d := todo.Parse("# Work\n\n- [ ] a\n- [ ] b\n")
	a, b := find(d, "a"), find(d, "b")
	if !d.Indent(b) {
		t.Fatal("Indent should report it moved")
	}
	if b.Parent != a {
		t.Errorf("b's parent = %v, want a", b.Parent)
	}
	if got := titles(a.Children); len(got) != 1 || got[0] != "b" {
		t.Errorf("a's children = %v, want [b]", got)
	}
	if got := titles(find(d, "Work").Children); len(got) != 1 || got[0] != "a" {
		t.Errorf("Work's children = %v, want [a]", got)
	}
}

func TestIndentFirstItemIsNoOp(t *testing.T) {
	d := todo.Parse("# Work\n\n- [ ] a\n- [ ] b\n")
	if d.Indent(find(d, "a")) {
		t.Error("Indent on the first sibling (no predecessor) should be a no-op")
	}
}

func TestIndentAcrossKindBlocked(t *testing.T) {
	// A subcategory whose predecessor is a task can't indent (a category may only
	// nest under a category).
	d := todo.Parse("# Work\n\n- [ ] task\n\n## Sub\n")
	if d.Indent(find(d, "Sub")) {
		t.Error("a category should not indent under a preceding task")
	}
}

func TestIndentCategoryBecomesSubcategoryAndRelevels(t *testing.T) {
	// A(1), B(1) where B already has a subcategory C(2). Indenting B under A makes
	// B a level-2 subcategory, and C must follow to level 3.
	d := todo.Parse("# A\n\n# B\n\n## C\n")
	a, b, c := find(d, "A"), find(d, "B"), find(d, "C")
	if !d.Indent(b) {
		t.Fatal("Indent(B) should move it under A")
	}
	if b.Parent != a || b.Level != 2 {
		t.Errorf("B should be a level-2 child of A, got parent=%v level=%d", b.Parent, b.Level)
	}
	if c.Parent != b || c.Level != 3 {
		t.Errorf("C should follow to a level-3 child of B, got parent=%v level=%d", c.Parent, c.Level)
	}
}

func TestIndentCategoryRoundTrips(t *testing.T) {
	// After indenting a category, the re-levelled tree must serialise and reparse
	// to the same nesting (the parser nests a header only under a shallower one).
	d := todo.Parse("# A\n\n# B\n\n## C\n")
	d.Indent(find(d, "B"))
	re := todo.Parse(d.Render())
	b, c := find(re, "B"), find(re, "C")
	if b == nil || b.Parent == nil || b.Parent.Title != "A" {
		t.Errorf("after round-trip B should nest under A:\n%s", d.Render())
	}
	if c == nil || c.Parent == nil || c.Parent.Title != "B" {
		t.Errorf("after round-trip C should nest under B:\n%s", d.Render())
	}
}

func TestOutdentSubtaskBecomesSiblingOfParentAfterIt(t *testing.T) {
	// Work > p > [c1, c2]. Outdenting c1 lifts it to be p's sibling, inserted
	// right after p; c2 stays under p.
	d := todo.Parse("# Work\n\n- [ ] p\n  - [ ] c1\n  - [ ] c2\n")
	work, p, c1 := find(d, "Work"), find(d, "p"), find(d, "c1")
	if !d.Outdent(c1) {
		t.Fatal("Outdent(c1) should report it moved")
	}
	if c1.Parent != work {
		t.Errorf("c1's parent = %v, want Work", c1.Parent)
	}
	if got := titles(work.Children); len(got) != 2 || got[0] != "p" || got[1] != "c1" {
		t.Errorf("Work's children = %v, want [p c1]", got)
	}
	if got := titles(p.Children); len(got) != 1 || got[0] != "c2" {
		t.Errorf("p's children = %v, want [c2]", got)
	}
}

func TestOutdentTopLevelTaskBlocked(t *testing.T) {
	// A task directly under a category can't outdent — it would become a root task
	// (there are none) or land after a subcategory.
	d := todo.Parse("# Work\n\n- [ ] a\n")
	if d.Outdent(find(d, "a")) {
		t.Error("a top-level task should not outdent out of its category")
	}
	if got := titles(find(d, "Work").Children); len(got) != 1 || got[0] != "a" {
		t.Errorf("a must stay under Work, got %v", got)
	}
}

func TestOutdentRootCategoryBlocked(t *testing.T) {
	d := todo.Parse("# A\n")
	if d.Outdent(find(d, "A")) {
		t.Error("a root category has no parent to outdent past")
	}
}

func TestOutdentSubcategoryRelevels(t *testing.T) {
	// A(1) > B(2) > C(3). Outdenting B lifts it to a root sibling of A at level 1,
	// and C follows down to level 2.
	d := todo.Parse("# A\n\n## B\n\n### C\n")
	b, c := find(d, "B"), find(d, "C")
	if !d.Outdent(b) {
		t.Fatal("Outdent(B) should move it to root")
	}
	if b.Parent != nil || b.Level != 1 {
		t.Errorf("B should be a level-1 root, got parent=%v level=%d", b.Parent, b.Level)
	}
	if c.Parent != b || c.Level != 2 {
		t.Errorf("C should follow to a level-2 child of B, got parent=%v level=%d", c.Parent, c.Level)
	}
	if got := titles(d.Roots); len(got) != 2 || got[0] != "A" || got[1] != "B" {
		t.Errorf("roots = %v, want [A B]", got)
	}
}

func TestOutdentCategoryRoundTrips(t *testing.T) {
	d := todo.Parse("# A\n\n## B\n\n### C\n")
	d.Outdent(find(d, "B"))
	re := todo.Parse(d.Render())
	b, c := find(re, "B"), find(re, "C")
	if b == nil || b.Parent != nil {
		t.Errorf("after round-trip B should be a root:\n%s", d.Render())
	}
	if c == nil || c.Parent == nil || c.Parent.Title != "B" {
		t.Errorf("after round-trip C should still nest under B:\n%s", d.Render())
	}
}
