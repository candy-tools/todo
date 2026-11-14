package todo

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// TestImportFileCleansUpNewDBOnImportFailure fails the import after the
// temporary db was created (cancelled context) and expects neither the target
// nor any temporary file to be left behind.
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
	assertNoTempFiles(t, dir)
	if got, _ := os.ReadFile(md); !bytes.Equal(got, src) {
		t.Fatal("markdown file was modified")
	}
}

// TestImportFileLosingFirstRunRaceKeepsWinnersDB stands in for two processes
// that both saw no store on first run: the first imports, then the second
// (whose existence check ran before the first created the store) imports too.
// The second must neither fail nor remove or replace the first's data.
func TestImportFileLosingFirstRunRaceKeepsWinnersDB(t *testing.T) {
	dir := t.TempDir()
	md := filepath.Join(dir, "TODO.md")
	if err := os.WriteFile(md, []byte("# A\n\n- [ ] one\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	db := filepath.Join(dir, ".todo.db")
	first, rep, err := importFile(context.Background(), db, md, false)
	if err != nil {
		t.Fatalf("first import: %v", err)
	}
	defer func() { _ = first.Close() }()
	if rep.Tasks != 1 || rep.Skipped {
		t.Fatalf("first report = %+v; want 1 task imported", rep)
	}
	id, err := first.AddTask(context.Background(), "winner's write", "", Open, mustRootID(t, first), 0)
	if err != nil || id == 0 {
		t.Fatalf("first store write: %v", err)
	}

	storeExists = func(string) (bool, error) { return false, nil }
	t.Cleanup(func() { storeExists = StoreExists })
	second, rep, err := importFile(context.Background(), db, md, false)
	if err != nil {
		t.Fatalf("second import must not fail: %v", err)
	}
	defer func() { _ = second.Close() }()
	if !rep.Skipped || rep.Tasks != 0 || rep.Categories != 0 || len(rep.Warnings) != 0 {
		t.Errorf("second report = %+v; want an empty, skipped report", rep)
	}
	for _, s := range []*Store{first, second} {
		doc, _, err := s.Load(context.Background())
		if err != nil {
			t.Fatalf("load: %v", err)
		}
		if len(doc.Roots) != 1 || len(doc.Roots[0].Children) != 2 {
			t.Errorf("%s: the winner's data must be intact, got %+v", s.Path(), doc.Roots)
		}
	}
	assertNoTempFiles(t, dir)
}

// TestImportFileNewDBMode checks a store created by an import gets the same
// 0644 permissions as one SQLite creates directly, not os.CreateTemp's 0600.
func TestImportFileNewDBMode(t *testing.T) {
	dir := t.TempDir()
	md := filepath.Join(dir, "TODO.md")
	if err := os.WriteFile(md, []byte("# A\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	db := filepath.Join(dir, ".todo.db")
	s, _, err := importFile(context.Background(), db, md, false)
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	defer func() { _ = s.Close() }()
	fi, err := os.Stat(db)
	if err != nil {
		t.Fatal(err)
	}
	if got := fi.Mode().Perm(); got != 0o644 {
		t.Errorf("mode = %v; want -rw-r--r--", got)
	}
}

// failLink stands in for a filesystem without hard links for the test.
func failLink(t *testing.T) {
	t.Helper()
	linkFile = func(string, string) error { return errors.New("hard links not supported") }
	t.Cleanup(func() { linkFile = os.Link })
}

// TestImportFileFallsBackToRename covers a filesystem without hard links: the
// temporary store is renamed into place instead.
func TestImportFileFallsBackToRename(t *testing.T) {
	failLink(t)
	dir := t.TempDir()
	md := filepath.Join(dir, "TODO.md")
	if err := os.WriteFile(md, []byte("# A\n\n- [ ] one\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	db := filepath.Join(dir, ".todo.db")
	s, rep, err := importFile(context.Background(), db, md, false)
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	defer func() { _ = s.Close() }()
	if rep.Skipped || rep.Tasks != 1 {
		t.Errorf("report = %+v; want 1 task imported", rep)
	}
	doc, _, err := s.Load(context.Background())
	if err != nil || len(doc.Roots) != 1 || len(doc.Roots[0].Children) != 1 {
		t.Fatalf("imported store = %+v, %v; want A with one task", doc, err)
	}
	if fi, err := os.Stat(db); err != nil || fi.Mode().Perm() != 0o644 {
		t.Errorf("renamed store: stat %v, err %v; want mode 0644", fi, err)
	}
	assertNoTempFiles(t, dir)
}

// TestImportFileRenameFallbackKeepsWinnersDB has the link fail for a reason
// other than an existing target while another process already created the
// target: the fallback must not rename over it but open it, Skipped.
func TestImportFileRenameFallbackKeepsWinnersDB(t *testing.T) {
	dir := t.TempDir()
	md := filepath.Join(dir, "TODO.md")
	if err := os.WriteFile(md, []byte("# A\n\n- [ ] one\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	db := filepath.Join(dir, ".todo.db")
	first, _, err := importFile(context.Background(), db, md, false)
	if err != nil {
		t.Fatalf("first import: %v", err)
	}
	defer func() { _ = first.Close() }()
	if _, err := first.AddTask(context.Background(), "winner's write", "", Open, mustRootID(t, first), 0); err != nil {
		t.Fatal(err)
	}

	failLink(t)
	storeExists = func(string) (bool, error) { return false, nil }
	t.Cleanup(func() { storeExists = StoreExists })
	second, rep, err := importFile(context.Background(), db, md, false)
	if err != nil {
		t.Fatalf("second import must not fail: %v", err)
	}
	defer func() { _ = second.Close() }()
	if !rep.Skipped {
		t.Errorf("report = %+v; want Skipped", rep)
	}
	doc, _, err := second.Load(context.Background())
	if err != nil || len(doc.Roots) != 1 || len(doc.Roots[0].Children) != 2 {
		t.Errorf("the winner's data must be intact, got %+v, %v", doc, err)
	}
	assertNoTempFiles(t, dir)
}

// mustRootID returns the ID of the first root item of s.
func mustRootID(t *testing.T, s *Store) uint {
	t.Helper()
	doc, _, err := s.Load(context.Background())
	if err != nil || len(doc.Roots) == 0 {
		t.Fatalf("load: %v (roots %d)", err, len(doc.Roots))
	}
	return doc.Roots[0].ID
}

// assertNoTempFiles fails when dir holds a leftover temporary import db.
func assertNoTempFiles(t *testing.T, dir string) {
	t.Helper()
	left, err := filepath.Glob(filepath.Join(dir, ".todo-import-*"))
	if err != nil {
		t.Fatal(err)
	}
	if len(left) > 0 {
		t.Errorf("temporary import files left behind: %v", left)
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
	assertNoTempFiles(t, dir)
}
