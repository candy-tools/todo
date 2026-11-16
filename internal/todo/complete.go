package todo

import (
	"context"
	"errors"
	"maps"
)

// ErrRestoreAndCascade is returned by a Reopen asked both to restore the
// subtasks from a snapshot and to open them all: the two disagree.
var ErrRestoreAndCascade = errors.New("a reopen cannot both restore the subtasks and open them all")

// ReopenTarget is the state reopening it lands on: Open for a subtask, which
// can only be open or done, and Working for any other task.
func ReopenTarget(it *Item) Status {
	if it.IsSubtask() {
		return Open
	}
	return Working
}

// ReopenStates is the set of statuses that reopens task id into st and gives
// the tasks in restore (an undo snapshot from SnapshotDone, nil for none) their
// recorded states back. It returns a new map and never changes restore.
func ReopenStates(id string, st Status, restore DoneStates) DoneStates {
	states := make(DoneStates, len(restore)+1)
	maps.Copy(states, restore)
	states[id] = st
	return states
}

// ReopenOptions say what a Reopen does to the subtasks. With neither set they
// keep their states.
type ReopenOptions struct {
	// Restore gives the tasks it holds their recorded states back (see
	// SnapshotDone and ReopenStates).
	Restore DoneStates
	// Cascade opens every task below the reopened one.
	Cascade bool
}

// StateOptions say what a SetState does beyond setting the task's own state.
// With neither set the subtasks keep their states.
type StateOptions struct {
	// Cascade sets every task below the task too: to Done when the new state
	// is Done, to Open otherwise.
	Cascade bool
	// Restore is the undo snapshot (see SnapshotDone) of the complete a
	// reopen takes back: the tasks it holds that are still Done get their
	// recorded states back. It is only used when the task leaves Done.
	Restore DoneStates
}

// SetState sets task id to st in one write, deciding from the task's state at
// write time, not the caller's copy, whether that completes it, reopens it or
// just changes its state (see Document.SetState). It is the one call for every
// state change. A change that changes nothing writes nothing.
func (s *Store) SetState(ctx context.Context, id string, st Status, o StateOptions) (WriteResult, error) {
	return s.write(ctx, func(d *Document) (bool, error) { return d.SetState(id, st, o) })
}

// Complete sets task id to Done, in one write. With cascade every item below
// it, as the list is at write time, is set to Done too (see CascadeStatus);
// without it the subtasks keep their states. A category fails it with
// ErrNotTask.
func (s *Store) Complete(ctx context.Context, id string, cascade bool) (WriteResult, error) {
	return s.change(ctx, func(d *Document) error { return d.Complete(id, cascade) })
}

// Reopen sets task id to st, in one write, and treats its subtasks as o says:
// o.Cascade opens them all (see CascadeStatus), o.Restore gives them their
// snapshot states back (see ReopenStates), and with neither they keep theirs.
// A task in o.Restore deleted since the snapshot, or no longer Done (changed
// since the complete it undoes), is left out (see Document.Reopen); task id
// itself missing fails the write with ErrNotFound. Setting both fails with
// ErrRestoreAndCascade, writing nothing. A category fails it with ErrNotTask,
// and a subtask given a state other than Open or Done with ErrSubtaskStatus.
func (s *Store) Reopen(ctx context.Context, id string, st Status, o ReopenOptions) (WriteResult, error) {
	return s.change(ctx, func(d *Document) error { return d.Reopen(id, st, o) })
}
