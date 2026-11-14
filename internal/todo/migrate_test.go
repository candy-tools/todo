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

	bak := BackupPath(path)
	if bak != path+".bak" {
		t.Fatalf("BackupPath = %q, want %q", bak, path+".bak")
	}
	if ok, err := StoreExists(bak); err != nil || !ok {
		t.Fatalf("backup %s missing (err=%v)", bak, err)
	}
	// A plain handle, not OpenStore, which would migrate the backup too.
	db, err := gormdb.Open(gormdb.Cfg{
		Driver: gormdb.DriverSqlite,
		Sqlite: gormdb.SqliteCfg{Path: bak},
		Logger: logger.Discard,
	}, gormdb.Drivers{Sqlite: sqlite.Driver{}})
	if err != nil {
		t.Fatalf("open backup: %v", err)
	}
	defer func() { _ = gormdb.Close(db) }()
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

func TestMigrateKeepsExistingBackup(t *testing.T) {
	path := seedV1(t)
	old := []byte("an older backup")
	if err := os.WriteFile(BackupPath(path), old, 0o600); err != nil {
		t.Fatal(err)
	}
	s := openStore(t, path)
	if v := storedVersion(t, s.db); v != 2 {
		t.Errorf("schema_version = %d, want 2", v)
	}
	if got, err := os.ReadFile(BackupPath(path)); err != nil || !bytes.Equal(got, old) {
		t.Errorf("backup = %q (err=%v), want it unchanged: %q", got, err, old)
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

// TestUpgradeToV2SkipsUpgradedStore covers a migration that lost the race: the
// store was upgraded between its version check and its write, so the write
// changes nothing and does not bump the revision.
func TestUpgradeToV2SkipsUpgradedStore(t *testing.T) {
	s := openStore(t, seedV1(t))
	want := snapshot(load(t, s))
	if err := s.upgradeToV2(context.Background()); err != nil {
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
	if _, err := os.Stat(BackupPath(path)); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("new store has a backup (stat err=%v), want none", err)
	}
}
