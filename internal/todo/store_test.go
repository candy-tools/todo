package todo

import (
	"context"
	"path/filepath"
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

func TestOpenStoreRejectsDirectoryPath(t *testing.T) {
	if s, err := OpenStore(t.TempDir()); err == nil {
		_ = s.Close()
		t.Fatal("OpenStore on a directory succeeded; want an error")
	}
}
