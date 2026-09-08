// Package tui is the Bubble Tea frontend of the TODO app: a single-panel
// category/task tree and an add/edit dialog that doubles as the selected item's
// detail view (it shows the item's status and subtask progress alongside its
// editable title and description), plus the keybindings that drive them. All
// domain logic lives in internal/todo; this package only renders it and routes keys.
package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/candy-tools/todo/app/metainfo"
	"github.com/candy-tools/todo/internal/todo"
	"github.com/atotto/clipboard"
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
	path    string
	version string
	doc     *todo.Document
	tree    tree
	width   int
	height  int

	// lastContent is the exact file content todo last read or wrote. The poll
	// loop compares the file against it to tell an external edit (reload) from
	// the app's own save (ignore).
	lastContent string

	mode mode
	form form

	// searching is true while the "/" search bar is focused and capturing keys.
	// The query itself lives in search; the resulting filter lives on the tree
	// (m.tree.filter), so it stays applied for navigation after search closes.
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

	// snapshots holds, per cascade-completed parent task, the descendant Done
	// states captured at completion time, so unchecking that parent can restore
	// them. Session-only: it lives here in memory and is never written to disk.
	snapshots map[*todo.Item]todo.DoneStates

	status string // transient one-line footer message
	err    error  // last save error, shown in the footer
}

// Run loads the TODO file at path and starts the interactive TUI.
func Run(path string) error {
	m, err := newModel(path)
	if err != nil {
		return err
	}
	_, err = tea.NewProgram(m, tea.WithAltScreen()).Run()
	return err
}

// newModel loads the file at path into a fresh model (main view, empty undo
// snapshots). Split out from Run so tests can drive the model without starting
// a real terminal program.
func newModel(path string) (model, error) {
	// Bootstrap the file when it is missing, so launching todo (with no argument,
	// or a new filename) starts from a real file on disk.
	if err := todo.EnsureFile(path); err != nil {
		return model{}, err
	}
	doc, err := todo.Load(path)
	if err != nil {
		return model{}, err
	}
	// Snapshot the raw bytes as the change-detection baseline. Best-effort: a
	// read error (e.g. the file doesn't exist yet) just leaves an empty
	// baseline, which the first save replaces.
	content, _ := os.ReadFile(path) //nolint:gosec // path is the user-provided file the app exists to read
	return model{
		path:        path,
		version:     metainfo.Version,
		doc:         doc,
		tree:        newTree(doc),
		mode:        modeMain,
		snapshots:   map[*todo.Item]todo.DoneStates{},
		lastContent: string(content),
	}, nil
}

// Init starts the file-poll loop that keeps the view in sync with external
// edits to the open file.
func (m model) Init() tea.Cmd { return pollCmd() }

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		// Tell the tree how tall its viewport is now, and re-anchor the scroll so
		// the cursor stays on screen across a resize.
		m.tree.viewHeight = m.treeViewHeight()
		m.tree.reconcileOffset()
		if m.mode == modeForm {
			m.form.setWidth(msg.Width)
		}
		if m.searching {
			m.search.Width = m.searchWidth()
		}
		return m, nil
	case reloadTickMsg:
		// Probe the file and re-arm the next tick; the probe runs off-goroutine.
		return m, tea.Batch(m.reloadCheckCmd(), pollCmd())
	case fileReloadedMsg:
		return m.handleReload(msg)
	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			return m, tea.Quit
		}
		// The search bar (opened with "/" from the main view) captures keys while
		// it is focused, ahead of the main-view bindings.
		if m.searching {
			return m.updateSearch(msg)
		}
		switch m.mode {
		case modeForm:
			return m.updateForm(msg)
		case modeConfirm:
			return m.updateConfirm(msg)
		default:
			return m.updateMain(msg)
		}
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

func (m model) updateMain(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// While an item is grabbed, the arrows drive the move instead of navigation.
	if m.tree.grabbed {
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
	case "up", "w", "k":
		m.tree.moveUp()
	case "down", "s", "j":
		m.tree.moveDown()
	case "pgup":
		m.tree.pageUp()
	case "pgdown":
		m.tree.pageDown()
	case "left", "h":
		m.tree.collapse()
	case "right", "l":
		m.tree.expand()
	default:
		return m.updateMainItemKey(msg)
	}
	return m, nil
}

// escOrQuit clears an active filter (dismissing the search the same way it's
// cancelled), or quits when there is nothing to dismiss.
func (m model) escOrQuit() (tea.Model, tea.Cmd) {
	if m.tree.filter != "" {
		m.clearFilter()
		return m, nil
	}
	return m, tea.Quit
}

// updateMainItemKey handles the item-scoped main-view keys (add/edit/delete/
// toggle/view) for a real selected item. The "+ new category" placeholder row
// has its own reduced key set (see updatePlaceholderKey).
func (m model) updateMainItemKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.tree.onPlaceholder() {
		return m.updatePlaceholderKey(msg)
	}
	switch msg.String() {
	case "enter":
		return m.openForm(targetEdit)
	case " ":
		// On a header (category) space expands/collapses it; on a task it toggles
		// completion. x stays a task-only "toggle done".
		if sel := m.tree.selected(); sel != nil && sel.Kind == todo.Category {
			m.tree.toggleFold()
			return m, nil
		}
		return m.toggleDone()
	case "x":
		return m.toggleDone()
	case "p":
		return m.setStatus(todo.InProgress)
	case ">":
		return m.setStatus(todo.Deferred)
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
// placeholder row, and refuses while a filter is active — the filtered view hides
// siblings and ignores folding, so reordering against it would be misleading.
func (m model) startMove() (tea.Model, tea.Cmd) {
	if m.tree.onPlaceholder() {
		return m, nil
	}
	if m.tree.filter != "" {
		m.status = "Clear the filter (esc) before moving items."
		return m, nil
	}
	m.tree.grabbed = true
	return m, nil
}

// updateMove handles keys while an item is grabbed: the arrows reorder it (↑/↓)
// or re-nest it (←/→), and m/esc/enter drop it. A move the document refuses (a
// clamp at the ends, or an illegal re-nest) leaves the item grabbed so the user
// can pick another direction.
func (m model) updateMove(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	m.status = ""
	switch msg.String() {
	case "up", "w", "k":
		return m.applyMove(m.doc.MoveUp)
	case "down", "s", "j":
		return m.applyMove(m.doc.MoveDown)
	case "left", "h":
		return m.applyMove(m.doc.Outdent)
	case "right", "l":
		return m.applyMove(m.doc.Indent)
	case "m", "esc", "enter":
		m.tree.grabbed = false
	}
	return m, nil
}

// applyMove runs one move op on the grabbed item. On success it persists,
// rebuilds, keeps the cursor on the moved item and expands its (possibly new)
// parent so it stays visible; the structural change also ends the accidental-
// complete undo window. A no-op result changes nothing and keeps the item grabbed.
func (m model) applyMove(op func(*todo.Item) bool) (tea.Model, tea.Cmd) {
	it := m.tree.selected()
	if it == nil || m.tree.onPlaceholder() || !op(it) {
		return m, nil
	}
	m.snapshots = map[*todo.Item]todo.DoneStates{}
	delete(m.tree.collapsed, it.Parent) // reveal the item under its (possibly new) parent
	m.save()
	m.tree.rebuild()
	m.tree.selectItem(it)
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
	ti.SetValue(m.tree.filter)
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
		m.tree.moveUp()
		return m, nil
	case "down":
		m.tree.moveDown()
		return m, nil
	case "pgup":
		m.tree.pageUp()
		return m, nil
	case "pgdown":
		m.tree.pageDown()
		return m, nil
	}
	var cmd tea.Cmd
	m.search, cmd = m.search.Update(msg)
	m.tree.setFilter(m.search.Value())
	return m, cmd
}

// clearFilter closes the search bar and drops the active filter, restoring the
// full tree (and its collapse state). The item under the cursor stays selected,
// so returning from a search leaves you on the item you navigated to rather than
// back at the top.
func (m *model) clearFilter() {
	m.searching = false
	sel := m.tree.selected()
	m.tree.setFilter("")
	if sel != nil {
		m.tree.revealItem(sel)
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
// safe Cancel button). A no-op when nothing is selected.
func (m model) openDelete() (tea.Model, tea.Cmd) {
	sel := m.tree.selected()
	if sel == nil || m.tree.onPlaceholder() {
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

// doClearDone removes every completed task, persists, and keeps the selection on
// the previously selected item when it survived (otherwise the top row). Like a
// delete, it ends the accidental-complete undo window.
func (m model) doClearDone() (tea.Model, tea.Cmd) {
	m.mode = modeMain
	sel := m.tree.selected()
	n := m.doc.RemoveDone()
	if n == 0 {
		return m, nil
	}
	m.snapshots = map[*todo.Item]todo.DoneStates{}
	m.save()
	m.tree.rebuild()
	m.tree.selectTop()
	if sel != nil {
		m.tree.selectItem(sel) // no-op if sel was removed → stays on the top row
	}
	m.status = fmt.Sprintf("Removed %d completed task(s).", n)
	return m, nil
}

// doDelete removes the confirmed item (and its subtree) from the document,
// persists, and moves the selection to a sensible neighbour — the row above the
// deleted item, which is never one of its descendants.
func (m model) doDelete() (tea.Model, tea.Cmd) {
	it := m.deleting
	m.deleting = nil
	m.mode = modeMain
	if it == nil {
		return m, nil
	}
	var prev *todo.Item
	if m.tree.cursor > 0 {
		prev = m.tree.rows[m.tree.cursor-1].item
	}
	m.doc.Remove(it)
	// A structural change ends the accidental-complete undo window.
	m.snapshots = map[*todo.Item]todo.DoneStates{}
	m.save()
	m.tree.rebuild()
	if prev != nil {
		m.tree.selectItem(prev)
	} else {
		m.tree.selectTop()
	}
	return m, nil
}

// toggleDone flips the selected task's completion. Marking a parent done
// snapshots its subtree and cascades completion to all descendants; unmarking a
// parent restores that snapshot if one exists (the accidental-complete undo).
// A manual toggle invalidates any ancestor's snapshot, since reverting it later
// would fight this explicit change.
func (m model) toggleDone() (tea.Model, tea.Cmd) {
	it := m.tree.selected()
	if it == nil || !it.IsTask() {
		m.status = "Only tasks can be completed."
		return m, nil
	}
	for p := it.Parent; p != nil; p = p.Parent {
		delete(m.snapshots, p)
	}
	switch {
	case it.IsDone():
		if snap, ok := m.snapshots[it]; ok {
			todo.RestoreDone(snap)
			delete(m.snapshots, it)
		} else {
			it.Status = todo.Open
		}
	case len(it.Children) > 0:
		m.snapshots[it] = todo.SnapshotDone(it)
		todo.CascadeSetDone(it, true)
	default:
		it.Status = todo.Done
	}
	m.save()
	m.tree.rebuild()
	return m, nil
}

// setStatus flags the selected task with target (in progress / deferred),
// toggling it back to Open when it already has that status. Unlike done, these
// flags never cascade — they describe the task itself, not its subtree — but
// setting one still invalidates any accidental-complete snapshot on the task or
// an ancestor, since a later parent-untoggle would otherwise fight this change.
func (m model) setStatus(target todo.Status) (tea.Model, tea.Cmd) {
	it := m.tree.selected()
	if it == nil || !it.IsTask() {
		m.status = "Only tasks can be flagged."
		return m, nil
	}
	for p := it.Parent; p != nil; p = p.Parent {
		delete(m.snapshots, p)
	}
	delete(m.snapshots, it)
	if it.Status == target {
		it.Status = todo.Open
	} else {
		it.Status = target
	}
	m.save()
	m.tree.rebuild()
	return m, nil
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
	it := m.tree.selected()
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

// openForm prepares and opens the add/edit modal for the given target.
func (m model) openForm(t formTarget) (tea.Model, tea.Cmd) {
	sel := m.tree.selected()
	switch t {
	case targetAddSibling, targetAddChild:
		if sel == nil || m.tree.onPlaceholder() {
			m.status = "Add a category first (press c) — tasks live under a category."
			return m, nil
		}
		m.form = newForm("", "", true, false)
	case targetEdit:
		if sel == nil || m.tree.onPlaceholder() {
			return m, nil
		}
		m.editing = sel
		m.form = newForm(sel.Title, sel.Description, sel.IsTask(), true)
		m.form.meta = itemMeta(sel)
	case targetAddCategory:
		m.form = newForm("", "", false, false)
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
// new task/subtask/category at the right spot. An empty title is ignored (the
// dialog stays open). Any structural or content change clears the undo
// snapshots — the accidental-complete window is meant to be immediate.
func (m model) submitForm() (tea.Model, tea.Cmd) {
	title, desc := m.form.values()
	if title == "" {
		return m, nil
	}
	sel := m.tree.selected()
	var focus *todo.Item

	switch m.target {
	case targetEdit:
		m.editing.Title = title
		if m.editing.IsTask() {
			m.editing.Description = desc
		}
		focus = m.editing
	case targetAddCategory:
		cat := &todo.Item{Kind: todo.Category, Title: title, Level: 1}
		// A category — or a task inside one — selected → subcategory of the
		// enclosing category; the root placeholder → a new top-level category.
		var parentCat *todo.Item
		if !m.tree.onPlaceholder() && sel != nil {
			parentCat = sel.EnclosingCategory()
		}
		if parentCat != nil {
			if cat.Level = parentCat.Level + 1; cat.Level > 6 {
				cat.Level = 6
			}
			parentCat.AppendChild(cat)
			delete(m.tree.collapsed, parentCat)
		} else {
			m.doc.AppendRoot(cat)
		}
		focus = cat
	case targetAddChild: // a subtask of a task, or a task inside a category
		task := todo.NewTask(title, desc, false)
		sel.AppendTask(task)
		delete(m.tree.collapsed, sel)
		focus = task
	default: // targetAddSibling: a task at the end of the selection's level
		task := todo.NewTask(title, desc, false)
		parent := sel
		if sel.IsTask() && sel.Parent != nil {
			parent = sel.Parent // a task's siblings live under its parent
		}
		parent.AppendTask(task)
		delete(m.tree.collapsed, parent)
		focus = task
	}

	m.snapshots = map[*todo.Item]todo.DoneStates{}
	m.mode = modeMain
	m.save()
	m.tree.rebuild()
	if focus != nil {
		m.tree.selectItem(focus)
	}
	return m, nil
}

// save writes the document to disk, recording any error for the footer. On
// success it refreshes lastContent to exactly what was written, so the poll
// loop recognises this as our own change and does not reload over it.
func (m *model) save() {
	if err := m.doc.Save(m.path); err != nil {
		m.err = err
		return
	}
	m.err = nil
	m.lastContent = m.doc.FileContent()
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
	bodyH := h - 2
	if bodyH < 3 {
		bodyH = 3
	}
	box := titledBox("Tasks", m.tree.view(w-2, m.treeViewHeight()), w, bodyH, !dim)
	return m.header(w) + "\n" + box + "\n" + m.footer(w)
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
	bodyH := h - 2
	if bodyH < 3 {
		bodyH = 3
	}
	vh := bodyH - 2
	if vh < 1 {
		vh = 1
	}
	return vh
}

func (m model) header(width int) string {
	done, total := overallCounts(m.doc)
	left := headerAppStyle.Render("todo") + " " + headerHintSty.Render(m.version) + "  " + headerHintSty.Render(filepath.Base(m.path))
	right := headerHintSty.Render(fmt.Sprintf("%d/%d done", done, total))
	gap := width - 1 - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		gap = 1
	}
	return ansi.Truncate(" "+left+strings.Repeat(" ", gap)+right, width, "")
}

func (m model) footer(width int) string {
	switch {
	case m.searching:
		// The live search bar replaces the footer while typing a query.
		return ansi.Truncate(" "+helpKeyStyle.Render("/")+" "+m.search.View(), width, "")
	case m.err != nil:
		return ansi.Truncate(" "+errStyle.Render("save failed: "+m.err.Error()), width, "")
	case m.tree.grabbed:
		// Move mode: the arrows reorder/re-nest the grabbed item.
		parts := []string{
			helpKeyStyle.Render("MOVING"),
			hint("↑↓", "Move"), hint("→", "Child"), hint("←", "Sibling"), hint("m/esc", "Drop"),
		}
		return ansi.Truncate(" "+strings.Join(parts, "  "), width, "")
	case m.tree.filter != "":
		// A filter is applied but the bar is closed: show it, with how to edit/clear.
		info := helpKeyStyle.Render("filter") + helpTextStyle.Render(": "+m.tree.filter+"  ") +
			hint("/", "Edit") + "  " + hint("esc", "Clear")
		return ansi.Truncate(" "+info, width, "")
	case m.status != "":
		return ansi.Truncate(" "+helpTextStyle.Render(m.status), width, "")
	}
	parts := []string{
		hint("↑↓", "Move"), hint("←→", "Fold"), hint("space", "Done"), hint("/", "Search"),
		hint("n/N", "New"), hint("c", "Cat"), hint("m", "Move"), hint("enter/e", "Edit"), hint("y", "Copy"), hint("d", "Del"), hint("D", "Clear"),
		hint("p", "Prog"), hint(">", "Defer"), hint("q", "Quit"),
	}
	return ansi.Truncate(" "+strings.Join(parts, "  "), width, "")
}

// itemMeta renders the read-only header of the edit dialog: a task's completion
// status (○ open / ● done) and subtask progress, or a category label and its task
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
	var status string
	switch it.Status {
	case todo.InProgress:
		status = progStyle.Render("◐ in progress")
	case todo.Deferred:
		status = deferStyle.Render("⏸ deferred")
	case todo.Done:
		status = doneStyle.Render("● done")
	default:
		status = helpTextStyle.Render("○ open")
	}
	if done, total := it.TaskCounts(); total > 0 {
		status += helpTextStyle.Render("  ·  ") + labelStyle.Render("Subtasks") + helpTextStyle.Render(fmt.Sprintf(" %d/%d done", done, total))
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
