package todo

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"

	closuretree "github.com/go-bumbu/closure-tree"
	"github.com/go-bumbu/db/gormdb"
	"github.com/go-bumbu/db/gormdb/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// v1Node is a row of a v1 fixture with the rows below it.
type v1Node struct {
	kind     Kind
	title    string
	status   Status
	children []v1Node
}

func v1Cat(title string, children ...v1Node) v1Node {
	return v1Node{kind: Category, title: title, children: children}
}

func v1Task(title string, st Status, children ...v1Node) v1Node {
	return v1Node{kind: Task, title: title, status: st, children: children}
}

// seedV1With builds a schema-1 store holding roots and returns its path. The
// rows go in through addRow, past every rule a write checks, so subtasks can
// nest at any depth and hold any state, as v1 allowed. The plan column, which
// v1 did not have, is dropped, and the meta row records schema version 1.
func seedV1With(t *testing.T, roots ...v1Node) string {
	t.Helper()
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), ".todo.db")
	s, err := OpenStore(path)
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	tree, err := closuretree.New(s.db, dbItem{})
	if err != nil {
		t.Fatal(err)
	}
	var add func(nodes []v1Node, parentID uint)
	add = func(nodes []v1Node, parentID uint) {
		var afterID uint
		for _, n := range nodes {
			row := &dbItem{Kind: int(n.kind), Title: n.title, Status: int(n.status)}
			if err := addRow(ctx, tree, row, parentID, afterID); err != nil {
				t.Fatalf("add %s: %v", n.title, err)
			}
			afterID = row.NodeId
			add(n.children, row.NodeId)
		}
	}
	add(roots, 0)
	if err := s.db.Migrator().DropColumn(&dbItem{}, "plan"); err != nil {
		t.Fatalf("drop plan: %v", err)
	}
	if err := s.db.Model(&dbMeta{}).Where("id = ?", 1).Update("schema_version", 1).Error; err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	return path
}

// seedV1 builds the standard v1 fixture: category C holding task T (Working),
// which holds a (Review) → a1 (Deferred) → a1x (Done), then b (Open).
func seedV1(t *testing.T) string {
	t.Helper()
	return seedV1With(t, v1Cat("C",
		v1Task("T", Working,
			v1Task("a", Review,
				v1Task("a1", Deferred,
					v1Task("a1x", Done))),
			v1Task("b", Open))))
}

// openStore opens the store at path, closed with the test.
func openStore(t *testing.T, path string) *Store {
	t.Helper()
	s, err := OpenStore(path)
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

// byTitle returns the single item of doc titled title.
func byTitle(t *testing.T, doc *Document, title string) *Item {
	t.Helper()
	found := doc.ByTitle(title)
	if len(found) != 1 {
		t.Fatalf("%d items titled %q, want 1", len(found), title)
	}
	return found[0]
}

// storedVersion reads the schema version from the meta row through db.
func storedVersion(t *testing.T, db *gorm.DB) int {
	t.Helper()
	var m dbMeta
	if err := db.First(&m, 1).Error; err != nil {
		t.Fatalf("read meta: %v", err)
	}
	return m.SchemaVersion
}

// snapshot renders doc one item per line, indented by depth, with kind, status
// and parent, so two loads of the same tree compare equal.
func snapshot(doc *Document) string {
	var b strings.Builder
	var walk func(items []*Item, depth int)
	walk = func(items []*Item, depth int) {
		for _, it := range items {
			fmt.Fprintf(&b, "%s%d %s %v\n", strings.Repeat("  ", depth), it.Kind, it.Title, it.Status)
			walk(it.Children, depth+1)
		}
	}
	walk(doc.Roots, 0)
	return b.String()
}

func TestMigrateV1FlattensAndNormalizes(t *testing.T) {
	s := openStore(t, seedV1(t))
	doc := load(t, s)

	T := byTitle(t, doc, "T")
	if got := titles(T.Children); !slices.Equal(got, []string{"a", "a1", "a1x", "b"}) {
		t.Errorf("T's children = %v, want [a a1 a1x b]", got)
	}
	for _, c := range T.Children {
		if len(c.Children) != 0 {
			t.Errorf("%s has %d children, want none", c.Title, len(c.Children))
		}
	}
	want := map[string]Status{"T": Working, "a": Open, "a1": Open, "a1x": Done, "b": Open}
	for title, st := range want {
		if got := byTitle(t, doc, title).Status; got != st {
			t.Errorf("%s status = %v, want %v", title, got, st)
		}
	}
	if v := storedVersion(t, s.db); v != 2 {
		t.Errorf("schema_version = %d, want 2", v)
	}
	if !s.db.Migrator().HasColumn(&dbItem{}, "plan") {
		t.Error("items has no plan column after the migration")
	}
}

// TestMigrateKeepsSiblingAfterDeepBranch covers a sibling that follows a deep
// branch: a2 comes after a1's child a1x, so it must land after a1x, which the
// migration has itself just moved.
func TestMigrateKeepsSiblingAfterDeepBranch(t *testing.T) {
	want := []string{"a", "a1", "a1x", "a2"}

	doc, _, err := ImportMarkdown([]byte("# C\n- [ ] T\n  - [ ] a\n    - [ ] a1\n      - [ ] a1x\n    - [ ] a2\n"))
	if err != nil {
		t.Fatal(err)
	}
	doc.FlattenSubtasks()
	if got := titles(byTitle(t, doc, "T").Children); !slices.Equal(got, want) {
		t.Fatalf("in memory: T's children = %v, want %v", got, want)
	}

	path := seedV1With(t, v1Cat("C",
		v1Task("T", Open,
			v1Task("a", Open,
				v1Task("a1", Open,
					v1Task("a1x", Open)),
				v1Task("a2", Open)))))
	s, err := OpenStore(path)
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	if got := titles(byTitle(t, load(t, s), "T").Children); !slices.Equal(got, want) {
		t.Errorf("after migration: T's children = %v, want %v", got, want)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}

	s = openStore(t, path)
	if got := titles(byTitle(t, load(t, s), "T").Children); !slices.Equal(got, want) {
		t.Errorf("after reload: T's children = %v, want %v", got, want)
	}
}

func TestMigrateV1WritesBackup(t *testing.T) {
	path := seedV1(t)
	s := openStore(t, path)
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}

	bak := BackupPath(path, 0)
	if bak != path+".bak" {
		t.Fatalf("BackupPath(path, 0) = %q, want %q", bak, path+".bak")
	}
	if got, want := BackupPath(path, 2), path+".bak.2"; got != want {
		t.Fatalf("BackupPath(path, 2) = %q, want %q", got, want)
	}
	if _, err := os.Stat(BackupPath(path, 1)); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("a first backup wrote .bak.1 too (stat err=%v)", err)
	}
	db := openPlain(t, bak)
	if v := storedVersion(t, db); v != 1 {
		t.Errorf("backup schema_version = %d, want 1", v)
	}
	if db.Migrator().HasColumn(&dbItem{}, "plan") {
		t.Error("backup has the plan column; want it taken before the schema changed")
	}
	var tasks int64
	if err := db.Model(&dbItem{}).Where("kind = ?", int(Task)).Count(&tasks).Error; err != nil {
		t.Fatal(err)
	}
	if tasks != 5 {
		t.Errorf("backup holds %d tasks, want 5", tasks)
	}
	// The closure row from the virtual root (ancestor 0) holds a node's level:
	// a subtask of a top-level task under a root category is at level 3.
	var deep int64
	if err := db.Table("closure_tree_rel_items").Where("ancestor_id = 0 AND depth > 3").Count(&deep).Error; err != nil {
		t.Fatal(err)
	}
	if deep != 2 {
		t.Errorf("backup holds %d items below a subtask, want 2 (a1, a1x): the v1 tree as it was", deep)
	}
	if left, _ := filepath.Glob(filepath.Join(filepath.Dir(path), ".todo-backup-*")); len(left) > 0 {
		t.Errorf("temporary backup files left behind: %v", left)
	}
}

// openPlain opens the store file at path through a plain handle, not
// OpenStore, which would migrate it. It closes with the test.
func openPlain(t *testing.T, path string) *gorm.DB {
	t.Helper()
	if ok, err := StoreExists(path); err != nil || !ok {
		t.Fatalf("store %s missing (err=%v)", path, err)
	}
	db, err := gormdb.Open(gormdb.Cfg{
		Driver: gormdb.DriverSqlite,
		Sqlite: gormdb.SqliteCfg{Path: path},
		Logger: logger.Discard,
	}, gormdb.Drivers{Sqlite: sqlite.Driver{}})
	if err != nil {
		t.Fatalf("open backup: %v", err)
	}
	t.Cleanup(func() { _ = gormdb.Close(db) })
	return db
}

// TestMigrateKeepsExistingBackup covers a store whose .bak (and .bak.1) already
// exist: they stay untouched, and the v1 store is backed up to the first free
// numbered name instead.
func TestMigrateKeepsExistingBackup(t *testing.T) {
	path := seedV1(t)
	old := map[int][]byte{0: []byte("an older backup"), 1: []byte("another older backup")}
	for n, data := range old {
		if err := os.WriteFile(BackupPath(path, n), data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	s := openStore(t, path)
	if v := storedVersion(t, s.db); v != 2 {
		t.Errorf("schema_version = %d, want 2", v)
	}
	for n, data := range old {
		if got, err := os.ReadFile(BackupPath(path, n)); err != nil || !bytes.Equal(got, data) {
			t.Errorf("backup %d = %q (err=%v), want it unchanged: %q", n, got, err, data)
		}
	}
	if v := storedVersion(t, openPlain(t, BackupPath(path, 2))); v != 1 {
		t.Errorf(".bak.2 schema_version = %d, want 1", v)
	}
}

// dumpTables renders every row of the items, closure and meta tables through
// db, so two states of one file compare equal only when nothing changed.
func dumpTables(t *testing.T, db *gorm.DB) string {
	t.Helper()
	var b strings.Builder
	for _, table := range []string{"items", "closure_tree_rel_items", "meta"} {
		var rows []map[string]any
		if err := db.Table(table).Order("rowid").Find(&rows).Error; err != nil {
			t.Fatalf("read %s: %v", table, err)
		}
		fmt.Fprintf(&b, "%s: %v\n", table, rows)
	}
	return b.String()
}

// TestMigrateFailureRollsBack makes the upgrade's last step fail, with a
// trigger refusing the schema_version update, after the moves and status
// writes ran: the open fails, and the file keeps its v1 data and schema, the
// plan column not added.
func TestMigrateFailureRollsBack(t *testing.T) {
	path := seedV1(t)
	db := openPlain(t, path)
	const trigger = "CREATE TRIGGER fail_upgrade BEFORE UPDATE OF schema_version ON meta " +
		"BEGIN SELECT RAISE(ABORT, 'injected upgrade failure'); END"
	if err := db.Exec(trigger).Error; err != nil {
		t.Fatal(err)
	}
	before := dumpTables(t, db)

	s, err := OpenStore(path)
	if err == nil {
		_ = s.Close()
		t.Fatal("OpenStore succeeded, want the injected upgrade failure")
	}
	if !strings.Contains(err.Error(), "injected upgrade failure") {
		t.Fatalf("OpenStore error = %v, want the injected upgrade failure", err)
	}
	if v := storedVersion(t, db); v != 1 {
		t.Errorf("schema_version = %d, want 1", v)
	}
	if db.Migrator().HasColumn(&dbItem{}, "plan") {
		t.Error("the plan column survived the failed upgrade")
	}
	if got := dumpTables(t, db); got != before {
		t.Errorf("the failed upgrade changed the store:\n%s\nwant\n%s", got, before)
	}
}

func TestMigrateRunsOnce(t *testing.T) {
	path := seedV1(t)

	first := openStore(t, path)
	second := openStore(t, path) // opened before the first closes
	want := snapshot(load(t, first))
	for i, s := range []*Store{first, second} {
		if got := snapshot(load(t, s)); got != want {
			t.Errorf("handle %d sees\n%s\nwant\n%s", i+1, got, want)
		}
		if rev, err := s.Revision(context.Background()); err != nil || rev != 1 {
			t.Errorf("handle %d: revision = %d (err=%v), want 1", i+1, rev, err)
		}
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	if err := second.Close(); err != nil {
		t.Fatal(err)
	}

	again := openStore(t, path)
	if got := snapshot(load(t, again)); got != want {
		t.Errorf("reopened store sees\n%s\nwant\n%s", got, want)
	}
	if rev, err := again.Revision(context.Background()); err != nil || rev != 1 {
		t.Errorf("reopened: revision = %d (err=%v), want 1", rev, err)
	}
}

// TestMigrateParallelOpens opens one file from several handles released at the
// same moment, as agents running the CLI side by side do: every open succeeds,
// and the store is created or upgraded exactly once.
func TestMigrateParallelOpens(t *testing.T) {
	const handles, iterations = 4, 8
	cases := []struct {
		name    string
		path    func(t *testing.T) string
		wantRev int64
	}{
		{"v1", seedV1, 1},
		{"new", func(t *testing.T) string { return filepath.Join(t.TempDir(), ".todo.db") }, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for range iterations {
				path := tc.path(t)
				stores := make([]*Store, handles)
				errs := make([]error, handles)
				start := make(chan struct{})
				var wg sync.WaitGroup
				for i := range handles {
					wg.Add(1)
					go func() {
						defer wg.Done()
						<-start
						stores[i], errs[i] = OpenStore(path)
					}()
				}
				close(start)
				wg.Wait()
				for i, s := range stores {
					if s != nil {
						t.Cleanup(func() { _ = s.Close() })
					}
					if errs[i] != nil {
						t.Fatalf("handle %d: OpenStore: %v", i+1, errs[i])
					}
				}
				want := snapshot(load(t, stores[0]))
				for i, s := range stores {
					if got := snapshot(load(t, s)); got != want {
						t.Errorf("handle %d sees\n%s\nwant\n%s", i+1, got, want)
					}
					if rev, err := s.Revision(context.Background()); err != nil || rev != tc.wantRev {
						t.Errorf("handle %d: revision = %d (err=%v), want %d", i+1, rev, err, tc.wantRev)
					}
				}
				if v := storedVersion(t, stores[0].db); v != 2 {
					t.Errorf("schema_version = %d, want 2", v)
				}
			}
		})
	}
}

// TestUpgradeToV2SkipsUpgradedStore covers an upgrade run on a store that is
// already on schema 2: it changes nothing and does not bump the revision.
func TestUpgradeToV2SkipsUpgradedStore(t *testing.T) {
	s := openStore(t, seedV1(t))
	want := snapshot(load(t, s))
	err := s.db.Transaction(func(tx *gorm.DB) error {
		return upgradeToV2(context.Background(), tx)
	})
	if err != nil {
		t.Fatalf("upgradeToV2 on a v2 store: %v", err)
	}
	if got := snapshot(load(t, s)); got != want {
		t.Errorf("tree changed:\n%s\nwant\n%s", got, want)
	}
	if rev, err := s.Revision(context.Background()); err != nil || rev != 1 {
		t.Errorf("revision = %d (err=%v), want 1", rev, err)
	}
}

func TestNewStoreIsV2(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".todo.db")
	s := openStore(t, path)
	if v := storedVersion(t, s.db); v != 2 {
		t.Errorf("schema_version = %d, want 2", v)
	}
	if _, err := os.Stat(BackupPath(path, 0)); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("new store has a backup (stat err=%v), want none", err)
	}
}
