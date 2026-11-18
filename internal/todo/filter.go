package todo

import (
	"slices"
	"strings"
)

// Matches reports whether query occurs, case-insensitively, in the item's
// title — or, for a task, in its reference or description. The query is a
// plain substring (grep-like), not a pattern. An empty query never matches;
// callers treat an empty query as "no filter" (see VisibleItems).
func (it *Item) Matches(query string) bool {
	if query == "" {
		return false
	}
	q := strings.ToLower(query)
	if strings.Contains(strings.ToLower(it.Title), q) {
		return true
	}
	return it.Kind == Task && (strings.Contains(strings.ToLower(it.Ref), q) ||
		strings.Contains(strings.ToLower(it.Description), q))
}

// VisibleItems returns the set of items to show when filtering by query, under
// "path to matches" semantics (see VisibleWhere). A blank query returns nil,
// meaning "no filter".
func (d *Document) VisibleItems(query string) map[*Item]bool {
	if strings.TrimSpace(query) == "" {
		return nil
	}
	return d.VisibleWhere(func(it *Item) bool { return it.Matches(query) })
}

// VisibleWhere returns the items to show for a predicate, under "path to
// matches" semantics: an item is visible when it matches, when a descendant
// matches (so the path from the root down to every match is kept), or when an
// ancestor matches (so a match keeps its whole subtree). Branches with no match
// anywhere on their root-to-node path or in their subtree are pruned.
func (d *Document) VisibleWhere(match func(*Item) bool) map[*Item]bool {
	visible := map[*Item]bool{}

	// subtreeMatch[it] is true when it, or any descendant, matches. It marks
	// every item on a path down to a match, so a match is never orphaned.
	subtreeMatch := map[*Item]bool{}
	var mark func(it *Item) bool
	mark = func(it *Item) bool {
		m := match(it)
		for _, c := range it.Children {
			if mark(c) {
				m = true
			}
		}
		subtreeMatch[it] = m
		return m
	}
	for _, r := range d.Roots {
		mark(r)
	}

	// Collect: an item is visible if its subtree holds a match, or it sits under
	// a matching ancestor — the latter keeps a match's whole subtree.
	var collect func(it *Item, underMatch bool)
	collect = func(it *Item, underMatch bool) {
		if subtreeMatch[it] || underMatch {
			visible[it] = true
		}
		under := underMatch || match(it)
		for _, c := range it.Children {
			collect(c, under)
		}
	}
	for _, r := range d.Roots {
		collect(r, false)
	}
	return visible
}

// inColumns reports whether it is a top-level task sitting in one of cols.
func inColumns(it *Item, cols []Column) bool {
	return it.Kind == Task && !it.IsSubtask() && slices.Contains(cols, it.Status.Column())
}

// ColumnItems returns the items to show for the board columns cols (one tab,
// or several at once): the top-level tasks in any of them, their subtasks, and
// the category path above them. When cols includes Backlog every category is
// shown too, so empty categories stay reachable for adding tasks. The result
// is never nil; an empty map means nothing shows.
func (d *Document) ColumnItems(cols ...Column) map[*Item]bool {
	visible := d.VisibleWhere(func(it *Item) bool { return inColumns(it, cols) })
	if slices.Contains(cols, ColBacklog) {
		var walk func(it *Item)
		walk = func(it *Item) {
			if it.Kind == Category {
				visible[it] = true
			}
			for _, ch := range it.Children {
				walk(ch)
			}
		}
		for _, r := range d.Roots {
			walk(r)
		}
	}
	return visible
}

// ColumnMatches returns the items shown for the columns cols while searching
// for query: the in-view items on a path to a match that is itself in view
// (see VisibleWhere), so a match in another column never keeps its category as
// a bare row. A blank query is no search: it returns ColumnItems(cols...).
// The TUI and `list --column --filter` both use it, so they agree.
func (d *Document) ColumnMatches(query string, cols ...Column) map[*Item]bool {
	inTab := d.ColumnItems(cols...)
	if strings.TrimSpace(query) == "" {
		return inTab
	}
	vis := d.VisibleWhere(func(it *Item) bool { return inTab[it] && it.Matches(query) })
	for it := range vis {
		if !inTab[it] {
			delete(vis, it)
		}
	}
	return vis
}

// KeepStates narrows vis, a visible set for the columns cols (see ColumnItems
// and ColumnMatches), to the top-level tasks in cols whose status is in
// states, their subtasks and the category path above them. Categories left
// without such a task drop out. It returns a new map and leaves vis untouched.
func (d *Document) KeepStates(vis map[*Item]bool, states map[Status]bool, cols ...Column) map[*Item]bool {
	keep := d.VisibleWhere(func(it *Item) bool { return inColumns(it, cols) && states[it.Status] })
	out := make(map[*Item]bool, len(keep))
	for it := range keep {
		if vis[it] {
			out[it] = true
		}
	}
	return out
}

// ColumnCounts counts the top-level tasks in each board column.
func (d *Document) ColumnCounts() map[Column]int {
	counts := map[Column]int{}
	for _, r := range d.Roots {
		r.countColumns(counts)
	}
	return counts
}

func (it *Item) countColumns(counts map[Column]int) {
	if it.Kind == Task {
		if !it.IsSubtask() {
			counts[it.Status.Column()]++
		}
		return
	}
	for _, c := range it.Children {
		c.countColumns(counts)
	}
}

// CountInColumn counts the top-level tasks in column c under the item,
// descending through subcategories.
func (it *Item) CountInColumn(c Column) int {
	counts := map[Column]int{}
	it.countColumns(counts)
	return counts[c]
}
