package todo

import (
	"context"
	"reflect"
	"testing"
)

func TestReopenTarget(t *testing.T) {
	cat := &Item{Kind: Category, Title: "C"}
	task := NewTask("task", "", true)
	sub := NewTask("sub", "", true)
	cat.AppendChild(task)
	task.AppendChild(sub)

	if got := ReopenTarget(task); got != Working {
		t.Errorf("ReopenTarget(task) = %v, want working", got)
	}
	if got := ReopenTarget(sub); got != Open {
		t.Errorf("ReopenTarget(subtask) = %v, want open", got)
	}
}

func TestReopenStatesCopiesRestore(t *testing.T) {
	restore := DoneStates{"1": Review, "2": Open, "3": Done}
	got := ReopenStates("1", New, restore)

	if want := (DoneStates{"1": New, "2": Open, "3": Done}); !reflect.DeepEqual(got, want) {
		t.Fatalf("ReopenStates = %v, want %v", got, want)
	}
	if want := (DoneStates{"1": Review, "2": Open, "3": Done}); !reflect.DeepEqual(restore, want) {
		t.Fatalf("restore after ReopenStates = %v, want it untouched: %v", restore, want)
	}
	got["2"] = Done
	if restore["2"] != Open {
		t.Fatalf("the result shares its map with restore")
	}
}

func TestReopenStatesNilRestore(t *testing.T) {
	if got, want := ReopenStates("7", Working, nil), (DoneStates{"7": Working}); !reflect.DeepEqual(got, want) {
		t.Fatalf("ReopenStates(nil) = %v, want %v", got, want)
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

	mustOK(t)(s.Complete(context.Background(), ids.task, false))

	if got, want := statuses(t, s, ids.task, ids.sub), []Status{Done, Open}; !reflect.DeepEqual(got, want) {
		t.Fatalf("task, sub after Complete without cascade = %v, want %v", got, want)
	}
}

func TestCompleteCascade(t *testing.T) {
	s := newStore(t)
	ids := addTaskTree(t, s, Working)

	mustOK(t)(s.Complete(context.Background(), ids.task, true))

	if got, want := statuses(t, s, ids.task, ids.sub), []Status{Done, Done}; !reflect.DeepEqual(got, want) {
		t.Fatalf("task, sub after Complete with cascade = %v, want %v", got, want)
	}
}

// doneTaskTree is addTaskTree with a second subtask, sub2, the task and both
// subtasks done.
func doneTaskTree(t *testing.T, s *Store) (ids taskTree, sub2 string) {
	t.Helper()
	ctx := context.Background()
	ids = addTaskTree(t, s, Working)
	sub2 = mustID(t)(s.AddTask(ctx, Fields{Title: "sub2"}, Open, ids.task, ids.sub))
	mustOK(t)(s.Complete(ctx, ids.task, true))
	return ids, sub2
}

func TestReopenKeepsSubtasks(t *testing.T) {
	s := newStore(t)
	ids, sub2 := doneTaskTree(t, s)

	mustOK(t)(s.Reopen(context.Background(), ids.task, Working, ReopenOptions{}))

	if got, want := statuses(t, s, ids.task, ids.sub, sub2), []Status{Working, Done, Done}; !reflect.DeepEqual(got, want) {
		t.Fatalf("task, sub, sub2 after Reopen = %v, want %v", got, want)
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

	mustOK(t)(s.Reopen(ctx, ids.task, Working, ReopenOptions{Restore: restore}))

	if got, want := statuses(t, s, ids.task, ids.sub), []Status{Working, Open}; !reflect.DeepEqual(got, want) {
		t.Fatalf("task, sub after Reopen with a vanished subtask = %v, want %v", got, want)
	}
	rejects(t, s, ErrNotFound, func() error {
		return errOf(s.Reopen(ctx, sub2, Open, ReopenOptions{Restore: DoneStates{ids.sub: Done}}))
	})
}

func TestReopenRestore(t *testing.T) {
	s := newStore(t)
	ids, sub2 := doneTaskTree(t, s)
	restore := DoneStates{ids.task: Review, ids.sub: Open, sub2: Done}

	mustOK(t)(s.Reopen(context.Background(), ids.task, New, ReopenOptions{Restore: restore}))

	if got, want := statuses(t, s, ids.task, ids.sub, sub2), []Status{New, Open, Done}; !reflect.DeepEqual(got, want) {
		t.Fatalf("task, sub, sub2 after Reopen with Restore = %v, want %v", got, want)
	}
	if restore[ids.task] != Review {
		t.Fatalf("Reopen changed the caller's Restore map: %v", restore)
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
	mustOK(t)(s.SetStatuses(ctx, DoneStates{ids.sub: Done}))
	snapshot := SnapshotDone(item(t, load(t, s), ids.task)) // task Working, sub Done, sub2 Open
	mustOK(t)(s.Complete(ctx, ids.task, true))

	mustOK(t)(NewStore(s.Path()).SetStatuses(ctx, DoneStates{ids.sub: Open})) // another writer
	mustOK(t)(s.Reopen(ctx, ids.task, Working, ReopenOptions{Restore: snapshot}))

	if got, want := statuses(t, s, ids.task, ids.sub, sub2), []Status{Working, Open, Open}; !reflect.DeepEqual(got, want) {
		t.Fatalf("task, sub, sub2 after the undo = %v, want %v (sub as the other writer left it)", got, want)
	}
}

func TestReopenCascade(t *testing.T) {
	s := newStore(t)
	ids, sub2 := doneTaskTree(t, s)

	mustOK(t)(s.Reopen(context.Background(), ids.task, Working, ReopenOptions{Cascade: true}))

	if got, want := statuses(t, s, ids.task, ids.sub, sub2), []Status{Working, Open, Open}; !reflect.DeepEqual(got, want) {
		t.Fatalf("task, sub, sub2 after Reopen with Cascade = %v, want %v", got, want)
	}
}

func TestReopenRestoreAndCascadeIsRefused(t *testing.T) {
	s := newStore(t)
	ids, sub2 := doneTaskTree(t, s)
	o := ReopenOptions{Restore: DoneStates{ids.task: Review, ids.sub: Open}, Cascade: true}

	rejects(t, s, ErrRestoreAndCascade, func() error { return errOf(s.Reopen(context.Background(), ids.task, Working, o)) })
	if got, want := statuses(t, s, ids.task, ids.sub, sub2), []Status{Done, Done, Done}; !reflect.DeepEqual(got, want) {
		t.Fatalf("task, sub, sub2 after the refused Reopen = %v, want %v", got, want)
	}
}

func TestCompleteAndReopenOnCategoryIsNotTask(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	ids := addTaskTree(t, s, Open)

	for _, cascade := range []bool{false, true} {
		rejects(t, s, ErrNotTask, func() error { return errOf(s.Complete(ctx, ids.C, cascade)) })
		rejects(t, s, ErrNotTask, func() error {
			return errOf(s.Reopen(ctx, ids.C, Working, ReopenOptions{Cascade: cascade}))
		})
	}
	rejects(t, s, ErrNotTask, func() error {
		return errOf(s.Reopen(ctx, ids.C, Working, ReopenOptions{Restore: DoneStates{ids.task: Open}}))
	})
}

func TestReopenSubtaskToBoardStateIsRefused(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	ids := addTaskTree(t, s, Working)
	mustOK(t)(s.Complete(ctx, ids.sub, false))

	rejects(t, s, ErrSubtaskStatus, func() error { return errOf(s.Reopen(ctx, ids.sub, Working, ReopenOptions{})) })
	rejects(t, s, ErrSubtaskStatus, func() error {
		return errOf(s.Reopen(ctx, ids.sub, Review, ReopenOptions{Cascade: true}))
	})
	mustOK(t)(s.Reopen(ctx, ids.sub, Open, ReopenOptions{}))
	if got := statuses(t, s, ids.sub); got[0] != Open {
		t.Fatalf("sub after Reopen to open = %v, want open", got[0])
	}
}
