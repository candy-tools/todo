package todo

import (
	"context"
	"errors"
	"fmt"
	"os"

	closuretree "github.com/go-bumbu/closure-tree"
	"github.com/go-bumbu/db/gormdb"
	"github.com/go-bumbu/db/gormdb/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// storeScope is the single closure-tree scope (closure-tree's "tenant") that
// holds the whole task tree of a store.
const storeScope = "todo"

// schemaVersion is the version recorded in the meta row of a new store.
const schemaVersion = 2

// ErrNewerSchema is returned when the store was written by a newer todo, whose
// schema this binary does not know and must not read or write. It is wrapped
// with the version found.
var ErrNewerSchema = errors.New("database was created by a newer todo")

// newerSchemaError returns ErrNewerSchema wrapped with the schema version found
// in the meta row.
func newerSchemaError(version int) error {
	return fmt.Errorf("%w (schema %d); upgrade todo", ErrNewerSchema, version)
}

// dbItem is a node of the task tree: closure-tree owns its identity, scope and
// position, and this adds the payload. The table is closure-tree's node table;
// the closure table next to it is named after it.
type dbItem struct {
	closuretree.Node
	Kind        int    `gorm:"not null"`
	Title       string `gorm:"not null"`
	Status      int    `gorm:"not null"`
	Description string `gorm:"not null"`
	Plan        string `gorm:"not null;default:''"`

	// Children is filled by TreeDescendants only, which needs the field to link
	// the nodes it reads into a tree.
	Children []*dbItem `gorm:"-"`
}

func (dbItem) TableName() string { return "items" }

// dbMeta is the single-row table holding the store's revision counter (bumped
// by every write, so a reader can tell the file changed) and schema version.
type dbMeta struct {
	ID            uint `gorm:"primaryKey"`
	Revision      int64
	SchemaVersion int
}

func (dbMeta) TableName() string { return "meta" }

// Store is a TODO list persisted in a SQLite file.
type Store struct {
	db   *gorm.DB
	path string
}

// OpenStore opens the SQLite store at path, creating the file and its schema
// when they do not exist yet.
func OpenStore(path string) (*Store, error) {
	db, err := gormdb.Open(gormdb.Cfg{
		Driver: gormdb.DriverSqlite,
		Sqlite: gormdb.SqliteCfg{Path: path},
		Logger: logger.Discard,
	}, gormdb.Drivers{Sqlite: sqlite.Driver{}})
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	s := &Store{db: db, path: path}
	if err := s.migrate(); err != nil {
		_ = gormdb.Close(db)
		return nil, fmt.Errorf("prepare %s: %w", path, err)
	}
	return s, nil
}

// migrate refuses a store from a newer todo, then creates the tables and makes
// sure the meta row exists.
func (s *Store) migrate() error {
	if err := s.checkSchemaVersion(); err != nil {
		return err
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
	return s.db.Where(dbMeta{ID: 1}).
		Attrs(dbMeta{Revision: 0, SchemaVersion: schemaVersion}).
		FirstOrCreate(&dbMeta{}).Error
}

// checkSchemaVersion returns an error when the meta row records a schema
// version newer than schemaVersion: this binary does not know that schema and
// must not migrate or write it. A new store, without a meta table or row yet,
// passes.
func (s *Store) checkSchemaVersion() error {
	if !s.db.Migrator().HasTable(&dbMeta{}) {
		return nil
	}
	var metas []dbMeta
	if err := s.db.Where("id = ?", 1).Limit(1).Find(&metas).Error; err != nil {
		return err
	}
	if len(metas) == 1 && metas[0].SchemaVersion > schemaVersion {
		return newerSchemaError(metas[0].SchemaVersion)
	}
	return nil
}

// StoreExists reports whether a store file exists at path.
func StoreExists(path string) (bool, error) {
	_, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// Close releases the database.
func (s *Store) Close() error { return gormdb.Close(s.db) }

// Path returns the path of the store file.
func (s *Store) Path() string { return s.path }

// Revision returns the store's write counter.
func (s *Store) Revision(ctx context.Context) (int64, error) {
	return revisionIn(ctx, s.db)
}

// revisionIn reads the revision through db, which may be a transaction handle.
// It returns ErrNewerSchema when a newer todo has since upgraded the store, so
// a process left running stops reading a schema it does not know.
func revisionIn(ctx context.Context, db *gorm.DB) (int64, error) {
	var m dbMeta
	if err := db.WithContext(ctx).First(&m, 1).Error; err != nil {
		return 0, err
	}
	if m.SchemaVersion > schemaVersion {
		return 0, newerSchemaError(m.SchemaVersion)
	}
	return m.Revision, nil
}

// Load reads the whole list and the revision it reflects. The revision is read
// first, without a transaction: a write landing in between only makes the
// caller reload once more later, never miss a change.
func (s *Store) Load(ctx context.Context) (*Document, int64, error) {
	rev, err := revisionIn(ctx, s.db)
	if err != nil {
		return nil, 0, err
	}
	doc, err := loadDocument(ctx, s.db)
	if err != nil {
		return nil, 0, err
	}
	return doc, rev, nil
}

// loadDocument reads the whole tree through db (which may be a transaction
// handle) and maps it to a Document. It is the only place rows become Items.
func loadDocument(ctx context.Context, db *gorm.DB) (*Document, error) {
	tree, err := closuretree.New(db, dbItem{})
	if err != nil {
		return nil, err
	}
	var rows []*dbItem
	if err := tree.TreeDescendants(ctx, 0, 0, storeScope, &rows); err != nil {
		return nil, err
	}
	doc := &Document{}
	for _, r := range rows {
		doc.Roots = append(doc.Roots, itemFromRow(r, nil))
	}
	return doc, nil
}

// itemFromRow converts a row and its subtree into an Item under parent. A
// category's Level is one more than its number of category ancestors.
func itemFromRow(r *dbItem, parent *Item) *Item {
	it := &Item{
		ID:          r.NodeId,
		Kind:        Kind(r.Kind),
		Title:       r.Title,
		Status:      Status(r.Status),
		Description: r.Description,
		Plan:        r.Plan,
		Parent:      parent,
	}
	if it.Kind == Category {
		it.Level = 1
		for p := parent; p != nil; p = p.Parent {
			if p.Kind == Category {
				it.Level++
			}
		}
	}
	for _, c := range r.Children {
		it.Children = append(it.Children, itemFromRow(c, it))
	}
	return it
}
