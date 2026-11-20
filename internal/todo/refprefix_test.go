package todo_test

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/candy-tools/todo/internal/todo"
)

// must returns v, and panics (failing the test) when err is not nil.
func must[T any](v T, err error) T {
	if err != nil {
		panic(err)
	}
	return v
}

// refOf returns the reference of task id.
func refOf(t *testing.T, d *todo.Document, id string) string {
	t.Helper()
	it, ok := d.ByID(id)
	if !ok {
		t.Fatalf("no item %s", id)
	}
	return it.Ref
}

func TestCleanPrefix(t *testing.T) {
	for _, tc := range []struct {
		in, want string
		err      error
	}{
		{" work ", "WORK", nil},
		{"A1", "A1", nil},
		{"", "", nil},
		{"ABCDEFGHIJ", "ABCDEFGHIJ", nil},
		{"wo rk", "", todo.ErrInvalidPrefix},
		{"1AB", "", todo.ErrInvalidPrefix},
		{"WORK-", "", todo.ErrInvalidPrefix},
		{"ABCDEFGHIJK", "", todo.ErrInvalidPrefix},
		{"ÁB", "", todo.ErrInvalidPrefix},
	} {
		got, err := todo.CleanPrefix(tc.in)
		if !errors.Is(err, tc.err) || got != tc.want {
			t.Errorf("CleanPrefix(%q) = %q, %v; want %q, %v", tc.in, got, err, tc.want, tc.err)
		}
	}
}

func TestPrefixPrecedence(t *testing.T) {
	d := &todo.Document{}
	if _, err := d.SetRefPrefix("todo"); err != nil {
		t.Fatal(err)
	}
	work := must(d.AppendCategoryWith(todo.Fields{Title: "Work", Prefix: "work"}, ""))
	sub := must(d.AppendCategory("Sub", work))
	other := must(d.AppendCategory("Other", ""))
	task := must(d.AppendTask(todo.Fields{Title: "t"}, todo.Open, sub))

	for _, tc := range []struct {
		name, parent, want string
	}{
		{"category prefix inherited", sub, "WORK"},
		{"list prefix", other, "TODO"},
		{"task parent", task, ""},
		{"missing parent", "nope", ""},
	} {
		if got := d.PrefixFor(tc.parent); got != tc.want {
			t.Errorf("%s: PrefixFor = %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestAddTaskGetsRef(t *testing.T) {
	d := &todo.Document{}
	must(d.SetRefPrefix("TODO"))
	cat := must(d.AppendCategory("C", ""))
	var refs []string
	var first string
	for _, title := range []string{"a", "b", "c"} {
		id := must(d.AppendTask(todo.Fields{Title: title}, todo.Open, cat))
		if first == "" {
			first = id
		}
		refs = append(refs, refOf(t, d, id))
	}
	if want := []string{"TODO-001", "TODO-002", "TODO-003"}; !slices.Equal(refs, want) {
		t.Errorf("refs = %v, want %v", refs, want)
	}
	sub := must(d.AppendTask(todo.Fields{Title: "s"}, todo.Open, first))
	if got := refOf(t, d, sub); got != "" {
		t.Errorf("subtask ref = %q, want none", got)
	}
}

func TestAddTaskNoPrefixNoRef(t *testing.T) {
	d := &todo.Document{}
	cat := must(d.AppendCategory("C", ""))
	id := must(d.AppendTask(todo.Fields{Title: "a"}, todo.Open, cat))
	if got := refOf(t, d, id); got != "" {
		t.Errorf("ref = %q, want none", got)
	}
	id = must(d.AppendTask(todo.Fields{Title: "b", Ref: "X-1"}, todo.Open, cat))
	if got := refOf(t, d, id); got != "X-1" {
		t.Errorf("ref = %q, want X-1", got)
	}
	if d.RefLast != nil {
		t.Errorf("RefLast = %v, want nil", d.RefLast)
	}
}

func TestAddTaskRefRefusedWithPrefix(t *testing.T) {
	d := &todo.Document{}
	cat := must(d.AppendCategoryWith(todo.Fields{Title: "C", Prefix: "work"}, ""))
	_, err := d.AppendTask(todo.Fields{Title: "a", Ref: "MINE-1"}, todo.Open, cat)
	if !errors.Is(err, todo.ErrRefGenerated) || !errors.Is(err, todo.ErrBadRequest) {
		t.Fatalf("err = %v, want ErrRefGenerated", err)
	}
	if n := len(d.Enumerate()); n != 1 {
		t.Errorf("%d items, want only the category", n)
	}
	// A typed reference that duplicates an existing one is still refused as
	// generated, not as a duplicate.
	d2 := &todo.Document{}
	plain := must(d2.AppendCategory("P", ""))
	must(d2.AppendTask(todo.Fields{Title: "x", Ref: "DUP-1"}, todo.Open, plain))
	if _, err := d2.Edit(plain, todo.Changes{Prefix: new("work")}); err != nil {
		t.Fatal(err)
	}
	if _, err := d2.AppendTask(todo.Fields{Title: "y", Ref: "DUP-1"}, todo.Open, plain); !errors.Is(err, todo.ErrRefGenerated) {
		t.Errorf("duplicate typed ref: err = %v, want ErrRefGenerated", err)
	}
	if d.RefLast != nil {
		t.Errorf("a refused add advanced the counter: %v", d.RefLast)
	}
	// A refusal for another reason must not advance it either.
	if _, err := d.AppendTask(todo.Fields{Title: " "}, todo.Open, cat); err == nil {
		t.Fatal("empty title accepted")
	}
	if _, err := d.AppendTask(todo.Fields{Title: "a"}, todo.Working, cat); err == nil {
		t.Fatal("a sprint state without a sprint accepted")
	}
	if d.RefLast != nil {
		t.Errorf("a refused add advanced the counter: %v", d.RefLast)
	}
}

func TestNextRefSkipsTaken(t *testing.T) {
	d := &todo.Document{}
	cat := must(d.AppendCategory("C", ""))
	must(d.AppendTask(todo.Fields{Title: "typed", Ref: "work-002"}, todo.Open, cat))
	if _, err := d.Edit(cat, todo.Changes{Prefix: new("work")}); err != nil {
		t.Fatal(err)
	}
	id := must(d.AppendTask(todo.Fields{Title: "next"}, todo.Open, cat))
	if got := refOf(t, d, id); got != "WORK-003" {
		t.Errorf("ref = %q, want WORK-003", got)
	}
}

func TestNextRefSharedPrefix(t *testing.T) {
	d := &todo.Document{}
	a := must(d.AppendCategoryWith(todo.Fields{Title: "A", Prefix: "WORK"}, ""))
	b := must(d.AppendCategoryWith(todo.Fields{Title: "B", Prefix: "WORK"}, ""))
	var refs []string
	for _, cat := range []string{a, b, a, b} {
		refs = append(refs, refOf(t, d, must(d.AppendTask(todo.Fields{Title: "t"}, todo.Open, cat))))
	}
	if want := []string{"WORK-001", "WORK-002", "WORK-003", "WORK-004"}; !slices.Equal(refs, want) {
		t.Errorf("refs = %v, want %v", refs, want)
	}
}

func TestNextRefAfterPrefixCleared(t *testing.T) {
	d := &todo.Document{}
	cat := must(d.AppendCategoryWith(todo.Fields{Title: "C", Prefix: "work"}, ""))
	must(d.AppendTask(todo.Fields{Title: "a"}, todo.Open, cat))
	second := must(d.AppendTask(todo.Fields{Title: "b"}, todo.Open, cat))
	if err := d.Delete(second); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{"", "work"} {
		if _, err := d.Edit(cat, todo.Changes{Prefix: new(p)}); err != nil {
			t.Fatal(err)
		}
	}
	id := must(d.AppendTask(todo.Fields{Title: "c"}, todo.Open, cat))
	if got := refOf(t, d, id); got != "WORK-003" {
		t.Errorf("ref = %q, want WORK-003", got)
	}
}

func TestNextRefIgnoresHugeTypedNumber(t *testing.T) {
	d := &todo.Document{}
	cat := must(d.AppendCategory("C", ""))
	must(d.AppendTask(todo.Fields{Title: "typed", Ref: "WORK-9223372036854775807"}, todo.Open, cat))
	must(d.AppendTask(todo.Fields{Title: "typed2", Ref: "WORK-99999999999999999999"}, todo.Open, cat))
	if _, err := d.Edit(cat, todo.Changes{Prefix: new("work")}); err != nil {
		t.Fatal(err)
	}
	id := must(d.AppendTask(todo.Fields{Title: "next"}, todo.Open, cat))
	if got := refOf(t, d, id); got != "WORK-001" {
		t.Errorf("ref = %q, want WORK-001", got)
	}
	if got := d.RefLast["WORK"]; got != 1 {
		t.Errorf("RefLast = %d, want 1", got)
	}
}

func TestCleanPrefixNonASCIIReason(t *testing.T) {
	_, err := todo.CleanPrefix("ÉÉÉÉÉÉ")
	if !errors.Is(err, todo.ErrInvalidPrefix) || !strings.Contains(err.Error(), "letter followed by") {
		t.Errorf("err = %v, want the character-rule reason", err)
	}
}

func TestRefGrowsPast999(t *testing.T) {
	d := &todo.Document{RefLast: map[string]int{"A": 999}}
	cat := must(d.AppendCategoryWith(todo.Fields{Title: "C", Prefix: "a"}, ""))
	id := must(d.AppendTask(todo.Fields{Title: "t"}, todo.Open, cat))
	if got := refOf(t, d, id); got != "A-1000" {
		t.Errorf("ref = %q, want A-1000", got)
	}
}

func TestEditPrefixKinds(t *testing.T) {
	d := &todo.Document{}
	cat := must(d.AppendCategory("C", ""))
	task := must(d.AppendTask(todo.Fields{Title: "t"}, todo.Open, cat))

	changed, err := d.Edit(cat, todo.Changes{Prefix: new("x")})
	if err != nil || !changed {
		t.Fatalf("Edit category prefix = %v, %v", changed, err)
	}
	if it, _ := d.ByID(cat); it.Prefix != "X" {
		t.Errorf("category prefix = %q, want X", it.Prefix)
	}
	if changed, err := d.Edit(cat, todo.Changes{Prefix: new("X")}); err != nil || changed {
		t.Errorf("same prefix = %v, %v; want unchanged", changed, err)
	}
	if _, err := d.Edit(cat, todo.Changes{Prefix: new("1x")}); !errors.Is(err, todo.ErrInvalidPrefix) {
		t.Errorf("bad prefix err = %v", err)
	}
	if _, err := d.Edit(task, todo.Changes{Prefix: new("x")}); !errors.Is(err, todo.ErrNotCategory) || !errors.Is(err, todo.ErrWrongKind) {
		t.Errorf("task prefix err = %v, want ErrNotCategory", err)
	}
	if _, err := d.Edit(cat, todo.Changes{Ref: new("A-1")}); !errors.Is(err, todo.ErrNotTask) {
		t.Errorf("category ref err = %v, want ErrNotTask", err)
	}

	// A task's reference stays editable while a prefix applies.
	must(d.SetRefPrefix("TODO"))
	if changed, err := d.Edit(task, todo.Changes{Ref: new("MINE-7")}); err != nil || !changed {
		t.Errorf("edit ref = %v, %v", changed, err)
	}
	if got := refOf(t, d, task); got != "MINE-7" {
		t.Errorf("ref = %q", got)
	}
}

func TestAddWithInvalidPrefixAddsNothing(t *testing.T) {
	d := &todo.Document{}
	if _, err := d.AppendCategoryWith(todo.Fields{Title: "C", Prefix: "1"}, ""); !errors.Is(err, todo.ErrInvalidPrefix) {
		t.Errorf("category err = %v", err)
	}
	if len(d.Roots) != 0 {
		t.Error("an invalid prefix still added something")
	}
	if _, err := d.SetRefPrefix("bad prefix"); !errors.Is(err, todo.ErrInvalidPrefix) {
		t.Errorf("SetRefPrefix err = %v", err)
	}
}

func TestCloneCopiesRefs(t *testing.T) {
	d := &todo.Document{RefLast: map[string]int{"A": 1}}
	must(d.SetRefPrefix("TODO"))
	cat := must(d.AppendCategoryWith(todo.Fields{Title: "C", Prefix: "c"}, ""))
	c := d.Clone()
	c.RefLast["A"] = 5
	if d.RefLast["A"] != 1 {
		t.Errorf("clone shares RefLast: %v", d.RefLast)
	}
	if c.RefPrefix != "TODO" {
		t.Errorf("RefPrefix = %q", c.RefPrefix)
	}
	if it, _ := c.ByID(cat); it.Prefix != "C" {
		t.Errorf("category prefix = %q", it.Prefix)
	}
}
