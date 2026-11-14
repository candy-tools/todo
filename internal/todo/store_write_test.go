package todo

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"testing"
)

// mustID wraps an Add* call: mustID(t)(s.AddTask(…)) returns the new ID and
// fails the test on error.
func mustID(t *testing.T) func(uint, error) uint {
	return func(id uint, err error) uint {
		t.Helper()
		if err != nil {
			t.Fatalf("add: %v", err)
		}
		return id
	}
}

// mustOK fails the test on err.
func mustOK(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("write: %v", err)
	}
}

// load reads the store's document, failing the test on error.
func load(t *testing.T, s *Store) *Document {
	t.Helper()
	doc, _, err := s.Load(context.Background())
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	return doc
}

// item looks id up in doc, failing the test when it is missing.
func item(t *testing.T, doc *Document, id uint) *Item {
	t.Helper()
	it, ok := doc.ByID(id)
	if !ok {
		t.Fatalf("item %d not in the loaded document", id)
	}
	return it
}

// titles returns the titles of items, in order.
func titles(items []*Item) []string {
	out := []string{}
	for _, it := range items {
		out = append(out, it.Title)
	}
	return out
}

// sampleIDs are the IDs of the tree addSample builds.
type sampleIDs struct{ A, t1, t2, B uint }

// addSample builds category A holding tasks t1 (Open, described) and t2
// (InProgress), then category B, in that order.
func addSample(t *testing.T, s *Store) sampleIDs {
	t.Helper()
	ctx := context.Background()
	var ids sampleIDs
	ids.A = mustID(t)(s.AddCategory(ctx, "A", 0, 0))
	ids.t1 = mustID(t)(s.AddTask(ctx, "t1", "d1", Open, ids.A, 0))
	ids.t2 = mustID(t)(s.AddTask(ctx, "t2", "", InProgress, ids.A, ids.t1))
	ids.B = mustID(t)(s.AddCategory(ctx, "B", ids.A, ids.t2))
	return ids
}

func TestAddKeepsOrder(t *testing.T) {
	s := newStore(t)
	ids := addSample(t, s)
	doc := load(t, s)

	if len(doc.Roots) != 1 {
		t.Fatalf("got %d roots, want 1", len(doc.Roots))
	}
	a := doc.Roots[0]
	if a.ID != ids.A || a.Kind != Category || a.Title != "A" || a.Level != 1 || a.Parent != nil {
		t.Fatalf("root = %+v; want category A at level 1 with no parent", a)
	}
	if got, want := titles(a.Children), []string{"t1", "t2", "B"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("A's children = %v, want %v", got, want)
	}
	for _, c := range a.Children {
		if c.Parent != a {
			t.Fatalf("%s.Parent = %v, want A", c.Title, c.Parent)
		}
	}
	t1, t2, b := a.Children[0], a.Children[1], a.Children[2]
	if t1.ID != ids.t1 || t1.Kind != Task || t1.Status != Open || t1.Description != "d1" || t1.Level != 0 {
		t.Fatalf("t1 = %+v; want open task with description d1", t1)
	}
	if t2.ID != ids.t2 || t2.Kind != Task || t2.Status != InProgress || t2.Level != 0 {
		t.Fatalf("t2 = %+v; want in-progress task", t2)
	}
	if b.ID != ids.B || b.Kind != Category || b.Level != 2 {
		t.Fatalf("B = %+v; want category at level 2", b)
	}
}

func TestEditKeepsKind(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	c := mustID(t)(s.AddCategory(ctx, "C", 0, 0))
	task := mustID(t)(s.AddTask(ctx, "t", "", Deferred, c, 0))

	mustOK(t, s.Edit(ctx, c, "C2", "x"))
	mustOK(t, s.Edit(ctx, task, "t2", "desc"))

	doc := load(t, s)
	if got := item(t, doc, c); got.Kind != Category || got.Title != "C2" || got.Description != "" {
		t.Fatalf("category after Edit = %+v; want category C2 with no description", got)
	}
	if got := item(t, doc, task); got.Kind != Task || got.Title != "t2" || got.Description != "desc" || got.Status != Deferred {
		t.Fatalf("task after Edit = %+v; want deferred task t2 described desc", got)
	}
	if err := s.Edit(ctx, 999, "x", ""); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Edit(999) = %v, want ErrNotFound", err)
	}
}

func TestSetStatusesBackToOpen(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	id := mustID(t)(s.AddTask(ctx, "t", "desc", Done, 0, 0))

	mustOK(t, s.SetStatuses(ctx, map[uint]Status{id: Open, 999: Done}))

	got := item(t, load(t, s), id)
	if got.Status != Open || got.Title != "t" || got.Description != "desc" {
		t.Fatalf("task = %+v; want open task t described desc", got)
	}
}

func TestCascadeStatus(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	p := mustID(t)(s.AddTask(ctx, "p", "", Open, 0, 0))
	c1 := mustID(t)(s.AddTask(ctx, "c1", "", InProgress, p, 0))
	c2 := mustID(t)(s.AddTask(ctx, "c2", "", Open, p, c1))

	mustOK(t, s.CascadeStatus(ctx, p, Done))

	doc := load(t, s)
	for _, id := range []uint{p, c1, c2} {
		if got := item(t, doc, id); got.Status != Done {
			t.Fatalf("%s status = %v, want done", got.Title, got.Status)
		}
	}
}

func TestMove(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	ids := addSample(t, s)

	mustOK(t, s.Move(ctx, ids.t2, ids.A, 0))
	a := item(t, load(t, s), ids.A)
	if got, want := titles(a.Children), []string{"t2", "t1", "B"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("A's children after moving t2 first = %v, want %v", got, want)
	}

	mustOK(t, s.Move(ctx, ids.t1, ids.B, 0))
	doc := load(t, s)
	if got, want := titles(item(t, doc, ids.A).Children), []string{"t2", "B"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("A's children after moving t1 under B = %v, want %v", got, want)
	}
	if got, want := titles(item(t, doc, ids.B).Children), []string{"t1"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("B's children = %v, want %v", got, want)
	}
}

func TestDeleteSubtree(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	p := mustID(t)(s.AddTask(ctx, "p", "", Open, 0, 0))
	c := mustID(t)(s.AddTask(ctx, "c", "", Open, p, 0))
	mustID(t)(s.AddTask(ctx, "keep", "", Open, 0, p))

	mustOK(t, s.Delete(ctx, p))

	doc := load(t, s)
	if got, want := titles(doc.Roots), []string{"keep"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("roots after Delete = %v, want %v", got, want)
	}
	if _, ok := doc.ByID(c); ok {
		t.Fatal("subtask of the deleted parent is still there")
	}
	if err := s.Delete(ctx, 999); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Delete(999) = %v, want ErrNotFound", err)
	}
}

func TestPruneUsesFreshData(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "t.db")
	s1, err := OpenStore(path)
	if err != nil {
		t.Fatalf("OpenStore s1: %v", err)
	}
	t.Cleanup(func() { _ = s1.Close() })
	a := mustID(t)(s1.AddCategory(ctx, "A", 0, 0))
	done := mustID(t)(s1.AddTask(ctx, "done", "", Done, a, 0))
	d1 := mustID(t)(s1.AddTask(ctx, "d1", "", Done, done, 0))

	// A second process adds an open subtask under done after s1 last looked.
	s2, err := OpenStore(path)
	if err != nil {
		t.Fatalf("OpenStore s2: %v", err)
	}
	t.Cleanup(func() { _ = s2.Close() })
	fresh := mustID(t)(s2.AddTask(ctx, "fresh", "", Open, done, d1))

	n, err := s1.Prune(ctx)
	if err != nil {
		t.Fatalf("Prune: %v", err)
	}
	if n != 1 {
		t.Fatalf("Prune removed %d, want 1", n)
	}
	doc := load(t, s1)
	item(t, doc, done)
	item(t, doc, fresh)
	if _, ok := doc.ByID(d1); ok {
		t.Fatal("d1 survived the prune")
	}
}

func TestOpenStoreReopenKeepsData(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "t.db")
	s, err := OpenStore(path)
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	id := mustID(t)(s.AddCategory(ctx, "kept", 0, 0))
	if err := s.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	s, err = OpenStore(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	doc := load(t, s)
	if len(doc.Roots) != 1 || doc.Roots[0].ID != id || doc.Roots[0].Title != "kept" || doc.Roots[0].Kind != Category {
		t.Fatalf("roots after reopen = %v, want the one category kept", titles(doc.Roots))
	}
}

func TestEveryWriteBumpsRevision(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	var c, task uint
	writes := []struct {
		name string
		fn   func() error
	}{
		{"AddCategory", func() (err error) { c, err = s.AddCategory(ctx, "C", 0, 0); return err }},
		{"AddTask", func() (err error) { task, err = s.AddTask(ctx, "t", "", Open, c, 0); return err }},
		{"Edit", func() error { return s.Edit(ctx, task, "t2", "d") }},
		{"SetStatuses", func() error { return s.SetStatuses(ctx, map[uint]Status{task: Done}) }},
		{"CascadeStatus", func() error { return s.CascadeStatus(ctx, task, Done) }},
		{"Move", func() error { return s.Move(ctx, task, 0, c) }},
		{"Prune", func() error { _, err := s.Prune(ctx); return err }},
		{"Delete", func() error { return s.Delete(ctx, c) }},
	}
	for _, w := range writes {
		before, err := s.Revision(ctx)
		if err != nil {
			t.Fatalf("Revision before %s: %v", w.name, err)
		}
		if err := w.fn(); err != nil {
			t.Fatalf("%s: %v", w.name, err)
		}
		after, err := s.Revision(ctx)
		if err != nil {
			t.Fatalf("Revision after %s: %v", w.name, err)
		}
		if after != before+1 {
			t.Fatalf("%s moved the revision from %d to %d, want +1", w.name, before, after)
		}
	}
}

func TestManyInsertsBetweenSiblingsStayOrdered(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	c := mustID(t)(s.AddCategory(ctx, "C", 0, 0))
	// head keeps t0's sort order off zero: bisecting towards 0.0 never runs out
	// of float64 precision, bisecting towards 10.0 does after about 50 halvings.
	head := mustID(t)(s.AddTask(ctx, "head", "", Open, c, 0))
	first := mustID(t)(s.AddTask(ctx, "t0", "", Open, c, head))
	const n = 60
	for i := 1; i <= n; i++ {
		mustID(t)(s.AddTask(ctx, fmt.Sprintf("t%d", i), "", Open, c, first))
	}

	// Each insert lands right after t0, so the newest comes first after it.
	want := []string{"head", "t0"}
	for i := n; i >= 1; i-- {
		want = append(want, fmt.Sprintf("t%d", i))
	}
	if got := titles(item(t, load(t, s), c).Children); !reflect.DeepEqual(got, want) {
		t.Fatalf("order after %d inserts = %v, want %v", n, got, want)
	}
}
