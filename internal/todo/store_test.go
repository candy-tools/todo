package todo

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// newStore returns a store on a missing .todo.json in a temporary directory.
func newStore(t *testing.T) *Store {
	t.Helper()
	return NewStore(filepath.Join(t.TempDir(), ".todo.json"))
}

// fileBytes returns the content of path, nil when it does not exist.
func fileBytes(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestLoadMissingFile(t *testing.T) {
	s := newStore(t)
	doc, fp, err := s.Load(context.Background())
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if doc == nil || len(doc.Roots) != 0 || !fp.IsMissing() {
		t.Fatalf("Load = %v, %x; want an empty document and a missing fingerprint", doc, fp)
	}
	for _, p := range []string{s.Path(), s.Path() + ".lock"} {
		if _, err := os.Stat(p); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("%s after Load: stat err = %v, want it not created", filepath.Base(p), err)
		}
	}
}

func TestStoreWriteCreatesFile(t *testing.T) {
	s := newStore(t)
	id, err := s.AppendCategory(context.Background(), "A", "")
	if err != nil {
		t.Fatalf("AppendCategory: %v", err)
	}
	for _, p := range []string{s.Path(), s.Path() + ".lock"} {
		if _, err := os.Stat(p); err != nil {
			t.Errorf("%s after the first write: %v", filepath.Base(p), err)
		}
	}
	doc := load(t, s)
	if len(doc.Roots) != 1 || doc.Roots[0].ID != id || doc.Roots[0].Title != "A" || !validID(id) {
		t.Fatalf("roots = %v (id %q); want category A with a valid ID", titles(doc.Roots), id)
	}
}

func TestStoreLastWrite(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)
	c := mustID(t)(s.AppendCategory(ctx, "C", ""))
	task := mustID(t)(s.AppendTask(ctx, Fields{Title: "t"}, Open, c))

	lw := s.LastWrite()
	if lw.Doc == nil {
		t.Fatal("LastWrite().Doc is nil after AppendTask")
	}
	if got := item(t, lw.Doc, task); got.Title != "t" || got.Parent == nil || got.Parent.ID != c {
		t.Fatalf("LastWrite().Doc task = %+v; want t under C", got)
	}
	_, fp, err := s.Load(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if lw.Wrote != fp || lw.Read == lw.Wrote {
		t.Fatalf("LastWrite Read %x, Wrote %x; want Wrote = Load's %x and Read different", lw.Read, lw.Wrote, fp)
	}

	before, err := os.Stat(s.Path())
	if err != nil {
		t.Fatal(err)
	}
	mustOK(t, s.Edit(ctx, task, Changes{Title: new("t")}))
	lw = s.LastWrite()
	if lw.Read != lw.Wrote || lw.Wrote != fp {
		t.Fatalf("no-op Edit: LastWrite Read %x, Wrote %x; want both %x", lw.Read, lw.Wrote, fp)
	}
	if got := item(t, lw.Doc, task); got.Title != "t" {
		t.Fatalf("no-op Edit: LastWrite().Doc task = %+v", got)
	}
	after, err := os.Stat(s.Path())
	if err != nil {
		t.Fatal(err)
	}
	if !after.ModTime().Equal(before.ModTime()) || !os.SameFile(before, after) {
		t.Fatal("a no-op Edit rewrote the file")
	}
}

// A failed write keeps the last successful write's result.
func TestStoreLastWriteKeptOnError(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)
	mustID(t)(s.AppendCategory(ctx, "C", ""))
	lw := s.LastWrite()
	if err := s.Delete(ctx, "zzzzzzz"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Delete(missing) = %v, want ErrNotFound", err)
	}
	if got := s.LastWrite(); got.Wrote != lw.Wrote || got.Doc != lw.Doc {
		t.Fatal("a failed write replaced LastWrite")
	}
}

// Another store that loaded before a change still writes against the file as
// it is now: a task it names that was deleted is not found, and an anchor that
// is gone is a conflict.
func TestStoreStaleCallerOps(t *testing.T) {
	ctx := context.Background()
	a := newStore(t)
	cat := mustID(t)(a.AppendCategory(ctx, "C", ""))
	x := mustID(t)(a.AppendTask(ctx, Fields{Title: "x"}, Open, cat))
	tk := mustID(t)(a.AppendTask(ctx, Fields{Title: "t"}, Open, cat))
	if _, _, err := a.Load(ctx); err != nil {
		t.Fatal(err)
	}
	b := NewStore(a.Path())
	mustOK(t, b.Delete(ctx, tk))

	want := fileBytes(t, a.Path())
	if err := a.Complete(ctx, tk, false); !errors.Is(err, ErrNotFound) {
		t.Fatalf("stale Complete = %v, want ErrNotFound", err)
	}
	if err := a.Move(ctx, x, cat, tk); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale Move = %v, want ErrConflict", err)
	}
	if got := fileBytes(t, a.Path()); !bytes.Equal(got, want) {
		t.Fatalf("file changed by refused writes:\n%s\nwant:\n%s", got, want)
	}
}

func TestStorePruneSeesFreshFile(t *testing.T) {
	ctx := context.Background()
	a := newStore(t)
	cat := mustID(t)(a.AppendCategory(ctx, "C", ""))
	done := mustID(t)(a.AppendTask(ctx, Fields{Title: "done"}, Done, cat))
	if got := item(t, load(t, a), done); got.Status != Done {
		t.Fatalf("done = %v", got.Status)
	}
	b := NewStore(a.Path())
	sub := mustID(t)(b.AppendTask(ctx, Fields{Title: "sub"}, Open, done))

	n, err := a.Prune(ctx)
	if err != nil || n != 0 {
		t.Fatalf("Prune = %d, %v; want 0, nil", n, err)
	}
	doc := load(t, a)
	item(t, doc, done)
	item(t, doc, sub)
}

// invalidFiles are list files every write must refuse with ErrInvalidList.
var invalidFiles = map[string]string{
	"broken reference": `{"version": 3, "categories": [{"id": "aaaaaaa", "title": "C"}],
		"tasks": [{"id": "bbbbbbb", "category": "ccccccc", "title": "t", "status": "open"}]}`,
	"syntax error": `{"version": 3, "categories": [`,
	"wrong type":   `{"version": "3", "categories": [], "tasks": []}`,
}

func TestStoreRefusesInvalidFile(t *testing.T) {
	ctx := context.Background()
	for name, content := range invalidFiles {
		t.Run(name, func(t *testing.T) {
			s := newStore(t)
			if err := os.WriteFile(s.Path(), []byte(content), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, _, err := s.Load(ctx); !errors.Is(err, ErrInvalidList) || !strings.Contains(err.Error(), s.Path()) {
				t.Fatalf("Load = %v; want ErrInvalidList naming the file", err)
			}
			for wname, w := range everyWrite(s, "aaaaaaa", "bbbbbbb") {
				err := w()
				if !errors.Is(err, ErrInvalidList) {
					t.Errorf("%s = %v, want ErrInvalidList", wname, err)
				} else if !strings.Contains(err.Error(), s.Path()) {
					t.Errorf("%s = %v, want the file path in the message", wname, err)
				}
				if got := string(fileBytes(t, s.Path())); got != content {
					t.Fatalf("%s changed the file to:\n%s", wname, got)
				}
			}
		})
	}
}

// TestStoreRefusedValueIsNotAnInvalidFile makes a write produce a list the
// format refuses: the file on disk is fine, so the error must not call it an
// invalid list file, and nothing is written.
func TestStoreRefusedValueIsNotAnInvalidFile(t *testing.T) {
	s := newStore(t)
	doc := &Document{}
	doc.AppendRoot(&Item{ID: "not-an-id", Kind: Category, Title: "New"})
	err := s.Import(context.Background(), doc, false)
	if err == nil {
		t.Fatal("Import of a list with a malformed ID returned nil error")
	}
	const prefix = "refused to write an invalid list: "
	if msg := err.Error(); !strings.HasPrefix(msg, prefix) || strings.Contains(msg, ErrInvalidList.Error()) {
		t.Errorf("err = %q, want it to start with %q and not say %q", msg, prefix, ErrInvalidList)
	}
	if b := fileBytes(t, s.Path()); b != nil {
		t.Errorf("the refused write created the file:\n%s", b)
	}
}

// everyWrite returns one call of every write method, on category cat and task
// task.
func everyWrite(s *Store, cat, task string) map[string]func() error {
	ctx := context.Background()
	return map[string]func() error{
		"AddCategory":    func() error { _, err := s.AddCategory(ctx, "X", "", ""); return err },
		"AppendCategory": func() error { _, err := s.AppendCategory(ctx, "X", ""); return err },
		"AddTask":        func() error { _, err := s.AddTask(ctx, Fields{Title: "x"}, Open, cat, ""); return err },
		"AppendTask":     func() error { _, err := s.AppendTask(ctx, Fields{Title: "x"}, Open, cat); return err },
		"Edit":           func() error { return s.Edit(ctx, task, Changes{Title: new("y")}) },
		"SetStatuses":    func() error { return s.SetStatuses(ctx, map[string]Status{task: Done}) },
		"CascadeStatus":  func() error { return s.CascadeStatus(ctx, task, Done) },
		"Complete":       func() error { return s.Complete(ctx, task, false) },
		"Reopen":         func() error { return s.Reopen(ctx, task, Working, ReopenOptions{}) },
		"Move":           func() error { return s.Move(ctx, task, cat, "") },
		"Delete":         func() error { return s.Delete(ctx, task) },
		"Prune":          func() error { _, err := s.Prune(ctx); return err },
		"Import":         func() error { return s.Import(ctx, &Document{}, true) },
	}
}

func TestStoreNewerVersion(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)
	content := `{"version": 4, "categories": [], "tasks": [], "tags": []}`
	if err := os.WriteFile(s.Path(), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	_, _, err := s.Load(ctx)
	if !errors.Is(err, ErrNewerSchema) {
		t.Fatalf("Load = %v, want ErrNewerSchema", err)
	}
	if !strings.Contains(err.Error(), "list was written by a newer todo") {
		t.Errorf("Load error %q does not say the list is from a newer todo", err)
	}
	for name, w := range everyWrite(s, "aaaaaaa", "bbbbbbb") {
		if err := w(); !errors.Is(err, ErrNewerSchema) {
			t.Errorf("%s = %v, want ErrNewerSchema", name, err)
		}
	}
	if got := string(fileBytes(t, s.Path())); got != content {
		t.Fatalf("the newer file was changed to:\n%s", got)
	}
}

func TestStoreChanged(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)
	if changed, fp, err := s.Changed(ctx, Fingerprint{}); err != nil || changed || !fp.IsMissing() {
		t.Fatalf("Changed(missing) on a missing file = %v, %x, %v; want false, missing", changed, fp, err)
	}
	mustID(t)(s.AppendCategory(ctx, "A", ""))
	changed, fp, err := s.Changed(ctx, Fingerprint{})
	if err != nil || !changed || fp != s.LastWrite().Wrote {
		t.Fatalf("Changed after a write = %v, %x, %v; want true and the written fingerprint", changed, fp, err)
	}
	if changed, _, err := s.Changed(ctx, fp); err != nil || changed {
		t.Fatalf("Changed(current) = %v, %v; want false", changed, err)
	}
}

func TestStoreDirectoryPathFails(t *testing.T) {
	s := NewStore(t.TempDir())
	if _, _, err := s.Load(context.Background()); err == nil {
		t.Fatal("Load of a directory succeeded; want an error")
	}
	if _, err := s.AppendCategory(context.Background(), "A", ""); err == nil {
		t.Fatal("a write to a directory succeeded; want an error")
	}
}
