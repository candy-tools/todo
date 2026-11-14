package todo

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
)

// TestImportFileCleansUpNewDBOnImportFailure fails the import after OpenStore
// created the db (cancelled context) and expects the db files to be removed.
func TestImportFileCleansUpNewDBOnImportFailure(t *testing.T) {
	dir := t.TempDir()
	md := filepath.Join(dir, "TODO.md")
	src := []byte("# A\n\n- [ ] one\n")
	if err := os.WriteFile(md, src, 0o600); err != nil {
		t.Fatal(err)
	}
	db := filepath.Join(dir, ".todo.db")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if s, _, err := importFile(ctx, db, md, false); err == nil {
		_ = s.Close()
		t.Fatal("importFile succeeded with a cancelled context")
	}
	for _, p := range []string{db, db + "-wal", db + "-shm"} {
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Errorf("%s exists after failed import (err=%v)", p, err)
		}
	}
	if got, _ := os.ReadFile(md); !bytes.Equal(got, src) {
		t.Fatal("markdown file was modified")
	}
}

// TestImportFileKeepsPreexistingDBOnFailure checks the cleanup spares a db that
// existed before the call.
func TestImportFileKeepsPreexistingDBOnFailure(t *testing.T) {
	dir := t.TempDir()
	md := filepath.Join(dir, "TODO.md")
	if err := os.WriteFile(md, []byte("# A\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	db := filepath.Join(dir, ".todo.db")
	s, err := OpenStore(db)
	if err != nil {
		t.Fatal(err)
	}
	_ = s.Close()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if s, _, err := importFile(ctx, db, md, false); err == nil {
		_ = s.Close()
		t.Fatal("importFile succeeded with a cancelled context")
	}
	if _, err := os.Stat(db); err != nil {
		t.Fatalf("preexisting db removed: %v", err)
	}
}
