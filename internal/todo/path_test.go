package todo_test

import (
	"reflect"
	"testing"

	"github.com/candy-tools/todo/internal/todo"
)

func TestItemPath(t *testing.T) {
	d := todo.Parse("# Work\n\n- [ ] a\n  - [ ] a1\n")
	a1 := d.Roots[0].Children[0].Children[0]
	got := a1.Path()
	want := []string{"Work", "a", "a1"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Path() = %v, want %v", got, want)
	}
}

func TestFindByPathRoundTrip(t *testing.T) {
	d := todo.Parse("# Work\n\n- [ ] a\n  - [ ] a1\n\n# Personal\n\n- [ ] b\n")
	a1 := d.Roots[0].Children[0].Children[0]
	if got := d.FindByPath(a1.Path()); got != a1 {
		t.Errorf("FindByPath(a1.Path()) should return a1")
	}
	personal := d.Roots[1]
	if got := d.FindByPath(personal.Path()); got != personal {
		t.Errorf("FindByPath should resolve a second root category")
	}
}

func TestFindByPathUnknownIsNil(t *testing.T) {
	d := todo.Parse("# Work\n\n- [ ] a\n")
	if got := d.FindByPath([]string{"Work", "nope"}); got != nil {
		t.Errorf("an unknown path should return nil, got %v", got)
	}
	if got := d.FindByPath(nil); got != nil {
		t.Errorf("an empty path should return nil")
	}
}
