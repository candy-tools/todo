package tui

import (
	"strings"

	"github.com/candy-tools/todo/internal/todo"
)

// viewKey identifies one view of the list for the per-view fold and selection
// memory a tree keeps: each board tab has its own.
type viewKey struct {
	col todo.Column
}

// viewState is what every pane shows the list through: the board tab, the
// search query and the state filter. The model owns it and every tree points
// at the same value, so a tab switch, a search or a filter change is made
// once and each tree only has to rebuild.
type viewState struct {
	// column is the board tab shown: only its tasks (with their subtasks and
	// category paths) are rows.
	column todo.Column
	// filter is the active search query. It is kept across tab switches.
	filter string
	// states is the state filter picked with "f": when it holds any of the
	// tab's states, the tab shows only top-level tasks in those states.
	// Columns have disjoint states, so one set serves every tab: each tab
	// only looks at its own states. Kept across tab switches, like filter.
	states map[todo.Status]bool
}

// newViewState returns the view the app opens on: the Backlog tab, with no
// search and no state filter.
func newViewState() *viewState {
	return &viewState{column: todo.ColBacklog, states: map[todo.Status]bool{}}
}

// key is the memory key of the current view.
func (v *viewState) key() viewKey { return viewKey{col: v.column} }

// columns lists the board columns the view shows.
func (v *viewState) columns() []todo.Column { return []todo.Column{v.column} }

// title names the view, e.g. "Backlog".
func (v *viewState) title() string { return v.column.Title() }

// searching reports whether a search query filters the rows. A blank query
// (spaces only) filters nothing, as in todo.VisibleItems.
func (v *viewState) searching() bool { return strings.TrimSpace(v.filter) != "" }

// activeStates returns the states of the state filter that belong to the
// current view, in column order; empty when the state filter is off for it.
func (v *viewState) activeStates() []todo.Status {
	var out []todo.Status
	for _, st := range v.column.States() {
		if v.states[st] {
			out = append(out, st)
		}
	}
	return out
}

// filtering reports whether the rows are narrowed, by a search query or by the
// state filter. A filtered view ignores folds and drops the placeholder row.
func (v *viewState) filtering() bool { return v.searching() || len(v.activeStates()) > 0 }

// setActiveStates replaces the current view's part of the state filter with
// next (nil or empty turns it off for the view); the other tabs keep theirs.
func (v *viewState) setActiveStates(next map[todo.Status]bool) {
	for _, st := range v.column.States() {
		delete(v.states, st)
	}
	for _, st := range v.column.States() {
		if next[st] {
			v.states[st] = true
		}
	}
}

// editsCategories reports whether categories can be added, renamed, moved or
// deleted in the view: only where every category is shown, which is Backlog.
func (v *viewState) editsCategories() bool { return v.column == todo.ColBacklog }

// newTaskState is the state a new top-level task takes in the view, so it
// shows where it was added.
func (v *viewState) newTaskState() todo.Status { return v.column.FirstState() }

// setColumn switches every tree to the board tab for column c; each tree
// gets back that tab's folds and selection (see tree.switchView). The search
// and the state filter carry over.
func (m *model) setColumn(c todo.Column) {
	from := m.view.key()
	m.view.column = c
	m.tree.switchView(from)
}

// setFilter applies a search query (empty clears it) and rebuilds, resetting
// the cursor to the first row so results start at the top.
func (m *model) setFilter(q string) {
	m.view.filter = q
	m.tree.cursor = 0
	m.tree.rebuild()
}

// setStates replaces the current view's state filter with next (nil or empty
// turns it off for the view) and rebuilds, resetting the cursor to the first
// row.
func (m *model) setStates(next map[todo.Status]bool) {
	m.view.setActiveStates(next)
	m.tree.cursor = 0
	m.tree.rebuild()
}

// clearStates turns the current view's state filter off (other tabs keep
// theirs), keeping the item under the cursor selected.
func (m *model) clearStates() {
	sel := m.tree.selected()
	m.setStates(nil)
	if sel != nil && !m.tree.onPlaceholder() {
		m.tree.revealItem(sel)
	}
}

// focused returns the tree the item keys act on: the one pane there is.
func (m *model) focused() *tree { return &m.tree }

// adoptAll swaps doc, a fresh load of the list, into every tree (see
// tree.adopt): the focused tree selects focusID (or goes back onto the
// placeholder row when onPH).
func (m *model) adoptAll(doc *todo.Document, focusID uint, onPH bool) {
	m.tree.adopt(doc, focusID, onPH)
}
