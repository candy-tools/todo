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
	checkVisible(t, d, d.ColumnItems("", todo.ColInProgress), map[string]bool{
		"B": true, "working": true, "sub": true,
		"A": false, "Empty": false, "backlogTask": false, "doneTask": false,
	})
}

func TestColumnItemsBacklogShowsAllCategories(t *testing.T) {
	d := todotest.Parse(t, board)
	checkVisible(t, d, d.ColumnItems("", todo.ColBacklog), map[string]bool{
		"A": true, "Empty": true, "B": true, "backlogTask": true, "working": false,
	})
}

func TestColumnItemsEmptyColumn(t *testing.T) {
	d := todotest.Parse(t, board)
	got := d.ColumnItems("", todo.ColNext)
	if got == nil || len(got) != 0 {
		t.Errorf("ColumnItems(ColNext) = %v, want non-nil empty", got)
	}
}

func TestSubtaskFollowsParentColumn(t *testing.T) {
	d := todotest.Parse(t, "# B\n- [/] working\n  - [x] sub\n")
	checkVisible(t, d, d.ColumnItems("", todo.ColInProgress), map[string]bool{"working": true, "sub": true})
	checkVisible(t, d, d.ColumnItems("", todo.ColDone), map[string]bool{"working": false, "sub": false})
}

func TestColumnCounts(t *testing.T) {
	d := todotest.Parse(t, board)
	got := d.ColumnCounts("")
	want := map[todo.Column]int{todo.ColBacklog: 1, todo.ColNext: 0, todo.ColInProgress: 1, todo.ColDone: 1}
	for c, n := range want {
		if got[c] != n {
			t.Errorf("ColumnCounts()[%v] = %d, want %d", c, got[c], n)
		}
	}
}

func TestCountInColumn(t *testing.T) {
	d := todotest.Parse(t, board)
	if n := findByTitle(d, "B").CountInColumn("", todo.ColInProgress); n != 1 {
		t.Errorf("B in progress = %d, want 1", n)
	}
	if n := findByTitle(d, "A").CountInColumn("", todo.ColBacklog); n != 0 {
		t.Errorf("A backlog = %d, want 0", n)
	}
}

func TestCountInColumnPerSprint(t *testing.T) {
	d, s1, _ := twoSprints(t)
	a := findByTitle(d, "A")
	if n := a.CountInColumn(s1, todo.ColInProgress); n != 1 {
		t.Errorf("A in progress in S1 = %d, want 1", n)
	}
	if n := a.CountInColumn("", todo.ColInProgress); n != 2 {
		t.Errorf("A in progress in any sprint = %d, want 2", n)
	}
}

func TestColumnMatchesBlankQueryIsColumn(t *testing.T) {
	d := todotest.Parse(t, board)
	checkVisible(t, d, d.ColumnMatches("  ", "", todo.ColInProgress), map[string]bool{
		"B": true, "working": true, "sub": true, "backlogTask": false,
	})
}

func TestColumnMatchesOnlyInTab(t *testing.T) {
	// beta matches but sits in Backlog, so the In Progress search must not keep
	// its category C as a bare row.
	d := todotest.Parse(t, "# C\n- [/] w\n- [ ] beta\n# Beta\n- [/] v\n# D\n- [/] betaWork\n  - [ ] s\n")
	checkVisible(t, d, d.ColumnMatches("beta", "", todo.ColInProgress), map[string]bool{
		"C": false, "w": false, "beta": false,
		"Beta": true, "v": true, // a matching category keeps its in-tab subtree
		"D": true, "betaWork": true, "s": true,
	})
}

func TestKeepStates(t *testing.T) {
	d := parse("# A\n- [ ] open\n  - [ ] sub\n- [>] deferred\n# B\n- [ ] other\n# Empty\n")
	vis := d.KeepStates(d.ColumnItems("", todo.ColBacklog), map[todo.Status]bool{todo.Open: true}, "", todo.ColBacklog)
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

func TestColumnItemsUnion(t *testing.T) {
	d := todotest.Parse(t, "# A\n- [ ] o\n# B\n- [x] d\n# E\n")
	findByTitle(d, "o").Status = todo.Ready // o now in Next
	checkVisible(t, d, d.ColumnItems("", todo.ColNext, todo.ColDone), map[string]bool{
		"A": true, "o": true, "B": true, "d": true,
		"E": false, // empty category: Backlog isn't in the set
	})
	if vis := d.ColumnItems("", todo.Columns()...); !vis[findByTitle(d, "E")] {
		t.Error("a set with Backlog shows every category")
	}
}

func TestKeepStatesAcrossColumns(t *testing.T) {
	d := todotest.Parse(t, "# A\n- [ ] o\n- [ ] r\n- [x] d\n")
	findByTitle(d, "r").Status = todo.Ready
	all := todo.Columns()
	got := d.KeepStates(d.ColumnItems("", all...), map[todo.Status]bool{todo.Ready: true, todo.Done: true}, "", all...)
	checkVisible(t, d, got, map[string]bool{"A": true, "r": true, "d": true, "o": false})
}

// twoSprints parses a board with a working task in each of two new sprints.
func twoSprints(t *testing.T) (d *todo.Document, s1, s2 string) {
	t.Helper()
	d = todotest.Parse(t, "# A\n- [/] one\n  - [ ] sub1\n- [/] two\n- [ ] loose\n# Empty\n")
	var err error
	if s1, err = d.AddSprint("S1", ""); err != nil {
		t.Fatal(err)
	}
	if s2, err = d.AddSprint("S2", ""); err != nil {
		t.Fatal(err)
	}
	findByTitle(d, "one").Sprint = s1
	findByTitle(d, "two").Sprint = s2
	return d, s1, s2
}

func TestColumnItemsScopedToSprint(t *testing.T) {
	d, s1, _ := twoSprints(t)
	checkVisible(t, d, d.ColumnItems(s1, todo.ColInProgress), map[string]bool{
		"A": true, "one": true, "sub1": true,
		"two": false, "loose": false, "Empty": false,
	})
	checkVisible(t, d, d.ColumnItems("", todo.ColInProgress), map[string]bool{"one": true, "two": true})
}

func TestColumnCountsPerSprint(t *testing.T) {
	d, s1, s2 := twoSprints(t)
	if got := d.ColumnCounts(s1); got[todo.ColInProgress] != 1 || got[todo.ColBacklog] != 0 {
		t.Errorf("ColumnCounts(s1) = %v, want one in progress, none in backlog", got)
	}
	if got := d.ColumnCounts(s2); got[todo.ColInProgress] != 1 {
		t.Errorf("ColumnCounts(s2) = %v, want one in progress", got)
	}
	if got := d.ColumnCounts(""); got[todo.ColInProgress] != 2 || got[todo.ColBacklog] != 1 {
		t.Errorf("ColumnCounts(\"\") = %v, want two in progress, one in backlog", got)
	}
}

func TestBacklogStillShowsEveryCategory(t *testing.T) {
	d, _, _ := twoSprints(t)
	checkVisible(t, d, d.ColumnItems("", todo.ColBacklog), map[string]bool{
		"A": true, "Empty": true, "loose": true, "one": false,
	})
}
