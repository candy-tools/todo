package todo

// FlattenSubtasks enforces the one-subtask-level rule on an in-memory tree:
// every task nested deeper than one level is re-parented to its top-level
// task, landing at its pre-order position so the reading order is preserved.
// Every subtask's status is then normalised (Done stays Done, anything else
// becomes Open), since a subtask can only be Open or Done.
func (d *Document) FlattenSubtasks() {
	var walk func(items []*Item)
	walk = func(items []*Item) {
		for _, it := range items {
			if it.Kind == Category {
				walk(it.Children)
				continue
			}
			flattenTask(it)
		}
	}
	walk(d.Roots)
}

// flattenTask makes every descendant of the top-level task t a direct child
// of t, in pre-order.
func flattenTask(t *Item) {
	subs := t.Descendants()
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
}
