package todo

import (
	"context"
	"errors"

	closuretree "github.com/go-bumbu/closure-tree"
	"gorm.io/gorm"
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

// write runs fn as one store write: a single transaction holding fn's changes
// through tx and a closure-tree bound to tx (so the tree's own transactions are
// savepoints of this one), the revision bump and, when the sort order of some
// sibling group runs out of room, its renormalization. Nothing in fn may use
// s.db: the SQLite writer is one connection, already held by tx. A store a
// newer todo has upgraded is refused with ErrNewerSchema before fn runs.
func (s *Store) write(ctx context.Context, fn func(tx *gorm.DB, tree *closuretree.Tree) error) error {
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return writeIn(ctx, tx, fn)
	})
	if errors.Is(err, closuretree.ErrNodeNotFound) || errors.Is(err, closuretree.ErrParentNotFound) {
		return ErrNotFound
	}
	return err
}

// writeIn is the body of a write, run in the transaction tx the caller holds:
// the newer-schema check, fn with a closure-tree bound to tx, the revision bump
// and any renormalization. The caller commits or rolls back.
func writeIn(ctx context.Context, tx *gorm.DB, fn func(tx *gorm.DB, tree *closuretree.Tree) error) error {
	if _, err := revisionIn(ctx, tx); err != nil {
		return err
	}
	tree, err := closuretree.New(tx, dbItem{})
	if err != nil {
		return err
	}
	if err := fn(tx, tree); err != nil {
		return err
	}
	if err := tx.Model(&dbMeta{}).Where("id = ?", 1).
		Update("revision", gorm.Expr("revision + 1")).Error; err != nil {
		return err
	}
	need, err := tree.NeedsRenormalizeAny(ctx, storeScope, closuretree.DefaultHalvingsBuffer)
	if err != nil {
		return err
	}
	if need {
		if _, err := tree.RenormalizeAll(ctx, storeScope, closuretree.DefaultHalvingsBuffer); err != nil {
			return err
		}
	}
	return nil
}

// kindAndParent reads row id as it is in the write's transaction (tree is bound
// to tx): its kind and status, and its parent's ID from the closure table in
// ParentId (0 for a root item). A missing row is closuretree.ErrNodeNotFound.
func kindAndParent(ctx context.Context, tree *closuretree.Tree, id uint) (dbItem, error) {
	var r dbItem
	err := tree.GetNode(ctx, id, storeScope, &r)
	return r, err
}

// isSubtaskRow reports whether r is a task whose parent is a task, reading the
// parent through tree.
func isSubtaskRow(ctx context.Context, tree *closuretree.Tree, r dbItem) (bool, error) {
	if Kind(r.Kind) != Task || r.ParentId == 0 {
		return false, nil
	}
	p, err := kindAndParent(ctx, tree, r.ParentId)
	if err != nil {
		return false, err
	}
	return Kind(p.Kind) == Task, nil
}

// taskParent reports whether parentID (0 for the root) is a task, so an item
// placed under it would be a subtask, and whether it is itself a subtask, so a
// task placed under it would be too deep.
func taskParent(ctx context.Context, tree *closuretree.Tree, parentID uint) (isTask, isSubtask bool, err error) {
	if parentID == 0 {
		return false, false, nil
	}
	p, err := kindAndParent(ctx, tree, parentID)
	if err != nil || Kind(p.Kind) != Task {
		return false, false, err
	}
	isSubtask, err = isSubtaskRow(ctx, tree, p)
	return true, isSubtask, err
}

// add inserts row under parentID, after the sibling afterID (0 places it
// first), and returns its new ID.
func (s *Store) add(ctx context.Context, row *dbItem, parentID, afterID uint) (uint, error) {
	err := s.write(ctx, func(_ *gorm.DB, tree *closuretree.Tree) error {
		return addRow(ctx, tree, row, parentID, afterID)
	})
	if err != nil {
		return 0, err
	}
	return row.NodeId, nil
}

// addRow inserts row through tree (bound to the caller's transaction) under
// parentID, after the sibling afterID; row.NodeId holds its ID afterwards.
func addRow(ctx context.Context, tree *closuretree.Tree, row *dbItem, parentID, afterID uint) error {
	return tree.Add(ctx, row, &parentID, &afterID, storeScope)
}

// AddCategory adds a category under parentID (0 for a root), after the sibling
// afterID (0 places it first), and returns its ID.
func (s *Store) AddCategory(ctx context.Context, title string, parentID, afterID uint) (uint, error) {
	return s.add(ctx, &dbItem{Kind: int(Category), Title: title}, parentID, afterID)
}

// AddTask adds a task with fields f and status st under parentID (0 for a
// root), after the sibling afterID (0 places it first), and returns its ID.
// Under a subtask it fails with ErrTooDeep; under a task (making a subtask)
// with a state other than Open or Done, with ErrSubtaskStatus.
func (s *Store) AddTask(ctx context.Context, f Fields, st Status, parentID, afterID uint) (uint, error) {
	row := &dbItem{Kind: int(Task), Title: f.Title, Status: int(st), Description: f.Description, Plan: f.Plan}
	err := s.write(ctx, func(_ *gorm.DB, tree *closuretree.Tree) error {
		underTask, underSubtask, err := taskParent(ctx, tree, parentID)
		if err != nil {
			return err
		}
		if underSubtask {
			return ErrTooDeep
		}
		if underTask && !st.SubtaskAllowed() {
			return ErrSubtaskStatus
		}
		return addRow(ctx, tree, row, parentID, afterID)
	})
	if err != nil {
		return 0, err
	}
	return row.NodeId, nil
}

// itemRows scopes a query to the rows of the store's tree with the given IDs.
func itemRows(tx *gorm.DB, ids ...uint) *gorm.DB {
	return tx.Model(&dbItem{}).Where("node_id IN ? AND tenant = ?", ids, storeScope)
}

// Edit sets the fields of item id that c names: its title and, for a task, its
// description and plan; a category ignores c.Description and c.Plan. Fields c
// leaves nil, and kind, status and placement, are kept.
func (s *Store) Edit(ctx context.Context, id uint, c Changes) error {
	return s.write(ctx, func(tx *gorm.DB, _ *closuretree.Tree) error {
		var kinds []int
		if err := itemRows(tx, id).Pluck("kind", &kinds).Error; err != nil {
			return err
		}
		if len(kinds) == 0 {
			return ErrNotFound
		}
		cols := map[string]any{}
		if c.Title != nil {
			cols["title"] = *c.Title
		}
		if Kind(kinds[0]) == Task {
			if c.Description != nil {
				cols["description"] = *c.Description
			}
			if c.Plan != nil {
				cols["plan"] = *c.Plan
			}
		}
		if len(cols) == 0 {
			return nil
		}
		return itemRows(tx, id).Updates(cols).Error
	})
}

// SetStatuses sets the status of every item in states. An ID the store does not
// hold is skipped, so restoring statuses after one of the tasks was deleted
// still restores the others. Giving a subtask a state other than Open or Done
// fails the whole write with ErrSubtaskStatus.
func (s *Store) SetStatuses(ctx context.Context, states map[uint]Status) error {
	byStatus := map[Status][]uint{}
	for id, st := range states {
		byStatus[st] = append(byStatus[st], id)
	}
	return s.write(ctx, func(tx *gorm.DB, tree *closuretree.Tree) error {
		for id, st := range states {
			if st.SubtaskAllowed() {
				continue
			}
			r, err := kindAndParent(ctx, tree, id)
			if errors.Is(err, closuretree.ErrNodeNotFound) {
				continue
			}
			if err != nil {
				return err
			}
			sub, err := isSubtaskRow(ctx, tree, r)
			if err != nil {
				return err
			}
			if sub {
				return ErrSubtaskStatus
			}
		}
		for st, ids := range byStatus {
			if err := itemRows(tx, ids...).Update("status", int(st)).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

// CascadeStatus sets item id to st and every item below it, as the tree is at
// write time, to Done when st is Done and to Open otherwise: a board state never
// reaches a subtask. When id is itself a subtask, st must be Open or Done, or
// the write fails with ErrSubtaskStatus.
func (s *Store) CascadeStatus(ctx context.Context, id uint, st Status) error {
	return s.write(ctx, func(tx *gorm.DB, tree *closuretree.Tree) error {
		r, err := kindAndParent(ctx, tree, id)
		if err != nil {
			return err
		}
		if !st.SubtaskAllowed() {
			sub, err := isSubtaskRow(ctx, tree, r)
			if err != nil {
				return err
			}
			if sub {
				return ErrSubtaskStatus
			}
		}
		ids, err := tree.DescendantIds(ctx, id, 0, storeScope)
		if err != nil {
			return err
		}
		below := Open
		if st == Done {
			below = Done
		}
		if len(ids) > 0 {
			if err := itemRows(tx, ids...).Update("status", int(below)).Error; err != nil {
				return err
			}
		}
		return itemRows(tx, id).Update("status", int(st)).Error
	})
}

// Move places item id, with its subtree, under parentID (0 for a root), after
// the sibling afterID (0 places it first). The existence check is the store's:
// closure-tree's Update takes a missing node for a root one and reports a move
// of it to the root as done.
//
// A task moved under a task must have no children, and the new parent must not
// be a subtask, or the move fails with ErrTooDeep. A task that becomes, stays
// or stops being a subtask takes its status from StatusAfterMove; a task moved
// between categories keeps its status.
func (s *Store) Move(ctx context.Context, id, parentID, afterID uint) error {
	return s.write(ctx, func(tx *gorm.DB, tree *closuretree.Tree) error {
		r, err := kindAndParent(ctx, tree, id)
		if err != nil {
			return err
		}
		if Kind(r.Kind) == Task {
			if err := moveTaskStatus(ctx, tx, tree, r, parentID); err != nil {
				return err
			}
		}
		// Update with a nil item moves only: an item would overwrite every payload
		// column, the status just set included.
		return tree.Update(ctx, id, nil, &parentID, &afterID, storeScope)
	})
}

// moveTaskStatus checks that moving task r under parentID keeps subtasks one
// level deep, and rewrites r's status for its new place when it becomes, stays
// or stops being a subtask.
func moveTaskStatus(ctx context.Context, tx *gorm.DB, tree *closuretree.Tree, r dbItem, parentID uint) error {
	toSubtask, parentIsSubtask, err := taskParent(ctx, tree, parentID)
	if err != nil {
		return err
	}
	if toSubtask {
		children, err := tree.DescendantIds(ctx, r.NodeId, 1, storeScope)
		if err != nil {
			return err
		}
		if len(children) > 0 || parentIsSubtask {
			return ErrTooDeep
		}
	}

	wasSubtask := false
	oldParent := Open
	if r.ParentId != 0 {
		p, err := kindAndParent(ctx, tree, r.ParentId)
		if err != nil {
			return err
		}
		if Kind(p.Kind) == Task {
			wasSubtask = true
			oldParent = Status(p.Status)
		}
	}
	if !toSubtask && !wasSubtask {
		return nil
	}
	st := StatusAfterMove(Status(r.Status), toSubtask, oldParent)
	return itemRows(tx, r.NodeId).Update("status", int(st)).Error
}

// Delete removes item id together with its subtree as it is at write time.
func (s *Store) Delete(ctx context.Context, id uint) error {
	return s.write(ctx, func(_ *gorm.DB, tree *closuretree.Tree) error {
		return tree.DeleteRecurse(ctx, id, storeScope)
	})
}

// Prune removes every fully-done task subtree (see Document.DoneSubtrees) and
// returns how many tasks went. It finds them in the tree as it is at write
// time, not in the caller's copy, so a subtask another process just added under
// a done task keeps that task.
func (s *Store) Prune(ctx context.Context) (int, error) {
	removed := 0
	err := s.write(ctx, func(tx *gorm.DB, tree *closuretree.Tree) error {
		doc, err := loadDocument(ctx, tx)
		if err != nil {
			return err
		}
		for _, it := range doc.DoneSubtrees() {
			if err := tree.DeleteRecurse(ctx, it.ID, storeScope); err != nil {
				return err
			}
			_, total := it.TaskCounts()
			removed += 1 + total
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	return removed, nil
}
