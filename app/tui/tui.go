// Package tui is the Bubble Tea frontend of the TODO app: a single-panel
// category/task tree and an add/edit dialog that doubles as the selected item's
// detail view (it shows the item's status and subtask progress alongside its
// editable title and description), plus the keybindings that drive them. All
// domain logic lives in internal/todo; this package only renders it and routes keys.
package tui

import (
	"context"
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
//   - An external reload is applied in modeMain and modeSearch, and deferred
//     in the modal modes and while an item is grabbed (see handleReload).
type model struct {
	// Persistence: the store, the document loaded from it, and the revision
	// and errors of the last load or write.
	store *todo.Store
	doc   *todo.Document
	// saved is an untouched copy of doc as the store last gave it (or, after a
	// write that succeeded but could not be reloaded, as that write left it).
	// The actions change doc in memory before they write, so when the write
	// fails and the reload cannot replace doc either, commit puts a copy of
	// saved back rather than show the unsaved change as if it were saved.
	saved *todo.Document
	// lastRev is the store revision doc reflects: the one todo last loaded,
	// which after its own write is the revision that write produced. The poll
	// loop compares the store against it to tell an external write (reload) from
	// the app's own (ignore).
	lastRev int64
	// unsynced is set when the app's own write succeeded but the reload after
	// it failed: lastRev is the revision that write produced, but doc is the
	// in-memory change, not read back from the store (a new item has no ID
	// yet). The next poll loads the list regardless, as the app's own write.
	unsynced bool
	err      error // last failed save or reload ("save failed: …" / "reload failed: …"), shown in the footer
	// pollErr is the reload failure last put in err, by the poll or by commit
	// after a write that succeeded, kept to show each distinct poll failure once
	// and to clear it when a later reload succeeds.
	pollErr error
	// opener opens the store again at its path, the way the caller opened it,
	// after the file there was replaced (see reopenStore).
	opener func() (*todo.Store, error)

	// Layout: what the trees show and how the screen is split.
	//
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

	// Mode and per-mode data: mode routes the keys; each field below is
	// meaningful only in the mode it names.
	mode mode

	form     form
	target   formTarget
	editing  *todo.Item // the item being edited, for targetEdit
	deleting *todo.Item // the item the confirm dialog is guarding, for modeConfirm
	// confirmOnDelete is which confirm-dialog button has focus: false = Cancel
	// (the safe default so a stray key can't delete), true = Delete.
	confirmOnDelete bool
	// confirmAction is what confirming the open dialog does — delete the selected
	// item, or clear all completed tasks.
	confirmAction confirmAction

	helpOffset int // how far the "?" shortcuts dialog is scrolled, for modeHelp

	pick     picker     // the open state dialog, for modeStatePick and modeStateFilter
	pickItem *todo.Item // the task the "t" dialog sets the state of

	search textinput.Model // the "/" search bar's input, for modeSearch

	// Session data: in memory only, never stored.
	//
	// snapshots holds, per task ID completed in this session, the states of the
	// task and its subtasks captured at completion time, so un-completing it can
	// restore them (the accidental-complete undo). Keyed by ID, it survives the
	// reload after the app's own write; an external reload clears it.
	snapshots map[string]todo.DoneStates
	status    string // transient one-line footer message
	version   string // the app version shown in the header
}

// BusyTimeout is how long a TUI write waits for another process to release the
// store's writer lock before it fails with todo.ErrBusy. Writes run on the UI
// thread, so the wait is kept well below the CLI's: the caller opens the store
// it passes to Run with todo.WithBusyTimeout(BusyTimeout).
const BusyTimeout = 750 * time.Millisecond

// Run loads the list from st and starts the interactive TUI. notice (e.g. the
// automatic TODO.md import summary) is shown in the footer at start; it may be
// empty. The caller owns st, opened with BusyTimeout, and closes it after Run
// returns; a store the TUI reopened after the file was replaced is closed by
// Run.
func Run(st *todo.Store, notice string) error {
	m, err := newModel(st, notice)
	if err != nil {
		return err
	}
	final, err := tea.NewProgram(m, tea.WithAltScreen()).Run()
	if fm, ok := final.(model); ok && fm.store != st {
		_ = fm.store.Close()
	}
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
	path := st.Path()
	return model{
		store:     st,
		opener:    func() (*todo.Store, error) { return todo.OpenStore(path, todo.WithBusyTimeout(BusyTimeout)) },
		version:   metainfo.Version,
		doc:       doc,
		saved:     doc.Clone(),
		view:      v,
		tree:      newTree(doc, v),
		side:      newSidebar(doc, v),
		mode:      modeMain,
		snapshots: map[string]todo.DoneStates{},
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
		switch m.mode {
		case modeForm:
			m.form.setWidth(msg.Width)
		case modeSearch:
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
	case storeReopenedMsg:
		return m.handleReopen(msg)
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
		return m, tea.Quit
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
	default:
		// While an item is grabbed, the arrows drive the move instead of navigation.
		if m.focused().grabbed {
			return m.updateMove(msg)
		}
		return m.updateMain(msg)
	}
}

func (m model) updateMain(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
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

// selectedItem returns the item under the cursor, or nil on an empty tab or
// the placeholder row, where the item keys do nothing.
func (m model) selectedItem() *todo.Item {
	if m.focused().onPlaceholder() {
		return nil
	}
	return m.focused().selected()
}
