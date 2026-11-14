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

// write runs fn as one store write: a single transaction holding fn's changes
// through tx and a closure-tree bound to tx (so the tree's own transactions are
// savepoints of this one), the revision bump and, when the sort order of some
// sibling group runs out of room, its renormalization. Nothing in fn may use
// s.db: the SQLite writer is one connection, already held by tx.
func (s *Store) write(ctx context.Context, fn func(tx *gorm.DB, tree *closuretree.Tree) error) error {
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
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
	})
	if errors.Is(err, closuretree.ErrNodeNotFound) || errors.Is(err, closuretree.ErrParentNotFound) {
		return ErrNotFound
	}
	return err
}

// add inserts row under parentID, after the sibling afterID (0 places it
// first), and returns its new ID.
func (s *Store) add(ctx context.Context, row *dbItem, parentID, afterID uint) (uint, error) {
	err := s.write(ctx, func(_ *gorm.DB, tree *closuretree.Tree) error {
		return tree.Add(ctx, row, &parentID, &afterID, storeScope)
	})
	if err != nil {
		return 0, err
	}
	return row.NodeId, nil
}

// AddCategory adds a category under parentID (0 for a root), after the sibling
// afterID (0 places it first), and returns its ID.
func (s *Store) AddCategory(ctx context.Context, title string, parentID, afterID uint) (uint, error) {
	return s.add(ctx, &dbItem{Kind: int(Category), Title: title}, parentID, afterID)
}

// AddTask adds a task under parentID (0 for a root), after the sibling afterID
// (0 places it first), and returns its ID.
func (s *Store) AddTask(ctx context.Context, title, desc string, st Status, parentID, afterID uint) (uint, error) {
	return s.add(ctx, &dbItem{Kind: int(Task), Title: title, Status: int(st), Description: desc}, parentID, afterID)
}

// itemRows scopes a query to the rows of the store's tree with the given IDs.
func itemRows(tx *gorm.DB, ids ...uint) *gorm.DB {
	return tx.Model(&dbItem{}).Where("node_id IN ? AND tenant = ?", ids, storeScope)
}

// requireItem returns ErrNotFound when the store holds no item id.
func requireItem(tx *gorm.DB, id uint) error {
	var count int64
	if err := itemRows(tx, id).Count(&count).Error; err != nil {
		return err
	}
	if count == 0 {
		return ErrNotFound
	}
	return nil
}

// Edit sets the title of item id and, for a task, its description; a
// category's description stays empty. Kind, status and placement are kept.
func (s *Store) Edit(ctx context.Context, id uint, title, desc string) error {
	return s.write(ctx, func(tx *gorm.DB, _ *closuretree.Tree) error {
		var kinds []int
		if err := itemRows(tx, id).Pluck("kind", &kinds).Error; err != nil {
			return err
		}
		if len(kinds) == 0 {
			return ErrNotFound
		}
		cols := map[string]any{"title": title}
		if Kind(kinds[0]) == Task {
			cols["description"] = desc
		}
		return itemRows(tx, id).Updates(cols).Error
	})
}

// SetStatuses sets the status of every item in states. An ID the store does not
// hold is skipped, so restoring statuses after one of the tasks was deleted
// still restores the others.
func (s *Store) SetStatuses(ctx context.Context, states map[uint]Status) error {
	byStatus := map[Status][]uint{}
	for id, st := range states {
		byStatus[st] = append(byStatus[st], id)
	}
	return s.write(ctx, func(tx *gorm.DB, _ *closuretree.Tree) error {
		for st, ids := range byStatus {
			if err := itemRows(tx, ids...).Update("status", int(st)).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

// CascadeStatus sets item id and every item below it, as the tree is at write
// time, to st.
func (s *Store) CascadeStatus(ctx context.Context, id uint, st Status) error {
	return s.write(ctx, func(tx *gorm.DB, tree *closuretree.Tree) error {
		if err := requireItem(tx, id); err != nil {
			return err
		}
		ids, err := tree.DescendantIds(ctx, id, 0, storeScope)
		if err != nil {
			return err
		}
		return itemRows(tx, append(ids, id)...).Update("status", int(st)).Error
	})
}

// Move places item id, with its subtree, under parentID (0 for a root), after
// the sibling afterID (0 places it first). The existence check is the store's:
// closure-tree's Update takes a missing node for a root one and reports a move
// of it to the root as done.
func (s *Store) Move(ctx context.Context, id, parentID, afterID uint) error {
	return s.write(ctx, func(tx *gorm.DB, tree *closuretree.Tree) error {
		if err := requireItem(tx, id); err != nil {
			return err
		}
		return tree.Update(ctx, id, nil, &parentID, &afterID, storeScope)
	})
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
