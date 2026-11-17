package todo

import (
	"context"

	"github.com/candy-tools/todo/libs/jsonfile"
)

// reopenTarget is the state reopening it lands on: Open for a subtask, which
// can only be open or done, and Working for any other task.
func reopenTarget(it *Item) Status {
	if it.IsSubtask() {
		return Open
	}
	return Working
}

// StateOptions say what a SetState does beyond setting the task's own state.
// With neither Cascade nor Restore set the subtasks keep their states.
type StateOptions struct {
	// Cascade sets every task below the task too: to Done when the new state
	// is Done, to Open otherwise. It applies to every change, not only into
	// or out of Done: `todo reopen --cascade` opens the subtasks of a task
	// that is not done.
	Cascade bool
	// Restore is the undo snapshot (see SnapshotDone) of the complete a
	// reopen takes back: the tasks it holds that are still below the task
	// and still Done get their recorded states back. It is only used when the
	// task leaves Done.
	Restore DoneStates
	// RestoreSince, when not zero, is the fingerprint of the list the
	// complete left (the caller's last read). Store.SetState applies Restore
	// only when the list it finds under the lock is still that one; when
	// someone wrote in between, the undo is a plain reopen. It is a content
	// check: a change undone byte for byte in between does not count. Zero
	// means no such check (the per-item rules of Restore still apply).
	RestoreSince Fingerprint
}

// SetState sets task id to st in one write, deciding from the task's state at
// write time, not the caller's copy, whether that completes it, reopens it or
// just changes its state (see Document.SetState), and checking o.RestoreSince
// against the list it read under the lock. It is the one call for every state
// change. A change that changes nothing writes nothing.
func (s *Store) SetState(ctx context.Context, id string, st Status, o StateOptions) (WriteResult, error) {
	return s.update(ctx, func(d *Document, cur jsonfile.Snapshot) (bool, error) {
		if o.Restore != nil && !o.Cascade && !o.RestoreSince.IsMissing() && cur.Fingerprint != o.RestoreSince {
			o.Restore = nil // someone wrote since the complete: no undo
		}
		return d.SetState(id, st, o)
	})
}

// Reopen takes task id out of Done in one write, deciding under the lock from
// the task as it is then, not the caller's copy: a done task goes to
// reopenTarget (Working, or Open for a subtask); with cascade every task below
// it is opened too. A task that is not done keeps its state: with cascade its
// subtasks are opened, without it nothing is written. A missing id fails
// with ErrNotFound and a category with ErrNotTask. It is the write-time form
// of the CLI's reopen; an undo with a snapshot uses SetState.
func (s *Store) Reopen(ctx context.Context, id string, cascade bool) (WriteResult, error) {
	return s.write(ctx, func(d *Document) (bool, error) {
		it, err := d.lookup(id)
		if err != nil {
			return false, err
		}
		if it.Kind != Task {
			return false, ErrNotTask
		}
		st := it.Status
		switch {
		case st == Done:
			st = reopenTarget(it)
		case !cascade:
			return false, nil
		}
		return d.SetState(id, st, StateOptions{Cascade: cascade})
	})
}
