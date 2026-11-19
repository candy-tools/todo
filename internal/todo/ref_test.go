package todo_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/candy-tools/todo/internal/todo"
	"github.com/candy-tools/todo/internal/todo/todotest"
)

func TestCleanRef(t *testing.T) {
	for _, tc := range []struct {
		in, want string
		err      error
	}{
		{"", "", nil},
		{"  APP-001 ", "APP-001", nil},
		{"free form ref", "free form ref", nil},
		{"A\tB", "", todo.ErrInvalidRef},
		{strings.Repeat("x", todo.MaxRefLen), strings.Repeat("x", todo.MaxRefLen), nil},
		{strings.Repeat("x", todo.MaxRefLen+1), "", todo.ErrInvalidRef},
	} {
		got, err := todo.CleanRef(tc.in)
		if !errors.Is(err, tc.err) || got != tc.want {
			t.Errorf("CleanRef(%q) = %q, %v; want %q, %v", tc.in, got, err, tc.want, tc.err)
		}
	}
}

func TestDisplayTitle(t *testing.T) {
	it := &todo.Item{Kind: todo.Task, Title: "Ship it"}
	if got := it.DisplayTitle(); got != "Ship it" {
		t.Errorf("without a ref: %q", got)
	}
	it.Ref = "App-001"
	if got := it.DisplayTitle(); got != "[App-001] Ship it" {
		t.Errorf("with a ref: %q", got)
	}
}

func TestOpAddTaskWithRef(t *testing.T) {
	d := opDoc(t, "# A\n")
	id, err := d.AppendTask(todo.Fields{Title: "t", Ref: " APP-001 "}, todo.Open, idOf(t, d, "A"))
	wantNoErr(t, err)
	it, _ := d.ByID(id)
	if it.Ref != "APP-001" {
		t.Errorf("Ref = %q, want the trimmed APP-001", it.Ref)
	}
	if got, ok := d.ByRef("app-001"); !ok || got != it {
		t.Errorf("ByRef ignoring case = %v, %v; want the task", got, ok)
	}
}

// References are unique in a list ignoring case; an empty one is "none" and
// may repeat.
func TestOpAddTaskDuplicateRef(t *testing.T) {
	d := opDoc(t, "# A\n")
	cat := idOf(t, d, "A")
	_, err := d.AppendTask(todo.Fields{Title: "t", Ref: "App-001"}, todo.Open, cat)
	wantNoErr(t, err)
	_, err = d.AppendTask(todo.Fields{Title: "u", Ref: "APP-001"}, todo.Open, cat)
	wantErr(t, err, todo.ErrDuplicateRef)
	if !errors.Is(err, todo.ErrBadRequest) {
		t.Errorf("err = %v, want class ErrBadRequest", err)
	}
	for _, title := range []string{"v", "w"} {
		_, err = d.AppendTask(todo.Fields{Title: title}, todo.Open, cat)
		wantNoErr(t, err)
	}
	wantDoc(t, d, "# A\n\n- [ ] t\n- [ ] v\n- [ ] w\n")
}

func TestOpAddSubtaskRefRefused(t *testing.T) {
	d := opDoc(t, "# A\n\n- [ ] t\n")
	_, err := d.AppendTask(todo.Fields{Title: "s", Ref: "APP-001"}, todo.Open, idOf(t, d, "t"))
	wantErr(t, err, todo.ErrSubtaskRef)
	wantDoc(t, d, "# A\n\n- [ ] t\n")
}

func TestOpEditRef(t *testing.T) {
	d := opDoc(t, "# A\n\n- [ ] t\n- [ ] u\n")
	ref := "APP-001"
	changed, err := d.Edit(idOf(t, d, "t"), todo.Changes{Ref: &ref})
	wantNoErr(t, err)
	if !changed {
		t.Error("setting a ref: changed = false")
	}
	// Setting the same ref again (in another case) on the same task is not a
	// duplicate.
	same := "app-001"
	changed, err = d.Edit(idOf(t, d, "t"), todo.Changes{Ref: &same})
	wantNoErr(t, err)
	if !changed || find(d, "t").Ref != "app-001" {
		t.Errorf("re-casing the ref: changed = %v, Ref = %q", changed, find(d, "t").Ref)
	}
	_, err = d.Edit(idOf(t, d, "u"), todo.Changes{Ref: &ref})
	wantErr(t, err, todo.ErrDuplicateRef)
	if find(d, "u").Ref != "" {
		t.Error("a refused edit changed the ref")
	}
	_, err = d.Edit(idOf(t, d, "t"), todo.Changes{Ref: new(string)})
	wantNoErr(t, err)
	if find(d, "t").Ref != "" {
		t.Error(`setting "" did not clear the ref`)
	}
	bad := "a\nb"
	_, err = d.Edit(idOf(t, d, "t"), todo.Changes{Ref: &bad})
	wantErr(t, err, todo.ErrInvalidRef)
}

func TestOpEditRefOnSubtaskOrCategory(t *testing.T) {
	d := opDoc(t, "# A\n\n- [ ] t\n  - [ ] s\n")
	ref := "APP-001"
	_, err := d.Edit(idOf(t, d, "s"), todo.Changes{Ref: &ref})
	wantErr(t, err, todo.ErrSubtaskRef)
	_, err = d.Edit(idOf(t, d, "s"), todo.Changes{Ref: new(string)})
	wantNoErr(t, err) // clearing what a subtask does not have is fine
	_, err = d.Edit(idOf(t, d, "A"), todo.Changes{Ref: new(string)})
	wantErr(t, err, todo.ErrNotTask)
}

// A task holding a reference cannot become a subtask; one without can.
func TestOpMoveRefTaskUnderTask(t *testing.T) {
	d := opDoc(t, "# A\n\n- [ ] t\n- [ ] u\n")
	ref := "APP-001"
	_, err := d.Edit(idOf(t, d, "u"), todo.Changes{Ref: &ref})
	wantNoErr(t, err)
	before := structure(d)
	wantErr(t, d.Move(idOf(t, d, "u"), idOf(t, d, "t"), ""), todo.ErrSubtaskRef)
	if got := structure(d); got != before {
		t.Errorf("a refused move changed the document:\n%s", got)
	}
	_, _, err = d.StepPlacement(idOf(t, d, "u"), todo.StepIn, nil)
	wantErr(t, err, todo.ErrSubtaskRef)
	wantNoErr(t, d.Move(idOf(t, d, "t"), idOf(t, d, "u"), ""))
}

func TestMatchesRef(t *testing.T) {
	it := &todo.Item{Kind: todo.Task, Title: "Ship it", Ref: "APP-001"}
	if !it.Matches("app-0") {
		t.Error("a task does not match a part of its reference")
	}
}

// A list file of format version 3, from before references, loads and is
// written back as the current version.
func TestDiskVersion3Loads(t *testing.T) {
	path := todotest.SeedJSON(t, `{"version": 3, "categories": [{"id": "aaaaaaa", "title": "A"}],
		"tasks": [{"id": "bbbbbbb", "category": "aaaaaaa", "title": "t", "status": "open"}]}`)
	s := todo.NewStore(path)
	ref := "APP-001"
	if _, err := s.Edit(context.Background(), "bbbbbbb", todo.Changes{Ref: &ref}); err != nil {
		t.Fatal(err)
	}
	file := readFile(t, path)
	if !strings.Contains(file, `"version": 5`) || !strings.Contains(file, `"ref": "APP-001"`) {
		t.Errorf("file =\n%s\nwant version 5 with the ref", file)
	}
}

// The store refuses a duplicate reference another process wrote meanwhile:
// the check runs on the list as it is at write time.
func TestStoreDuplicateRef(t *testing.T) {
	ctx := context.Background()
	path := todotest.SeedJSON(t, `{"version": 4, "categories": [{"id": "aaaaaaa", "title": "A"}],
		"tasks": [{"id": "bbbbbbb", "category": "aaaaaaa", "ref": "APP-001", "title": "t", "status": "open"}]}`)
	_, _, err := todo.NewStore(path).AppendTask(ctx, todo.Fields{Title: "u", Ref: "app-001"}, todo.Open, "aaaaaaa")
	wantErr(t, err, todo.ErrDuplicateRef)
}
