package todo

// FlattenSubtasks enforces the one-subtask-level rule on an in-memory tree: every
// task nested deeper than one level is re-parented to its top-level task, landing
// at its pre-order position so the reading order is preserved. Every subtask's
// status is then normalised (Done stays Done, anything else becomes Open), since a
// subtask can only be Open or Done.
//
// It returns the items whose parent changed, in their final order under each
// top-level task, so a caller can persist them one by one with d.Placement. It
// returns nil when the tree was already flat.
func (d *Document) FlattenSubtasks() []*Item {
	var moved []*Item
	var walk func(items []*Item)
	walk = func(items []*Item) {
		for _, it := range items {
			if it.Kind == Category {
				walk(it.Children)
				continue
			}
			moved = append(moved, flattenTask(it)...)
		}
	}
	walk(d.Roots)
	return moved
}

// flattenTask makes every descendant of the top-level task t a direct child of t,
// in pre-order, and returns those that had to change parent.
func flattenTask(t *Item) []*Item {
	var moved []*Item
	subs := t.Descendants()
	for _, s := range subs {
		if s.Parent != t {
			moved = append(moved, s)
		}
	}
	for _, s := range subs {
		s.Parent = t
		s.Children = nil
		if s.Status != Done {
			s.Status = Open
		}
	}
	if len(subs) > 0 {
		t.Children = subs
	}
	return moved
}
