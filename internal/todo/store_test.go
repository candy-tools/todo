package todo

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
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
}

func TestOpenStoreRejectsDirectoryPath(t *testing.T) {
	if s, err := OpenStore(t.TempDir()); err == nil {
		_ = s.Close()
		t.Fatal("OpenStore on a directory succeeded; want an error")
	}
}
