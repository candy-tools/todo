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
func mustID(t *testing.T) func(string, error) string {
	return func(id string, err error) string {
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
func item(t *testing.T, doc *Document, id string) *Item {
	t.Helper()
	it, ok := doc.ByID(id)
	if !ok {
		t.Fatalf("item %s not in the loaded document", id)
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
type sampleIDs struct{ A, t1, t2, B string }

// addSample builds category A holding tasks t1 (Open, described) and t2
// (Working), then category B, in that order.
func addSample(t *testing.T, s *Store) sampleIDs {
	t.Helper()
	ctx := context.Background()
	var ids sampleIDs
	ids.A = mustID(t)(s.AddCategory(ctx, "A", "", ""))
	ids.t1 = mustID(t)(s.AddTask(ctx, Fields{Title: "t1", Description: "d1"}, Open, ids.A, ""))
	ids.t2 = mustID(t)(s.AddTask(ctx, Fields{Title: "t2"}, Working, ids.A, ids.t1))
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
}

func TestAddStoresItemFields(t *testing.T) {
	s := newStore(t)
	ids := addSample(t, s)
	a := load(t, s).Roots[0]
	t1, t2, b := a.Children[0], a.Children[1], a.Children[2]
	if t1.ID != ids.t1 || t1.Kind != Task || t1.Status != Open || t1.Description != "d1" || t1.Level != 0 {
		t.Fatalf("t1 = %+v; want open task with description d1", t1)
	}
	if t2.ID != ids.t2 || t2.Kind != Task || t2.Status != Working || t2.Level != 0 {
		t.Fatalf("t2 = %+v; want in-progress task", t2)
	}
	if b.ID != ids.B || b.Kind != Category || b.Level != 2 {
		t.Fatalf("B = %+v; want category at level 2", b)
	}
}

func TestEditKeepsKind(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	c := mustID(t)(s.AddCategory(ctx, "C", "", ""))
	task := mustID(t)(s.AddTask(ctx, Fields{Title: "t"}, Deferred, c, ""))

	mustOK(t, s.Edit(ctx, c, Changes{Title: new("C2"), Description: new("x")}))
	mustOK(t, s.Edit(ctx, task, Changes{Title: new("t2"), Description: new("desc")}))

	doc := load(t, s)
	if got := item(t, doc, c); got.Kind != Category || got.Title != "C2" || got.Description != "" {
		t.Fatalf("category after Edit = %+v; want category C2 with no description", got)
	}
	if got := item(t, doc, task); got.Kind != Task || got.Title != "t2" || got.Description != "desc" || got.Status != Deferred {
		t.Fatalf("task after Edit = %+v; want deferred task t2 described desc", got)
	}
	if err := s.Edit(ctx, "999", Changes{Title: new("x")}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Edit(999) = %v, want ErrNotFound", err)
	}
}

func TestSetStatusesBackToOpen(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	c := mustID(t)(s.AddCategory(ctx, "C", "", ""))
	id := mustID(t)(s.AddTask(ctx, Fields{Title: "t", Description: "desc"}, Done, c, ""))

	mustOK(t, s.SetStatuses(ctx, map[string]Status{id: Open}))

	got := item(t, load(t, s), id)
	if got.Status != Open || got.Title != "t" || got.Description != "desc" {
		t.Fatalf("task = %+v; want open task t described desc", got)
	}
}

// TestSetStatusesRejectsMissingID: a write naming an item the store does not
// hold, such as one another process deleted since the caller's read, fails
// whole with ErrNotFound instead of reporting success for 0 rows.
func TestSetStatusesRejectsMissingID(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	c := mustID(t)(s.AddCategory(ctx, "C", "", ""))
	id := mustID(t)(s.AddTask(ctx, Fields{Title: "t"}, Done, c, ""))
	gone := mustID(t)(s.AddTask(ctx, Fields{Title: "gone"}, Open, c, id))
	mustOK(t, s.Delete(ctx, gone))

	rejects(t, s, ErrNotFound, func() error { return s.SetStatuses(ctx, map[string]Status{gone: Working}) })
	rejects(t, s, ErrNotFound, func() error { return s.SetStatuses(ctx, map[string]Status{id: Open, "999": Done}) })
	rejects(t, s, ErrNotFound, func() error { return s.Complete(ctx, gone, false) })
	rejects(t, s, ErrNotFound, func() error { return s.Reopen(ctx, gone, Working, ReopenOptions{}) })

	if got := item(t, load(t, s), id); got.Status != Done {
		t.Fatalf("task after rejected writes = %+v; want it still done", got)
	}
}

func TestCascadeDoneAndReopen(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	c := mustID(t)(s.AddCategory(ctx, "C", "", ""))
	p := mustID(t)(s.AddTask(ctx, Fields{Title: "p"}, Review, c, ""))
	c1 := mustID(t)(s.AddTask(ctx, Fields{Title: "c1"}, Open, p, ""))
	c2 := mustID(t)(s.AddTask(ctx, Fields{Title: "c2"}, Done, p, c1))

	mustOK(t, s.CascadeStatus(ctx, p, Done))
	doc := load(t, s)
	for _, id := range []string{p, c1, c2} {
		if got := item(t, doc, id); got.Status != Done {
			t.Fatalf("after Cascade(done): %s status = %v, want done", got.Title, got.Status)
		}
	}

	mustOK(t, s.CascadeStatus(ctx, p, Working))
	doc = load(t, s)
	if got := item(t, doc, p); got.Status != Working {
		t.Fatalf("after Cascade(working): p status = %v, want working", got.Status)
	}
	for _, id := range []string{c1, c2} {
		if got := item(t, doc, id); got.Status != Open {
			t.Fatalf("after Cascade(working): %s status = %v, want open", got.Title, got.Status)
		}
	}
}

func TestMove(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	ids := addSample(t, s)

	mustOK(t, s.Move(ctx, ids.t2, ids.A, ""))
	a := item(t, load(t, s), ids.A)
	if got, want := titles(a.Children), []string{"t2", "t1", "B"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("A's children after moving t2 first = %v, want %v", got, want)
	}

	mustOK(t, s.Move(ctx, ids.t1, ids.B, ""))
	doc := load(t, s)
	if got, want := titles(item(t, doc, ids.A).Children), []string{"t2", "B"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("A's children after moving t1 under B = %v, want %v", got, want)
	}
	if got, want := titles(item(t, doc, ids.B).Children), []string{"t1"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("B's children = %v, want %v", got, want)
	}
}

func TestMoveMissingIsNotFound(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	ids := addSample(t, s)
	before, err := s.Revision(ctx)
	if err != nil {
		t.Fatalf("Revision: %v", err)
	}

	// To the root is the case closure-tree's Update lets through silently.
	if err := s.Move(ctx, "999", "", ""); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Move(999, root) = %v, want ErrNotFound", err)
	}
	if err := s.Move(ctx, "999", ids.A, ""); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Move(999, A) = %v, want ErrNotFound", err)
	}
	if after, err := s.Revision(ctx); err != nil || after != before {
		t.Fatalf("Revision after failed moves = %d, %v; want %d", after, err, before)
	}
}

func TestDeleteSubtree(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	cat := mustID(t)(s.AddCategory(ctx, "C", "", ""))
	p := mustID(t)(s.AddTask(ctx, Fields{Title: "p"}, Open, cat, ""))
	c := mustID(t)(s.AddTask(ctx, Fields{Title: "c"}, Open, p, ""))
	mustID(t)(s.AddTask(ctx, Fields{Title: "keep"}, Open, cat, p))

	mustOK(t, s.Delete(ctx, p))

	doc := load(t, s)
	if got, want := titles(item(t, doc, cat).Children), []string{"keep"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("C's children after Delete = %v, want %v", got, want)
	}
	if _, ok := doc.ByID(c); ok {
		t.Fatal("subtask of the deleted parent is still there")
	}
	if err := s.Delete(ctx, "999"); !errors.Is(err, ErrNotFound) {
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
	a := mustID(t)(s1.AddCategory(ctx, "A", "", ""))
	done := mustID(t)(s1.AddTask(ctx, Fields{Title: "done"}, Done, a, ""))
	d1 := mustID(t)(s1.AddTask(ctx, Fields{Title: "d1"}, Done, done, ""))

	// A second process adds an open subtask under done after s1 last looked.
	s2, err := OpenStore(path)
	if err != nil {
		t.Fatalf("OpenStore s2: %v", err)
	}
	t.Cleanup(func() { _ = s2.Close() })
	fresh := mustID(t)(s2.AddTask(ctx, Fields{Title: "fresh"}, Open, done, d1))

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
	id := mustID(t)(s.AddCategory(ctx, "kept", "", ""))
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
	var c, task string
	writes := []struct {
		name string
		fn   func() error
	}{
		{"AddCategory", func() (err error) { c, err = s.AddCategory(ctx, "C", "", ""); return err }},
		{"AddTask", func() (err error) { task, err = s.AddTask(ctx, Fields{Title: "t"}, Open, c, ""); return err }},
		{"AppendCategory", func() error { _, err := s.AppendCategory(ctx, "D", c); return err }},
		{"AppendTask", func() error { _, err := s.AppendTask(ctx, Fields{Title: "u"}, Open, c); return err }},
		{"Edit", func() error { return s.Edit(ctx, task, Changes{Title: new("t2"), Description: new("d")}) }},
		{"SetStatuses", func() error { return s.SetStatuses(ctx, map[string]Status{task: Done}) }},
		{"CascadeStatus", func() error { return s.CascadeStatus(ctx, task, Done) }},
		{"Reopen", func() error { return s.Reopen(ctx, task, Working, ReopenOptions{}) }},
		{"Complete", func() error { return s.Complete(ctx, task, false) }},
		{"Reopen restore", func() error {
			return s.Reopen(ctx, task, Working, ReopenOptions{Restore: DoneStates{task: Done}})
		}},
		{"Complete cascade", func() error { return s.Complete(ctx, task, true) }},
		{"Reopen cascade", func() error { return s.Reopen(ctx, task, Working, ReopenOptions{Cascade: true}) }},
		{"Move", func() error { return s.Move(ctx, task, c, "") }},
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
	c := mustID(t)(s.AddCategory(ctx, "C", "", ""))
	// head keeps t0's sort order off zero: bisecting towards 0.0 never runs out
	// of float64 precision, bisecting towards 10.0 does after about 50 halvings.
	head := mustID(t)(s.AddTask(ctx, Fields{Title: "head"}, Open, c, ""))
	first := mustID(t)(s.AddTask(ctx, Fields{Title: "t0"}, Open, c, head))
	const n = 60
	for i := 1; i <= n; i++ {
		mustID(t)(s.AddTask(ctx, Fields{Title: fmt.Sprintf("t%d", i)}, Open, c, first))
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

// rejects runs a write that must fail with want, and checks that it left the
// store untouched: the revision did not move.
func rejects(t *testing.T, s *Store, want error, write func() error) {
	t.Helper()
	ctx := context.Background()
	before, err := s.Revision(ctx)
	if err != nil {
		t.Fatalf("Revision: %v", err)
	}
	if err := write(); !errors.Is(err, want) {
		t.Fatalf("write = %v, want %v", err, want)
	}
	if after, err := s.Revision(ctx); err != nil || after != before {
		t.Fatalf("revision after a rejected write = %d, %v; want %d", after, err, before)
	}
}

// taskTree is category C holding task task (in state st) with one open subtask
// sub.
type taskTree struct{ C, task, sub string }

func addTaskTree(t *testing.T, s *Store, st Status) taskTree {
	t.Helper()
	ctx := context.Background()
	var ids taskTree
	ids.C = mustID(t)(s.AddCategory(ctx, "C", "", ""))
	ids.task = mustID(t)(s.AddTask(ctx, Fields{Title: "task"}, st, ids.C, ""))
	ids.sub = mustID(t)(s.AddTask(ctx, Fields{Title: "sub"}, Open, ids.task, ""))
	return ids
}

func TestPlanRoundTrip(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	c := mustID(t)(s.AddCategory(ctx, "C", "", ""))
	id := mustID(t)(s.AddTask(ctx, Fields{Title: "t", Description: "d", Plan: "docs/p.md"}, Ready, c, ""))

	if got := item(t, load(t, s), id); got.Plan != "docs/p.md" || got.Description != "d" {
		t.Fatalf("task after AddTask = %+v; want plan docs/p.md, description d", got)
	}

	mustOK(t, s.Edit(ctx, id, Changes{Title: new("t2"), Description: new("d2"), Plan: new("")}))
	if got := item(t, load(t, s), id); got.Plan != "" || got.Title != "t2" || got.Description != "d2" || got.Status != Ready {
		t.Fatalf("task after Edit = %+v; want ready task t2 described d2 with no plan", got)
	}
}

// An edit naming only the title must not touch a description or plan another
// process wrote since the caller read the task.
func TestEditKeepsFieldsItDoesNotName(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	c := mustID(t)(s.AddCategory(ctx, "C", "", ""))
	id := mustID(t)(s.AddTask(ctx, Fields{Title: "t", Description: "d", Plan: "docs/old.md"}, Ready, c, ""))
	mustOK(t, s.Edit(ctx, id, Changes{Description: new("d2"), Plan: new("docs/new.md")}))

	mustOK(t, s.Edit(ctx, id, Changes{Title: new("t2")}))

	if got := item(t, load(t, s), id); got.Title != "t2" || got.Description != "d2" || got.Plan != "docs/new.md" {
		t.Fatalf("task after title-only Edit = %+v; want t2 described d2 with plan docs/new.md", got)
	}
	mustOK(t, s.Edit(ctx, id, Changes{}))
	if got := item(t, load(t, s), id); got.Title != "t2" || got.Description != "d2" || got.Plan != "docs/new.md" {
		t.Fatalf("task after empty Edit = %+v; want it unchanged", got)
	}
}

// An Edit naming no field is a no-op: it opens no write, so the revision
// stays put and pollers see no change.
func TestEmptyEditKeepsRevision(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	c := mustID(t)(s.AddCategory(ctx, "C", "", ""))
	id := mustID(t)(s.AddTask(ctx, Fields{Title: "t"}, Open, c, ""))
	before, err := s.Revision(ctx)
	mustOK(t, err)

	mustOK(t, s.Edit(ctx, id, Changes{}))

	after, err := s.Revision(ctx)
	mustOK(t, err)
	if after != before {
		t.Fatalf("an empty Edit moved the revision from %d to %d", before, after)
	}
}

func TestEditCategoryIgnoresPlan(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	c := mustID(t)(s.AddCategory(ctx, "C", "", ""))

	mustOK(t, s.Edit(ctx, c, Changes{Title: new("C2"), Description: new("x"), Plan: new("docs/p.md")}))

	if got := item(t, load(t, s), c); got.Title != "C2" || got.Description != "" || got.Plan != "" {
		t.Fatalf("category after Edit = %+v; want C2 with no description or plan", got)
	}
}

// An Edit of a category that names only the fields a category ignores changes
// nothing it stores, so it must not bump the revision (the TUI reads a bump as an
// external change), and a missing item is still ErrNotFound.
func TestEditCategoryIgnoredFieldsKeepRevision(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	c := mustID(t)(s.AddCategory(ctx, "C", "", ""))
	for name, ch := range map[string]Changes{
		"description": {Description: new("x")},
		"plan":        {Plan: new("docs/p.md")},
		"both":        {Description: new("x"), Plan: new("docs/p.md")},
	} {
		before, err := s.Revision(ctx)
		mustOK(t, err)
		mustOK(t, s.Edit(ctx, c, ch))
		after, err := s.Revision(ctx)
		mustOK(t, err)
		if after != before {
			t.Fatalf("%s: a category Edit of ignored fields moved the revision from %d to %d", name, before, after)
		}
		if err := s.Edit(ctx, c+"00", ch); !errors.Is(err, ErrNotFound) {
			t.Fatalf("%s: Edit of a missing item = %v; want ErrNotFound", name, err)
		}
	}
	if got := item(t, load(t, s), c); got.Title != "C" || got.Description != "" || got.Plan != "" {
		t.Fatalf("category after Edit = %+v; want C with no description or plan", got)
	}
}

func TestAddUnderSubtaskIsTooDeep(t *testing.T) {
	s := newStore(t)
	ids := addTaskTree(t, s, Open)
	rejects(t, s, ErrTooDeep, func() error {
		_, err := s.AddTask(context.Background(), Fields{Title: "deep"}, Open, ids.sub, "")
		return err
	})
}

func TestAddSubtaskWithBoardStateRejected(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	ids := addTaskTree(t, s, Working)
	rejects(t, s, ErrSubtaskStatus, func() error {
		_, err := s.AddTask(ctx, Fields{Title: "r"}, Review, ids.task, ids.sub)
		return err
	})
	// The classic states stay allowed for a subtask, and any state for a task.
	mustID(t)(s.AddTask(ctx, Fields{Title: "d"}, Done, ids.task, ids.sub))
	mustID(t)(s.AddTask(ctx, Fields{Title: "r"}, Review, ids.C, ids.task))
}

func TestMoveTaskWithChildrenUnderTaskIsTooDeep(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	ids := addTaskTree(t, s, Open)
	other := mustID(t)(s.AddTask(ctx, Fields{Title: "other"}, Open, ids.C, ids.task))

	rejects(t, s, ErrTooDeep, func() error { return s.Move(ctx, ids.task, other, "") })
	// A childless task under a subtask would be depth 3 as well.
	rejects(t, s, ErrTooDeep, func() error { return s.Move(ctx, other, ids.sub, "") })
}

func TestMoveIntoSubtaskNormalizesStatus(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	c := mustID(t)(s.AddCategory(ctx, "C", "", ""))
	parent := mustID(t)(s.AddTask(ctx, Fields{Title: "parent"}, Working, c, ""))
	rev := mustID(t)(s.AddTask(ctx, Fields{Title: "rev"}, Review, c, parent))
	done := mustID(t)(s.AddTask(ctx, Fields{Title: "done"}, Done, c, rev))

	mustOK(t, s.Move(ctx, rev, parent, ""))
	mustOK(t, s.Move(ctx, done, parent, rev))

	doc := load(t, s)
	if got := item(t, doc, rev); got.Status != Open || got.Parent == nil || got.Parent.ID != parent {
		t.Fatalf("rev after the move = %+v; want an open subtask of parent", got)
	}
	if got := item(t, doc, done); got.Status != Done {
		t.Fatalf("done after the move = %v, want done", got.Status)
	}
	if got := item(t, doc, parent); got.Status != Working {
		t.Fatalf("parent = %v after the moves, want working", got.Status)
	}
}

func TestMoveSubtaskOutTakesParentState(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	ids := addTaskTree(t, s, Ready)

	mustOK(t, s.Move(ctx, ids.sub, ids.C, ids.task))

	got := item(t, load(t, s), ids.sub)
	if got.Status != Ready || got.Parent == nil || got.Parent.ID != ids.C {
		t.Fatalf("sub after the move = %+v; want a ready task of C", got)
	}
}

func TestMoveTaskBetweenCategoriesKeepsStatus(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	a := mustID(t)(s.AddCategory(ctx, "A", "", ""))
	b := mustID(t)(s.AddCategory(ctx, "B", "", a))
	id := mustID(t)(s.AddTask(ctx, Fields{Title: "t"}, Review, a, ""))

	mustOK(t, s.Move(ctx, id, b, ""))

	if got := item(t, load(t, s), id); got.Status != Review {
		t.Fatalf("status after moving to another category = %v, want review", got.Status)
	}
}

func TestSetStatusesRejectsBoardStateOnSubtask(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	ids := addTaskTree(t, s, Open)

	rejects(t, s, ErrSubtaskStatus, func() error {
		return s.SetStatuses(ctx, map[string]Status{ids.task: Review, ids.sub: Review})
	})
	if got := item(t, load(t, s), ids.task); got.Status != Open {
		t.Fatalf("task status after the rejected write = %v, want open", got.Status)
	}

	mustOK(t, s.SetStatuses(ctx, map[string]Status{ids.task: Review, ids.sub: Done}))
	doc := load(t, s)
	if got := item(t, doc, ids.task); got.Status != Review {
		t.Fatalf("task status = %v, want review", got.Status)
	}
	if got := item(t, doc, ids.sub); got.Status != Done {
		t.Fatalf("sub status = %v, want done", got.Status)
	}
}

func TestCascadeOnSubtaskWithBoardStateRejected(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	ids := addTaskTree(t, s, Open)

	rejects(t, s, ErrSubtaskStatus, func() error { return s.CascadeStatus(ctx, ids.sub, Review) })
	mustOK(t, s.CascadeStatus(ctx, ids.sub, Done))
	if got := item(t, load(t, s), ids.sub); got.Status != Done {
		t.Fatalf("sub status = %v, want done", got.Status)
	}
}

func TestAddCategoryUnderTaskIsWrongParent(t *testing.T) {
	s := newStore(t)
	ids := addTaskTree(t, s, Open)
	rejects(t, s, ErrWrongParent, func() error {
		_, err := s.AddCategory(context.Background(), "X", ids.task, "")
		return err
	})
	rejects(t, s, ErrWrongParent, func() error {
		_, err := s.AddCategory(context.Background(), "X", ids.sub, "")
		return err
	})
}

func TestAddTaskAtRootIsWrongParent(t *testing.T) {
	s := newStore(t)
	rejects(t, s, ErrWrongParent, func() error {
		_, err := s.AddTask(context.Background(), Fields{Title: "loose"}, Open, "", "")
		return err
	})
}

func TestMoveCategoryUnderTaskIsWrongParent(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	ids := addTaskTree(t, s, Open)
	other := mustID(t)(s.AddCategory(ctx, "D", "", ids.C))

	rejects(t, s, ErrWrongParent, func() error { return s.Move(ctx, other, ids.task, "") })
	rejects(t, s, ErrWrongParent, func() error { return s.Move(ctx, other, ids.sub, "") })
	// A category may still move under a category or to the root.
	mustOK(t, s.Move(ctx, other, ids.C, ids.task))
	mustOK(t, s.Move(ctx, other, "", ids.C))
}

func TestMoveTaskToRootIsWrongParent(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	ids := addTaskTree(t, s, Open)

	rejects(t, s, ErrWrongParent, func() error { return s.Move(ctx, ids.task, "", ids.C) })
	rejects(t, s, ErrWrongParent, func() error { return s.Move(ctx, ids.sub, "", ids.C) })
}

func TestStatusOnCategoryIsNotTask(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	ids := addTaskTree(t, s, Open)

	rejects(t, s, ErrNotTask, func() error { return s.SetStatuses(ctx, map[string]Status{ids.C: Done}) })
	rejects(t, s, ErrNotTask, func() error { return s.SetStatuses(ctx, map[string]Status{ids.task: Done, ids.C: Working}) })
	rejects(t, s, ErrNotTask, func() error { return s.CascadeStatus(ctx, ids.C, Done) })
	if got := item(t, load(t, s), ids.task); got.Status != Open {
		t.Fatalf("task status after the rejected writes = %v, want open", got.Status)
	}
}

// An add whose sibling was moved away by another process since the caller
// read the tree is a conflict, not a raw closure-tree error.
func TestAddAfterMovedSiblingIsConflict(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	ids := addSample(t, s)
	mustOK(t, s.Move(ctx, ids.t2, ids.B, "")) // another process moves t2 away

	rejects(t, s, ErrConflict, func() error {
		_, err := s.AddTask(ctx, Fields{Title: "t3"}, Open, ids.A, ids.t2)
		return err
	})
	rejects(t, s, ErrConflict, func() error {
		_, err := s.AddCategory(ctx, "C", ids.A, ids.t2)
		return err
	})
	rejects(t, s, ErrConflict, func() error { return s.Move(ctx, ids.t1, ids.A, ids.t2) })
}

// Moving a category under its own descendant is an invalid move, not a
// conflict: reloading and retrying would fail the same way.
func TestMoveUnderOwnDescendantIsInvalidMove(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	ids := addSample(t, s)
	rejects(t, s, ErrInvalidMove, func() error { return s.Move(ctx, ids.A, ids.B, "") })
	if err := s.Move(ctx, ids.A, ids.B, ""); errors.Is(err, ErrConflict) {
		t.Fatalf("Move under own descendant = %v, want no ErrConflict", err)
	}
}

// AppendTask places the task after the last task child of its parent, before
// any subcategory, as the tree is at write time.
func TestAppendTaskPlacesAfterLastTask(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	ids := addSample(t, s)
	mustOK(t, s.Move(ctx, ids.t2, ids.B, "")) // another process moves t2 away

	t3 := mustID(t)(s.AppendTask(ctx, Fields{Title: "t3", Description: "d3", Plan: "p3"}, Working, ids.A))
	doc := load(t, s)
	if got, want := titles(item(t, doc, ids.A).Children), []string{"t1", "t3", "B"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("A's children = %v, want %v", got, want)
	}
	if got := item(t, doc, t3); got.Status != Working || got.Description != "d3" || got.Plan != "p3" {
		t.Fatalf("t3 = %+v; want working task described d3 with plan p3", got)
	}

	// Only a subcategory: the task goes first. No children: it is the only one.
	mustID(t)(s.AppendTask(ctx, Fields{Title: "u"}, Open, mustID(t)(s.AddCategory(ctx, "D", "", ids.A))))
	c := mustID(t)(s.AddCategory(ctx, "E", "", ""))
	sub := mustID(t)(s.AddCategory(ctx, "Esub", c, ""))
	mustID(t)(s.AppendTask(ctx, Fields{Title: "e1"}, Open, c))
	// Under a task: a subtask, appended after the last one.
	mustID(t)(s.AppendTask(ctx, Fields{Title: "s3"}, Done, ids.t1))
	mustID(t)(s.AppendTask(ctx, Fields{Title: "s4"}, Open, ids.t1))

	doc = load(t, s)
	if got, want := titles(item(t, doc, c).Children), []string{"e1", "Esub"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("E's children = %v, want %v", got, want)
	}
	if item(t, doc, sub).Parent.ID != c {
		t.Fatalf("Esub moved away from E")
	}
	if got, want := titles(item(t, doc, ids.t1).Children), []string{"s3", "s4"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("t1's children = %v, want %v", got, want)
	}
}

// AppendTask keeps the store's tree rules.
func TestAppendTaskRules(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	tt := addTaskTree(t, s, Open)
	rejects(t, s, ErrWrongParent, func() error {
		_, err := s.AppendTask(ctx, Fields{Title: "x"}, Open, "")
		return err
	})
	rejects(t, s, ErrTooDeep, func() error {
		_, err := s.AppendTask(ctx, Fields{Title: "x"}, Open, tt.sub)
		return err
	})
	rejects(t, s, ErrSubtaskStatus, func() error {
		_, err := s.AppendTask(ctx, Fields{Title: "x"}, Working, tt.task)
		return err
	})
	rejects(t, s, ErrNotFound, func() error {
		_, err := s.AppendTask(ctx, Fields{Title: "x"}, Open, "999")
		return err
	})
	rejects(t, s, ErrInvalidTitle, func() error {
		_, err := s.AppendTask(ctx, Fields{Title: " "}, Open, tt.C)
		return err
	})
}

// AppendCategory places the category after the last child of its parent — after
// its tasks and subcategories — as the tree is at write time: a last sibling
// another process deleted since the caller read the tree does not matter.
func TestAppendCategoryPlacesLast(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	ids := addSample(t, s)
	gone := mustID(t)(s.AddCategory(ctx, "Gone", "", ids.A))
	mustOK(t, s.Delete(ctx, gone)) // another process deletes the last root

	x := mustID(t)(s.AppendCategory(ctx, " X ", ""))
	c := mustID(t)(s.AppendCategory(ctx, "C", ids.A))
	only := mustID(t)(s.AppendCategory(ctx, "Only", x))
	doc := load(t, s)
	if got, want := titles(doc.Roots), []string{"A", "X"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("roots = %v, want %v", got, want)
	}
	if got, want := titles(item(t, doc, ids.A).Children), []string{"t1", "t2", "B", "C"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("A's children = %v, want %v", got, want)
	}
	if got, want := titles(item(t, doc, x).Children), []string{"Only"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("X's children = %v, want %v", got, want)
	}
	for _, id := range []string{x, c, only} {
		if got := item(t, doc, id); got.Kind != Category {
			t.Fatalf("%q kind = %v, want a category", got.Title, got.Kind)
		}
	}
}

// AppendCategory keeps the store's tree rules.
func TestAppendCategoryRules(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	tt := addTaskTree(t, s, Open)
	rejects(t, s, ErrWrongParent, func() error {
		_, err := s.AppendCategory(ctx, "x", tt.task)
		return err
	})
	rejects(t, s, ErrNotFound, func() error {
		_, err := s.AppendCategory(ctx, "x", "999")
		return err
	})
	rejects(t, s, ErrInvalidTitle, func() error {
		_, err := s.AppendCategory(ctx, " ", tt.C)
		return err
	})
}
