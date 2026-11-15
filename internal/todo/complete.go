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
func ReopenStates(id uint, st Status, restore DoneStates) DoneStates {
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

// Complete sets task id to Done, in one write. With cascade every item below
// it, as the tree is at write time, is set to Done too (see CascadeStatus);
// without it the subtasks keep their states. A category fails it with
// ErrNotTask.
func (s *Store) Complete(ctx context.Context, id uint, cascade bool) error {
	if cascade {
		return s.CascadeStatus(ctx, id, Done)
	}
	return s.SetStatuses(ctx, map[uint]Status{id: Done})
}

// Reopen sets task id to st, in one write, and treats its subtasks as o says:
// o.Cascade opens them all (see CascadeStatus), o.Restore gives them their
// snapshot states back (see ReopenStates), and with neither they keep theirs.
// Setting both fails with ErrRestoreAndCascade before anything is written. A
// category fails it with ErrNotTask, and a subtask given a state other than
// Open or Done with ErrSubtaskStatus.
func (s *Store) Reopen(ctx context.Context, id uint, st Status, o ReopenOptions) error {
	if o.Restore != nil && o.Cascade {
		return ErrRestoreAndCascade
	}
	if o.Cascade {
		return s.CascadeStatus(ctx, id, st)
	}
	return s.SetStatuses(ctx, ReopenStates(id, st, o.Restore))
}
