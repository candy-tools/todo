// Package tui is the Bubble Tea frontend of the TODO app: a single-panel
// category/task tree and an add/edit dialog that doubles as the selected item's
// detail view (it shows the item's status and subtask progress alongside its
// editable title and description), plus the keybindings that drive them. All
// domain logic lives in internal/todo; this package only renders it and routes keys.
package tui

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/candy-tools/todo/app/metainfo"
	"github.com/candy-tools/todo/internal/todo"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

type mode int

const (
	modeMain mode = iota
	modeForm
	modeConfirm
	modeHelp        // the "?" shortcuts dialog
	modeStatePick   // the "t" dialog: pick the selected task's state
	modeStateFilter // the "f" dialog: tick the states the tab shows
	modeSprintPick  // the K, a and o dialog: pick a sprint (or the Backlog)
	// modeSearch is the "/" search bar, focused and capturing keys. The query
	// lives in search; the resulting filter lives on the view (m.view.filter),
	// so it stays applied for navigation after the bar closes.
	modeSearch
)

// model is the whole TUI state. Its fields fall into four groups:
// persistence, layout, the mode with its per-mode data, and session data.
//
// Invariants:
//   - An item is grabbed (tree.grabbed or side.grabbed) only in modeMain and
//     only on the focused tree, m.focused().
//   - sideFocus implies split.
//   - listFocus means something only in the Sprints workspace (view.onBoard),
//     and holds there while the sidebar's cursor is on no sprint (the Closed
//     group row, an empty list): there is no board to take the keys.
//   - On screenPlan the unified view is off and the trees show nothing.
//   - An external reload is applied in modeMain and modeSearch, and deferred
//     in the modal modes and while an item is grabbed (see handleReload).
type model struct {
	// Persistence: the store, the document loaded from it, and the
	// fingerprint and errors of the last load or write.
	store *todo.Store
	// doc is the list as the store last gave it: a load, or a write's result.
	// It is read only: an action sends its change to the store and shows the
	// list the write left (see commit).
	doc *todo.Document
	// lastFP is the fingerprint of the list file doc reflects: the one todo
	// last loaded, which after its own write is the file that write left. The
	// poll loop asks the store whether the file changed since, to tell an
	// external write (reload) from the app's own (ignore).
	lastFP todo.Fingerprint
	// docGen counts the lists the model took (see take); a probe carries the
	// count it started at, so a late one is told from a current one even when
	// the file went back to the same bytes in between.
	docGen uint64
	// probing is true while a probe of the file is in flight; no other starts
	// until its message arrives (see startProbe).
	probing bool
	err     error // last failed save or reload ("save failed: …" / "reload failed: …"), shown in the footer
	// quitErr is the failure of the view save quit made on the way out, for
	// Run to report once the screen is gone (see quitWarning).
	quitErr error
	// pollErr is the poll failure last put in err, kept to show each distinct
	// poll failure once and to clear it when a later poll succeeds.
	pollErr error
	// pollFails counts the polls in a row that failed; a failure is shown only
	// once pollFailsShown of them did (see handleReloadFailed).
	pollFails int

	// Layout: what the trees show and how the screen is split.
	//
	// view is the screen, search query and state filter every tree shows
	// the list through; the model changes it (switchScreen,
	// setFilter, setStates) and rebuilds the trees.
	view *viewState
	// sprints is the Sprints workspace's sidebar, beside the shown sprint's
	// board; listFocus is whether the keys go to it (true) or to the board.
	sprints   sprintList
	listFocus bool
	// lastSprint is the ID of the sprint whose board was shown last, where
	// "k" goes back to from the other screens.
	lastSprint string
	// baseView is the view (as viewOnScreen puts it) this session last knows
	// the list file holds: the one it opened on, last stored, or last found the
	// file opening on (see saveView). saveView stores no view equal to it, so a
	// view another session stored since is kept unless this one changed its own.
	baseView todo.View
	// plan is Planning's state (see planner), meaningful on screenPlan.
	plan planner
	// tree is the main pane: the category/task tree, or — in the split view —
	// the task list of the category picked in side, the sidebar.
	tree tree
	side tree
	// split is the split view ("v"): side on the left, tree as a task list on
	// the right. sideFocus is whether the keys go to the sidebar (tab flips it).
	// The Sprints workspace keeps split but does not show it (see splitShown).
	split     bool
	sideFocus bool
	width     int
	height    int

	// Mode and per-mode data: mode routes the keys; each field below is
	// meaningful only in the mode it names.
	mode mode

	form     form
	target   formTarget
	editing  *todo.Item // the item being edited, for targetEdit
	deleting *todo.Item // the item the confirm dialog is guarding, for modeConfirm
	// editingSprint is the ID of the sprint the open form edits, for
	// targetEditSprint; deletingSprint is the one the confirm dialog guards, for
	// confirmDeleteSprint.
	editingSprint  string
	deletingSprint *todo.Sprint
	// confirmOnYes is which confirm-dialog button has focus: false = Cancel/No
	// (the destructive dialogs' safe default so a stray key can't delete), true
	// = Delete/Yes (the quit dialog's default, so enter quits).
	confirmOnYes bool
	// confirmAction is what confirming the open dialog does — delete the selected
	// item, or clear all completed tasks.
	confirmAction confirmAction

	helpOffset int // how far the "?" shortcuts dialog is scrolled, for modeHelp

	pick     picker     // the open state dialog, for modeStatePick and modeStateFilter
	pickItem *todo.Item // the task the "t" dialog sets the state of
	// sprintPick is the open sprint dialog, for modeSprintPick.
	sprintPick sprintPicker

	search textinput.Model // the "/" search bar's input, for modeSearch

	// Session data: in memory only, never stored.
	//
	// snapshots holds, per task ID completed in this session, the states of the
	// task and its subtasks captured at completion time, so un-completing it can
	// restore them (the accidental-complete undo). Keyed by ID, it survives the
	// app's own writes; an external change clears it as soon as a write or a
	// poll finds it, even when the poll defers the reload.
	snapshots map[string]todo.DoneStates
	status    string // transient one-line footer message
	version   string // the app version shown in the header
}

// BusyTimeout is how long a TUI write waits for another process to release the
// list's write lock before it fails with todo.ErrBusy. Writes run on the UI
// thread, so the wait is kept well below the CLI's: the caller makes the store
// it passes to Run with todo.WithBusyTimeout(BusyTimeout).
const BusyTimeout = 750 * time.Millisecond

// Run loads the list from st and starts the interactive TUI. notice (e.g. the
// automatic TODO.md import summary) is shown in the footer at start; it may be
// empty. The caller makes st with BusyTimeout.
func Run(st *todo.Store, notice string) error {
	m, err := newModel(st, notice)
	if err != nil {
		return err
	}
	final, err := tea.NewProgram(m, tea.WithAltScreen()).Run()
	if err != nil {
		return err
	}
	// The task writes of the session succeeded: a view that could not be
	// stored is worth a line, not a failing exit.
	if fm, ok := final.(model); ok {
		if w := quitWarning(fm); w != "" {
			fmt.Fprintln(os.Stderr, w)
		}
	}
	return nil
}

// quitWarning is the line Run prints to stderr when the quit-time save of the
// view failed, or "" when it did not.
func quitWarning(m model) string {
	if m.quitErr == nil {
		return ""
	}
	return "todo: the view was not saved: " + m.quitErr.Error()
}

// newModel loads the list from st into a fresh model (main view, empty undo
// snapshots) with notice as the initial footer status. A multi-line notice (an
// import summary followed by its warnings) is joined onto the single footer
// line. Split out from Run so tests can drive the model without starting a real
// terminal program. It is all of Run's startup, and it only reads: opening a
// list to look at it leaves the file as it is.
func newModel(st *todo.Store, notice string) (model, error) {
	doc, fp, err := st.Load(context.Background())
	if err != nil {
		return model{}, err
	}
	v := newViewState()
	m := model{
		store:     st,
		version:   metainfo.Version,
		doc:       doc,
		view:      v,
		tree:      newTree(doc, v),
		side:      newSidebar(doc, v),
		mode:      modeMain,
		snapshots: map[string]todo.DoneStates{},
		lastFP:    fp,
		status:    strings.ReplaceAll(notice, "\n", " · "),
	}
	m.applyView(doc.View)
	m.baseView = m.viewOnScreen()
	return m, nil
}

// Init starts the change-poll loop that keeps the view in sync with external
// writes to the store.
func (m model) Init() tea.Cmd { return pollCmd() }

// fitTrees tells the trees how tall their viewports are for the view on
// screen, and re-anchors their scroll so the cursor stays visible; in
// Planning it does the same for both of its panes. Update runs it after every
// message, since a resize, a key (another workspace, sprint, the unified view)
// or a reload can change the height (the board's head takes rows); the stored
// height must always be the one mainView draws.
func (m *model) fitTrees() {
	if m.view.screen == screenPlan {
		m.plan.fit(m.doc, m.paneRows())
	}
	h := m.treeViewHeight()
	if m.tree.viewHeight != h {
		m.tree.viewHeight = h
		m.tree.reconcileOffset()
	}
	if m.side.viewHeight != h {
		m.side.viewHeight = h
		m.side.reconcileOffset()
	}
}

// Update handles msg (see update), then fits the trees to the view it left
// on screen.
func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	next, cmd := m.update(msg)
	if nm, ok := next.(model); ok {
		nm.fitTrees()
		return nm, cmd
	}
	return next, cmd
}

func (m model) update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		switch m.mode {
		case modeForm:
			m.form.setWidth(msg.Width)
		case modeSearch:
			m.search.Width = m.searchWidth()
		}
		return m, nil
	case reloadTickMsg:
		// Probe the store and re-arm the next tick; the probe runs off-goroutine.
		// startProbe marks the probe in flight on m, so it runs before m is
		// returned.
		probe := m.startProbe()
		return m, tea.Batch(probe, pollCmd())
	case probeMsg:
		return m.handleProbe(msg)
	case tea.KeyMsg:
		nm, cmd := m.updateKey(msg)
		// In the split view the task list follows the sidebar's category,
		// whatever the key did to it.
		if mm, ok := nm.(model); ok && mm.splitShown() {
			mm.syncScope()
			return mm, cmd
		}
		return nm, cmd
	}
	// Non-key messages (cursor blink, etc.) drive the focused text input.
	var cmd tea.Cmd
	switch m.mode {
	case modeForm:
		m.form, cmd = m.form.update(msg)
	case modeSearch:
		m.search, cmd = m.search.Update(msg)
	}
	return m, cmd
}

// updateKey routes a key press to the search bar (which captures keys while it
// is focused, ahead of the main-view bindings), the open dialog, the move of a
// grabbed item, or the main view.
func (m model) updateKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if msg.String() == "ctrl+c" {
		return m.quit()
	}
	switch m.mode {
	case modeSearch:
		return m.updateSearch(msg)
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
	case modeSprintPick:
		return m.updateSprintPick(msg)
	default:
		// While an item is grabbed, the arrows drive the move instead of navigation.
		if m.focused().grabbed {
			return m.updateMove(msg)
		}
		return m.updateMain(msg)
	}
}

// updateMain handles the main-view keys: Planning has its own (see
// updatePlan); elsewhere the screen keys and the view keys work everywhere,
// the Sprints workspace's sidebar has its own (see updateSprintList), and the
// rest act on the trees.
func (m model) updateMain(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	m.status = ""
	if m.view.screen == screenPlan {
		return m.updatePlan(msg)
	}
	if next, cmd, ok := m.updateScreenKey(msg); ok {
		return next, cmd
	}
	if next, cmd, ok := m.updateViewKey(msg); ok {
		return next, cmd
	}
	if m.onList() {
		return m.updateSprintList(msg)
	}
	switch msg.String() {
	case "tab", "shift+tab":
		switch {
		case m.view.onBoard():
			m.listFocus = true
		case m.splitShown():
			m.sideFocus = !m.sideFocus
		}
	case "up":
		m.focused().moveUp()
	case "down":
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
		if c, ok := tabKey(msg.String(), m.boardColumn()); ok && m.view.screen == screenSprint {
			if m.view.all {
				m.status = "press u to show the board"
				return m, nil
			}
			dir := map[string]int{"]": 1, "[": -1}[msg.String()]
			m.jumpSection(c, dir)
			return m, nil
		}
		return m.updateMainItemKey(msg)
	}
	return m, nil
}

// updateScreenKey handles the keys that work on every screen but Planning
// (which takes b, s, q, ? and S from here): q (quit), esc, ?, S (settings) and
// the screen jumps b, s, p, k and K. The bool reports whether the key was one
// of them.
func (m model) updateScreenKey(msg tea.KeyMsg) (tea.Model, tea.Cmd, bool) {
	switch msg.String() {
	case "q":
		next, cmd := m.askQuit()
		return next, cmd, true
	case "esc":
		next, cmd := m.escOrQuit()
		return next, cmd, true
	case "?":
		m.mode, m.helpOffset = modeHelp, 0
		return m, nil, true
	case "S":
		next, cmd := m.openSettings()
		return next, cmd, true
	case "b":
		m.showBacklog()
		return m, nil, true
	case "s":
		m.showSprints()
		return m, nil, true
	case "p":
		m.openPlanning()
		return m, nil, true
	case "k":
		if id, ok := m.nextOpenSprint(); ok {
			m.showSprint(id)
		} else {
			m.status = msgNoOpenSprints
		}
		return m, nil, true
	case "K":
		next, cmd := m.openJumpPicker()
		return next, cmd, true
	}
	return m, nil, false
}

// updateViewKey handles the keys that change how the trees show the list,
// from the Sprints workspace's sidebar too, where they act on the board: /
// (search), f (state filter), u (unified view) and v (split view, which the
// Sprints workspace does not show). On the Closed group row, which shows no
// board, / and f do nothing. The bool reports whether the key was one of
// them.
func (m model) updateViewKey(msg tea.KeyMsg) (tea.Model, tea.Cmd, bool) {
	if k := msg.String(); m.onClosedRow() && (k == "/" || k == "f") {
		return m, nil, true // the Closed row shows no board to search or filter
	}
	switch msg.String() {
	case "/":
		next, cmd := m.openSearch()
		return next, cmd, true
	case "f":
		if cols := m.view.columns(); len(cols) > 0 {
			m.pick = newFilterPicker("Filter "+m.view.title()+" by state", cols, m.view.filterSet())
			m.mode = modeStateFilter
		}
	case "u":
		m.setUnified(!m.view.all)
		m.saveView()
	case "v":
		switch {
		case m.view.onBoard():
			m.status = msgSplitsBacklog
		case m.split:
			m.toggleSplit()
			m.saveView()
		default:
			m.openSplitOnSelection()
			m.saveView()
		}
	default:
		return m, nil, false
	}
	return m, nil, true
}

// updateSprintList handles the keys of the Sprints workspace's sidebar: the
// arrows move the selection and show its sprint's board, enter and tab focus
// the board, n adds a sprint, e edits the selected one, c closes or reopens it
// and d deletes it. Every other key does nothing there.
func (m model) updateSprintList(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "n":
		return m.openSprintForm(nil)
	case "e":
		if s := m.sprints.selected(m.doc); s != nil {
			return m.openSprintForm(s)
		}
	case "c":
		return m.toggleSprintClosed()
	case "d":
		return m.openDeleteSprint()
	case "up":
		m.moveSprints(-1)
	case "down":
		m.moveSprints(1)
	case "pgup":
		m.moveSprints(-navStep)
	case "pgdown":
		m.moveSprints(navStep)
	case "right", "left":
		m.foldClosed(msg.String() == "right")
	case "enter":
		if m.sprints.onGroup(m.doc) {
			m.sprints.closedOpen = !m.sprints.closedOpen
			m.sprints.reconcile(m.doc, m.sprintListHeight())
			return m, nil
		}
		m.focusBoard()
	case "tab", "shift+tab":
		m.focusBoard()
	}
	return m, nil
}

// foldClosed handles → (unfold) and ← (fold) on the sidebar: on the Closed
// group row they unfold and fold it; ← on a done sprint goes to the group row.
func (m *model) foldClosed(unfold bool) {
	switch s := m.sprints.selected(m.doc); {
	case m.sprints.onGroup(m.doc):
		m.sprints.closedOpen = unfold
	case s != nil && s.Status == todo.SprintDone && !unfold:
		m.sprints.selectGroup(m.doc)
	}
	m.sprints.reconcile(m.doc, m.sprintListHeight())
}

// focusBoard moves the keys from the sidebar to the board of the selected
// sprint. Nothing on the Closed group row or an empty list: no sprint, no board.
func (m *model) focusBoard() {
	if m.sprints.selected(m.doc) == nil {
		return
	}
	m.listFocus = false
	m.saveView()
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
// cancelled), else the tab's state filter. When there is nothing to dismiss it
// focuses the sidebar from the pane beside it — the sprint list from the
// Sprints workspace's board, the categories from a split's task
// list — and asks to quit elsewhere (see askQuit).
func (m model) escOrQuit() (tea.Model, tea.Cmd) {
	if m.view.filter != "" {
		m.clearFilter()
		return m, nil
	}
	if states := m.view.activeStates(); len(states) > 0 {
		m.clearStates()
		return m, nil
	}
	switch {
	case m.view.onBoard() && !m.listFocus:
		m.listFocus = true
		return m, nil
	case m.splitShown() && !m.sideFocus:
		m.sideFocus = true
		return m, nil
	}
	return m.askQuit()
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
		if m.focused().onAll() {
			return m.openForm(targetAddCategory) // the sidebar's All: a top-level category
		}
		return m.openForm(targetEdit)
	case " ":
		// Space folds a category. A task's state is set only through the t dialog.
		if sel := m.focused().selected(); sel != nil && sel.Kind == todo.Category {
			m.focused().toggleFold()
		}
		return m, nil
	case "a":
		return m.shiftColumn(1)
	case "o":
		return m.openSendToPicker()
	case "A":
		return m.shiftColumn(-1)
	case "t":
		return m.openStatePicker()
	case "n":
		return m.openForm(targetAddSibling)
	case "N":
		return m.openForm(targetAddChild)
	case "c":
		return m.openForm(targetAddCategory)
	case "z":
		m.status = m.focused().toggleFoldLevel()
		return m, nil
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

// selectedItem returns the item under the cursor, or nil on an empty tab or
// the placeholder row, where the item keys do nothing.
func (m model) selectedItem() *todo.Item {
	if m.focused().onPlaceholder() {
		return nil
	}
	return m.focused().selected()
}
