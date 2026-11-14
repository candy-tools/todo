package todo_test

import (
	"slices"
	"testing"

	"github.com/candy-tools/todo/internal/todo"
	"github.com/candy-tools/todo/internal/todo/todotest"
)

func TestFlattenKeepsReadingOrder(t *testing.T) {
	d := todotest.Parse(t, "# C\n- [ ] T\n  - [ ] a\n    - [ ] a1\n      - [ ] a1x\n  - [ ] b\n")
	moved := d.FlattenSubtasks()
	T := find(d, "T")
	if got := titles(T.Children); !slices.Equal(got, []string{"a", "a1", "a1x", "b"}) {
		t.Errorf("T's children = %v, want [a a1 a1x b]", got)
	}
	for _, c := range T.Children {
		if c.Parent != T || len(c.Children) != 0 {
			t.Errorf("%s: parent=%v children=%d, want parent T and no children", c.Title, c.Parent, len(c.Children))
		}
	}
	if got := titles(moved); !slices.Equal(got, []string{"a1", "a1x"}) {
		t.Errorf("moved = %v, want [a1 a1x]", got)
	}
}

func TestFlattenNormalizesSubtaskStatus(t *testing.T) {
	d := todotest.Parse(t, "# C\n- [/] T\n  - [/] w\n  - [>] d\n  - [x] x\n")
	d.FlattenSubtasks()
	for title, want := range map[string]todo.Status{"T": todo.Working, "w": todo.Open, "d": todo.Open, "x": todo.Done} {
		if got := find(d, title).Status; got != want {
			t.Errorf("%s status = %v, want %v", title, got, want)
		}
	}
}

func TestFlattenNoopOnFlatTree(t *testing.T) {
	d := todotest.Parse(t, "# C\n- [ ] T\n  - [ ] a\n  - [ ] b\n- [ ] U\n")
	if moved := d.FlattenSubtasks(); moved != nil {
		t.Errorf("moved = %v, want nil", titles(moved))
	}
	if got := titles(find(d, "T").Children); !slices.Equal(got, []string{"a", "b"}) {
		t.Errorf("T's children = %v, want [a b]", got)
	}
}
