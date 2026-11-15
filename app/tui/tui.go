// Package tui is the Bubble Tea frontend of the TODO app: a single-panel
// category/task tree and an add/edit dialog that doubles as the selected item's
// detail view (it shows the item's status and subtask progress alongside its
// editable title and description), plus the keybindings that drive them. All
// domain logic lives in internal/todo; this package only renders it and routes keys.
package tui

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

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
	// saved is an untouched copy of doc as the store last gave it (or, after a
	// write that succeeded but could not be reloaded, as that write left it).
	// The actions change doc in memory before they write, so when the write
	// fails and the reload cannot replace doc either, commit puts a copy of
	// saved back rather than show the unsaved change as if it were saved.
	saved *todo.Document
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

	// pollErr is the poll failure last put in err, kept to show each distinct
	// failure once and to clear it when a later poll succeeds.
	pollErr error
}

// BusyTimeout is how long a TUI write waits for another process to release the
// store's writer lock before it fails with todo.ErrBusy. Writes run on the UI
// thread, so the wait is kept well below the CLI's: the caller opens the store
// it passes to Run with todo.WithBusyTimeout(BusyTimeout).
const BusyTimeout = 750 * time.Millisecond

// Run loads the list from st and starts the interactive TUI. notice (e.g. the
// automatic TODO.md import summary) is shown in the footer at start; it may be
// empty. The caller owns st, opened with BusyTimeout, and closes it after Run
// returns.
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
		saved:     doc.Clone(),
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
	case reloadFailedMsg:
		return m.handleReloadFailed(msg)
	case reloadRecoveredMsg:
		m.clearPollErr()
		return m, nil
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
	// In the sidebar no neighbour is picked: the deleted category's place
	// falls back to All (see adoptAll).
	var prevID uint
	if t := m.focused(); t.cursor > 0 && t.pane != paneCategories {
		if r := t.rows[t.cursor-1]; r.kind == rowItem {
			prevID = r.item.ID
		}
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
