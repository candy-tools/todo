package todo_test

import (
	"testing"

	"github.com/candy-tools/todo/internal/todo"
)

func TestEnumerateAndByNumber(t *testing.T) {
	d := todo.Parse("# Work\n\n- [ ] a\n  - [ ] a1\n- [ ] b\n")
	items := d.Enumerate()
	wantTitles := []string{"Work", "a", "a1", "b"}
	if len(items) != len(wantTitles) {
		t.Fatalf("Enumerate() len = %d, want %d", len(items), len(wantTitles))
	}
	for i, w := range wantTitles {
		if items[i].Title != w {
			t.Errorf("item %d = %q, want %q", i+1, items[i].Title, w)
		}
	}
	if it, ok := d.ByNumber(3); !ok || it.Title != "a1" {
		t.Errorf("ByNumber(3) = %v ok=%v, want a1", it, ok)
	}
	if _, ok := d.ByNumber(0); ok {
		t.Error("ByNumber(0) should be out of range")
	}
	if _, ok := d.ByNumber(99); ok {
		t.Error("ByNumber(99) should be out of range")
	}
}

func TestByLine(t *testing.T) {
	d := todo.Parse("# Work\n\n- [ ] a\n")
	if it, ok := d.ByLine(3); !ok || it.Title != "a" {
		t.Errorf("ByLine(3) = %v ok=%v, want a", it, ok)
	}
	if _, ok := d.ByLine(2); ok { // blank line, no item
		t.Error("ByLine(2) should miss (blank line)")
	}
}

func TestByTitle(t *testing.T) {
	d := todo.Parse("# A\n\n- [ ] dup\n\n# B\n\n- [ ] dup\n- [ ] uniq\n")
	if got := d.ByTitle("uniq"); len(got) != 1 || got[0].Title != "uniq" {
		t.Errorf("ByTitle(uniq) = %v, want one match", got)
	}
	if got := d.ByTitle("dup"); len(got) != 2 {
		t.Errorf("ByTitle(dup) = %d matches, want 2", len(got))
	}
	if got := d.ByTitle("nope"); len(got) != 0 {
		t.Errorf("ByTitle(nope) = %d matches, want 0", len(got))
	}
}
