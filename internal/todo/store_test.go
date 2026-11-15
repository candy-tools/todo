package todo

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-bumbu/db/gormdb"
	"github.com/go-bumbu/db/gormdb/sqlite"
	"gorm.io/gorm/logger"
)

// newStore opens a fresh store in a temporary directory, closed with the test.
func newStore(t *testing.T) *Store {
	t.Helper()
	s, err := OpenStore(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func TestOpenStoreCreatesEmpty(t *testing.T) {
	s := newStore(t)
	doc, rev, err := s.Load(context.Background())
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(doc.Roots) != 0 || rev != 0 {
		t.Fatalf("got %d roots, revision %d; want empty store at revision 0", len(doc.Roots), rev)
	}
	if ok, err := StoreExists(s.Path()); err != nil || !ok {
		t.Fatalf("StoreExists(created) = %v, %v; want true, nil", ok, err)
	}
	missing := filepath.Join(t.TempDir(), "missing.db")
	if ok, err := StoreExists(missing); err != nil || ok {
		t.Fatalf("StoreExists(missing) = %v, %v; want false, nil", ok, err)
	}
}

func TestOpenStoreRefusesNewerSchema(t *testing.T) {
	s := newStore(t)
	if err := s.db.Model(&dbMeta{}).Where("id = ?", 1).Update("schema_version", schemaVersion+1).Error; err != nil {
		t.Fatal(err)
	}
	path := s.Path()
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s2, err := OpenStore(path)
	if err == nil {
		_ = s2.Close()
		t.Fatal("OpenStore on a newer schema succeeded; want an error")
	}
	want := fmt.Sprintf("database was created by a newer todo (schema %d); upgrade todo", schemaVersion+1)
	if !strings.Contains(err.Error(), want) {
		t.Errorf("err = %q; want it to contain %q", err, want)
	}
	if !errors.Is(err, ErrNewerSchema) {
		t.Errorf("err = %v; want it to wrap ErrNewerSchema", err)
	}
}

// bumpSchemaVersion records a schema version newer than this binary's in the
// store at path through a second database handle, standing in for a newer todo
// that migrated the file while s stays open.
func bumpSchemaVersion(t *testing.T, path string) {
	t.Helper()
	db, err := gormdb.Open(gormdb.Cfg{
		Driver: gormdb.DriverSqlite,
		Sqlite: gormdb.SqliteCfg{Path: path},
		Logger: logger.Discard,
	}, gormdb.Drivers{Sqlite: sqlite.Driver{}})
	if err != nil {
		t.Fatalf("open second handle: %v", err)
	}
	defer func() { _ = gormdb.Close(db) }()
	if err := db.Model(&dbMeta{}).Where("id = ?", 1).Update("schema_version", schemaVersion+1).Error; err != nil {
		t.Fatalf("bump schema version: %v", err)
	}
}

func TestLoadRefusesNewerSchema(t *testing.T) {
	s := newStore(t)
	bumpSchemaVersion(t, s.Path())

	if _, _, err := s.Load(context.Background()); !errors.Is(err, ErrNewerSchema) {
		t.Fatalf("Load = %v, want ErrNewerSchema", err)
	}
}

func TestWriteRefusesNewerSchema(t *testing.T) {
	s := newStore(t)
	bumpSchemaVersion(t, s.Path())

	if _, err := s.AddCategory(context.Background(), "C", 0, 0); !errors.Is(err, ErrNewerSchema) {
		t.Fatalf("AddCategory = %v, want ErrNewerSchema", err)
	}
	var count int64
	if err := s.db.Model(&dbItem{}).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("store holds %d items after the refused write, want 0", count)
	}
}

func TestOpenStoreRejectsDirectoryPath(t *testing.T) {
	if s, err := OpenStore(t.TempDir()); err == nil {
		_ = s.Close()
		t.Fatal("OpenStore on a directory succeeded; want an error")
	}
}

// TestWriteRefusesReplacedFile removes the store file, or renames another store
// over it, while s stays open: SQLite would keep writing to the unlinked inode,
// so the check must report ErrFileReplaced and every write must be refused.
func TestWriteRefusesReplacedFile(t *testing.T) {
	replace := map[string]func(t *testing.T, path string){
		"removed": func(t *testing.T, path string) {
			if err := os.Remove(path); err != nil {
				t.Fatal(err)
			}
		},
		"renamed over": func(t *testing.T, path string) {
			other := newStore(t)
			if err := other.Close(); err != nil {
				t.Fatal(err)
			}
			if err := os.Rename(other.Path(), path); err != nil {
				t.Fatal(err)
			}
		},
	}
	for name, fn := range replace {
		t.Run(name, func(t *testing.T) {
			s := newStore(t)
			if err := s.CheckFile(); err != nil {
				t.Fatalf("CheckFile on the open file = %v, want nil", err)
			}
			fn(t, s.Path())
			if err := s.CheckFile(); !errors.Is(err, ErrFileReplaced) {
				t.Fatalf("CheckFile = %v, want ErrFileReplaced", err)
			}
			if _, err := s.AddCategory(context.Background(), "C", 0, 0); !errors.Is(err, ErrFileReplaced) {
				t.Fatalf("AddCategory = %v, want ErrFileReplaced", err)
			}
			doc, _, err := s.Load(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			if len(doc.Roots) != 0 {
				t.Fatalf("the refused write reached the unlinked file: %d roots", len(doc.Roots))
			}
		})
	}
}

// TestOpenStoreFileReplacedWhileOpening renames another store, holding a
// category B, over the path after OpenStore's final open and before it records
// the file's identity. The store must not take the new file's identity for the
// one its connections hold: its writes must land in the file at the path (next
// to B), or OpenStore must fail with ErrFileReplaced.
func TestOpenStoreFileReplacedWhileOpening(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		name   string
		exists bool
		opts   []Option
	}{
		{name: "existing file", exists: true},
		{name: "existing file, short busy timeout", exists: true, opts: []Option{WithBusyTimeout(100 * time.Millisecond)}},
		{name: "new file"},
		{name: "new file, short busy timeout", opts: []Option{WithBusyTimeout(100 * time.Millisecond)}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "t.db")
			if tc.exists {
				storeWithCategory(t, path, "A")
			}
			openStoreBeforeIdentity = func() {
				openStoreBeforeIdentity = nil
				other := filepath.Join(t.TempDir(), "other.db")
				storeWithCategory(t, other, "B")
				if err := os.Rename(other, path); err != nil {
					t.Fatal(err)
				}
			}
			t.Cleanup(func() { openStoreBeforeIdentity = nil })

			s, err := OpenStore(path, tc.opts...)
			if err != nil {
				if !errors.Is(err, ErrFileReplaced) {
					t.Fatalf("OpenStore = %v, want success or ErrFileReplaced", err)
				}
				return
			}
			if _, err := s.AppendCategory(ctx, "C", 0); err != nil {
				t.Fatalf("AppendCategory: %v", err)
			}
			if err := s.Close(); err != nil {
				t.Fatal(err)
			}
			if got := rootTitles(t, path); got != "B,C" {
				t.Fatalf("the file at the path holds %q, want \"B,C\": the write went to the replaced file", got)
			}
		})
	}
}

// TestOpenStoreFileKeepsBeingReplaced removes the file at every attempt:
// OpenStore must give up with ErrFileReplaced.
func TestOpenStoreFileKeepsBeingReplaced(t *testing.T) {
	path := filepath.Join(t.TempDir(), "t.db")
	storeWithCategory(t, path, "A")
	openStoreBeforeIdentity = func() {
		if err := os.Remove(path); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() { openStoreBeforeIdentity = nil })
	if s, err := OpenStore(path); !errors.Is(err, ErrFileReplaced) {
		if err == nil {
			_ = s.Close()
		}
		t.Fatalf("OpenStore = %v, want ErrFileReplaced", err)
	}
}

// storeWithCategory creates a store at path holding one category, title, and
// closes it.
func storeWithCategory(t *testing.T, path, title string) {
	t.Helper()
	s, err := OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.AppendCategory(context.Background(), title, 0); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
}

// rootTitles opens the store at path and returns its root titles, joined by
// commas in tree order.
func rootTitles(t *testing.T, path string) string {
	t.Helper()
	s, err := OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	doc, _, err := s.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var titles []string
	for _, r := range doc.Roots {
		titles = append(titles, r.Title)
	}
	return strings.Join(titles, ",")
}

func TestLoadRevisionMatchesTree(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)
	other, err := OpenStore(s.Path())
	if err != nil {
		t.Fatalf("OpenStore second handle: %v", err)
	}
	t.Cleanup(func() { _ = other.Close() })
	before, err := s.Revision(ctx)
	if err != nil {
		t.Fatal(err)
	}

	loadBetweenReads = func() {
		loadBetweenReads = nil
		if _, err := other.AppendCategory(ctx, "external", 0); err != nil {
			t.Errorf("external write: %v", err)
		}
	}
	t.Cleanup(func() { loadBetweenReads = nil })

	doc, rev, err := s.Load(ctx)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := len(doc.Roots); (rev == before) != (got == 0) {
		t.Fatalf("Load = %d roots at revision %d (before the external write: %d); want the tree and revision from one snapshot", got, rev, before)
	}
}
