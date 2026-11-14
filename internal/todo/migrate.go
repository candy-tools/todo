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

// errAlreadyUpgraded ends an upgrade write that finds the store already on
// schema 2, because another process migrated it first. Returning an error rolls
// the empty transaction back, so the revision is not bumped a second time.
var errAlreadyUpgraded = errors.New("store already upgraded")

// migrate brings the store to schemaVersion. It refuses a store from a newer
// todo, backs up a v1 store before its schema changes, creates or extends the
// tables, then writes the meta row of a new store or upgrades a v1 one.
func (s *Store) migrate() error {
	version, err := s.storedSchemaVersion()
	if err != nil {
		return err
	}
	if version > schemaVersion {
		return newerSchemaError(version)
	}
	if version == 1 {
		if err := s.backup(); err != nil {
			return fmt.Errorf("back up to %s: %w", BackupPath(s.path), err)
		}
	}
	tree, err := closuretree.New(s.db, dbItem{})
	if err != nil {
		return err
	}
	if err := tree.Migrate(); err != nil {
		return err
	}
	if err := s.db.AutoMigrate(&dbMeta{}); err != nil {
		return err
	}
	switch version {
	case 0:
		return s.db.Where(dbMeta{ID: 1}).
			Attrs(dbMeta{Revision: 0, SchemaVersion: schemaVersion}).
			FirstOrCreate(&dbMeta{}).Error
	case 1:
		if err := s.upgradeToV2(context.Background()); err != nil {
			return fmt.Errorf("upgrade to schema 2: %w", err)
		}
	}
	return nil
}

// storedSchemaVersion reads the schema version from the meta row. A new store,
// without a meta table or row yet, is version 0.
func (s *Store) storedSchemaVersion() (int, error) {
	if !s.db.Migrator().HasTable(&dbMeta{}) {
		return 0, nil
	}
	var metas []dbMeta
	if err := s.db.Where("id = ?", 1).Limit(1).Find(&metas).Error; err != nil {
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
// closure table stays consistent. A store another process has upgraded since
// is left alone.
func (s *Store) upgradeToV2(ctx context.Context) error {
	err := s.write(ctx, func(tx *gorm.DB, tree *closuretree.Tree) error {
		var m dbMeta
		if err := tx.First(&m, 1).Error; err != nil {
			return err
		}
		if m.SchemaVersion >= 2 {
			return errAlreadyUpgraded
		}
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
	if errors.Is(err, errAlreadyUpgraded) {
		return nil
	}
	return err
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
