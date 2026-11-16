package tui

import (
	"context"
	"slices"

	"github.com/candy-tools/todo/internal/todo"
	tea "github.com/charmbracelet/bubbletea"
)

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
// exactly (see openStatePicker and reopen). The store write always cascades,
// so it also reaches subtasks another process added since the last poll.
func (m model) complete(it *todo.Item) (tea.Model, tea.Cmd) {
	id := it.ID
	m.snapshots[id] = todo.SnapshotDone(it)
	todo.CascadeSetDone(it, true)
	m.commitStatus(id, func(ctx context.Context) (todo.WriteResult, error) { return m.store.Complete(ctx, id, true) })
	return m, nil
}

// reopen takes done top-level task it out of Done into state st. With an undo
// snapshot from completing it, its subtasks get their snapshot states back;
// without one they keep theirs. The snapshot is used up either way.
func (m model) reopen(it *todo.Item, st todo.Status) (tea.Model, tea.Cmd) {
	id := it.ID
	restore := m.snapshots[id]
	delete(m.snapshots, id)
	m.doc.ApplyStatuses(todo.ReopenStates(id, st, restore))
	m.commitStatus(id, func(ctx context.Context) (todo.WriteResult, error) {
		return m.store.Reopen(ctx, id, st, todo.ReopenOptions{Restore: restore})
	})
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
func (m model) setStatusesWrite(id string, st todo.Status) storeWrite {
	return func(ctx context.Context) (todo.WriteResult, error) {
		return m.store.SetStatuses(ctx, map[string]todo.Status{id: st})
	}
}
