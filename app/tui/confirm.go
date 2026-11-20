package tui

import (
	"context"
	"fmt"
	"strings"

	"github.com/candy-tools/todo/internal/todo"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// deletionQuestion phrases the delete prompt, noting how many nested items are
// removed along with the selected one.
func deletionQuestion(it *todo.Item) string {
	kind := "task"
	if it.Kind == todo.Category {
		kind = "category"
	}
	if n := len(it.Descendants()); n > 0 {
		return fmt.Sprintf("Delete %s %q and its %d nested item(s)?", kind, it.DisplayTitle(), n)
	}
	return fmt.Sprintf("Delete %s %q?", kind, it.DisplayTitle())
}

// confirmLabels is the wording of a confirm dialog: its title, its two
// buttons, and the key(s) its hint row offers for the confirming button.
type confirmLabels struct{ title, yes, no, yesKeys string }

var (
	// deleteLabels word the destructive dialogs (delete, clear done, delete
	// sprint); they open on the safe Cancel button.
	deleteLabels = confirmLabels{title: "Confirm delete", yes: "Delete", no: "Cancel", yesKeys: "y"}
	// quitLabels word the quit dialog, which opens on Yes so enter quits.
	quitLabels = confirmLabels{title: "Confirm quit", yes: "Yes", no: "No", yesKeys: "y/enter"}
)

// confirmModal renders a confirm dialog worded by l, with the confirming
// button focused when onYes. The destructive dialogs open with focus on Cancel
// (onYes=false) so a stray keystroke can't delete — the dibs convention for
// destructive dialogs.
func confirmModal(question string, l confirmLabels, onYes bool, termWidth int) string {
	if termWidth <= 0 {
		termWidth = 80
	}
	buttons := lipgloss.JoinHorizontal(lipgloss.Top,
		button(l.yes, onYes), "   ", button(l.no, !onYes))
	sep := helpTextStyle.Render(" · ")
	help := hint(l.yesKeys, l.yes) + sep + hint("n/esc", l.no) + sep + hint("tab", "Move")

	contentW := lipgloss.Width(question)
	for _, el := range []string{help, buttons} {
		if w := lipgloss.Width(el); w > contentW {
			contentW = w
		}
	}
	const pad = 2
	width := contentW + 2 + 2*pad
	if max := termWidth - 8; width > max {
		width = max
	}
	if width < 30 {
		width = 30
	}
	contentW = width - 2 - 2*pad

	buttonRow := lipgloss.NewStyle().Width(contentW).Align(lipgloss.Right).Render(buttons)
	body := padBody(question+"\n\n"+buttonRow+"\n\n"+help, pad)
	return titledBox(l.title, body, width, lipgloss.Height(body)+2, true)
}

// padBody indents every non-empty line by pad columns and adds a blank row
// above and below, so the content floats inside the border.
func padBody(body string, pad int) string {
	margin := strings.Repeat(" ", pad)
	lines := strings.Split(body, "\n")
	for i, l := range lines {
		if l != "" {
			lines[i] = margin + l
		}
	}
	return "\n" + strings.Join(lines, "\n") + "\n"
}

// confirmAction records what the open confirm dialog should do when confirmed.
type confirmAction int

const (
	confirmDelete       confirmAction = iota // delete the selected item (m.deleting)
	confirmClearDone                         // remove the board's sprint's completed tasks
	confirmDeleteSprint                      // delete the selected sprint (m.deletingSprint)
	confirmQuit                              // quit the app
)

// askQuit opens the quit dialog, focused on Yes so enter quits and esc stays.
// q, and esc with nothing left to clear, come here; ctrl+c quits at once.
func (m model) askQuit() (tea.Model, tea.Cmd) {
	m.confirmOnYes = true
	m.confirmAction = confirmQuit
	m.mode = modeConfirm
	return m, nil
}

// confirmLabels is the wording of the open confirm dialog.
func (m model) confirmLabels() confirmLabels {
	if m.confirmAction == confirmQuit {
		return quitLabels
	}
	return deleteLabels
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
	m.confirmOnYes = false
	m.confirmAction = confirmDelete
	m.mode = modeConfirm
	return m, nil
}

// openClearDone is D: on a sprint board it opens the confirm dialog for
// removing that sprint's completed tasks (focused on the safe Cancel button),
// or says there are none. The Backlog holds no done tasks, and the unified view
// would reach several sprints' history at once, so both only get a footer
// hint.
func (m model) openClearDone() (tea.Model, tea.Cmd) {
	switch {
	case m.view.all:
		m.status = "open a sprint to remove its done tasks"
		return m, nil
	case !m.view.onBoard():
		m.status = "no done tasks in the " + todo.ColBacklog.Title()
		return m, nil
	case m.doc.RemovableDoneIn(m.view.sprint) == 0:
		m.status = "No completed tasks to remove."
		return m, nil
	}
	m.deleting = nil
	m.confirmOnYes = false
	m.confirmAction = confirmClearDone
	m.mode = modeConfirm
	return m, nil
}

// updateConfirm handles the confirm dialog: y, or enter on the confirming
// button, confirms; n/esc cancels; tab/arrows toggle the focused button.
func (m model) updateConfirm(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "y", "Y":
		return m.doConfirm()
	case "n", "N", "esc":
		m.mode = modeMain
		return m, nil
	case "left", "right", "tab", "shift+tab":
		m.confirmOnYes = !m.confirmOnYes
		return m, nil
	case "enter", " ":
		if m.confirmOnYes {
			return m.doConfirm()
		}
		m.mode = modeMain
		return m, nil
	}
	return m, nil
}

// doConfirm runs the confirmed action for the open dialog.
func (m model) doConfirm() (tea.Model, tea.Cmd) {
	switch m.confirmAction {
	case confirmClearDone:
		return m.doClearDone()
	case confirmDeleteSprint:
		return m.doDeleteSprint()
	case confirmQuit:
		m.mode = modeMain
		return m.quit()
	}
	return m.doDelete()
}

// doClearDone removes the completed tasks of the board's sprint from the store
// (which works out the fully-done subtrees itself, as they are at write time),
// reloads, and keeps the selection on the previously selected item when it
// survived (otherwise the top row). Like a delete, it ends the
// accidental-complete undo window. The footer says how many tasks went, and
// keeps the notice that the list changed externally when the write found it
// so.
func (m model) doClearDone() (tea.Model, tea.Cmd) {
	m.mode = modeMain
	var focus string
	if sel := m.focused().selected(); sel != nil && !m.focused().onPlaceholder() {
		focus = sel.ID
	}
	m.snapshots = map[string]todo.DoneStates{}
	m.status = "" // so that only the write's own notice is kept below
	var removed int
	sprint := m.view.sprint
	saved := m.commit(focus, func(ctx context.Context, st *todo.Store) (res todo.WriteResult, err error) {
		removed, res, err = st.PruneSprint(ctx, sprint)
		return res, err
	})
	if sel := m.focused().selected(); focus != "" && (sel == nil || sel.ID != focus) {
		m.focused().selectTop() // sel was removed
	}
	if saved {
		// The write's external-change notice, when it found one, stays.
		m.status = strings.TrimSpace(fmt.Sprintf("Removed %d completed task(s). %s", removed, m.status))
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
	var prevID string
	if t := m.focused(); t.cursor > 0 && t.pane != paneCategories {
		if r := t.rows[t.cursor-1]; r.kind == rowItem {
			prevID = r.item.ID
		}
	}
	// A structural change ends the accidental-complete undo window.
	m.snapshots = map[string]todo.DoneStates{}
	id := it.ID
	m.commit(prevID, func(ctx context.Context, st *todo.Store) (todo.WriteResult, error) {
		return st.Delete(ctx, id)
	})
	return m, nil
}

// confirmQuestion phrases the prompt for the open confirm dialog.
func (m model) confirmQuestion() string {
	switch m.confirmAction {
	case confirmClearDone:
		return fmt.Sprintf("Remove %d completed task(s)?", m.doc.RemovableDoneIn(m.view.sprint))
	case confirmDeleteSprint:
		return fmt.Sprintf("Delete sprint %q? Its unfinished tasks go back to the Backlog.", todo.Printable(m.deletingSprint.Name))
	case confirmQuit:
		return "Quit?"
	}
	return deletionQuestion(m.deleting)
}
