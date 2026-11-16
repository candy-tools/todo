package todo_test

import (
	"errors"
	"testing"

	"github.com/candy-tools/todo/internal/todo"
	"github.com/candy-tools/todo/internal/todo/todotest"
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

// step runs one move-mode step on it the way the TUI does: StepPlacement
// works out where the step goes, and Move, the op that decides validity, puts
// it there. Every placement the query gives must be one Move accepts. It
// reports whether the item moved.
func step(t *testing.T, d *todo.Document, it *todo.Item, s todo.MoveStep) bool {
	t.Helper()
	d.AssignIDs()
	parentID, afterID, err := d.StepPlacement(it.ID, s, nil)
	if err != nil {
		return false
	}
	if err := d.Move(it.ID, parentID, afterID); err != nil {
		t.Fatalf("Move refused the step's placement (%q, %q): %v", parentID, afterID, err)
	}
	return true
}

// TestStepPlacementLeavesDocumentUnchanged asks for every step of every item:
// the query never changes the document it is asked on.
func TestStepPlacementLeavesDocumentUnchanged(t *testing.T) {
	d := todotest.Parse(t, "# Work\n\n- [ ] a\n  - [ ] a1\n- [ ] b\n\n## Sub\n\n- [ ] w1\n\n# Personal\n\n- [ ] p1\n")
	d.AssignIDs()
	want := todotest.Render(d)
	for _, it := range d.Enumerate() {
		for _, s := range []todo.MoveStep{todo.StepUp, todo.StepDown, todo.StepIn, todo.StepOut} {
			_, _, _ = d.StepPlacement(it.ID, s, nil)
			_, _, _ = d.StepPlacement(it.ID, s, func(string) bool { return false })
		}
	}
	if got := todotest.Render(d); got != want {
		t.Errorf("the document changed:\n%s\nwant:\n%s", got, want)
	}
}

// TestStepPlacementSkipsHiddenItems hides b and d: a step down from a
// passes b and the shown c, landing after c, and a step down from c, with
// only hidden items left below it, goes nowhere.
func TestStepPlacementSkipsHiddenItems(t *testing.T) {
	d := todotest.Parse(t, "# Work\n\n- [ ] a\n- [ ] b\n- [ ] c\n- [ ] d\n")
	d.AssignIDs()
	hidden := map[string]bool{find(d, "b").ID: true, find(d, "d").ID: true}
	shown := func(id string) bool { return !hidden[id] }

	parentID, afterID, err := d.StepPlacement(find(d, "a").ID, todo.StepDown, shown)
	if err != nil || parentID != find(d, "Work").ID || afterID != find(d, "c").ID {
		t.Errorf("a down = (%q, %q, %v), want after c in Work", parentID, afterID, err)
	}
	if _, _, err := d.StepPlacement(find(d, "c").ID, todo.StepDown, shown); !errors.Is(err, todo.ErrNowhere) {
		t.Errorf("c down past only hidden items = %v, want ErrNowhere", err)
	}
	if _, _, err := d.StepPlacement(find(d, "c").ID, todo.StepDown, nil); err != nil {
		t.Errorf("without a view, c steps down past d: %v", err)
	}
}

// TestStepPlacementSkipsHiddenItemsUp mirrors the step down: with a and c
// hidden, a step up from d passes c and the shown b, landing after a, and a
// step up from b, with only hidden items above it, goes nowhere.
func TestStepPlacementSkipsHiddenItemsUp(t *testing.T) {
	d := todotest.Parse(t, "# Work\n\n- [ ] a\n- [ ] b\n- [ ] c\n- [ ] d\n")
	d.AssignIDs()
	hidden := map[string]bool{find(d, "a").ID: true, find(d, "c").ID: true}
	shown := func(id string) bool { return !hidden[id] }

	parentID, afterID, err := d.StepPlacement(find(d, "d").ID, todo.StepUp, shown)
	if err != nil || parentID != find(d, "Work").ID || afterID != find(d, "a").ID {
		t.Errorf("d up = (%q, %q, %v), want after a in Work", parentID, afterID, err)
	}
	if _, _, err := d.StepPlacement(find(d, "b").ID, todo.StepUp, shown); !errors.Is(err, todo.ErrNowhere) {
		t.Errorf("b up past only hidden items = %v, want ErrNowhere", err)
	}
	if _, _, err := d.StepPlacement(find(d, "b").ID, todo.StepUp, nil); err != nil {
		t.Errorf("without a view, b steps up past a: %v", err)
	}
}

// TestStepPlacementStopsAtNewParent crosses a task into the next category,
// and back up into the one before: entering another parent ends the step even
// though no shown item was passed.
func TestStepPlacementStopsAtNewParent(t *testing.T) {
	d := todotest.Parse(t, "# Work\n\n- [ ] a\n\n# Personal\n\n- [ ] p1\n")
	d.AssignIDs()
	none := func(string) bool { return false }
	parentID, afterID, err := d.StepPlacement(find(d, "a").ID, todo.StepDown, none)
	if err != nil || parentID != find(d, "Personal").ID || afterID != "" {
		t.Errorf("a down = (%q, %q, %v), want first in Personal", parentID, afterID, err)
	}
	parentID, afterID, err = d.StepPlacement(find(d, "p1").ID, todo.StepUp, none)
	if err != nil || parentID != find(d, "Work").ID || afterID != find(d, "a").ID {
		t.Errorf("p1 up = (%q, %q, %v), want last in Work, after a", parentID, afterID, err)
	}
}

// TestStepPlacementInAndOutIgnoreShown: a step in nests under the sibling
// before even when the view hides it, and a step out lifts the item out of
// its parent whatever the view shows; the caller decides what a hidden parent
// means for its view.
func TestStepPlacementInAndOutIgnoreShown(t *testing.T) {
	d := todotest.Parse(t, "# Work\n\n- [ ] a\n- [ ] b\n  - [ ] s\n")
	d.AssignIDs()
	none := func(string) bool { return false }
	if p, a, err := d.StepPlacement(find(d, "b").ID, todo.StepIn, none); !errors.Is(err, todo.ErrTooDeep) {
		t.Errorf("b (with a subtask) in = (%q, %q, %v), want ErrTooDeep", p, a, err)
	}
	d = todotest.Parse(t, "# Work\n\n- [ ] a\n- [ ] b\n- [ ] c\n  - [ ] s\n")
	d.AssignIDs()
	parentID, afterID, err := d.StepPlacement(find(d, "b").ID, todo.StepIn, none)
	if err != nil || parentID != find(d, "a").ID || afterID != "" {
		t.Errorf("b in = (%q, %q, %v), want under a", parentID, afterID, err)
	}
	parentID, afterID, err = d.StepPlacement(find(d, "s").ID, todo.StepOut, none)
	if err != nil || parentID != find(d, "Work").ID || afterID != find(d, "c").ID {
		t.Errorf("s out = (%q, %q, %v), want after c in Work", parentID, afterID, err)
	}
}

// TestStepPlacementReportsMoveError: a step Move refuses reports Move's error,
// so a caller can word it, and a step with nowhere to go reports ErrNowhere.
func TestStepPlacementReportsMoveError(t *testing.T) {
	d := todotest.Parse(t, "# Work\n\n- [ ] a\n  - [ ] s1\n  - [ ] s2\n")
	d.AssignIDs()
	if _, _, err := d.StepPlacement(find(d, "s2").ID, todo.StepIn, nil); !errors.Is(err, todo.ErrTooDeep) {
		t.Errorf("s2 in under s1 = %v, want ErrTooDeep", err)
	}
	if _, _, err := d.StepPlacement(find(d, "a").ID, todo.StepIn, nil); !errors.Is(err, todo.ErrNowhere) {
		t.Errorf("a in with no sibling before = %v, want ErrNowhere", err)
	}
	if _, _, err := d.StepPlacement(find(d, "a").ID, todo.StepOut, nil); !errors.Is(err, todo.ErrNowhere) {
		t.Errorf("a out of its category = %v, want ErrNowhere", err)
	}
}

// TestStepPlacementMissingItem asks about an ID the document does not hold.
func TestStepPlacementMissingItem(t *testing.T) {
	d := todotest.Parse(t, "# Work\n\n- [ ] a\n")
	if _, _, err := d.StepPlacement("zzzzzzz", todo.StepDown, nil); !errors.Is(err, todo.ErrNotFound) {
		t.Errorf("a missing item = %v, want ErrNotFound", err)
	}
}

func TestMoveDownSwapsWithNextSibling(t *testing.T) {
	d := todotest.Parse(t, "# Work\n\n- [ ] a\n- [ ] b\n- [ ] c\n")
	a := find(d, "a")
	if !step(t, d, a, todo.StepDown) {
		t.Fatal("MoveDown should report it moved")
	}
	if got := titles(find(d, "Work").Children); len(got) != 3 || got[0] != "b" || got[1] != "a" || got[2] != "c" {
		t.Errorf("order after MoveDown(a) = %v, want [b a c]", got)
	}
}

func TestMoveUpSwapsWithPrevSibling(t *testing.T) {
	d := todotest.Parse(t, "# Work\n\n- [ ] a\n- [ ] b\n- [ ] c\n")
	c := find(d, "c")
	if !step(t, d, c, todo.StepUp) {
		t.Fatal("MoveUp should report it moved")
	}
	if got := titles(find(d, "Work").Children); got[1] != "c" || got[2] != "b" {
		t.Errorf("order after MoveUp(c) = %v, want [a c b]", got)
	}
}

func TestMoveUpClampsAtStart(t *testing.T) {
	d := todotest.Parse(t, "# Work\n\n- [ ] a\n- [ ] b\n")
	if step(t, d, find(d, "a"), todo.StepUp) {
		t.Error("MoveUp on the first sibling should be a no-op")
	}
	if got := titles(find(d, "Work").Children); got[0] != "a" || got[1] != "b" {
		t.Errorf("order unchanged expected, got %v", got)
	}
}

func TestMoveDownClampsAtEnd(t *testing.T) {
	d := todotest.Parse(t, "# Work\n\n- [ ] a\n- [ ] b\n")
	if step(t, d, find(d, "b"), todo.StepDown) {
		t.Error("MoveDown on the last sibling should be a no-op")
	}
}

func TestMoveDownTaskDescendsIntoSubcategory(t *testing.T) {
	// A task that is the last one in its category flows down into that category's
	// first subcategory, landing as the subcategory's first task (it can't sit
	// after a subheader, so "down" carries it in).
	d := todotest.Parse(t, "# Work\n\n- [ ] a\n- [ ] b\n\n## Sub\n\n- [ ] w1\n")
	b := find(d, "b")
	if !step(t, d, b, todo.StepDown) {
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
	d := todotest.Parse(t, "# Work\n\n- [ ] b\n\n# Personal\n\n- [ ] p1\n")
	b := find(d, "b")
	if !step(t, d, b, todo.StepDown) {
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
	d := todotest.Parse(t, "# Work\n\n- [ ] a\n\n# Personal\n\n- [ ] b\n")
	b := find(d, "b")
	if !step(t, d, b, todo.StepUp) {
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
	d := todotest.Parse(t, "# Work\n\n- [ ] a\n\n## Sub\n\n- [ ] b\n")
	b := find(d, "b")
	if !step(t, d, b, todo.StepUp) {
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
	d := todotest.Parse(t, "# Work\n\n## Sub\n\n- [ ] w1\n\n# Personal\n\n- [ ] b\n")
	b := find(d, "b")
	if !step(t, d, b, todo.StepUp) {
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
	d := todotest.Parse(t, "# Work\n\n## Sub\n\n- [ ] w1\n\n# Personal\n\n- [ ] b\n")
	b := find(d, "b")
	if !step(t, d, b, todo.StepUp) || b.Parent.Title != "Sub" { // up: into the end of Work's subtree
		t.Fatalf("MoveUp should land b in Sub, parent=%v", b.Parent)
	}
	if !step(t, d, b, todo.StepDown) || b.Parent.Title != "Personal" { // down: back out to Personal
		t.Fatalf("MoveDown should return b to Personal, parent=%v", b.Parent)
	}
	if got := titles(find(d, "Personal").Children); len(got) != 1 || got[0] != "b" {
		t.Errorf("Personal's children = %v, want [b]", got)
	}
}

func TestMoveSubtaskDoesNotCrossCategories(t *testing.T) {
	// Sub-task nesting is changed with indent/outdent, not up/down: a lone subtask
	// neither bubbles out nor crosses categories on up/down.
	d := todotest.Parse(t, "# Work\n\n- [ ] p\n  - [ ] c1\n\n# Personal\n")
	c1 := find(d, "c1")
	if step(t, d, c1, todo.StepDown) {
		t.Error("a subtask should not cross categories on MoveDown")
	}
	if step(t, d, c1, todo.StepUp) {
		t.Error("a subtask should not cross categories on MoveUp")
	}
	if c1.Parent != find(d, "p") {
		t.Errorf("c1 should stay a subtask of p, got parent %v", c1.Parent)
	}
}

func TestMoveTaskAcrossCategoriesRoundTrips(t *testing.T) {
	d := todotest.Parse(t, "# Work\n\n- [ ] b\n\n## Sub\n\n- [ ] w1\n")
	step(t, d, find(d, "b"), todo.StepDown) // b descends into Sub
	re := todotest.Parse(t, todotest.Render(d))
	b := find(re, "b")
	if b == nil || b.Parent == nil || b.Parent.Title != "Sub" {
		t.Errorf("after round-trip b should nest under Sub:\n%s", todotest.Render(d))
	}
}

func TestMoveDownSubcategoryDescendsIntoNextCategory(t *testing.T) {
	// The last subcategory of a category crosses to become the first subcategory of
	// the next category, re-levelled to sit one below its new parent.
	d := todotest.Parse(t, "# Work\n\n## Sub\n\n- [ ] x\n\n# Personal\n")
	sub := find(d, "Sub")
	if !step(t, d, sub, todo.StepDown) {
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

func TestMoveDownSubcategoryLandsAfterTasksBeforeSubcategories(t *testing.T) {
	// Crossing into a category that already holds tasks and subcategories, the
	// moved subcategory becomes the first subcategory: after p, before Other.
	d := todotest.Parse(t, "# Work\n\n## Sub\n\n- [ ] x\n\n# Personal\n\n- [ ] p\n\n## Other\n")
	if !step(t, d, find(d, "Sub"), todo.StepDown) {
		t.Fatal("MoveDown should carry the subcategory across to the next category")
	}
	if got := titles(find(d, "Personal").Children); len(got) != 3 || got[0] != "p" || got[1] != "Sub" || got[2] != "Other" {
		t.Errorf("Personal's children = %v, want [p Sub Other]", got)
	}
}

func TestMoveUpSubcategoryAscendsIntoPreviousCategory(t *testing.T) {
	d := todotest.Parse(t, "# Work\n\n# Personal\n\n## Sub\n\n- [ ] x\n")
	sub := find(d, "Sub")
	if !step(t, d, sub, todo.StepUp) {
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
	d := todotest.Parse(t, "# Work\n\n## Sub\n\n### Deep\n\n# Personal\n")
	sub, deep := find(d, "Sub"), find(d, "Deep")
	if !step(t, d, sub, todo.StepDown) {
		t.Fatal("MoveDown(Sub) should cross into Personal")
	}
	if sub.Level != 2 || deep.Level != 3 {
		t.Errorf("levels after cross: Sub=%d Deep=%d, want 2 and 3", sub.Level, deep.Level)
	}
	re := todotest.Parse(t, todotest.Render(d))
	rs, rd := find(re, "Sub"), find(re, "Deep")
	if rs == nil || rs.Parent == nil || rs.Parent.Title != "Personal" {
		t.Errorf("after round-trip Sub should nest under Personal:\n%s", todotest.Render(d))
	}
	if rd == nil || rd.Parent == nil || rd.Parent.Title != "Sub" {
		t.Errorf("after round-trip Deep should nest under Sub:\n%s", todotest.Render(d))
	}
}

func TestMoveRootCategoriesReorder(t *testing.T) {
	d := todotest.Parse(t, "# A\n\n# B\n")
	if !step(t, d, find(d, "A"), todo.StepDown) {
		t.Fatal("root categories should reorder")
	}
	if got := titles(d.Roots); got[0] != "B" || got[1] != "A" {
		t.Errorf("root order after MoveDown(A) = %v, want [B A]", got)
	}
}

func TestIndentMakesTaskSubtaskOfPrevious(t *testing.T) {
	d := todotest.Parse(t, "# Work\n\n- [ ] a\n- [ ] b\n")
	a, b := find(d, "a"), find(d, "b")
	if !step(t, d, b, todo.StepIn) {
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
	d := todotest.Parse(t, "# Work\n\n- [ ] a\n- [ ] b\n")
	if step(t, d, find(d, "a"), todo.StepIn) {
		t.Error("Indent on the first sibling (no predecessor) should be a no-op")
	}
}

func TestIndentAcrossKindBlocked(t *testing.T) {
	// A subcategory whose predecessor is a task can't indent (a category may only
	// nest under a category).
	d := todotest.Parse(t, "# Work\n\n- [ ] task\n\n## Sub\n")
	if step(t, d, find(d, "Sub"), todo.StepIn) {
		t.Error("a category should not indent under a preceding task")
	}
}

func TestIndentCategoryBecomesSubcategoryAndRelevels(t *testing.T) {
	// A(1), B(1) where B already has a subcategory C(2). Indenting B under A makes
	// B a level-2 subcategory, and C must follow to level 3.
	d := todotest.Parse(t, "# A\n\n# B\n\n## C\n")
	a, b, c := find(d, "A"), find(d, "B"), find(d, "C")
	if !step(t, d, b, todo.StepIn) {
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
	d := todotest.Parse(t, "# A\n\n# B\n\n## C\n")
	step(t, d, find(d, "B"), todo.StepIn)
	re := todotest.Parse(t, todotest.Render(d))
	b, c := find(re, "B"), find(re, "C")
	if b == nil || b.Parent == nil || b.Parent.Title != "A" {
		t.Errorf("after round-trip B should nest under A:\n%s", todotest.Render(d))
	}
	if c == nil || c.Parent == nil || c.Parent.Title != "B" {
		t.Errorf("after round-trip C should nest under B:\n%s", todotest.Render(d))
	}
}

func TestOutdentSubtaskBecomesSiblingOfParentAfterIt(t *testing.T) {
	// Work > p > [c1, c2]. Outdenting c1 lifts it to be p's sibling, inserted
	// right after p; c2 stays under p.
	d := todotest.Parse(t, "# Work\n\n- [ ] p\n  - [ ] c1\n  - [ ] c2\n")
	work, p, c1 := find(d, "Work"), find(d, "p"), find(d, "c1")
	if !step(t, d, c1, todo.StepOut) {
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
	d := todotest.Parse(t, "# Work\n\n- [ ] a\n")
	if step(t, d, find(d, "a"), todo.StepOut) {
		t.Error("a top-level task should not outdent out of its category")
	}
	if got := titles(find(d, "Work").Children); len(got) != 1 || got[0] != "a" {
		t.Errorf("a must stay under Work, got %v", got)
	}
}

func TestOutdentRootCategoryBlocked(t *testing.T) {
	d := todotest.Parse(t, "# A\n")
	if step(t, d, find(d, "A"), todo.StepOut) {
		t.Error("a root category has no parent to outdent past")
	}
}

func TestOutdentSubcategoryRelevels(t *testing.T) {
	// A(1) > B(2) > C(3). Outdenting B lifts it to a root sibling of A at level 1,
	// and C follows down to level 2.
	d := todotest.Parse(t, "# A\n\n## B\n\n### C\n")
	b, c := find(d, "B"), find(d, "C")
	if !step(t, d, b, todo.StepOut) {
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
	d := todotest.Parse(t, "# A\n\n## B\n\n### C\n")
	step(t, d, find(d, "B"), todo.StepOut)
	re := todotest.Parse(t, todotest.Render(d))
	b, c := find(re, "B"), find(re, "C")
	if b == nil || b.Parent != nil {
		t.Errorf("after round-trip B should be a root:\n%s", todotest.Render(d))
	}
	if c == nil || c.Parent == nil || c.Parent.Title != "B" {
		t.Errorf("after round-trip C should still nest under B:\n%s", todotest.Render(d))
	}
}

func TestIndentUnderSubtaskRefused(t *testing.T) {
	d := todotest.Parse(t, "# C\n- [ ] T\n  - [ ] a\n  - [ ] b\n")
	if step(t, d, find(d, "b"), todo.StepIn) {
		t.Error("Indent under a subtask should be refused")
	}
	if got := titles(find(d, "T").Children); len(got) != 2 {
		t.Errorf("T's children = %v, want [a b]", got)
	}
}

func TestIndentTaskWithChildrenRefused(t *testing.T) {
	d := todotest.Parse(t, "# C\n- [ ] P\n- [ ] T\n  - [ ] a\n")
	if step(t, d, find(d, "T"), todo.StepIn) {
		t.Error("Indent of a task that has subtasks should be refused")
	}
}

func TestIndentNormalizesStatus(t *testing.T) {
	d := todotest.Parse(t, "# C\n- [ ] P\n- [ ] T\n- [x] D\n")
	tk, dn := find(d, "T"), find(d, "D")
	tk.Status = todo.Review
	if !step(t, d, tk, todo.StepIn) {
		t.Fatal("Indent(T) should succeed")
	}
	if tk.Status != todo.Open {
		t.Errorf("T status = %v, want Open", tk.Status)
	}
	if !step(t, d, dn, todo.StepIn) {
		t.Fatal("Indent(D) should succeed")
	}
	if dn.Status != todo.Done {
		t.Errorf("D status = %v, want Done", dn.Status)
	}
}

func TestOutdentTakesParentState(t *testing.T) {
	d := todotest.Parse(t, "# C\n- [ ] P\n  - [ ] a\n- [x] Q\n  - [ ] b\n")
	find(d, "P").Status = todo.Ready
	a, b := find(d, "a"), find(d, "b")
	if !step(t, d, a, todo.StepOut) {
		t.Fatal("Outdent(a) should succeed")
	}
	if a.Status != todo.Ready {
		t.Errorf("a status = %v, want Ready", a.Status)
	}
	if !step(t, d, b, todo.StepOut) {
		t.Fatal("Outdent(b) should succeed")
	}
	if b.Status != todo.Working {
		t.Errorf("b status = %v, want Working", b.Status)
	}
}
