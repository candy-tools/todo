package tui

import (
	"context"
	"slices"

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
	m.snapshots = map[string]todo.DoneStates{}
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
