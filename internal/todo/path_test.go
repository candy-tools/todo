package todo_test

import (
	"reflect"
	"testing"

	"github.com/candy-tools/todo/internal/todo/todotest"
)

func TestItemPath(t *testing.T) {
	d := todotest.Parse(t, "# Work\n\n- [ ] a\n  - [ ] a1\n")
	a1 := d.Roots[0].Children[0].Children[0]
	got := a1.Path()
	want := []string{"Work", "a", "a1"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Path() = %v, want %v", got, want)
	}
}
