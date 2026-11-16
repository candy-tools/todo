package todo

// This file holds the structural move operations that power the TUI's "move
// mode": walking an item through document order (MoveUp/MoveDown) and re-nesting
// it (Indent/Outdent). Up/down reorder among same-kind siblings and, at the ends
// of a run, cross into the adjacent category — so a grabbed task or subcategory
// can be carried across categories a step at a time — while indent/outdent make
// the same-kind depth changes. Every operation preserves the tree's invariants —
// a task always lives under a category, a category's direct tasks precede its
// subcategories (the move rules here and the displayed tree depend on it), and
// a category never nests under a task — refusing (returning false) any move
// with nowhere valid to go rather than corrupting the tree.

// MoveStep is one step of the TUI's move mode (see StepPlacement).
type MoveStep int

const (
	// StepUp moves an item one place earlier in document order: a swap with
	// the same-kind sibling before it or, first in its run, a crossing into
	// the category before (see crossingUp).
	StepUp MoveStep = iota
	// StepDown moves an item one place later in document order: a swap with
	// the same-kind sibling after it or, last in its run, a crossing into the
	// category after (see crossingDown).
	StepDown
	// StepIn nests an item under the same-kind sibling before it, as that
	// sibling's last child.
	StepIn
	// StepOut lifts an item out of its parent to sit right after it. A task
	// lifts only out of a task: a task directly under a category stays there.
	StepOut
)

// StepPlacement returns where step s takes item id, as the parentID and
// afterID Move takes, without changing d. ok is false when the step has
// nowhere to go, or Move refuses it: the step is made with Move on a copy of
// d, so the op stays the one judge of what is valid.
//
// For StepUp and StepDown, shown, when not nil, says which items a view shows:
// the step repeats until the item has passed one more shown item in document
// order, or has entered another parent (one category per step, as when
// nothing is hidden). When it runs out of room first, it only passed hidden
// items, and ok is false. StepIn and StepOut ignore shown.
func (d *Document) StepPlacement(id string, s MoveStep, shown func(id string) bool) (parentID, afterID string, ok bool) {
	c := d.Clone()
	it, found := c.ByID(id)
	if !found {
		return "", "", false
	}
	parent, before := it.Parent, c.shownBefore(it, shown)
	for {
		p, a, can := c.nextPlacement(it, s)
		if !can || c.Move(id, p, a) != nil {
			return "", "", false
		}
		if shown == nil || s == StepIn || s == StepOut || it.Parent != parent || c.shownBefore(it, shown) != before {
			p, a = c.Placement(it)
			return p, a, true
		}
	}
}

// shownBefore counts the items before it in document order that shown
// reports true for; 0 when shown is nil.
func (d *Document) shownBefore(it *Item, shown func(id string) bool) int {
	n := 0
	if shown == nil {
		return n
	}
	for _, x := range d.Enumerate() {
		if x == it {
			break
		}
		if shown(x.ID) {
			n++
		}
	}
	return n
}

// nextPlacement returns where one step s takes it, as Move's parentID and
// afterID, reading d only; ok is false when the step has nowhere to go.
func (d *Document) nextPlacement(it *Item, s MoveStep) (parentID, afterID string, ok bool) {
	list, idx, found := d.siblings(it)
	if !found {
		return "", "", false
	}
	if it.Parent != nil {
		parentID = it.Parent.ID
	}
	sameKind := func(j int) bool { return j >= 0 && j < len(list) && list[j].Kind == it.Kind }
	switch s {
	case StepUp:
		switch {
		case sameKind(idx-1) && idx > 1:
			return parentID, list[idx-2].ID, true
		case sameKind(idx - 1):
			return parentID, "", true
		}
		return d.crossingUp(it)
	case StepDown:
		if sameKind(idx + 1) {
			return parentID, list[idx+1].ID, true
		}
		return d.crossingDown(it)
	case StepIn:
		if sameKind(idx - 1) {
			prev := list[idx-1]
			return prev.ID, lastChildID(prev.Children, len(prev.Children)), true
		}
	case StepOut:
		p := it.Parent
		if p != nil && (it.Kind == Category || p.Kind == Task) {
			gp := ""
			if p.Parent != nil {
				gp = p.Parent.ID
			}
			return gp, p.ID, true
		}
	}
	return "", "", false
}

// crossingDown is where a StepDown takes it when no same-kind sibling follows
// it. A task, last in its category, goes in as the first task of that
// category's first subcategory or, failing that, of the next category in
// document order. A category, last in its run, becomes the first subcategory
// of the next category. A subtask never crosses (its nesting changes with
// StepOut), and ok is false when there is nowhere further to go.
func (d *Document) crossingDown(it *Item) (parentID, afterID string, ok bool) {
	p := it.Parent
	if it.Kind == Task {
		if p == nil || p.Kind != Category {
			return "", "", false
		}
		if sc := firstSubcategory(p); sc != nil {
			return sc.ID, "", true
		}
		if nc := d.nextSiblingCategory(p); nc != nil {
			return nc.ID, "", true
		}
		return "", "", false
	}
	if nc := d.nextSiblingCategory(it); nc != nil {
		return nc.ID, "", true // "" places a category first among the subcategories
	}
	return "", "", false
}

// crossingUp is the mirror of crossingDown. A task, first in its category,
// goes to the end of the previous category's subtree, as the last task of
// its deepest trailing category (so a StepUp retraces a StepDown one visible
// row at a time), or, heading its parent's first subcategory, up into that
// parent as its last task. A category, first in its run, becomes the last
// subcategory of the category before its parent.
func (d *Document) crossingUp(it *Item) (parentID, afterID string, ok bool) {
	p := it.Parent
	if p == nil {
		return "", "", false // a first root category has nowhere above it
	}
	if it.Kind == Task {
		if p.Kind != Category {
			return "", "", false
		}
		if pc := d.prevSiblingCategory(p); pc != nil {
			to := deepestTrailingCategory(pc)
			return to.ID, lastChildID(to.Children, firstCategory(to.Children)), true
		}
		if pp := p.Parent; pp != nil && pp.Kind == Category {
			return pp.ID, lastChildID(pp.Children, firstCategory(pp.Children)), true
		}
		return "", "", false
	}
	if pc := d.prevSiblingCategory(p); pc != nil {
		return pc.ID, lastChildID(pc.Children, len(pc.Children)), true
	}
	return "", "", false
}

// lastChildID returns the ID of children[n-1], the last of the first n
// children, or "" when n is 0.
func lastChildID(children []*Item, n int) string {
	if n == 0 {
		return ""
	}
	return children[n-1].ID
}

// MoveUp moves it one step earlier in document order. Among same-kind siblings
// that is a plain swap; at the start of its run it instead crosses into the
// previous category (see crossUp), so pressing ↑ repeatedly walks a task or
// subcategory backwards across category boundaries. It is a no-op (returning
// false) only when it is already at the very top with nowhere to go.
func (d *Document) MoveUp(it *Item) bool {
	if d.swapSibling(it, -1) {
		return true
	}
	return d.crossUp(it)
}

// MoveDown moves it one step later in document order: a same-kind sibling swap,
// or, at the end of its run, a crossing into the next category (see crossDown).
// Pressing ↓ repeatedly thus walks the item forwards across category boundaries.
// It is a no-op (returning false) at the very end of the document.
func (d *Document) MoveDown(it *Item) bool {
	if d.swapSibling(it, 1) {
		return true
	}
	return d.crossDown(it)
}

// swapSibling exchanges it with the sibling dir places away (-1 previous, +1
// next), when that neighbour exists and is the same kind. The sibling slice
// shares its backing array with the parent's Children (or the document roots),
// so swapping two elements in place is enough — no write-back needed.
func (d *Document) swapSibling(it *Item, dir int) bool {
	list, idx, ok := d.siblings(it)
	if !ok {
		return false
	}
	j := idx + dir
	if j < 0 || j >= len(list) {
		return false
	}
	if list[j].Kind != it.Kind {
		return false
	}
	list[idx], list[j] = list[j], list[idx]
	return true
}

// crossDown moves it into an adjacent category when a same-kind sibling swap
// can't carry it further down. A task that is the last of its category descends
// into that category's first subcategory (as the subcategory's first task) or,
// failing that, bubbles out to become the first task of the next category in
// document order. A category that is the last of its run becomes the first
// subcategory of the next category, re-levelled to stay one below its new
// parent. Sub-tasks never cross (their nesting is changed with Outdent), and it
// is a no-op (returning false) when there is nowhere further to go.
func (d *Document) crossDown(it *Item) bool {
	p := it.Parent
	if it.Kind == Task {
		if p == nil || p.Kind != Category {
			return false
		}
		if sc := firstSubcategory(p); sc != nil {
			d.Remove(it)
			prependTask(sc, it)
			return true
		}
		if nc := d.nextSiblingCategory(p); nc != nil {
			d.Remove(it)
			prependTask(nc, it)
			return true
		}
		return false
	}
	nc := d.nextSiblingCategory(it)
	if nc == nil {
		return false
	}
	d.Remove(it)
	insertBeforeCategories(nc, it) // its first subcategory
	relevel(it, nc.Level+1)
	return true
}

// crossUp is the mirror of crossDown for upward moves. A task that is first in
// its category crosses up to the end of the previous category's subtree —
// descending into that category's last subcategory so a ↑ retraces a ↓ one
// visible row at a time — or, when it heads its parent's first subcategory, up
// into the parent as the parent's last task. A category that is first in its run
// becomes the last subcategory of the previous category, re-levelled. It is a
// no-op (returning false) when it is already at the top.
func (d *Document) crossUp(it *Item) bool {
	p := it.Parent
	if it.Kind == Task {
		if p == nil || p.Kind != Category {
			return false
		}
		if pc := d.prevSiblingCategory(p); pc != nil {
			d.Remove(it)
			deepestTrailingCategory(pc).AppendTask(it)
			return true
		}
		if p.Parent != nil && p.Parent.Kind == Category {
			cc := p.Parent
			d.Remove(it)
			cc.AppendTask(it)
			return true
		}
		return false
	}
	if p == nil {
		return false // a first root category has nowhere above it
	}
	pc := d.prevSiblingCategory(p)
	if pc == nil {
		return false
	}
	d.Remove(it)
	pc.AppendChild(it)
	relevel(it, pc.Level+1)
	return true
}

// Indent makes it a child of its immediately-preceding sibling, nesting it one
// level deeper: a task becomes a sub-task of the task above it, a category a
// sub-category of the category above it. It requires a preceding sibling of the
// same kind — a task can only indent under a task, a category under a category —
// so it is a no-op (returning false) for the first item in a list or across the
// task/category boundary. A moved category's whole subtree is re-levelled so its
// category depth stays consistent.
func (d *Document) Indent(it *Item) bool {
	list, idx, ok := d.siblings(it)
	if !ok || idx == 0 {
		return false
	}
	prev := list[idx-1]
	if prev.Kind != it.Kind {
		return false
	}
	if it.Kind == Task && (prev.IsSubtask() || len(it.Children) > 0) {
		return false // tasks nest one level only: no sub-subtasks, and no subtasks of a subtask
	}
	d.Remove(it)
	prev.AppendChild(it) // last child: for a task parent there are no categories to sit before
	if it.Kind == Category {
		relevel(it, prev.Level+1)
	} else {
		it.Status = StatusAfterMove(it.Status, true, prev.Status)
	}
	return true
}

// Outdent moves it out to become a sibling of its parent, inserted immediately
// after it, nesting it one level shallower: a sub-task becomes a sibling of its
// parent task, a sub-category a sibling of its parent category. It is a no-op
// (returning false) when the move would break the model: a top-level task
// (directly under a category) can't outdent — that would make it a root task or
// drop it after a subcategory — and a root item has no parent to outdent past. A
// moved category's subtree is re-levelled.
func (d *Document) Outdent(it *Item) bool {
	parent := it.Parent
	if parent == nil {
		return false
	}
	// A task must stay under a category, and a category's tasks must precede its
	// subcategories; lifting a task out to sit beside its category breaks both, so
	// a task may only outdent when its parent is itself a task.
	if it.Kind == Task && parent.Kind != Task {
		return false
	}
	d.Remove(it)
	d.InsertAfter(parent, it) // sets it.Parent to the grandparent (or nil at root)
	if it.Kind == Category {
		relevel(it, parent.Level)
	} else {
		it.Status = StatusAfterMove(it.Status, false, parent.Status)
	}
	return true
}

// relevel sets cat's Level and recurses into its sub-categories (each one
// deeper). Tasks carry no Level and are skipped. A category's Level is its depth
// in the category tree, so a sub-category always outranks its parent.
func relevel(cat *Item, level int) {
	cat.Level = level
	for _, c := range cat.Children {
		if c.Kind == Category {
			relevel(c, level+1)
		}
	}
}

// firstSubcategory returns cat's earliest category child, or nil when cat holds
// only tasks. Since a category's tasks precede its subcategories, this is simply
// the first child that is a category.
func firstSubcategory(cat *Item) *Item {
	for _, c := range cat.Children {
		if c.Kind == Category {
			return c
		}
	}
	return nil
}

// lastSubcategory returns cat's last category child, or nil when it has none.
func lastSubcategory(cat *Item) *Item {
	var last *Item
	for _, c := range cat.Children {
		if c.Kind == Category {
			last = c
		}
	}
	return last
}

// deepestTrailingCategory follows cat's last-subcategory chain to its end: cat
// itself when it has no subcategories, otherwise the category reached by always
// descending into the last subcategory. That is the category at the very end of
// cat's subtree in document order — where a task lands when it crosses up into
// cat, so ↑ undoes the ↓ that would carry the task back out.
func deepestTrailingCategory(cat *Item) *Item {
	for {
		last := lastSubcategory(cat)
		if last == nil {
			return cat
		}
		cat = last
	}
}

// nextSiblingCategory returns the first category that follows it's whole subtree
// in document order: the next category among it's later siblings, or, failing
// that, the next category following each ancestor in turn. It is nil when it is
// the last category in the document.
func (d *Document) nextSiblingCategory(it *Item) *Item {
	for n := it; n != nil; n = n.Parent {
		list, idx, ok := d.siblings(n)
		if !ok {
			return nil
		}
		for j := idx + 1; j < len(list); j++ {
			if list[j].Kind == Category {
				return list[j]
			}
		}
	}
	return nil
}

// prevSiblingCategory returns the nearest category among it's preceding siblings,
// or nil when none precedes it (it heads its list). It does not climb to
// ancestors — callers handle the "first in its run" case themselves.
func (d *Document) prevSiblingCategory(it *Item) *Item {
	list, idx, ok := d.siblings(it)
	if !ok {
		return nil
	}
	for j := idx - 1; j >= 0; j-- {
		if list[j].Kind == Category {
			return list[j]
		}
	}
	return nil
}

// prependTask inserts task as cat's first child — its first task, since tasks
// precede subcategories — and sets task.Parent.
func prependTask(cat, task *Item) {
	task.Parent = cat
	cat.Children = append([]*Item{task}, cat.Children...)
}
