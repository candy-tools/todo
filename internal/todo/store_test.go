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
	id, _, err := s.AppendCategory(context.Background(), "A", "")
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

func TestStoreWriteResult(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)
	c := mustID(t)(s.AppendCategory(ctx, "C", ""))
	task, res, err := s.AppendTask(ctx, Fields{Title: "t"}, Open, c)
	if err != nil {
		t.Fatal(err)
	}
	if res.Doc == nil {
		t.Fatal("AppendTask result Doc is nil")
	}
	if got := item(t, res.Doc, task); got.Title != "t" || got.Parent == nil || got.Parent.ID != c {
		t.Fatalf("result Doc task = %+v; want t under C", got)
	}
	_, fp, err := s.Load(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Written || res.Wrote != fp || res.Read == res.Wrote {
		t.Fatalf("result Written %v, Read %x, Wrote %x; want written, Wrote = Load's %x and Read different",
			res.Written, res.Read, res.Wrote, fp)
	}

	before, err := os.Stat(s.Path())
	if err != nil {
		t.Fatal(err)
	}
	res = mustOK(t)(s.Edit(ctx, task, Changes{Title: new("t")}))
	if res.Written || res.Read != res.Wrote || res.Wrote != fp {
		t.Fatalf("no-op Edit: Written %v, Read %x, Wrote %x; want not written, both %x", res.Written, res.Read, res.Wrote, fp)
	}
	if got := item(t, res.Doc, task); got.Title != "t" {
		t.Fatalf("no-op Edit: result Doc task = %+v", got)
	}
	after, err := os.Stat(s.Path())
	if err != nil {
		t.Fatal(err)
	}
	if !after.ModTime().Equal(before.ModTime()) || !os.SameFile(before, after) {
		t.Fatal("a no-op Edit rewrote the file")
	}
}

// Each write's Doc is the caller's own: two writes never share one, and
// changing it changes neither the file nor a later write's Doc.
func TestStoreWriteResultDocIsCallers(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)
	c := mustID(t)(s.AppendCategory(ctx, "C", ""))
	first := mustOK(t)(s.Edit(ctx, c, Changes{}))
	first.Doc.Roots[0].Title = "changed in memory"
	second := mustOK(t)(s.Edit(ctx, c, Changes{}))
	if second.Doc == first.Doc || second.Doc.Roots[0].Title != "C" {
		t.Fatalf("second Doc = %p (title %q), first %p; want a fresh copy titled C",
			second.Doc, second.Doc.Roots[0].Title, first.Doc)
	}
}

// A failed write returns the zero WriteResult.
func TestStoreFailedWriteResultIsZero(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)
	mustID(t)(s.AppendCategory(ctx, "C", ""))
	res, err := s.Delete(ctx, "zzzzzzz")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("Delete(missing) = %v, want ErrNotFound", err)
	}
	if res != (WriteResult{}) {
		t.Fatalf("failed write result = %+v, want the zero WriteResult", res)
	}
	id, res, err := s.AppendTask(ctx, Fields{Title: "x"}, Open, "zzzzzzz")
	if err == nil || id != "" || res != (WriteResult{}) {
		t.Fatalf("failed add = %q, %+v, %v; want \"\", the zero WriteResult and an error", id, res, err)
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
	mustOK(t)(b.Delete(ctx, tk))

	want := fileBytes(t, a.Path())
	if _, err := a.Complete(ctx, tk, false); !errors.Is(err, ErrNotFound) {
		t.Fatalf("stale Complete = %v, want ErrNotFound", err)
	}
	if _, err := a.Move(ctx, x, cat, tk); !errors.Is(err, ErrConflict) {
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

	n, _, err := a.Prune(ctx)
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
	_, err := s.Import(context.Background(), doc, false)
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

// A store from Existing refuses every write while the list file is missing,
// with ErrListMissing and without creating the file, even a write that would
// change nothing; once the file exists, its writes go through.
func TestStoreExistingRefusesMissingFile(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)
	for name, w := range everyWrite(s.Existing(), "aaaaaaa", "bbbbbbb") {
		if err := w(); !errors.Is(err, ErrListMissing) {
			t.Errorf("%s on a missing file = %v, want ErrListMissing", name, err)
		}
		if b := fileBytes(t, s.Path()); b != nil {
			t.Fatalf("%s created the list file:\n%s", name, b)
		}
	}
	if _, _, err := s.Existing().Prune(ctx); !errors.Is(err, ErrListMissing) {
		t.Errorf("Prune of nothing on a missing file = %v, want ErrListMissing", err)
	}

	c := mustID(t)(s.AppendCategory(ctx, "C", ""))
	id, res, err := s.Existing().AppendTask(ctx, Fields{Title: "t"}, Open, c)
	if err != nil || !res.Written {
		t.Fatalf("Existing AppendTask on an existing file = %q, %+v, %v; want it written", id, res, err)
	}
	item(t, load(t, s), id)
	if s.Existing().Path() != s.Path() {
		t.Errorf("Existing().Path() = %q, want %q", s.Existing().Path(), s.Path())
	}
}

// everyWrite returns one call of every write method, on category cat and task
// task.
func everyWrite(s *Store, cat, task string) map[string]func() error {
	ctx := context.Background()
	return map[string]func() error{
		"AddCategory":    func() error { _, _, err := s.AddCategory(ctx, "X", "", ""); return err },
		"AppendCategory": func() error { _, _, err := s.AppendCategory(ctx, "X", ""); return err },
		"AddTask":        func() error { _, _, err := s.AddTask(ctx, Fields{Title: "x"}, Open, cat, ""); return err },
		"AppendTask":     func() error { _, _, err := s.AppendTask(ctx, Fields{Title: "x"}, Open, cat); return err },
		"Edit":           func() error { return errOf(s.Edit(ctx, task, Changes{Title: new("y")})) },
		"SetStatuses":    func() error { return errOf(s.SetStatuses(ctx, map[string]Status{task: Done})) },
		"CascadeStatus":  func() error { return errOf(s.CascadeStatus(ctx, task, Done)) },
		"Complete":       func() error { return errOf(s.Complete(ctx, task, false)) },
		"Reopen":         func() error { return errOf(s.Reopen(ctx, task, Working, ReopenOptions{})) },
		"SetState":       func() error { return errOf(s.SetState(ctx, task, Working, StateOptions{})) },
		"Move":           func() error { return errOf(s.Move(ctx, task, cat, "")) },
		"Delete":         func() error { return errOf(s.Delete(ctx, task)) },
		"Prune":          func() error { _, _, err := s.Prune(ctx); return err },
		"Import":         func() error { return errOf(s.Import(ctx, &Document{}, true)) },
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
	_, res, err := s.AppendCategory(ctx, "A", "")
	if err != nil {
		t.Fatal(err)
	}
	changed, fp, err := s.Changed(ctx, Fingerprint{})
	if err != nil || !changed || fp != res.Wrote {
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
	if _, _, err := s.AppendCategory(context.Background(), "A", ""); err == nil {
		t.Fatal("a write to a directory succeeded; want an error")
	}
}
