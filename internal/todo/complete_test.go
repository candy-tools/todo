package todo

import (
	"context"
	"reflect"
	"testing"
)

func TestReopenTargetIsWorkingOrOpen(t *testing.T) {
	cat := &Item{Kind: Category, Title: "C"}
	task := &Item{Kind: Task, Title: "task", Status: Done}
	sub := &Item{Kind: Task, Title: "sub", Status: Done}
	cat.AppendChild(task)
	task.AppendChild(sub)

	if got := reopenTarget(task); got != Working {
		t.Errorf("reopenTarget(task) = %v, want working", got)
	}
	if got := reopenTarget(sub); got != Open {
		t.Errorf("reopenTarget(subtask) = %v, want open", got)
	}
}

// statuses loads the store and returns the status of each id.
func statuses(t *testing.T, s *Store, ids ...string) []Status {
	t.Helper()
	doc := load(t, s)
	out := make([]Status, 0, len(ids))
	for _, id := range ids {
		out = append(out, item(t, doc, id).Status)
	}
	return out
}

func TestCompleteTargetOnly(t *testing.T) {
	s := newStore(t)
	ids := addTaskTree(t, s, Working)

	mustOK(t)(s.SetState(context.Background(), ids.task, Done, StateOptions{}))

	if got, want := statuses(t, s, ids.task, ids.sub), []Status{Done, Open}; !reflect.DeepEqual(got, want) {
		t.Fatalf("task, sub after completing without cascade = %v, want %v", got, want)
	}
}

func TestCompleteCascade(t *testing.T) {
	s := newStore(t)
	ids := addTaskTree(t, s, Working)

	mustOK(t)(s.SetState(context.Background(), ids.task, Done, StateOptions{Cascade: true}))

	if got, want := statuses(t, s, ids.task, ids.sub), []Status{Done, Done}; !reflect.DeepEqual(got, want) {
		t.Fatalf("task, sub after completing with cascade = %v, want %v", got, want)
	}
}

// doneTaskTree is addTaskTree with a second subtask, sub2, the task and both
// subtasks done.
func doneTaskTree(t *testing.T, s *Store) (ids taskTree, sub2 string) {
	t.Helper()
	ctx := context.Background()
	ids = addTaskTree(t, s, Working)
	sub2 = mustID(t)(s.AddTask(ctx, Fields{Title: "sub2"}, Open, ids.task, ids.sub))
	mustOK(t)(s.SetState(ctx, ids.task, Done, StateOptions{Cascade: true}))
	return ids, sub2
}

func TestReopenKeepsSubtasks(t *testing.T) {
	s := newStore(t)
	ids, sub2 := doneTaskTree(t, s)

	mustOK(t)(s.Reopen(context.Background(), ids.task, false))

	if got, want := statuses(t, s, ids.task, ids.sub, sub2), []Status{Working, Done, Done}; !reflect.DeepEqual(got, want) {
		t.Fatalf("task, sub, sub2 after Reopen = %v, want %v", got, want)
	}
}

// Reopen takes the state it lands on from the task as it is under the lock: a
// task another writer took out of Done since the caller read it keeps that
// writer's state, and nothing is written.
func TestReopenDecidesUnderTheLock(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)
	ids, _ := doneTaskTree(t, s)
	mustOK(t)(NewStore(s.Path()).SetState(ctx, ids.task, Review, StateOptions{})) // another writer

	keepsFile(t, s, "Reopen of a task no longer done", func() (WriteResult, error) { return s.Reopen(ctx, ids.task, false) })
	if got := statuses(t, s, ids.task); got[0] != Review {
		t.Fatalf("task after Reopen = %v, want the other writer's review", got[0])
	}
}

// With cascade, Reopen of a task that is not done keeps its state and opens
// its subtasks.
func TestReopenCascadeOnTaskNotDone(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)
	ids := addTaskTree(t, s, Review)
	mustOK(t)(s.SetState(ctx, ids.sub, Done, StateOptions{}))

	mustOK(t)(s.Reopen(ctx, ids.task, true))

	if got, want := statuses(t, s, ids.task, ids.sub), []Status{Review, Open}; !reflect.DeepEqual(got, want) {
		t.Fatalf("task, sub after Reopen with cascade = %v, want %v", got, want)
	}
}

// TestReopenRestoreSkipsVanishedSubtask: a subtask deleted since the snapshot
// was taken is left out of the restore, so the reopen still lands.
func TestReopenRestoreSkipsVanishedSubtask(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	ids, sub2 := doneTaskTree(t, s)
	restore := DoneStates{ids.task: Review, ids.sub: Open, sub2: Open}
	mustOK(t)(s.Delete(ctx, sub2))

	mustOK(t)(s.SetState(ctx, ids.task, Working, StateOptions{Restore: restore}))

	if got, want := statuses(t, s, ids.task, ids.sub), []Status{Working, Open}; !reflect.DeepEqual(got, want) {
		t.Fatalf("task, sub after the undo with a vanished subtask = %v, want %v", got, want)
	}
	rejects(t, s, ErrNotFound, func() error {
		return errOf(s.SetState(ctx, sub2, Open, StateOptions{Restore: DoneStates{ids.sub: Done}}))
	})
}

func TestReopenRestore(t *testing.T) {
	s := newStore(t)
	ids, sub2 := doneTaskTree(t, s)
	restore := DoneStates{ids.task: Review, ids.sub: Open, sub2: Done}

	mustOK(t)(s.SetState(context.Background(), ids.task, New, StateOptions{Restore: restore}))

	if got, want := statuses(t, s, ids.task, ids.sub, sub2), []Status{New, Open, Done}; !reflect.DeepEqual(got, want) {
		t.Fatalf("task, sub, sub2 after the undo = %v, want %v", got, want)
	}
	if restore[ids.task] != Review {
		t.Fatalf("SetState changed the caller's Restore map: %v", restore)
	}
}

// An undo restores only what the complete left: a subtask another writer
// changed since (here, reopened) keeps that writer's state, while the rest go
// back to their snapshot states.
func TestReopenRestoreSkipsSubtaskChangedSince(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)
	ids := addTaskTree(t, s, Working)
	sub2 := mustID(t)(s.AddTask(ctx, Fields{Title: "sub2"}, Open, ids.task, ids.sub))
	mustOK(t)(s.SetState(ctx, ids.sub, Done, StateOptions{}))
	snapshot := SnapshotDone(item(t, load(t, s), ids.task)) // task Working, sub Done, sub2 Open
	mustOK(t)(s.SetState(ctx, ids.task, Done, StateOptions{Cascade: true}))

	mustOK(t)(NewStore(s.Path()).SetState(ctx, ids.sub, Open, StateOptions{})) // another writer
	mustOK(t)(s.SetState(ctx, ids.task, Working, StateOptions{Restore: snapshot}))

	if got, want := statuses(t, s, ids.task, ids.sub, sub2), []Status{Working, Open, Open}; !reflect.DeepEqual(got, want) {
		t.Fatalf("task, sub, sub2 after the undo = %v, want %v (sub as the other writer left it)", got, want)
	}
}

func TestReopenCascade(t *testing.T) {
	s := newStore(t)
	ids, sub2 := doneTaskTree(t, s)

	mustOK(t)(s.Reopen(context.Background(), ids.task, true))

	if got, want := statuses(t, s, ids.task, ids.sub, sub2), []Status{Working, Open, Open}; !reflect.DeepEqual(got, want) {
		t.Fatalf("task, sub, sub2 after Reopen with Cascade = %v, want %v", got, want)
	}
}

func TestReopenRestoreAndCascadeIsRefused(t *testing.T) {
	s := newStore(t)
	ids, sub2 := doneTaskTree(t, s)
	o := StateOptions{Restore: DoneStates{ids.task: Review, ids.sub: Open}, Cascade: true}

	rejects(t, s, ErrRestoreAndCascade, func() error { return errOf(s.SetState(context.Background(), ids.task, Working, o)) })
	if got, want := statuses(t, s, ids.task, ids.sub, sub2), []Status{Done, Done, Done}; !reflect.DeepEqual(got, want) {
		t.Fatalf("task, sub, sub2 after the refused undo = %v, want %v", got, want)
	}
}

func TestStateChangesOnCategoryAreNotTask(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	ids := addTaskTree(t, s, Open)

	for _, cascade := range []bool{false, true} {
		rejects(t, s, ErrNotTask, func() error { return errOf(s.SetState(ctx, ids.C, Done, StateOptions{Cascade: cascade})) })
		rejects(t, s, ErrNotTask, func() error { return errOf(s.Reopen(ctx, ids.C, cascade)) })
	}
	rejects(t, s, ErrNotTask, func() error {
		return errOf(s.SetState(ctx, ids.C, Working, StateOptions{Restore: DoneStates{ids.task: Open}}))
	})
}

func TestSubtaskReopensToOpenAndRefusesBoardStates(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	ids := addTaskTree(t, s, Working)
	mustOK(t)(s.SetState(ctx, ids.sub, Done, StateOptions{}))

	rejects(t, s, ErrSubtaskStatus, func() error { return errOf(s.SetState(ctx, ids.sub, Working, StateOptions{})) })
	rejects(t, s, ErrSubtaskStatus, func() error {
		return errOf(s.SetState(ctx, ids.sub, Review, StateOptions{Cascade: true}))
	})
	mustOK(t)(s.Reopen(ctx, ids.sub, false)) // a subtask reopens to Open
	if got := statuses(t, s, ids.sub); got[0] != Open {
		t.Fatalf("sub after Reopen = %v, want open", got[0])
	}
}

// SetState decides from the task's state at write time: an undo sent for a
// task another writer already took out of Done is a plain state change, so
// the snapshot is not applied; one sent while it is still done restores.
func TestStoreSetStateDecidesAtWriteTime(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)
	ids := addTaskTree(t, s, Working) // sub open
	snapshot := SnapshotDone(item(t, load(t, s), ids.task))
	res := mustOK(t)(s.SetState(ctx, ids.task, Done, StateOptions{Cascade: true}))
	if got := item(t, res.Doc, ids.sub).Status; !res.Written || got != Done {
		t.Fatalf("complete with cascade: written %v, sub %v; want written, sub done", res.Written, got)
	}

	other := NewStore(s.Path())
	mustOK(t)(other.SetState(ctx, ids.task, Review, StateOptions{}))
	mustOK(t)(s.SetState(ctx, ids.task, Working, StateOptions{Restore: snapshot}))
	if got, want := statuses(t, s, ids.task, ids.sub), []Status{Working, Done}; !reflect.DeepEqual(got, want) {
		t.Fatalf("undo after another writer reopened: task, sub = %v, want %v", got, want)
	}

	mustOK(t)(s.SetState(ctx, ids.task, Done, StateOptions{}))
	mustOK(t)(s.SetState(ctx, ids.task, Working, StateOptions{Restore: snapshot}))
	if got, want := statuses(t, s, ids.task, ids.sub), []Status{Working, Open}; !reflect.DeepEqual(got, want) {
		t.Fatalf("undo of a done task: task, sub = %v, want %v", got, want)
	}

	keepsFile(t, s, "SetState to the current state", func() (WriteResult, error) {
		return s.SetState(ctx, ids.task, Working, StateOptions{})
	})
	rejects(t, s, ErrNotFound, func() error { return errOf(s.SetState(ctx, "zzzzzzz", Done, StateOptions{})) })
}

// completeForUndo builds task (Working) with open subtask sub under C,
// snapshots it and completes it with cascade, as the TUI does. It returns the
// snapshot and the fingerprint the complete left.
func completeForUndo(t *testing.T, s *Store) (ids taskTree, snapshot DoneStates, after Fingerprint) {
	t.Helper()
	ids = addTaskTree(t, s, Working)
	snapshot = SnapshotDone(item(t, load(t, s), ids.task))
	res := mustOK(t)(s.SetState(context.Background(), ids.task, Done, StateOptions{Cascade: true}))
	return ids, snapshot, res.Wrote
}

// With RestoreSince the undo restores only when the list is still the one the
// complete left: here it is, so sub goes back to open.
func TestStoreSetStateRestoreSinceUnchangedRestores(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)
	ids, snapshot, after := completeForUndo(t, s)
	mustOK(t)(s.SetState(ctx, ids.task, Working, StateOptions{Restore: snapshot, RestoreSince: after}))
	if got, want := statuses(t, s, ids.task, ids.sub), []Status{Working, Open}; !reflect.DeepEqual(got, want) {
		t.Fatalf("task, sub = %v, want %v", got, want)
	}
}

// Another writer reopens sub, edits it and completes it again: sub is Done as
// the complete left it, but the list changed since, so with RestoreSince the
// undo is a plain reopen and sub keeps the other writer's Done.
func TestStoreSetStateRestoreSinceSkipsChangedList(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)
	ids, snapshot, after := completeForUndo(t, s)
	other := NewStore(s.Path())
	mustOK(t)(other.SetState(ctx, ids.sub, Open, StateOptions{}))
	mustOK(t)(other.Edit(ctx, ids.sub, Changes{Description: new("checked again")}))
	mustOK(t)(other.SetState(ctx, ids.sub, Done, StateOptions{}))

	mustOK(t)(s.SetState(ctx, ids.task, Working, StateOptions{Restore: snapshot, RestoreSince: after}))
	if got, want := statuses(t, s, ids.task, ids.sub), []Status{Working, Done}; !reflect.DeepEqual(got, want) {
		t.Fatalf("task, sub = %v, want %v (sub as the other writer left it)", got, want)
	}
}

// A done subtask another writer moved under another task since the complete is
// no longer below the task, so the undo leaves it alone.
func TestStoreSetStateRestoresOnlyDescendants(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)
	ids, snapshot, _ := completeForUndo(t, s)
	other := NewStore(s.Path())
	u := mustID(t)(other.AddTask(ctx, Fields{Title: "u"}, Working, ids.C, ids.task))
	mustOK(t)(other.Move(ctx, ids.sub, u, ""))

	mustOK(t)(s.SetState(ctx, ids.task, Working, StateOptions{Restore: snapshot}))
	if got, want := statuses(t, s, ids.task, ids.sub), []Status{Working, Done}; !reflect.DeepEqual(got, want) {
		t.Fatalf("task, moved sub = %v, want %v", got, want)
	}
}
