package tui

import (
	"strings"

	"github.com/candy-tools/todo/internal/todo"
)

// viewKey identifies one view of the list for the per-view fold and selection
// memory a tree keeps: each board tab has its own, and so does the unified
// view.
type viewKey struct {
	col todo.Column
	all bool
}

// viewState is what every pane shows the list through: the board tab (or the
// unified view of every state), the search query and the state filter. The
// model owns it and every tree points at the same value, so a tab switch, a
// search or a filter change is made once and each tree only has to rebuild.
type viewState struct {
	// column is the board tab shown: only its tasks (with their subtasks and
	// category paths) are rows. It is kept while the unified view is on, so
	// turning that off returns to the last tab.
	column todo.Column
	// all is the unified view ("k"): the tabs are hidden and every task, in
	// any state, is shown.
	all bool
	// filter is the active search query. It is kept across tab switches.
	filter string
	// states is the board tabs' state filter, picked with "f": when it holds
	// any of the tab's states, the tab shows only top-level tasks in those
	// states. Columns have disjoint states, so one set serves every tab: each
	// tab only looks at its own states. Kept across tab switches, like filter.
	states map[todo.Status]bool
	// allStates is the unified view's own state filter, apart from the tabs'.
	allStates map[todo.Status]bool
}

// newViewState returns the view the app opens on: the Backlog tab, with no
// search and no state filter.
func newViewState() *viewState {
	return &viewState{column: todo.ColBacklog, states: map[todo.Status]bool{}, allStates: map[todo.Status]bool{}}
}

// key is the memory key of the current view.
func (v *viewState) key() viewKey { return viewKey{col: v.column, all: v.all} }

// columns lists the board columns the view shows.
func (v *viewState) columns() []todo.Column {
	if v.all {
		return todo.Columns()
	}
	return []todo.Column{v.column}
}

// title names the view, e.g. "Backlog" or "All states".
func (v *viewState) title() string {
	if v.all {
		return "All states"
	}
	return v.column.Title()
}

// searching reports whether a search query filters the rows. A blank query
// (spaces only) filters nothing, as in todo.VisibleItems.
func (v *viewState) searching() bool { return strings.TrimSpace(v.filter) != "" }

// filterSet is the state-filter set the current view reads: the unified
// view's own, or the one the tabs share.
func (v *viewState) filterSet() map[todo.Status]bool {
	if v.all {
		return v.allStates
	}
	return v.states
}

// viewStates lists the states the current view can filter by, in board order.
func (v *viewState) viewStates() []todo.Status {
	var out []todo.Status
	for _, c := range v.columns() {
		out = append(out, c.States()...)
	}
	return out
}

// activeStates returns the states of the state filter that belong to the
// current view, in board order; empty when the state filter is off for it.
func (v *viewState) activeStates() []todo.Status {
	set := v.filterSet()
	var out []todo.Status
	for _, st := range v.viewStates() {
		if set[st] {
			out = append(out, st)
		}
	}
	return out
}

// filtering reports whether the rows are narrowed, by a search query or by the
// state filter. A filtered view ignores folds and drops the placeholder row.
func (v *viewState) filtering() bool { return v.searching() || len(v.activeStates()) > 0 }

// setActiveStates replaces the current view's part of the state filter with
// next (nil or empty turns it off for the view); the other views keep theirs.
func (v *viewState) setActiveStates(next map[todo.Status]bool) {
	set := v.filterSet()
	for _, st := range v.viewStates() {
		delete(set, st)
		if next[st] {
			set[st] = true
		}
	}
}

// editsCategories reports whether categories can be added, renamed, moved or
// deleted in the view: only where every category is shown, which is Backlog
// and the unified view.
func (v *viewState) editsCategories() bool { return v.all || v.column == todo.ColBacklog }

// newTaskState is the state a new top-level task takes in the view, so it
// shows where it was added: the tab's first state, or open (Backlog's) in the
// unified view.
func (v *viewState) newTaskState() todo.Status {
	if v.all {
		return todo.ColBacklog.FirstState()
	}
	return v.column.FirstState()
}

// setColumn switches every tree to the board tab for column c: the main tree
// gets back that tab's folds and selection (see tree.switchView), the sidebar
// that tab's folds while keeping its category (or falling back to All). The
// search and the state filter carry over.
func (m *model) setColumn(c todo.Column) {
	from := m.view.key()
	m.view.column = c
	m.refreshSide()
	m.tree.switchView(from)
}

// setUnified turns the unified view on or off. Every tree switches to that
// view's own folds; the main tree's selected item stays selected when the
// destination shows it, else its cursor goes to the top row. The tab is kept,
// so turning it off returns to the last tab.
func (m *model) setUnified(on bool) {
	if m.view.all == on {
		return
	}
	sel := m.tree.selectedID()
	from := m.view.key()
	m.view.all = on
	m.refreshSide()
	m.tree.switchView(from)
	if !m.tree.selectID(sel) {
		m.tree.selectTop()
	}
}

// setFilter applies a search query (empty clears it) and rebuilds, resetting
// the main tree's cursor to the first row so results start at the top.
func (m *model) setFilter(q string) {
	m.view.filter = q
	m.refreshSide()
	m.tree.cursor = 0
	m.tree.rebuild()
}

// setStates replaces the current view's state filter with next (nil or empty
// turns it off for the view) and rebuilds, resetting the main tree's cursor to
// the first row.
func (m *model) setStates(next map[todo.Status]bool) {
	m.view.setActiveStates(next)
	m.refreshSide()
	m.tree.cursor = 0
	m.tree.rebuild()
}

// clearStates turns the current view's state filter off (other tabs keep
// theirs), keeping the item under the cursor selected.
func (m *model) clearStates() {
	t := m.focused()
	sel := t.selected()
	m.setStates(nil)
	if sel != nil && !t.onPlaceholder() {
		t.revealItem(sel)
	}
}

// refreshSide rebuilds the sidebar for the current view — its folds, and its
// category or All when the view no longer shows it — and points the main
// tree's task list at that category. The main tree is rebuilt by the caller.
func (m *model) refreshSide() {
	m.side.useFolds(m.view.key())
	m.side.refreshSidebar()
	m.tree.scope = m.side.selectedID()
}

// syncScope points the task list at the sidebar's category, after a key moved
// the sidebar (or anything else changed it).
func (m *model) syncScope() { m.tree.setScope(m.side.selectedID()) }

// toggleSplit turns the split view on or off. On, the sidebar starts on All
// with the focus, and the task list keeps the selected task. Off, the tree
// selects what had focus: the task list's task, or the sidebar's category.
func (m *model) toggleSplit() {
	if !m.split {
		sel := m.tree.selectedID()
		m.split, m.sideFocus = true, true
		m.side.refreshSidebar()
		m.side.selectTop()
		m.tree.scope = 0
		m.tree.setPane(paneTasks)
		if !m.tree.selectID(sel) {
			m.tree.selectTop()
		}
		return
	}
	target := m.tree.selected()
	if m.sideFocus {
		if it := m.side.selected(); it != nil && !m.side.onPlaceholder() {
			target = it
		}
	}
	m.split, m.sideFocus = false, false
	m.tree.setPane(paneMixed)
	if target != nil {
		m.tree.revealItem(target)
	}
}

// sidebarWidth is the split view's sidebar width for a terminal w wide: 30%,
// kept between 20 and 40 columns.
func sidebarWidth(w int) int { return min(max(w*30/100, 20), 40) }

// focused returns the tree the item keys act on: the sidebar while it has the
// split view's focus, else the main tree.
func (m *model) focused() *tree {
	if m.split && m.sideFocus {
		return &m.side
	}
	return &m.tree
}

// adoptAll swaps doc, a fresh load of the list, into both trees (see
// tree.adopt). The focused tree selects focusID (or goes back onto the
// placeholder row when onPH); the other keeps its own selection. The sidebar
// goes first and falls back to All when its category is gone, so the task
// list is rebuilt for the category that is left.
func (m *model) adoptAll(doc *todo.Document, focusID uint, onPH bool) {
	sideID, sidePH := m.side.selectedID(), m.side.onPlaceholder()
	mainID, mainPH := m.tree.selectedID(), m.tree.onPlaceholder()
	if m.split && m.sideFocus {
		// No item to focus (an add whose new ID is selected afterwards) keeps
		// the sidebar's category.
		if focusID != 0 || onPH {
			sideID, sidePH = focusID, onPH
		}
	} else {
		mainID, mainPH = focusID, onPH
	}
	m.side.adopt(doc, sideID, sidePH)
	if (sideID == 0 || m.side.selectedID() != sideID) && (!sidePH || !m.side.onPlaceholder()) {
		m.side.selectTop()
	}
	m.tree.scope = m.side.selectedID()
	m.tree.adopt(doc, mainID, mainPH)
}
