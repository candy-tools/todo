package todo

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	closuretree "github.com/go-bumbu/closure-tree"
	"gorm.io/gorm"
)

// BackupPath returns where the migration keeps a copy of the store at dbPath
// as it was before the upgrade.
func BackupPath(dbPath string) string { return dbPath + ".bak" }

// errNotBackedUp stops a migration that finds a v1 store inside its transaction
// after the check before it found none, so no backup was taken: a v1 todo
// created the file in between. migrate starts over, which backs it up.
var errNotBackedUp = errors.New("v1 store was not backed up")

// migrate brings the store to schemaVersion. It refuses a store from a newer
// todo, backs up a v1 store before its schema changes, then, in one write
// transaction, creates or extends the tables and writes the meta row of a new
// store or upgrades a v1 one.
func (s *Store) migrate() error {
	err := s.migrateOnce()
	if errors.Is(err, errNotBackedUp) {
		err = s.migrateOnce()
	}
	return err
}

// migrateOnce is one migration attempt. The backup runs first, outside any
// transaction, because VACUUM INTO cannot run inside one. Everything after it
// runs in a single transaction, which takes the write lock as it begins: when
// several processes open the file at once, they migrate one after the other,
// and each decides from the version as the one before it left it. A new store
// is created once and a v1 store upgraded once; a failed upgrade rolls the
// schema changes back with the data.
func (s *Store) migrateOnce() error {
	ctx := context.Background()
	version, err := storedSchemaVersion(s.db)
	if err != nil {
		return err
	}
	if version > schemaVersion {
		return newerSchemaError(version)
	}
	backedUp := false
	if version == 1 {
		if err := s.backup(); err != nil {
			return fmt.Errorf("back up to %s: %w", BackupPath(s.path), err)
		}
		backedUp = true
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		version, err := storedSchemaVersion(tx)
		if err != nil {
			return err
		}
		if version > schemaVersion {
			return newerSchemaError(version)
		}
		if version == 1 && !backedUp {
			return errNotBackedUp
		}
		tree, err := closuretree.New(tx, dbItem{})
		if err != nil {
			return err
		}
		if err := tree.Migrate(); err != nil {
			return err
		}
		if err := tx.AutoMigrate(&dbMeta{}); err != nil {
			return err
		}
		switch version {
		case 0:
			return tx.Where(dbMeta{ID: 1}).
				Attrs(dbMeta{Revision: 0, SchemaVersion: schemaVersion}).
				FirstOrCreate(&dbMeta{}).Error
		case 1:
			if err := upgradeToV2(ctx, tx); err != nil {
				return fmt.Errorf("upgrade to schema 2: %w", err)
			}
		}
		return nil
	})
}

// storedSchemaVersion reads the schema version from the meta row through db,
// which may be a transaction handle. A new store, without a meta table or row
// yet, is version 0.
func storedSchemaVersion(db *gorm.DB) (int, error) {
	if !db.Migrator().HasTable(&dbMeta{}) {
		return 0, nil
	}
	var metas []dbMeta
	if err := db.Where("id = ?", 1).Limit(1).Find(&metas).Error; err != nil {
		return 0, err
	}
	if len(metas) == 0 {
		return 0, nil
	}
	return metas[0].SchemaVersion, nil
}

// backup copies the store, as it is now, to BackupPath. An existing backup is
// kept: a migration that is retried, or that races another process, never
// replaces the copy of the original v1 store. The copy is made with VACUUM
// INTO, which reads a consistent snapshot of a WAL-mode database, into a
// temporary file next to the store, and only the finished file is linked (or
// renamed) into place, so BackupPath never holds a partial copy.
func (s *Store) backup() error {
	bak := BackupPath(s.path)
	exists, err := StoreExists(bak)
	if err != nil || exists {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(bak), ".todo-backup-*.db")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer removeStoreFiles(tmp)
	if err := f.Close(); err != nil {
		return err
	}
	if err := s.db.Exec("VACUUM INTO " + sqlString(tmp)).Error; err != nil {
		return err
	}
	_, err = placeStore(tmp, bak)
	return err
}

// sqlString quotes v as an SQL string literal.
func sqlString(v string) string {
	return "'" + strings.ReplaceAll(v, "'", "''") + "'"
}

// upgradeToV2 rewrites a v1 store for schema 2 in one write: every task nested
// below a subtask moves up to its top-level task, at its reading-order place
// (see Document.FlattenSubtasks), every subtask's status becomes Open or Done,
// and the meta row records version 2. The moves go through closure-tree, so the
// closure table stays consistent. It runs in the transaction tx the caller
// holds. A store already on schema 2 is left alone, its revision included.
func upgradeToV2(ctx context.Context, tx *gorm.DB) error {
	version, err := storedSchemaVersion(tx)
	if err != nil {
		return err
	}
	if version >= 2 {
		return nil
	}
	return writeIn(ctx, tx, func(tx *gorm.DB, tree *closuretree.Tree) error {
		doc, err := loadDocument(ctx, tx)
		if err != nil {
			return err
		}
		for _, it := range doc.FlattenSubtasks() {
			parentID, afterID := doc.Placement(it)
			// A nil item moves only. Store.Move is not used: it refuses a task
			// that still has children of its own, as a1 does until a1x moves.
			if err := tree.Update(ctx, it.ID, nil, &parentID, &afterID, storeScope); err != nil {
				return err
			}
		}
		if err := writeSubtaskStatuses(tx, doc); err != nil {
			return err
		}
		return tx.Model(&dbMeta{}).Where("id = ?", 1).Update("schema_version", 2).Error
	})
}

// writeSubtaskStatuses stores the status every subtask of doc holds in memory,
// one update per status value. FlattenSubtasks normalizes subtasks it did not
// move as well, so all of them are written, not only the moved ones.
func writeSubtaskStatuses(tx *gorm.DB, doc *Document) error {
	byStatus := map[Status][]uint{}
	for _, it := range doc.Enumerate() {
		if it.IsSubtask() {
			byStatus[it.Status] = append(byStatus[it.Status], it.ID)
		}
	}
	for st, ids := range byStatus {
		if err := itemRows(tx, ids...).Update("status", int(st)).Error; err != nil {
			return err
		}
	}
	return nil
}
