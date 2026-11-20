package tui

import (
	"context"
	"slices"
	"strings"

	"github.com/candy-tools/todo/internal/todo"
	tea "github.com/charmbracelet/bubbletea"
)

// screen is one of the TUI's workspaces: the Backlog, or the Sprints
// workspace (the sprint list beside one sprint's board).
type screen int

const (
	screenBacklog screen = iota // the category tree with the sprint-less tasks
	screenSprint                // the Sprints workspace: the sprint list beside the shown sprint's board
)

// stored returns the screen as the list file's view holds it, with sprint the
// shown sprint: the Sprints workspace is stored as that sprint's board, or as
// the sprint list when it shows none.
func (s screen) stored(sprint string) todo.Screen {
	switch {
	case s == screenSprint && sprint != "":
		return todo.ScreenSprint
	case s == screenSprint:
		return todo.ScreenSprints
	}
	return todo.ScreenBacklog
}

// viewKey identifies one view of the list for the per-view fold and selection
// memory a tree keeps: the Backlog, each tab of each sprint board, and the
// unified view have their own.
type viewKey struct {
	screen screen
	sprint string
	col    todo.Column
	all    bool
}

// viewState is what every pane shows the list through: the screen (with the
// board's sprint and tab), or the unified view of every task on top of it, the
// search query and the state filter. The model owns it and every tree points
// at the same value, so a screen or tab switch, a search or a filter change is
// made once and each tree only has to rebuild.
type viewState struct {
	// screen is the screen shown under the unified view, or alone when that
	// is off. On screenSprint, sprint is the board's sprint ID, "" when the
	// workspace shows none (no sprint to show); it is "" on the Backlog.
	screen screen
	sprint string
	// column is the board tab shown on screenSprint: Next, In Progress or
	// Done. Only its tasks of the sprint (with their subtasks and category
	// paths) are rows. It means nothing on the other screens.
	column todo.Column
	// all is the unified view ("u"): a mode on top of the screen where the
	// tabs are hidden and every task, in any state and any sprint, is shown.
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

// newViewState returns the view the app opens on: the Backlog, with no search
// and no state filter.
func newViewState() *viewState {
	return &viewState{screen: screenBacklog, column: todo.ColNext, states: map[todo.Status]bool{}, allStates: map[todo.Status]bool{}}
}

// key is the memory key of the current view. The unified view has one, on top
// of whichever screen.
func (v *viewState) key() viewKey {
	switch {
	case v.all:
		return viewKey{all: true}
	case v.screen == screenSprint:
		return viewKey{screen: screenSprint, sprint: v.sprint, col: v.column}
	}
	return viewKey{screen: v.screen}
}

// onBoard reports whether the Sprints workspace is shown (its board, even an
// empty one), without the unified view over it.
func (v *viewState) onBoard() bool { return !v.all && v.screen == screenSprint }

// scopeSprint is the sprint whose tasks the view keeps: the board's sprint, or
// "" (any membership) elsewhere.
func (v *viewState) scopeSprint() string {
	if v.onBoard() {
		return v.sprint
	}
	return ""
}

// columns lists the board columns the view shows: every one in the unified
// view, Backlog on the Backlog, the tab on a sprint board, and none in the
// Sprints workspace while it shows no sprint.
func (v *viewState) columns() []todo.Column {
	switch {
	case v.all:
		return todo.Columns()
	case v.screen == screenBacklog:
		return []todo.Column{todo.ColBacklog}
	case v.sprint == "":
		return nil
	}
	return []todo.Column{v.column}
}

// title names the view, e.g. "Backlog", "Next" or "All states".
func (v *viewState) title() string {
	switch {
	case v.all:
		return "All states"
	case v.screen == screenSprint:
		return v.column.Title()
	}
	return todo.ColBacklog.Title()
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
// deleted in the view: only where every category is shown, which is the
// Backlog and the unified view.
func (v *viewState) editsCategories() bool { return v.all || v.screen == screenBacklog }

// newTaskPlacement is the state and sprint a new top-level task takes in the
// view, so it shows where it was added: on a sprint board the tab's first
// state in the board's sprint, elsewhere (the Backlog and the unified view)
// open in the Backlog.
func (v *viewState) newTaskPlacement() (todo.Status, string) {
	if v.onBoard() {
		return v.column.FirstState(), v.sprint
	}
	return todo.ColBacklog.FirstState(), ""
}

// setColumn switches every tree to the board tab for column c, one of the
// sprint board's: the main tree gets back that tab's folds and selection (see
// tree.switchView), the sidebar that tab's folds while keeping its category
// (or falling back to All). The board remembers the tab for when it is opened
// again. The search and the state filter carry over.
func (m *model) setColumn(c todo.Column) {
	from := m.view.key()
	m.view.column = c
	m.boardTabs[m.view.sprint] = c
	m.refreshSide()
	m.tree.switchView(from)
}

// switchScreen turns the unified view off and switches every tree to screen
// sc — for screenSprint, the board of sprint ("" for none), on the tab it last
// showed (Next the first time) — as setColumn switches tabs. A board's sprint
// becomes the last viewed one. The view is not stored; see showBacklog and the
// others.
func (m *model) switchScreen(sc screen, sprint string) {
	from := m.view.key()
	m.view.all = false
	m.view.screen, m.view.sprint = sc, ""
	if sc == screenSprint {
		m.view.sprint = sprint
		if sprint != "" {
			m.lastSprint = sprint
		}
		m.view.column = todo.ColNext
		if c, ok := m.boardTabs[sprint]; ok {
			m.view.column = c
		}
	}
	m.tree.pane = m.mainPane()
	m.refreshSide()
	m.tree.switchView(from)
}

// showBacklog shows the Backlog ("b") and stores the view.
func (m *model) showBacklog() {
	m.switchScreen(screenBacklog, "")
	m.saveView()
}

// showSprints opens the Sprints workspace ("s") on the last viewed sprint
// while the list holds it, else on the first open sprint (see
// defaultSprint), and stores the view.
func (m *model) showSprints() {
	id := m.lastSprint
	if _, ok := m.doc.SprintByID(id); !ok {
		id = m.defaultSprint()
	}
	m.showSprint(id)
}

// showSprint opens the Sprints workspace on sprint id and stores the view.
func (m *model) showSprint(id string) {
	m.openWorkspace(id)
	m.saveView()
}

// openWorkspace opens the Sprints workspace on sprint id ("" for none), with
// the sprint selected in the focused sidebar. The view is not stored.
func (m *model) openWorkspace(id string) {
	m.switchScreen(screenSprint, id)
	m.listFocus = true
	m.selectInList(id)
}

// selectInList puts the sidebar's cursor on sprint id, or on no row for "",
// and keeps it shown.
func (m *model) selectInList(id string) {
	if id == "" {
		m.sprints.deselect()
		return
	}
	m.sprints.selectID(m.doc, id)
	m.sprints.reconcile(m.doc, m.sprintListHeight())
}

// defaultSprint is the sprint the Sprints workspace shows when it has no
// other to show: the first open one, or "" when none is open.
func (m *model) defaultSprint() string {
	if open := m.doc.OpenSprints(); len(open) > 0 {
		return open[0].ID
	}
	return ""
}

// followList shows the board of the sprint under the sidebar's cursor, after
// the cursor moved. The view is not stored (see quit).
func (m *model) followList() {
	if s := m.sprints.selected(m.doc); s != nil && s.ID != m.view.sprint {
		m.switchScreen(screenSprint, s.ID)
	}
}

// onList reports whether the keys go to the Sprints workspace's sidebar.
func (m *model) onList() bool { return m.view.onBoard() && m.listFocus }

// splitShown reports whether the split view is on screen: it is on, and the
// Sprints workspace, whose sidebar is the sprint list, is not shown.
func (m model) splitShown() bool { return m.split && !m.view.onBoard() }

// mainPane is the pane the main tree shows: the task list beside the
// categories sidebar while the split is shown, else the mixed tree.
func (m *model) mainPane() paneKind {
	if m.splitShown() {
		return paneTasks
	}
	return paneMixed
}

// msgNoOpenSprints is the footer hint for "k" with no open sprint to go to,
// and "enter" in a Sprints workspace that shows no sprint.
const msgNoOpenSprints = "no open sprints — press s, then n to create one"

// msgSplitsBacklog is the footer hint for "v" in the Sprints workspace, whose
// sidebar is the sprint list.
const msgSplitsBacklog = "v splits the Backlog"

// nextOpenSprint returns the sprint "k" goes to. On a sprint board it is the
// next open sprint after the board's in creation order, wrapping around (the
// board's own when it is the only open one). Elsewhere it is the last viewed
// sprint while it is open, else the first open one. ok is false when there is
// none.
func (m *model) nextOpenSprint() (id string, ok bool) {
	d := m.doc
	if m.view.screen == screenSprint {
		start := slices.IndexFunc(d.Sprints, func(s *todo.Sprint) bool { return s.ID == m.view.sprint })
		for i := 1; i <= len(d.Sprints); i++ {
			if s := d.Sprints[(start+i)%len(d.Sprints)]; s.Status == todo.SprintOpen {
				return s.ID, true
			}
		}
		return "", false
	}
	if s, ok := d.SprintByID(m.lastSprint); ok && s.Status == todo.SprintOpen {
		return m.lastSprint, true
	}
	if open := d.OpenSprints(); len(open) > 0 {
		return open[0].ID, true
	}
	return "", false
}

// setUnified turns the unified view on or off. Every tree switches to that
// view's own folds; the main tree's selected item stays selected when the
// destination shows it, else its cursor goes to the top row. The screen and
// its tab are kept, so turning it off returns to them. The keys go to the
// trees, not the Sprints workspace's sidebar.
func (m *model) setUnified(on bool) {
	if m.view.all == on {
		return
	}
	sel := m.tree.selectedID()
	from := m.view.key()
	m.view.all = on
	m.listFocus = false
	m.tree.pane = m.mainPane()
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
// In the Sprints workspace the split is kept for the other views but not
// shown (see splitShown).
func (m *model) toggleSplit() {
	if !m.split {
		sel := m.tree.selectedID()
		m.split, m.sideFocus = true, true
		m.side.refreshSidebar()
		m.side.selectTop()
		m.tree.scope = ""
		m.tree.setPane(m.mainPane())
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

// openSplitOnSelection is v from the single tree: it opens the split on what
// the cursor is on. A task opens the split on All with the task list focused
// and that task selected; a category is selected in the sidebar, which keeps
// the focus. Anything else (the placeholder row, an empty tab) opens it as
// toggleSplit does.
func (m *model) openSplitOnSelection() {
	var sel *todo.Item
	if id := m.tree.selectedID(); id != "" {
		sel, _ = m.doc.ByID(id)
	}
	m.toggleSplit()
	switch {
	case sel == nil:
	case sel.IsTask():
		m.sideFocus = false
	case sel.Kind == todo.Category:
		m.side.revealItem(sel)
		m.syncScope()
	}
}

// applyView puts the model in the view v, the one the list file holds, when
// the app opens: its screen (the Sprints workspace on the stored sprint, or on
// the first open one when the view names none or a sprint that is gone), the
// unified view on top of it, and the split. A later change of the file's
// view, by another session, is not applied: it would switch the view under
// the user.
func (m *model) applyView(v todo.View) {
	switch v.Screen {
	case todo.ScreenSprints, todo.ScreenSprint:
		id := v.Sprint
		if _, ok := m.doc.SprintByID(id); !ok || v.Screen == todo.ScreenSprints {
			id = m.defaultSprint()
		}
		m.openWorkspace(id)
	}
	m.setUnified(v.Unified)
	if v.Split != m.split {
		m.toggleSplit()
	}
}

// viewOnScreen is the view on screen as the list file holds it: the screen and
// its sprint, unified or not, split or not.
func (m *model) viewOnScreen() todo.View {
	return todo.View{Unified: m.view.all, Split: m.split, Screen: m.view.screen.stored(m.view.sprint), Sprint: m.view.sprint}
}

// saveView stores the view on screen in the list file, so the next session
// opens on it. It is a write like any other (see commit): a failed one shows
// in the footer, and the view stays as it is on screen.
func (m *model) saveView() {
	v := m.viewOnScreen()
	m.commit(m.focused().selectedID(), func(ctx context.Context, st *todo.Store) (todo.WriteResult, error) {
		return st.SetView(ctx, v)
	})
}

// quit stores the view on screen when the list file holds another — so a
// sprint browsed to in the Sprints workspace's sidebar is the one the next
// session opens on — and quits. Every way out of the app goes through it.
func (m model) quit() (tea.Model, tea.Cmd) {
	if m.viewOnScreen() != m.doc.View {
		m.saveView()
	}
	return m, tea.Quit
}

// sidebarWidth is the split view's sidebar width for a terminal w wide: 30%,
// kept between 20 and 40 columns.
func sidebarWidth(w int) int { return min(max(w*30/100, 20), 40) }

// focused returns the tree the item keys act on: the sidebar while it has the
// shown split view's focus, else the main tree.
func (m *model) focused() *tree {
	if m.splitShown() && m.sideFocus {
		return &m.side
	}
	return &m.tree
}

// adoptAll swaps doc, a fresh load of the list, into both trees (see
// tree.adopt). The focused tree selects focusID (or goes back onto the
// placeholder row when onPH); the other keeps its own selection. The sidebar
// goes first and falls back to All when its category is gone, so the task
// list is rebuilt for the category that is left. A sprint board whose sprint
// doc no longer holds gives way to another sprint (see leaveGoneSprint).
func (m *model) adoptAll(doc *todo.Document, focusID string, onPH bool) {
	sideID, sidePH := m.side.selectedID(), m.side.onPlaceholder()
	mainID, mainPH := m.tree.selectedID(), m.tree.onPlaceholder()
	if m.splitShown() && m.sideFocus {
		// No item to focus (an add whose new ID is selected afterwards) keeps
		// the sidebar's category.
		if focusID != "" || onPH {
			sideID, sidePH = focusID, onPH
		}
	} else {
		mainID, mainPH = focusID, onPH
	}
	m.side.adopt(doc, sideID, sidePH)
	if (sideID == "" || m.side.selectedID() != sideID) && (!sidePH || !m.side.onPlaceholder()) {
		m.side.selectTop()
	}
	m.tree.scope = m.side.selectedID()
	m.tree.adopt(doc, mainID, mainPH)
	m.leaveGoneSprint()
}

// updateStateFilter handles keys in the "f" dialog: the arrows move, space or x
// ticks a state, enter applies the ticked states to the tab (none ticked turns
// the state filter off for it), esc or q cancels.
func (m model) updateStateFilter(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "up":
		m.pick.move(-1)
	case "down":
		m.pick.move(1)
	case " ", "x":
		m.pick.toggle()
	case "esc", "q":
		m.mode = modeMain
	case "enter":
		m.mode = modeMain
		m.clearStates()
		m.setStates(m.pick.checked)
	}
	return m, nil
}
