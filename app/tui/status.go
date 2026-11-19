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
	case "up":
		m.pick.move(-1)
	case "down":
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
	m.commit(it.ID, stateWrite(it.ID, st, todo.StateOptions{}))
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
	m.commitStatus(id, stateWrite(id, todo.Done, todo.StateOptions{Cascade: true}))
	return m, nil
}

// reopen takes done top-level task it out of Done into state st. With an undo
// snapshot from completing it, its subtasks get their snapshot states back,
// but only when the list is still the one the model shows (RestoreSince):
// after any other write the store makes it a plain reopen. Without a snapshot
// the subtasks keep their states. A write that succeeds uses the snapshot up;
// one that fails (the list busy, say) keeps it, so the user can retry.
func (m model) reopen(it *todo.Item, st todo.Status) (tea.Model, tea.Cmd) {
	id := it.ID
	if m.commitStatus(id, stateWrite(id, st, todo.StateOptions{Restore: m.snapshots[id], RestoreSince: m.lastFP})) {
		delete(m.snapshots, id)
	}
	return m, nil
}

// sprintColumns are the columns of a sprint board, in board order.
var sprintColumns = []todo.Column{todo.ColNext, todo.ColInProgress, todo.ColDone}

// shiftColumn is p/P: it moves the selected top-level task delta columns along
// its sprint's board (no wrap), into the target column's first state. Into
// Done it completes the task (cascade and undo snapshot); out of Done it
// reopens it, restoring its subtasks from the snapshot when there is one. P
// from Next sends the task back to the Backlog (it becomes open). On a backlog
// task p opens the dialog that assigns it to a sprint, and P only hints. A
// subtask (it follows its task) and a task at Done's edge only get a footer
// hint.
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
	case it.SprintID() == "" && delta > 0:
		return m.openAssignPicker(it)
	case it.SprintID() == "":
		m.status = "already in the " + todo.ColBacklog.Title()
		return m, nil
	}
	cur := max(slices.Index(sprintColumns, it.Column()), 0) // a state off the sprint board counts as Next
	i := cur + delta
	switch {
	case i < 0:
		id := it.ID
		m.commitMoved(id, func(ctx context.Context, s *todo.Store) (todo.WriteResult, error) {
			return s.Unassign(ctx, id)
		})
		return m, nil
	case i >= len(sprintColumns):
		m.status = "no column beyond " + sprintColumns[cur].Title()
		return m, nil
	}
	target := sprintColumns[i]
	switch {
	case target == todo.ColDone:
		return m.complete(it)
	case it.IsDone():
		return m.reopen(it, target.FirstState())
	}
	m.commitStatus(it.ID, stateWrite(it.ID, target.FirstState(), todo.StateOptions{}))
	return m, nil
}

// openStatePicker is t: it opens the dialog that sets the selected task's state
// (a top-level task's Backlog or sprint states, by its membership; open or
// done for a subtask), the cursor on its current state. On a task completed in
// this session the cursor starts on the state it had before, so t then enter
// undoes an accidental complete. Categories have no state, so they get a
// footer hint instead.
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
	m.commitStatus(it.ID, stateWrite(it.ID, st, todo.StateOptions{}))
	return m, nil
}

// stateWrite returns the store write that sets task id to st with options o.
// The store decides under its lock, from the task as the file holds it then,
// whether that completes, reopens or just changes it (see todo.Store.SetState).
func stateWrite(id string, st todo.Status, o todo.StateOptions) storeWrite {
	return func(ctx context.Context, s *todo.Store) (todo.WriteResult, error) {
		return s.SetState(ctx, id, st, o)
	}
}
