// Package tui is the Bubble Tea frontend of the TODO app: a single-panel
// category/task tree and an add/edit dialog that doubles as the selected item's
// detail view (it shows the item's status and subtask progress alongside its
// editable title and description), plus the keybindings that drive them. All
// domain logic lives in internal/todo; this package only renders it and routes keys.
package tui

import (
	"context"
	"fmt"
	"maps"
	"path/filepath"
	"slices"
	"strings"

	"github.com/atotto/clipboard"
	"github.com/candy-tools/todo/app/metainfo"
	"github.com/candy-tools/todo/internal/todo"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

type mode int

const (
	modeMain mode = iota
	modeForm
	modeConfirm
	modeHelp        // the "?" shortcuts dialog
	modeStatePick   // the "t" dialog: pick the selected task's state
	modeStateFilter // the "f" dialog: tick the states the tab shows
)

// formTarget records what submitting the open form should do.
type formTarget int

const (
	// targetAddSibling adds a task as a sibling of the selection — at the end of
	// the current level (after its peer tasks). On a category, where a task can't
	// be a sibling, it falls back to adding the task inside the category (like
	// targetAddChild). Tasks always live under a category, so this needs a
	// selection (there are no root-level tasks).
	targetAddSibling formTarget = iota
	// targetAddChild adds a task nested under the selection: a subtask of a
	// selected task, or a task inside a selected category.
	targetAddChild
	// targetAddCategory adds a category: a top-level one on the root
	// placeholder, or a subcategory of the enclosing category when a category
	// (or a task inside one) is selected.
	targetAddCategory
	targetEdit // rename/re-describe the selected item
)

// confirmAction records what the open confirm dialog should do when confirmed.
type confirmAction int

const (
	confirmDelete    confirmAction = iota // delete the selected item (m.deleting)
	confirmClearDone                      // remove every completed task
)

type model struct {
	store   *todo.Store
	version string
	doc     *todo.Document
	// view is the tab, search query and state filter every tree shows the
	// list through; the model changes it (setColumn, setFilter, setStates)
	// and rebuilds the trees.
	view *viewState
	// tree is the main pane: the category/task tree, or — in the split view —
	// the task list of the category picked in side, the sidebar.
	tree tree
	side tree
	// split is the split view ("v"): side on the left, tree as a task list on
	// the right. sideFocus is whether the keys go to the sidebar ("x" flips it).
	split     bool
	sideFocus bool
	width     int
	height    int

	// lastRev is the store revision doc reflects: the one todo last loaded,
	// which after its own write is the revision that write produced. The poll
	// loop compares the store against it to tell an external write (reload) from
	// the app's own (ignore).
	lastRev int64

	mode mode
	form form

	// searching is true while the "/" search bar is focused and capturing keys.
	// The query itself lives in search; the resulting filter lives on the tree
	// (m.view.filter), so it stays applied for navigation after search closes.
	searching bool
	search    textinput.Model

	target   formTarget
	editing  *todo.Item // the item being edited, for targetEdit
	deleting *todo.Item // the item the confirm dialog is guarding, for modeConfirm
	// confirmOnDelete is which confirm-dialog button has focus: false = Cancel
	// (the safe default so a stray key can't delete), true = Delete.
	confirmOnDelete bool
	// confirmAction is what confirming the open dialog does — delete the selected
	// item, or clear all completed tasks.
	confirmAction confirmAction

	// snapshots holds, per task ID completed in this session, the states of the
	// task and its subtasks captured at completion time, so un-completing it can
	// restore them (the accidental-complete undo). Session-only: it lives here in
	// memory and is never stored. Keyed by ID, it survives the reload after the
	// app's own write; an external reload clears it.
	snapshots map[uint]todo.DoneStates

	helpOffset int // how far the "?" shortcuts dialog is scrolled, for modeHelp

	pick     picker     // the open state dialog, for modeStatePick and modeStateFilter
	pickItem *todo.Item // the task the "t" dialog sets the state of

	status string // transient one-line footer message
	err    error  // last failed save or reload ("save failed: …" / "reload failed: …"), shown in the footer
}

// Run loads the list from st and starts the interactive TUI. notice (e.g. the
// automatic TODO.md import summary) is shown in the footer at start; it may be
// empty. The caller owns st and closes it after Run returns.
func Run(st *todo.Store, notice string) error {
	m, err := newModel(st, notice)
	if err != nil {
		return err
	}
	_, err = tea.NewProgram(m, tea.WithAltScreen()).Run()
	return err
}

// newModel loads the list from st into a fresh model (main view, empty undo
// snapshots) with notice as the initial footer status. A multi-line notice (an
// import summary followed by its warnings) is joined onto the single footer
// line. Split out from Run so tests can drive the model without starting a real
// terminal program.
func newModel(st *todo.Store, notice string) (model, error) {
	doc, rev, err := st.Load(context.Background())
	if err != nil {
		return model{}, err
	}
	v := newViewState()
	return model{
		store:     st,
		version:   metainfo.Version,
		doc:       doc,
		view:      v,
		tree:      newTree(doc, v),
		side:      newSidebar(doc, v),
		mode:      modeMain,
		snapshots: map[uint]todo.DoneStates{},
		lastRev:   rev,
		status:    strings.ReplaceAll(notice, "\n", " · "),
	}, nil
}

// Init starts the revision-poll loop that keeps the view in sync with external
// writes to the store.
func (m model) Init() tea.Cmd { return pollCmd() }

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		// Tell the tree how tall its viewport is now, and re-anchor the scroll so
		// the cursor stays on screen across a resize.
		m.tree.viewHeight = m.treeViewHeight()
		m.tree.reconcileOffset()
		m.side.viewHeight = m.treeViewHeight()
		m.side.reconcileOffset()
		if m.mode == modeForm {
			m.form.setWidth(msg.Width)
		}
		if m.searching {
			m.search.Width = m.searchWidth()
		}
		return m, nil
	case reloadTickMsg:
		// Probe the store and re-arm the next tick; the probe runs off-goroutine.
		return m, tea.Batch(m.reloadCheckCmd(), pollCmd())
	case storeReloadedMsg:
		return m.handleReload(msg)
	case tea.KeyMsg:
		nm, cmd := m.updateKey(msg)
		// In the split view the task list follows the sidebar's category,
		// whatever the key did to it.
		if mm, ok := nm.(model); ok && mm.split {
			mm.syncScope()
			return mm, cmd
		}
		return nm, cmd
	}
	// Non-key messages (cursor blink, etc.) drive the focused text input.
	if m.mode == modeForm {
		var cmd tea.Cmd
		m.form, cmd = m.form.update(msg)
		return m, cmd
	}
	if m.searching {
		var cmd tea.Cmd
		m.search, cmd = m.search.Update(msg)
		return m, cmd
	}
	return m, nil
}

// updateKey routes a key press to the search bar (which captures keys while it
// is focused, ahead of the main-view bindings), the open dialog, or the main
// view.
func (m model) updateKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if msg.String() == "ctrl+c" {
		return m, tea.Quit
	}
	if m.searching {
		return m.updateSearch(msg)
	}
	switch m.mode {
	case modeForm:
		return m.updateForm(msg)
	case modeConfirm:
		return m.updateConfirm(msg)
	case modeHelp:
		return m.updateHelp(msg)
	case modeStatePick:
		return m.updateStatePick(msg)
	case modeStateFilter:
		return m.updateStateFilter(msg)
	default:
		return m.updateMain(msg)
	}
}

func (m model) updateMain(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// While an item is grabbed, the arrows drive the move instead of navigation.
	if m.focused().grabbed {
		return m.updateMove(msg)
	}
	m.status = ""
	switch msg.String() {
	case "q":
		return m, tea.Quit
	case "esc":
		return m.escOrQuit()
	case "/":
		return m.openSearch()
	case "?":
		m.mode, m.helpOffset = modeHelp, 0
	case "f":
		m.pick = newFilterPicker("Filter "+m.view.title()+" by state", m.view.columns(), m.view.filterSet())
		m.mode = modeStateFilter
	case "k":
		m.setUnified(!m.view.all)
	case "v":
		m.toggleSplit()
	case "x":
		if m.split {
			m.sideFocus = !m.sideFocus
		}
	case "up", "w":
		m.focused().moveUp()
	case "down", "s":
		m.focused().moveDown()
	case "pgup":
		m.focused().pageUp()
	case "pgdown":
		m.focused().pageDown()
	case "left":
		m.focused().collapse()
	case "right":
		m.focused().expand()
	default:
		if c, ok := tabKey(msg.String(), m.view.column); ok {
			if m.view.all {
				m.status = "press k to show the board"
				return m, nil
			}
			m.setColumn(c)
			return m, nil
		}
		return m.updateMainItemKey(msg)
	}
	return m, nil
}

// updateHelp handles keys while the shortcuts dialog is open: ?, esc, q or
// enter close it (q does not quit from here), the arrows and PgUp/PgDn scroll it
// when it is taller than the terminal; everything else is ignored.
func (m model) updateHelp(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	maxOff := helpMaxOffset(m.width, m.height)
	switch msg.String() {
	case "?", "esc", "q", "enter":
		m.mode = modeMain
	case "up", "w":
		m.helpOffset--
	case "down", "s":
		m.helpOffset++
	case "pgup":
		m.helpOffset -= 10
	case "pgdown":
		m.helpOffset += 10
	}
	m.helpOffset = max(min(m.helpOffset, maxOff), 0)
	return m, nil
}

// msgCategoriesInBacklog is the footer hint for a category key pressed outside
// the Backlog tab: a category made or kept elsewhere would show only while it
// holds that tab's tasks, so categories are managed in Backlog alone.
const msgCategoriesInBacklog = "categories are managed in Backlog"

// categoryLocked reports whether rename, delete or move of it is refused: it is
// a category and the tab is not Backlog.
func (m model) categoryLocked(it *todo.Item) bool {
	return it != nil && it.Kind == todo.Category && !m.view.editsCategories()
}

// escOrQuit clears an active search (dismissing it the same way it's
// cancelled), else the tab's state filter, or quits when there is nothing to
// dismiss.
func (m model) escOrQuit() (tea.Model, tea.Cmd) {
	if m.view.filter != "" {
		m.clearFilter()
		return m, nil
	}
	if states := m.view.activeStates(); len(states) > 0 {
		m.clearStates()
		return m, nil
	}
	return m, tea.Quit
}

// updateStatePick handles keys in the "t" dialog: the arrows move between
// states, enter sets the highlighted one, esc or q cancels.
func (m model) updateStatePick(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "up", "w":
		m.pick.move(-1)
	case "down", "s":
		m.pick.move(1)
	case "esc", "q":
		m.mode, m.pickItem = modeMain, nil
	case "enter":
		it, st := m.pickItem, m.pick.selected()
		m.mode, m.pickItem = modeMain, nil
		return m.setState(it, st)
	}
	return m, nil
}

// updateStateFilter handles keys in the "f" dialog: the arrows move, space or x
// ticks a state, enter applies the ticked states to the tab (none ticked turns
// the state filter off for it), esc or q cancels.
func (m model) updateStateFilter(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "up", "w":
		m.pick.move(-1)
	case "down", "s":
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

// updateMainItemKey handles the item-scoped main-view keys (add/edit/delete/
// toggle/view) for a real selected item. The "+ new category" placeholder row
// has its own reduced key set (see updatePlaceholderKey).
func (m model) updateMainItemKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.focused().onPlaceholder() {
		return m.updatePlaceholderKey(msg)
	}
	switch msg.String() {
	case "enter":
		return m.openForm(targetEdit)
	case " ":
		// Space folds a category. A task's state is set only through the t dialog.
		if sel := m.focused().selected(); sel != nil && sel.Kind == todo.Category {
			m.focused().toggleFold()
		}
		return m, nil
	case "p":
		return m.shiftColumn(1)
	case "P":
		return m.shiftColumn(-1)
	case "t":
		return m.openStatePicker()
	case "n":
		return m.openForm(targetAddSibling)
	case "N":
		return m.openForm(targetAddChild)
	case "c":
		return m.openForm(targetAddCategory)
	case "m":
		return m.startMove()
	case "e":
		return m.openForm(targetEdit)
	case "y":
		return m.copySelection()
	case "d":
		return m.openDelete()
	case "D":
		return m.openClearDone()
	}
	return m, nil
}

// updatePlaceholderKey handles the item keys while the cursor is on the
// "+ new category" placeholder row, where the only meaningful actions add a
// top-level category (enter/n/N/c) or clear completed tasks (D); every other
// item key is a no-op.
func (m model) updatePlaceholderKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter", "n", "N", "c":
		return m.openForm(targetAddCategory)
	case "D":
		return m.openClearDone()
	}
	return m, nil
}

// startMove picks up the selected item for moving. It is a no-op on the
// placeholder row or an empty tab, refuses a category outside Backlog, and
// refuses while a filter is active — the filtered view hides siblings and
// ignores folding, so reordering against it would be misleading.
func (m model) startMove() (tea.Model, tea.Cmd) {
	if m.focused().onPlaceholder() || m.focused().selected() == nil {
		return m, nil
	}
	if m.categoryLocked(m.focused().selected()) {
		m.status = msgCategoriesInBacklog
		return m, nil
	}
	if m.focused().filtering() {
		m.status = "Clear the filter (esc) before moving items."
		return m, nil
	}
	m.focused().grabbed = true
	return m, nil
}

// updateMove handles keys while an item is grabbed: the arrows reorder it (↑/↓)
// or re-nest it (←/→), and m/esc/enter drop it. A move the document refuses (a
// clamp at the ends, or an illegal re-nest) leaves the item grabbed so the user
// can pick another direction.
func (m model) updateMove(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	m.status = ""
	switch msg.String() {
	case "up", "w":
		return m.applyMove(m.stepPastShown(m.doc.MoveUp, m.doc.MoveDown))
	case "down", "s":
		return m.applyMove(m.stepPastShown(m.doc.MoveDown, m.doc.MoveUp))
	case "left":
		return m.applyMove(m.doc.Outdent)
	case "right":
		return m.applyIndent()
	case "m", "esc", "enter":
		m.focused().grabbed = false
	}
	return m, nil
}

// stepPastShown wraps a one-step reorder so that it moves the item past one
// row the tab shows. Every tab can hide items between its rows (Backlog hides
// the other columns' tasks, other tabs hide Backlog's, a search hides
// non-matches), so op repeats until the item has passed a row that was shown
// before the move or has entered another parent (one category per key press,
// as when nothing is hidden). If op refuses first, the item only stepped over
// hidden siblings, so undo walks it back and the move is a no-op; it reports
// whether the item moved. Where nothing is hidden, the first step already
// passes a shown row.
func (m model) stepPastShown(op, undo func(*todo.Item) bool) func(*todo.Item) bool {
	return func(it *todo.Item) bool {
		start, parent := m.focused().shownBefore(it), it.Parent
		steps := 0
		for op(it) {
			steps++
			if it.Parent != parent || m.focused().shownBefore(it) != start {
				return true
			}
		}
		// Every step was a swap with a hidden sibling under the same parent, which
		// undo swaps straight back.
		for ; steps > 0; steps-- {
			undo(it)
		}
		return false
	}
}

// applyIndent indents the grabbed item under the item just above it among its
// siblings. A subtask, or a task with subtasks, never indents (tasks nest one
// level only, so Document.Indent refuses it), and neither does an item whose
// new parent the tab hides, since it would vanish into that hidden task; the
// footer says why. Any other refusal (nothing above to nest under) stays
// silent, as before.
func (m model) applyIndent() (tea.Model, tea.Cmd) {
	it := m.selectedItem()
	if it == nil {
		return m, nil
	}
	if it.IsTask() && (it.IsSubtask() || len(it.Children) > 0) {
		m.status = "subtasks can't have subtasks"
		return m, nil
	}
	if prev := prevSibling(it); prev != nil && prev.Kind == it.Kind && !m.focused().shown[prev] {
		m.status = "can't nest under a task hidden in this tab"
		return m, nil
	}
	return m.applyMove(m.doc.Indent)
}

// prevSibling returns the sibling just before it under the same parent, or nil
// when it is first or a root item.
func prevSibling(it *todo.Item) *todo.Item {
	if it.Parent == nil {
		return nil
	}
	if i := slices.Index(it.Parent.Children, it); i > 0 {
		return it.Parent.Children[i-1]
	}
	return nil
}

// applyMove runs one move op on the grabbed item. On success it persists the
// item's new placement, reloads, keeps the cursor on the moved item (still
// grabbed) and expands its (possibly new) parent so it stays visible; the
// structural change also ends the accidental-complete undo window. A no-op
// result changes nothing and keeps the item grabbed. If the item is no longer
// shown in the tab afterwards (a re-nest changed its state, or a write landed
// from elsewhere), the grab ends, the cursor keeps its row index (clamped) and
// the footer says where the item went.
func (m model) applyMove(op func(*todo.Item) bool) (tea.Model, tea.Cmd) {
	it := m.selectedItem()
	if it == nil || !op(it) {
		return m, nil
	}
	m.snapshots = map[uint]todo.DoneStates{}
	if it.Parent != nil {
		delete(m.focused().collapsed, it.Parent.ID) // reveal the item under its (possibly new) parent
	}
	id := it.ID
	parentID, afterID := m.doc.Placement(it)
	m.commit(id, func(ctx context.Context) error {
		return m.store.Move(ctx, id, parentID, afterID)
	})
	if m.noteLeftTab(id) {
		m.focused().grabbed = false
	}
	return m, nil
}

// openSearch focuses the "/" search bar. It seeds the input with the current
// filter so pressing "/" again refines the existing query instead of starting
// over; the filtered view stays put while typing.
func (m model) openSearch() (tea.Model, tea.Cmd) {
	ti := textinput.New()
	ti.Prompt = ""
	ti.CharLimit = 100
	ti.Width = m.searchWidth()
	ti.SetValue(m.view.filter)
	ti.CursorEnd()
	m.search = ti
	m.searching = true
	return m, tea.Batch(m.search.Focus(), textinput.Blink)
}

// updateSearch handles keys while the search bar is focused: esc cancels (and
// clears the filter), enter confirms (keeps the filter, returns to navigation),
// up/down (and PgUp/PgDn) move the selection through the results, and everything
// else edits the query and re-filters live.
func (m model) updateSearch(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.clearFilter()
		return m, nil
	case "enter":
		m.searching = false
		m.search.Blur()
		return m, nil
	case "up":
		m.focused().moveUp()
		return m, nil
	case "down":
		m.focused().moveDown()
		return m, nil
	case "pgup":
		m.focused().pageUp()
		return m, nil
	case "pgdown":
		m.focused().pageDown()
		return m, nil
	}
	var cmd tea.Cmd
	m.search, cmd = m.search.Update(msg)
	m.setFilter(m.search.Value())
	return m, cmd
}

// clearFilter closes the search bar and drops the active filter, restoring the
// full tree (and its collapse state). The item under the cursor stays selected,
// so returning from a search leaves you on the item you navigated to rather than
// back at the top.
func (m *model) clearFilter() {
	m.searching = false
	sel := m.focused().selected()
	m.setFilter("")
	if sel != nil {
		m.focused().revealItem(sel)
	}
}

// searchWidth sizes the search input to the footer width.
func (m model) searchWidth() int {
	w := m.width - 6
	if w < 10 {
		w = 10
	}
	return w
}

// openDelete opens the confirm dialog for the selected item (focused on the
// safe Cancel button). A no-op when nothing is selected; a category outside
// Backlog is refused with a footer hint.
func (m model) openDelete() (tea.Model, tea.Cmd) {
	sel := m.focused().selected()
	if sel == nil || m.focused().onPlaceholder() {
		return m, nil
	}
	if m.categoryLocked(sel) {
		m.status = msgCategoriesInBacklog
		return m, nil
	}
	m.deleting = sel
	m.confirmOnDelete = false
	m.confirmAction = confirmDelete
	m.mode = modeConfirm
	return m, nil
}

// openClearDone opens the confirm dialog for removing every completed task
// (focused on the safe Cancel button). A no-op with a status hint when there is
// nothing to remove.
func (m model) openClearDone() (tea.Model, tea.Cmd) {
	if m.doc.RemovableDone() == 0 {
		m.status = "No completed tasks to remove."
		return m, nil
	}
	m.deleting = nil
	m.confirmOnDelete = false
	m.confirmAction = confirmClearDone
	m.mode = modeConfirm
	return m, nil
}

// updateConfirm handles the confirm dialog: y/enter-on-Delete confirms, n/esc
// cancels, tab/arrows toggle the focused button.
func (m model) updateConfirm(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "y", "Y":
		return m.doConfirm()
	case "n", "N", "esc":
		m.mode = modeMain
		return m, nil
	case "left", "right", "tab", "shift+tab":
		m.confirmOnDelete = !m.confirmOnDelete
		return m, nil
	case "enter", " ":
		if m.confirmOnDelete {
			return m.doConfirm()
		}
		m.mode = modeMain
		return m, nil
	}
	return m, nil
}

// doConfirm runs the confirmed action for the open dialog.
func (m model) doConfirm() (tea.Model, tea.Cmd) {
	if m.confirmAction == confirmClearDone {
		return m.doClearDone()
	}
	return m.doDelete()
}

// doClearDone removes every completed task from the store (which works out the
// fully-done subtrees itself, as they are at write time), reloads, and keeps
// the selection on the previously selected item when it survived (otherwise the
// top row). Like a delete, it ends the accidental-complete undo window.
func (m model) doClearDone() (tea.Model, tea.Cmd) {
	m.mode = modeMain
	var focus uint
	if sel := m.focused().selected(); sel != nil && !m.focused().onPlaceholder() {
		focus = sel.ID
	}
	m.snapshots = map[uint]todo.DoneStates{}
	var removed int
	var pruneErr error
	m.commit(focus, func(ctx context.Context) error {
		removed, pruneErr = m.store.Prune(ctx)
		return pruneErr
	})
	if sel := m.focused().selected(); focus != 0 && (sel == nil || sel.ID != focus) {
		m.focused().selectTop() // sel was removed
	}
	if pruneErr == nil {
		m.status = fmt.Sprintf("Removed %d completed task(s).", removed)
	}
	return m, nil
}

// doDelete removes the confirmed item (and its subtree) from the store,
// reloads, and moves the selection to a sensible neighbour — the row above the
// deleted item, which is never one of its descendants.
func (m model) doDelete() (tea.Model, tea.Cmd) {
	it := m.deleting
	m.deleting = nil
	m.mode = modeMain
	if it == nil {
		return m, nil
	}
	var prevID uint
	if m.focused().cursor > 0 {
		prevID = m.focused().rows[m.focused().cursor-1].item.ID
	}
	m.doc.Remove(it)
	// A structural change ends the accidental-complete undo window.
	m.snapshots = map[uint]todo.DoneStates{}
	id := it.ID
	m.commit(prevID, func(ctx context.Context) error {
		return m.store.Delete(ctx, id)
	})
	return m, nil
}

// selectedItem returns the item under the cursor, or nil on an empty tab or
// the placeholder row, where the item keys do nothing.
func (m model) selectedItem() *todo.Item {
	if m.focused().onPlaceholder() {
		return nil
	}
	return m.focused().selected()
}

// toggleSubtask flips subtask it between open and done. The manual change
// invalidates its parent's undo snapshot, since reverting the parent later
// would fight it.
func (m model) toggleSubtask(it *todo.Item) (tea.Model, tea.Cmd) {
	delete(m.snapshots, it.Parent.ID)
	st := todo.Done
	if it.IsDone() {
		st = todo.Open
	}
	it.Status = st
	m.commit(it.ID, m.setStatusesWrite(it.ID, st))
	return m, nil
}

// complete moves top-level task it into Done and cascades done to its
// subtasks. It first records the undo snapshot — the task's own state
// included, even without subtasks — so an accidental complete can be undone
// exactly (see openStatePicker and reopen).
func (m model) complete(it *todo.Item) (tea.Model, tea.Cmd) {
	id := it.ID
	m.snapshots[id] = todo.SnapshotDone(it)
	write := m.setStatusesWrite(id, todo.Done)
	if len(it.Children) > 0 {
		write = func(ctx context.Context) error { return m.store.CascadeStatus(ctx, id, todo.Done) }
	}
	todo.CascadeSetDone(it, true)
	m.commitStatus(id, write)
	return m, nil
}

// reopen takes done top-level task it out of Done into state st. With an undo
// snapshot from completing it, its subtasks get their snapshot states back;
// without one they keep theirs. The snapshot is used up either way.
func (m model) reopen(it *todo.Item, st todo.Status) (tea.Model, tea.Cmd) {
	states := map[uint]todo.Status{}
	maps.Copy(states, m.snapshots[it.ID])
	delete(m.snapshots, it.ID)
	states[it.ID] = st
	m.doc.ApplyStatuses(states)
	m.commitStatus(it.ID, func(ctx context.Context) error { return m.store.SetStatuses(ctx, states) })
	return m, nil
}

// shiftColumn is p/P: it moves the selected top-level task delta columns along
// the board (no wrap), into the target column's first state. Into Done it
// completes the task (cascade and undo snapshot); out of Done it reopens it,
// restoring its subtasks from the snapshot when there is one. A subtask (it
// follows its task) and a task at the board's edge only get a footer hint.
func (m model) shiftColumn(delta int) (tea.Model, tea.Cmd) {
	it := m.selectedItem()
	switch {
	case it == nil:
		return m, nil
	case !it.IsTask():
		m.status = "only tasks move between columns"
		return m, nil
	case it.IsSubtask():
		m.status = "subtasks move with their task"
		return m, nil
	}
	cols := todo.Columns()
	cur := slices.Index(cols, it.Status.Column())
	if cur < 0 {
		cur = 0 // a column off the board (an unknown status) counts as Backlog
	}
	i := cur + delta
	if i < 0 || i >= len(cols) {
		m.status = "no column beyond " + cols[cur].Title()
		return m, nil
	}
	target := cols[i]
	switch {
	case target == todo.ColDone:
		return m.complete(it)
	case it.IsDone():
		return m.reopen(it, target.FirstState())
	}
	it.Status = target.FirstState()
	m.commitStatus(it.ID, m.setStatusesWrite(it.ID, it.Status))
	return m, nil
}

// openStatePicker is t: it opens the dialog that sets the selected task's state
// (any state for a top-level task, open or done for a subtask), the cursor on
// its current state. On a task completed in this session the cursor starts on
// the state it had before, so t then enter undoes an accidental complete.
// Categories have no state, so they get a footer hint instead.
func (m model) openStatePicker() (tea.Model, tea.Cmd) {
	it := m.selectedItem()
	switch {
	case it == nil:
		return m, nil
	case !it.IsTask():
		m.status = "only tasks have a state"
		return m, nil
	}
	m.pick, m.pickItem = newStatePicker(it), it
	if prev, ok := m.snapshots[it.ID][it.ID]; ok && it.IsDone() {
		m.pick.focus(prev)
	}
	m.mode = modeStatePick
	return m, nil
}

// setState moves task it to state st, picked in the "t" dialog. A subtask
// toggles between open and done. Into Done a top-level task completes (cascade
// and undo snapshot), out of Done it reopens; any other change is a plain state
// write, which may move the task to another column. Picking the current state
// changes nothing.
func (m model) setState(it *todo.Item, st todo.Status) (tea.Model, tea.Cmd) {
	switch {
	case it == nil || st == it.Status:
		return m, nil
	case it.IsSubtask():
		return m.toggleSubtask(it)
	case st == todo.Done:
		return m.complete(it)
	case it.IsDone():
		return m.reopen(it, st)
	}
	it.Status = st
	m.commitStatus(it.ID, m.setStatusesWrite(it.ID, st))
	return m, nil
}

// setStatusesWrite returns the store write that sets item id to st.
func (m model) setStatusesWrite(id uint, st todo.Status) func(ctx context.Context) error {
	return func(ctx context.Context) error {
		return m.store.SetStatuses(ctx, map[uint]todo.Status{id: st})
	}
}

// commitStatus commits write, a status change of top-level task id, like
// commit. When the change took the task out of the tab, the cursor keeps its
// row index (clamped), now on the row that took the task's place, and the
// footer says where the task went, e.g. "moved to Next — new". A failed write
// or an external-reload notice takes the footer instead.
func (m *model) commitStatus(id uint, write func(ctx context.Context) error) {
	m.commit(id, write)
	m.noteLeftTab(id)
}

// noteLeftTab reports whether item id, after a commit, is no longer shown in
// the tab (moved to another column, or gone). When a task left for another
// column, the footer says where it went, e.g. "moved to Next — new" (a subtask
// goes with its task, so its task's column and state are named), unless a save
// error or the external-reload notice already holds the footer.
func (m *model) noteLeftTab(id uint) bool {
	it, ok := m.doc.ByID(id)
	if ok && m.focused().shown[it] {
		return false
	}
	if ok && it.IsTask() && m.err == nil && m.status == "" {
		task := it
		if it.IsSubtask() {
			task = it.Parent
		}
		m.status = fmt.Sprintf("moved to %s — %s", task.Status.Column().Title(), task.Status.Label())
	}
	return true
}

// writeClipboard copies text to the system clipboard. It is a package variable
// so tests can stub it — and so the copy path never touches the real clipboard
// in CI.
var writeClipboard = clipboard.WriteAll

// copySelection copies the selected item to the system clipboard as plain text,
// so a task with a multi-line description can be grabbed whole: the edit dialog
// word-wraps it inside a bordered box, which the terminal's mouse selection can't
// cleanly capture. On the platforms without a clipboard tool (a bare Linux box),
// it reports how to get one; any copy error and the success both show in the footer.
func (m model) copySelection() (tea.Model, tea.Cmd) {
	it := m.focused().selected()
	if it == nil {
		return m, nil
	}
	if clipboard.Unsupported {
		m.status = "No clipboard tool found — install xclip, xsel or wl-clipboard."
		return m, nil
	}
	if err := writeClipboard(itemCopyText(it)); err != nil {
		m.status = "Copy failed: " + err.Error()
		return m, nil
	}
	m.status = "Copied to clipboard."
	return m, nil
}

// itemCopyText renders an item as plain text for the clipboard: the title, then
// a blank line and the raw (unwrapped) description when the item is a task that
// has one. This is the whole item as text, independent of how the pane wraps it.
func itemCopyText(it *todo.Item) string {
	if it.IsTask() && it.Description != "" {
		return it.Title + "\n\n" + it.Description
	}
	return it.Title
}

// openForm prepares and opens the add/edit modal for the given target. Outside
// Backlog it refuses to add, rename or delete a category, and it refuses a
// subtask under a subtask.
func (m model) openForm(t formTarget) (tea.Model, tea.Cmd) {
	sel := m.focused().selected()
	switch t {
	case targetAddSibling, targetAddChild:
		if sel == nil || m.focused().onPlaceholder() {
			// Outside Backlog a tab shows only categories holding its tasks, and c
			// is refused there, so point at a tab that shows the category instead.
			m.status = "Add a category first (press c) — tasks live under a category."
			if !m.view.editsCategories() {
				m.status = "add tasks from a tab that shows their category (Backlog shows all)"
			}
			return m, nil
		}
		if t == targetAddChild && sel.IsSubtask() {
			m.status = "subtasks can't have subtasks"
			return m, nil
		}
		m.form = newForm("", "", "", true, false)
	case targetEdit:
		if sel == nil || m.focused().onPlaceholder() {
			return m, nil
		}
		if m.categoryLocked(sel) {
			m.status = msgCategoriesInBacklog
			return m, nil
		}
		m.editing = sel
		m.form = newForm(sel.Title, sel.Description, sel.Plan, sel.IsTask(), true)
		m.form.meta = itemMeta(sel)
	case targetAddCategory:
		if !m.view.editsCategories() {
			m.status = msgCategoriesInBacklog
			return m, nil
		}
		m.form = newForm("", "", "", false, false)
	}
	m.target = t
	m.form.setWidth(m.width)
	m.mode = modeForm
	return m, textinput.Blink
}

func (m model) updateForm(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.mode = modeMain
		return m, nil
	case "tab":
		return m, m.form.focusStep(1)
	case "shift+tab":
		return m, m.form.focusStep(-1)
	case "left":
		if m.form.focus == focusCancel {
			m.form.focus = focusSave
			return m, m.form.applyFocus()
		}
	case "right":
		if m.form.focus == focusSave {
			m.form.focus = focusCancel
			return m, m.form.applyFocus()
		}
	case "enter":
		// Enter in the description inserts a newline; on the title or Save it
		// submits, on Cancel it closes.
		if m.form.focus == focusDesc {
			break
		}
		if m.form.focus == focusCancel {
			m.mode = modeMain
			return m, nil
		}
		return m.submitForm()
	}
	var cmd tea.Cmd
	m.form, cmd = m.form.update(msg)
	return m, cmd
}

// submitForm applies the open form: editing the selected item, or inserting a
// new task/subtask/category at the right spot. The insert is first made in
// memory, which works out its placement, and then stored there. An empty title
// is ignored (the dialog stays open). Any structural or content change clears
// the undo snapshots — the accidental-complete window is meant to be immediate.
func (m model) submitForm() (tea.Model, tea.Cmd) {
	title, desc, plan := m.form.values()
	if title == "" {
		return m, nil
	}
	m.snapshots = map[uint]todo.DoneStates{}
	m.mode = modeMain

	if m.target == targetEdit {
		m.editing.Title = title
		if m.editing.IsTask() {
			m.editing.Description = desc
			m.editing.Plan = plan
		}
		id := m.editing.ID
		m.commit(id, func(ctx context.Context) error {
			return m.store.Edit(ctx, id, todo.Fields{Title: title, Description: desc, Plan: plan})
		})
		return m, nil
	}

	sel := m.focused().selected()
	var added *todo.Item
	switch m.target {
	case targetAddCategory:
		added = &todo.Item{Kind: todo.Category, Title: title, Level: 1}
		// A category — or a task inside one — selected → subcategory of the
		// enclosing category; the root placeholder → a new top-level category.
		var parentCat *todo.Item
		if !m.focused().onPlaceholder() && sel != nil {
			parentCat = sel.EnclosingCategory()
		}
		if parentCat != nil {
			added.Level = parentCat.Level + 1
			parentCat.AppendChild(added)
			delete(m.focused().collapsed, parentCat.ID)
		} else {
			m.doc.AppendRoot(added)
		}
	case targetAddChild: // a subtask of a task, or a task inside a category
		added = todo.NewTask(title, desc, false)
		added.Plan = plan
		sel.AppendTask(added)
		delete(m.focused().collapsed, sel.ID)
	default: // targetAddSibling: a task at the end of the selection's level
		added = todo.NewTask(title, desc, false)
		added.Plan = plan
		parent := sel
		if sel.IsTask() && sel.Parent != nil {
			parent = sel.Parent // a task's siblings live under its parent
		}
		parent.AppendTask(added)
		delete(m.focused().collapsed, parent.ID)
	}
	// A new task enters the current tab in its column's first state, so it shows
	// where it was added; a new subtask starts open (subtasks are open or done).
	if added.Kind == todo.Task && !added.IsSubtask() {
		added.Status = m.view.newTaskState()
	}

	parentID, afterID := m.doc.Placement(added)
	var newID uint
	m.commit(0, func(ctx context.Context) error {
		var err error
		if added.Kind == todo.Category {
			newID, err = m.store.AddCategory(ctx, title, parentID, afterID)
		} else {
			newID, err = m.store.AddTask(ctx, todo.Fields{Title: title, Description: desc, Plan: plan}, added.Status, parentID, afterID)
		}
		return err
	})
	m.focused().selectID(newID) // the ID is only known once the write ran
	return m, nil
}

// commit runs write against the store and then reloads, so the view always
// shows what the store holds. A write error is kept for the footer (a success
// clears the last one). The reload happens either way: it records the
// revision this write produced (so the poll does not report it as an external
// change) and rebuilds the tree, keeping the folds of the items that still
// exist, the viewport, the filter and the grab state, and selecting focusID if
// present (else keeping the cursor index, clamped). The undo snapshots are
// kept, since this is the app's own write — unless the reload shows that
// someone else wrote too: a successful store write bumps the revision exactly
// once and a failed one rolls back, so any other revision than that means an
// external write landed since the last load, and the reload is handled as
// external (snapshots dropped, the status line says so). If the reload itself
// fails, the current document stays, its rows are rebuilt (the caller may have
// changed its structure) with focusID selected, and the error is shown: the
// write's when it failed, else the reload's.
func (m *model) commit(focusID uint, write func(ctx context.Context) error) {
	ctx := context.Background()
	m.err = nil
	expected := m.lastRev
	if err := write(ctx); err != nil {
		m.err = fmt.Errorf("save failed: %w", err)
	} else {
		expected++
	}
	doc, rev, err := m.store.Load(ctx)
	if err != nil {
		if m.err == nil {
			m.err = fmt.Errorf("reload failed: %w", err)
		}
		m.focused().rebuild()
		m.focused().selectID(focusID)
		return
	}
	onPH := m.focused().onPlaceholder()
	m.doc, m.lastRev = doc, rev
	m.adoptAll(doc, focusID, onPH)
	if rev != expected {
		m.snapshots = map[uint]todo.DoneStates{}
		m.status = "reloaded — list changed externally"
	}
}

// confirmQuestion phrases the prompt for the open confirm dialog.
func (m model) confirmQuestion() string {
	if m.confirmAction == confirmClearDone {
		return fmt.Sprintf("Remove %d completed task(s)?", m.doc.RemovableDone())
	}
	return deletionQuestion(m.deleting)
}

func (m model) View() string {
	switch m.mode {
	case modeForm:
		return placeCenter(m.mainView(true), m.form.view())
	case modeConfirm:
		return placeCenter(m.mainView(true), confirmModal(m.confirmQuestion(), m.confirmOnDelete, m.width))
	case modeHelp:
		return placeCenter(m.mainView(true), helpModal(m.width, m.height, m.helpOffset))
	case modeStatePick, modeStateFilter:
		return placeCenter(m.mainView(true), m.pick.view())
	default:
		return m.mainView(false)
	}
}

// mainView composes the header, the task tree, and the footer to the terminal
// size. When dim is true the tree renders unfocused (the dimmed backdrop behind
// a modal).
func (m model) mainView(dim bool) string {
	w, h := m.width, m.height
	if w == 0 {
		w, h = 80, 24
	}
	bodyH := h - headerHeight - 1 // the footer takes the last line
	if bodyH < 3 {
		bodyH = 3
	}
	var body string
	if m.split {
		sw := sidebarWidth(w)
		left := titledBox("Categories", m.side.view(sw-2, m.treeViewHeight()), sw, bodyH, !dim && m.sideFocus)
		right := titledBox("Tasks", m.tree.view(w-sw-2, m.treeViewHeight()), w-sw, bodyH, !dim && !m.sideFocus)
		body = lipgloss.JoinHorizontal(lipgloss.Top, left, right)
	} else {
		body = titledBox("Tasks", m.tree.view(w-2, m.treeViewHeight()), w, bodyH, !dim)
	}
	return m.header(w) + "\n" + body + "\n" + m.footer(w)
}

// treeViewHeight is how many task rows the tree pane can show at the current
// terminal size — the height handed to tree.view. WindowSizeMsg stores it on the
// tree so its scroll math (reconcileOffset) matches what mainView renders; the
// two must agree or the viewport clamp would fight the offset. It mirrors the
// body-height layout math in mainView.
func (m model) treeViewHeight() int {
	h := m.height
	if m.width == 0 {
		h = 24 // matches mainView's fallback when the terminal size is unknown
	}
	bodyH := h - headerHeight - 1
	if bodyH < 3 {
		bodyH = 3
	}
	vh := bodyH - 2
	if vh < 1 {
		vh = 1
	}
	return vh
}

// headerHeight is how many lines the header takes: the title line and the tab
// bar.
const headerHeight = 2

// header renders the title line (app, version, file, overall progress) and,
// under it, the board's tab bar.
func (m model) header(width int) string {
	done, total := overallCounts(m.doc)
	left := headerAppStyle.Render("todo") + " " + headerHintSty.Render(m.version) + "  " + headerHintSty.Render(filepath.Base(m.store.Path()))
	right := headerHintSty.Render(fmt.Sprintf("%d/%d done", done, total))
	gap := width - 1 - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		gap = 1
	}
	title := ansi.Truncate(" "+left+strings.Repeat(" ", gap)+right, width, "")
	counts := m.doc.ColumnCounts()
	if m.view.all {
		total := 0
		for _, n := range counts {
			total += n
		}
		return title + "\n" + allStatesBar(total, width)
	}
	return title + "\n" + tabBar(counts, m.view.column, width)
}

func (m model) footer(width int) string {
	switch {
	case m.searching:
		// The live search bar replaces the footer while typing a query.
		return ansi.Truncate(" "+helpKeyStyle.Render("/")+" "+m.search.View(), width, "")
	case m.err != nil:
		return ansi.Truncate(" "+errStyle.Render(m.err.Error()), width, "")
	case m.status != "":
		// A transient message (a refused move, where a task went) wins over the
		// move and filter lines; the next key clears it.
		return ansi.Truncate(" "+helpTextStyle.Render(m.status), width, "")
	case m.focused().grabbed:
		// Move mode: the arrows reorder/re-nest the grabbed item.
		parts := []string{
			helpKeyStyle.Render("MOVING"),
			hint("↑↓", "Move"), hint("→", "Child"), hint("←", "Sibling"), hint("m/esc", "Drop"),
		}
		return ansi.Truncate(" "+strings.Join(parts, "  "), width, "")
	case m.view.filter != "" || len(m.view.activeStates()) > 0:
		// A search and/or state filter is applied but the bar is closed: show it,
		// with how to edit/clear.
		var info []string
		if m.view.filter != "" {
			info = append(info, helpKeyStyle.Render("filter")+helpTextStyle.Render(": "+m.view.filter), hint("/", "Edit"))
		}
		if states := m.view.activeStates(); len(states) > 0 {
			names := make([]string, len(states))
			for i, st := range states {
				names[i] = st.Label()
			}
			info = append(info, helpKeyStyle.Render("states")+helpTextStyle.Render(": "+strings.Join(names, ", ")), hint("f", "Edit"))
		}
		info = append(info, hint("esc", "Clear"))
		return ansi.Truncate(" "+strings.Join(info, "  "), width, "")
	}
	// The full key list lives in the "?" dialog; the footer only points at it.
	parts := []string{hint("?", "Help"), hint("/", "Search"), hint("q", "Quit")}
	if m.split {
		parts = append(parts, hint("x", "Pane"))
	}
	return ansi.Truncate(" "+strings.Join(parts, "  "), width, "")
}

// itemMeta renders the read-only header of the edit dialog: a task's state glyph,
// column and state, subtask progress and a missing-plan flag, or a category label and its task
// count. It carries the old details view's at-a-glance info into the combined
// view/edit dialog. Empty when there is nothing to show.
func itemMeta(it *todo.Item) string {
	if it == nil {
		return ""
	}
	if it.Kind == todo.Category {
		done, total := it.TaskCounts()
		return labelStyle.Render("Category") + helpTextStyle.Render(fmt.Sprintf("  ·  %d of %d tasks done", done, total))
	}
	glyph, st := statusGlyph(it.Status, it.IsSubtask())
	col := it.Status.Column()
	if it.IsSubtask() && it.Parent != nil {
		col = it.Parent.Status.Column() // a subtask sits in its parent's column
	}
	status := st.Render(glyph + " " + col.Title() + " · " + it.Status.Label())
	if done, total := it.TaskCounts(); total > 0 {
		status += helpTextStyle.Render("  ·  ") + labelStyle.Render("Subtasks") + helpTextStyle.Render(fmt.Sprintf(" %d/%d done", done, total))
	}
	if it.NeedsPlan() {
		status += helpTextStyle.Render("  ·  ") + deferStyle.Render("⚠ no plan")
	}
	return status
}

// overallCounts totals done/total tasks across the whole document.
func overallCounts(d *todo.Document) (done, total int) {
	for _, r := range d.Roots {
		if r.Kind == todo.Task {
			total++
			if r.IsDone() {
				done++
			}
		}
		dn, tt := r.TaskCounts()
		done += dn
		total += tt
	}
	return done, total
}
