package todo

import (
	"context"
	"errors"
)

// ErrNotFound is returned by a write that names an item (or a parent) the store
// does not hold.
var ErrNotFound = errors.New("item not found")

// ErrTooDeep is returned by a write that would nest a task below a subtask:
// subtasks cannot have subtasks.
var ErrTooDeep = errors.New("subtasks cannot have subtasks")

// ErrSubtaskStatus is returned by a write that would give a subtask a state
// other than Open or Done.
var ErrSubtaskStatus = errors.New("a subtask can only be open or done")

// ErrWrongParent is returned by a write that would place a task at the root or
// a category under a task: a task always sits under a category or a task, and a
// category under a category or at the root.
var ErrWrongParent = errors.New("a task must sit under a category and a category cannot sit under a task")

// ErrNotTask is returned by a write that would set the status of a category:
// only a task has a status.
var ErrNotTask = errors.New("only a task has a status")

// ErrConflict is returned by a write whose placement no longer fits the tree
// it finds: the sibling it names to place an item after is no longer under that
// parent. It usually means another process changed the list since the caller
// read it.
var ErrConflict = errors.New("the list changed since it was read; reload and try again")

// ErrInvalidMove is returned by a move that would put an item under itself or
// one of its descendants. It is usually a bad request, but can also follow a
// stale read (another process made the target a descendant of the item since
// the caller read the list); either way, retrying the same move fails again.
var ErrInvalidMove = errors.New("cannot move an item into its own subtree")

// Fields are the editable text fields of an item. A category uses Title only.
type Fields struct {
	Title       string
	Description string
	Plan        string // a path or reference to the task's plan
}

// Changes are the text fields an Edit sets. A nil field keeps what the store
// holds, so an edit never reverts a field another process changed since the
// caller read the item. A category takes Title only.
type Changes struct {
	Title       *string
	Description *string
	Plan        *string
}

// AddCategory adds a category under parentID ("" for a root), after the
// sibling afterID ("" places it first among the categories; a task places it
// first among the subcategories, after every task), and returns its ID. Under a
// task it fails with ErrWrongParent; a title CleanTitle refuses fails it with
// ErrInvalidTitle. It is the explicit-placement form of AppendCategory, for a
// caller that must put the category at a given position (tests use it to
// build an exact order); an afterID that is no longer a child of parentID
// fails it with ErrConflict. A caller that only wants the category last uses
// AppendCategory, which cannot conflict.
func (s *Store) AddCategory(ctx context.Context, title string, parentID, afterID string) (string, error) {
	return s.add(ctx, func(d *Document) (string, error) { return d.AddCategory(title, parentID, afterID) })
}

// AppendCategory adds a category like AddCategory, placed after the last child
// of parentID ("" for the root) — after its tasks and subcategories — as the
// list is at write time: a sibling another process moved or deleted since the
// caller read the list cannot make the add fail.
func (s *Store) AppendCategory(ctx context.Context, title string, parentID string) (string, error) {
	return s.add(ctx, func(d *Document) (string, error) { return d.AppendCategory(title, parentID) })
}

// AddTask adds a task with fields f and status st under parentID, after the
// sibling afterID ("" places it first), and returns its ID. At the root it
// fails with ErrWrongParent; under a subtask with ErrTooDeep; under a task
// (making a subtask) with a state other than Open or Done, with
// ErrSubtaskStatus. A title CleanTitle refuses fails it with ErrInvalidTitle.
// It is the explicit-placement form of AppendTask, for a caller that must put
// the task at a given position (tests use it to build an exact order); an
// afterID that is no longer a child of parentID fails it with ErrConflict. A
// caller that only wants the task after its last task sibling uses
// AppendTask, which cannot conflict.
func (s *Store) AddTask(ctx context.Context, f Fields, st Status, parentID, afterID string) (string, error) {
	return s.add(ctx, func(d *Document) (string, error) { return d.AddTask(f, st, parentID, afterID) })
}

// AppendTask adds a task like AddTask, placed after the last task child of
// parentID and before any subcategory, as the list is at write time: a
// sibling another process moved or deleted since the caller read the list
// cannot make the add fail.
func (s *Store) AppendTask(ctx context.Context, f Fields, st Status, parentID string) (string, error) {
	return s.add(ctx, func(d *Document) (string, error) { return d.AppendTask(f, st, parentID) })
}

// add runs the Document add op as one write and returns the new item's ID.
func (s *Store) add(ctx context.Context, op func(d *Document) (string, error)) (string, error) {
	var id string
	err := s.write(ctx, func(d *Document) (bool, error) {
		var err error
		id, err = op(d)
		return err == nil, err
	})
	if err != nil {
		return "", err
	}
	return id, nil
}

// change runs op, a Document op that always changes the list when it
// succeeds, as one write.
func (s *Store) change(ctx context.Context, op func(d *Document) error) error {
	return s.write(ctx, func(d *Document) (bool, error) {
		if err := op(d); err != nil {
			return false, err
		}
		return true, nil
	})
}

// Edit sets the fields of item id that c names: its title and, for a task, its
// description and plan; a category ignores c.Description and c.Plan. Fields c
// leaves nil, and kind, status and placement, are kept, so an edit never
// reverts a field another process changed since the caller read the item. A
// title CleanTitle refuses fails the edit with ErrInvalidTitle, and a missing
// id with ErrNotFound. An edit that changes nothing (c names no field, only
// fields the item ignores, or only values it already holds) writes nothing.
func (s *Store) Edit(ctx context.Context, id string, c Changes) error {
	return s.write(ctx, func(d *Document) (bool, error) { return d.Edit(id, c) })
}

// SetStatuses sets the status of every item in states. Naming an ID the list
// does not hold, such as a task another process deleted since the caller's
// read, fails the whole write with ErrNotFound; giving a subtask a state other
// than Open or Done fails it with ErrSubtaskStatus, and naming a category with
// ErrNotTask.
func (s *Store) SetStatuses(ctx context.Context, states map[string]Status) error {
	return s.change(ctx, func(d *Document) error { return d.SetStatuses(states) })
}

// CascadeStatus sets item id to st and every item below it, as the list is at
// write time, to Done when st is Done and to Open otherwise: a board state
// never reaches a subtask. When id is itself a subtask, st must be Open or
// Done, or the write fails with ErrSubtaskStatus. A category fails it with
// ErrNotTask.
func (s *Store) CascadeStatus(ctx context.Context, id string, st Status) error {
	return s.change(ctx, func(d *Document) error { return d.CascadeStatus(id, st) })
}

// Move places item id, with its subtree, under parentID ("" for a root), after
// the sibling afterID ("" places it first), resolving both in the list as it
// is at write time. A missing item or parent is ErrNotFound, and an afterID no
// longer under parentID ErrConflict.
//
// A task moved to the root, or a category moved under a task, fails with
// ErrWrongParent. A task moved under a task must have no children, and the
// new parent must not be a subtask, or the move fails with ErrTooDeep; a move
// into the item's own subtree fails with ErrInvalidMove. A task that becomes,
// stays or stops being a subtask takes its status from StatusAfterMove; a task
// moved between categories keeps its status.
func (s *Store) Move(ctx context.Context, id, parentID, afterID string) error {
	return s.change(ctx, func(d *Document) error { return d.Move(id, parentID, afterID) })
}

// Delete removes item id together with its subtree as it is at write time. A
// missing id is ErrNotFound.
func (s *Store) Delete(ctx context.Context, id string) error {
	return s.change(ctx, func(d *Document) error { return d.Delete(id) })
}

// Prune removes every fully-done task subtree (see Document.DoneSubtrees) and
// returns how many tasks went. It finds them in the list as it is at write
// time, not in the caller's copy, so a subtask another process just added
// under a done task keeps that task. When nothing is done, nothing is written.
func (s *Store) Prune(ctx context.Context) (int, error) {
	removed := 0
	err := s.write(ctx, func(d *Document) (bool, error) {
		removed = d.Prune()
		return removed > 0, nil
	})
	if err != nil {
		return 0, err
	}
	return removed, nil
}
