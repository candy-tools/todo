package todo

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

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

// ErrFileReplaced is returned when the store file was removed, or another file
// was moved in at its path, since the store was opened. SQLite keeps using the
// file it opened, so a write would land in an unlinked copy and be lost; the
// store must be reopened instead.
var ErrFileReplaced = errors.New("database file was removed or replaced; reopen todo")

// ErrBusy is returned by a write that gave up waiting for another process to
// release the SQLite writer lock (SQLITE_BUSY after the store's busy timeout).
var ErrBusy = errors.New("list busy (another todo is writing), try again")

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
	file os.FileInfo // the file at path when the store was opened
}

// DefaultBusyTimeout is how long a store's writes wait, by default, for another
// process to release the SQLite writer lock before failing with ErrBusy.
const DefaultBusyTimeout = 5 * time.Second

// Option configures OpenStore.
type Option func(*storeCfg)

// storeCfg holds the settings Options change.
type storeCfg struct {
	busyTimeout time.Duration
}

// WithBusyTimeout sets how long the store's writes wait for another process to
// release the writer lock before failing with ErrBusy (DefaultBusyTimeout when
// not given). An interactive caller keeps it short, so a stuck writer elsewhere
// does not freeze it.
func WithBusyTimeout(d time.Duration) Option {
	return func(c *storeCfg) { c.busyTimeout = d }
}

// OpenStore opens the SQLite store at path, creating the file and its schema
// when they do not exist yet.
func OpenStore(path string, opts ...Option) (*Store, error) {
	cfg := storeCfg{busyTimeout: DefaultBusyTimeout}
	for _, o := range opts {
		o(&cfg)
	}
	db, err := openDB(path, cfg.busyTimeout)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	s := &Store{db: db, path: path}
	if err := s.migrate(); err != nil {
		_ = gormdb.Close(db)
		return nil, fmt.Errorf("prepare %s: %w", path, err)
	}
	fi, err := os.Stat(path)
	if err != nil {
		_ = gormdb.Close(db)
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	s.file = fi
	return s, nil
}

// CheckFile returns ErrFileReplaced when the file at the store's path is no
// longer the one it opened: it was removed, or another file took its place (a
// `git checkout`, a sync tool renaming a new copy in).
func (s *Store) CheckFile() error {
	fi, err := os.Stat(s.path)
	if errors.Is(err, os.ErrNotExist) || (err == nil && !os.SameFile(fi, s.file)) {
		return fmt.Errorf("%s: %w", s.path, ErrFileReplaced)
	}
	return err
}

// openBusyDelay is the pause between openDB's attempts at an open that SQLite
// reports busy.
const openBusyDelay = 10 * time.Millisecond

// sqliteBusy is SQLite's SQLITE_BUSY result code.
const sqliteBusy = 5

// openDB opens the SQLite file at path with connections that wait up to
// busyTimeout for a lock another process holds. Processes that open a new file
// at the same time each switch it to WAL mode, and SQLite fails all but one of
// them with SQLITE_BUSY at once, without waiting out the busy timeout, to avoid
// a deadlock. Such an open is retried for up to busyTimeout: once the file is in
// WAL mode, the switch is a no-op.
func openDB(path string, busyTimeout time.Duration) (*gorm.DB, error) {
	deadline := time.Now().Add(busyTimeout)
	for {
		db, err := gormdb.Open(gormdb.Cfg{
			Driver: gormdb.DriverSqlite,
			Sqlite: gormdb.SqliteCfg{Path: path, BusyTimeout: busyTimeout},
			Logger: logger.Discard,
		}, gormdb.Drivers{Sqlite: sqlite.Driver{}})
		if err == nil || !isBusy(err) || time.Now().After(deadline) {
			return db, err
		}
		time.Sleep(openBusyDelay)
	}
}

// isBusy reports whether err is SQLite's SQLITE_BUSY, extended codes included.
func isBusy(err error) bool {
	var coded interface{ Code() int }
	return errors.As(err, &coded) && coded.Code()&0xff == sqliteBusy
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

// loadBetweenReads, when set by a test, runs in Load between the revision read
// and the tree read, so a test can land a write from another handle there.
var loadBetweenReads func()

// Load reads the whole list and the revision it reflects. Both are read in one
// transaction on the read-only pool, which begins deferred (not holding the
// writer) and whose first read fixes the snapshot (SQLite WAL), so a write
// landing in between cannot pair a newer tree with an older revision.
func (s *Store) Load(ctx context.Context) (*Document, int64, error) {
	var doc *Document
	var rev int64
	err := s.db.WithContext(ctx).Clauses(gormdb.Read).Transaction(func(tx *gorm.DB) error {
		var err error
		if rev, err = revisionIn(ctx, tx); err != nil {
			return err
		}
		if loadBetweenReads != nil {
			loadBetweenReads()
		}
		doc, err = loadDocument(ctx, tx)
		return err
	})
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
