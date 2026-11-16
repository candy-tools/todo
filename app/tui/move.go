package tui

import (
	"context"
	"errors"

	"github.com/candy-tools/todo/internal/todo"
	tea "github.com/charmbracelet/bubbletea"
)

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
// or re-nest it (←/→), and m/esc/enter drop it. A move with nowhere valid to
// go (a clamp at the ends, or an illegal re-nest) leaves the item grabbed so
// the user can pick another direction.
func (m model) updateMove(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	m.status = ""
	switch msg.String() {
	case "up", "w":
		return m.applyMove(todo.StepUp)
	case "down", "s":
		return m.applyMove(todo.StepDown)
	case "left":
		return m.applyMove(todo.StepOut)
	case "right":
		return m.applyIndent()
	case "m", "esc", "enter":
		m.focused().grabbed = false
	}
	return m, nil
}

// applyIndent indents the grabbed item under the item just above it among its
// siblings. When the list refuses it as too deep (a subtask, or a task with
// subtasks: tasks nest one level only), or the new parent is hidden in the tab
// (the item would vanish into it), nothing moves and the footer says why. Any
// other refusal (nothing above to nest under) stays silent.
func (m model) applyIndent() (tea.Model, tea.Cmd) {
	it := m.selectedItem()
	if it == nil {
		return m, nil
	}
	parentID, afterID, err := m.doc.StepPlacement(it.ID, todo.StepIn, nil)
	switch {
	case errors.Is(err, todo.ErrTooDeep):
		m.status = "subtasks can't have subtasks"
		return m, nil
	case err != nil:
		return m, nil
	}
	if p, ok := m.doc.ByID(parentID); ok && !m.focused().shown[p] {
		m.status = "can't nest under a task hidden in this tab"
		return m, nil
	}
	return m.moveTo(it.ID, parentID, afterID)
}

// applyMove makes one step s of the grabbed item. The list works out where
// the step goes (todo.Document.StepPlacement) without changing anything; an
// up or down step skips the items the tab hides (Backlog hides the other
// columns' tasks, other tabs hide Backlog's, a search hides non-matches), so
// the item passes one shown row per key press. A step with nowhere to go
// changes nothing and keeps the item grabbed. Otherwise the store moves the
// item there, and the view shows the list the write left: the cursor stays on
// the moved item (still grabbed) and its new parent is expanded so it stays
// visible; the structural change also ends the accidental-complete undo
// window. If the item is no longer shown in the tab afterwards (a re-nest
// changed its state, or a write landed from elsewhere), the grab ends, the
// cursor keeps its row index (clamped) and the footer says where it went.
func (m model) applyMove(s todo.MoveStep) (tea.Model, tea.Cmd) {
	it := m.selectedItem()
	if it == nil {
		return m, nil
	}
	parentID, afterID, err := m.doc.StepPlacement(it.ID, s, m.focused().shownID())
	if err != nil {
		return m, nil
	}
	return m.moveTo(it.ID, parentID, afterID)
}

// moveTo moves the grabbed item id under parentID after afterID, as
// applyMove describes.
func (m model) moveTo(id, parentID, afterID string) (tea.Model, tea.Cmd) {
	m.snapshots = map[string]todo.DoneStates{}
	delete(m.focused().collapsed, parentID) // reveal the item under its (possibly new) parent
	m.commit(id, func(ctx context.Context, st *todo.Store) (todo.WriteResult, error) {
		return st.Move(ctx, id, parentID, afterID)
	})
	if m.noteLeftTab(id) {
		m.focused().grabbed = false
	}
	return m, nil
}
