package todo

// This file holds the placement query behind the TUI's "move mode": where one
// step takes an item, walking it through document order (StepUp/StepDown) or
// re-nesting it (StepIn/StepOut). Up/down reorder among same-kind siblings
// and, at the ends of a run, cross into the adjacent category — so a grabbed
// task or subcategory can be carried across categories a step at a time —
// while in/out make the same-kind depth changes. The query only reads the
// list; the move itself is Move, which keeps the tree's invariants (a task
// always lives under a category, a category's subcategories precede its
// direct tasks, a category never nests under a task) and is the one judge of
// which placements are valid.

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
// afterID Move takes, without changing d. The step is made with Move on a copy
// of d, so the op stays the one judge of what is valid: a step Move refuses
// fails with Move's error (ErrTooDeep for a nesting too deep, say), a step
// with nowhere to go with ErrNowhere, and a missing id with ErrNotFound. A
// StepIn of a subtask, or of a task with subtasks, fails with ErrTooDeep even
// with no sibling to nest under, as no sibling could take it.
//
// For StepUp and StepDown, shown, when not nil, says which items a view shows:
// the step repeats until the item has passed one more shown item in document
// order, or has entered another parent (one category per step, as when
// nothing is hidden). When it runs out of room first, it only passed hidden
// items, and the step fails with ErrNowhere. StepIn and StepOut ignore shown.
func (d *Document) StepPlacement(id string, s MoveStep, shown func(id string) bool) (parentID, afterID string, err error) {
	c := d.Clone()
	it, err := c.lookup(id)
	if err != nil {
		return "", "", err
	}
	if s == StepIn || s == StepOut {
		shown = nil
	}
	parent, before := it.Parent, c.shownBefore(it, shown)
	for {
		p, a, can := c.nextPlacement(it, s)
		switch {
		case !can && s == StepIn && it.Kind == Task && tooDeepUnder(it, it.IsSubtask()):
			// No sibling task to nest under, but none could take it: the
			// one it would go under sits at its own level.
			return "", "", ErrTooDeep
		case !can:
			return "", "", ErrNowhere
		}
		if err := c.Move(id, p, a); err != nil {
			return "", "", err
		}
		if shown == nil || it.Parent != parent || c.shownBefore(it, shown) != before {
			p, a = c.placement(it)
			return p, a, nil
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
			return prev.ID, lastOfKind(prev.Children, it.Kind, it), true
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
// it: the next slot of its kind one row further down. A category's
// subcategories come before its tasks, so a task, last in its category p, sits
// at the end of p's subtree: it goes in as the first task of the next sibling
// category of p, descending through that category's leading subcategories
// (which come before its tasks) or, when p is the last of its siblings, as the
// first task of p's parent, after its subcategories. A category, last in its
// run, becomes the first subcategory of the next sibling category of its
// parent, mirroring crossingUp; it does not climb further, which would carry
// it past its ancestors' tasks.
// A subtask never crosses (its nesting changes with StepOut), and ok is false
// when there is nowhere further to go.
func (d *Document) crossingDown(it *Item) (parentID, afterID string, ok bool) {
	p := it.Parent
	if it.Kind == Task {
		if p == nil || p.Kind != Category {
			return "", "", false
		}
		if nc := d.laterSiblingCategory(p); nc != nil {
			return leadingLeaf(nc).ID, "", true
		}
		if pp := p.Parent; pp != nil && pp.Kind == Category {
			return pp.ID, "", true // "" places a task first among the tasks
		}
		return "", "", false
	}
	if p == nil {
		return "", "", false // the last root category
	}
	if nc := d.laterSiblingCategory(p); nc != nil {
		return nc.ID, "", true // "" places a category first
	}
	return "", "", false
}

// crossingUp is the mirror of crossingDown, so a StepUp retraces a StepDown
// one visible row at a time. A task, first in its category p, goes to the
// slot just above: the last task of p's last subcategory or, when p has none,
// the last task of the category before p, climbing out through the ancestors
// p heads. A category, first in its run, becomes the last subcategory of the
// category before its parent.
func (d *Document) crossingUp(it *Item) (parentID, afterID string, ok bool) {
	p := it.Parent
	if p == nil {
		return "", "", false // a first root category has nowhere above it
	}
	if it.Kind == Task {
		if p.Kind != Category {
			return "", "", false
		}
		if ls := lastSubcategory(p); ls != nil {
			return ls.ID, lastOfKind(ls.Children, Task, it), true
		}
		n := p
		for d.prevSiblingCategory(n) == nil && n.Parent != nil && n.Parent.Kind == Category {
			n = n.Parent
		}
		if ps := d.prevSiblingCategory(n); ps != nil {
			return ps.ID, lastOfKind(ps.Children, Task, it), true
		}
		return "", "", false
	}
	if pc := d.prevSiblingCategory(p); pc != nil {
		return pc.ID, lastOfKind(pc.Children, Category, it), true
	}
	return "", "", false
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

// leadingLeaf follows cat's first-subcategory chain to its end: cat itself
// when its first child is not a category, otherwise the category reached by
// always descending into the first subcategory. Its first task slot is the
// first one below cat's row.
func leadingLeaf(cat *Item) *Item {
	for len(cat.Children) > 0 && cat.Children[0].Kind == Category {
		cat = cat.Children[0]
	}
	return cat
}

// laterSiblingCategory returns the nearest category among it's following
// siblings, or nil when none follows it. It does not climb to ancestors.
func (d *Document) laterSiblingCategory(it *Item) *Item {
	list, idx, ok := d.siblings(it)
	if !ok {
		return nil
	}
	for j := idx + 1; j < len(list); j++ {
		if list[j].Kind == Category {
			return list[j]
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
