package todo_test

import (
	"testing"

	"github.com/candy-tools/todo/internal/todo"
	"github.com/candy-tools/todo/internal/todo/todotest"
)

const board = "# A\n## Empty\n# B\n- [ ] backlogTask\n- [/] working\n  - [ ] sub\n- [x] doneTask\n"

func findByTitle(d *todo.Document, title string) *todo.Item {
	var found *todo.Item
	var walk func(it *todo.Item)
	walk = func(it *todo.Item) {
		if it.Title == title {
			found = it
		}
		for _, c := range it.Children {
			walk(c)
		}
	}
	for _, r := range d.Roots {
		walk(r)
	}
	return found
}

func checkVisible(t *testing.T, d *todo.Document, vis map[*todo.Item]bool, want map[string]bool) {
	t.Helper()
	for title, w := range want {
		it := findByTitle(d, title)
		if it == nil {
			t.Fatalf("item %q not found", title)
		}
		if vis[it] != w {
			t.Errorf("%q visible = %v, want %v", title, vis[it], w)
		}
	}
}

func TestColumnItemsKeepsCategoryPath(t *testing.T) {
	d := todotest.Parse(t, board)
	checkVisible(t, d, d.ColumnItems(todo.ColInProgress), map[string]bool{
		"B": true, "working": true, "sub": true,
		"A": false, "Empty": false, "backlogTask": false, "doneTask": false,
	})
}

func TestColumnItemsBacklogShowsAllCategories(t *testing.T) {
	d := todotest.Parse(t, board)
	checkVisible(t, d, d.ColumnItems(todo.ColBacklog), map[string]bool{
		"A": true, "Empty": true, "B": true, "backlogTask": true, "working": false,
	})
}

func TestColumnItemsEmptyColumn(t *testing.T) {
	d := todotest.Parse(t, board)
	got := d.ColumnItems(todo.ColNext)
	if got == nil || len(got) != 0 {
		t.Errorf("ColumnItems(ColNext) = %v, want non-nil empty", got)
	}
}

func TestSubtaskFollowsParentColumn(t *testing.T) {
	d := todotest.Parse(t, "# B\n- [/] working\n  - [x] sub\n")
	checkVisible(t, d, d.ColumnItems(todo.ColInProgress), map[string]bool{"working": true, "sub": true})
	checkVisible(t, d, d.ColumnItems(todo.ColDone), map[string]bool{"working": false, "sub": false})
}

func TestColumnCounts(t *testing.T) {
	d := todotest.Parse(t, board)
	got := d.ColumnCounts()
	want := map[todo.Column]int{todo.ColBacklog: 1, todo.ColNext: 0, todo.ColInProgress: 1, todo.ColDone: 1}
	for c, n := range want {
		if got[c] != n {
			t.Errorf("ColumnCounts()[%v] = %d, want %d", c, got[c], n)
		}
	}
}

func TestCountInColumn(t *testing.T) {
	d := todotest.Parse(t, board)
	if n := findByTitle(d, "B").CountInColumn(todo.ColInProgress); n != 1 {
		t.Errorf("B in progress = %d, want 1", n)
	}
	if n := findByTitle(d, "A").CountInColumn(todo.ColBacklog); n != 0 {
		t.Errorf("A backlog = %d, want 0", n)
	}
}

func TestColumnMatchesBlankQueryIsColumn(t *testing.T) {
	d := todotest.Parse(t, board)
	checkVisible(t, d, d.ColumnMatches(todo.ColInProgress, "  "), map[string]bool{
		"B": true, "working": true, "sub": true, "backlogTask": false,
	})
}

func TestColumnMatchesOnlyInTab(t *testing.T) {
	// beta matches but sits in Backlog, so the In Progress search must not keep
	// its category C as a bare row.
	d := todotest.Parse(t, "# C\n- [/] w\n- [ ] beta\n# Beta\n- [/] v\n# D\n- [/] betaWork\n  - [ ] s\n")
	checkVisible(t, d, d.ColumnMatches(todo.ColInProgress, "beta"), map[string]bool{
		"C": false, "w": false, "beta": false,
		"Beta": true, "v": true, // a matching category keeps its in-tab subtree
		"D": true, "betaWork": true, "s": true,
	})
}

func TestKeepStates(t *testing.T) {
	d := parse("# A\n- [ ] open\n  - [ ] sub\n- [>] deferred\n# B\n- [ ] other\n# Empty\n")
	vis := d.KeepStates(d.ColumnItems(todo.ColBacklog), todo.ColBacklog, map[todo.Status]bool{todo.Open: true})
	for _, title := range []string{"A", "open", "sub", "B", "other"} {
		if !vis[findByTitle(d, title)] {
			t.Errorf("%q should be kept", title)
		}
	}
	for _, title := range []string{"deferred", "Empty"} {
		if vis[findByTitle(d, title)] {
			t.Errorf("%q should be dropped", title)
		}
	}
}
