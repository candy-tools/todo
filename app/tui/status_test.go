package tui

import (
	"context"
	"strings"
	"testing"

	"github.com/candy-tools/todo/internal/todo"
	"github.com/candy-tools/todo/internal/todo/todotest"
	"github.com/charmbracelet/x/ansi"
)

// storeRev returns the store's current revision, for asserting a key wrote
// nothing.
func storeRev(t *testing.T, m model) int64 {
	t.Helper()
	_, rev, err := m.store.Load(context.Background())
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	return rev
}

// selectTitle moves the cursor onto the row titled title in the current tab.
func selectTitle(t *testing.T, m model, title string) model {
	t.Helper()
	it := find(m.doc, title)
	if it == nil || !m.tree.selectID(it.ID) {
		t.Fatalf("%q is not shown on %s (rows %v)", title, m.tree.column.Title(), rowTitles(m.tree))
	}
	return m
}

func TestBracketMovesToNextColumn(t *testing.T) {
	m, path := newTestModel(t, "# C\n- [ ] a\n- [ ] b\n")
	m = press(m, "down") // a, row 1
	m = press(m, "]")
	if st := storedItem(t, path, "a").Status; st != todo.New {
		t.Fatalf("] on a Backlog task stored %v, want new", st)
	}
	if contains(rowTitles(m.tree), "a") {
		t.Errorf("a must leave Backlog, rows = %v", rowTitles(m.tree))
	}
	if m.tree.cursor != 1 {
		t.Errorf("the cursor must stay at row 1, got %d", m.tree.cursor)
	}
	if got := m.tree.selected(); got == nil || got.Title != "b" {
		t.Errorf("the row that took a's place must be selected, got %v", got)
	}
	if got := m.footer(200); !strings.Contains(got, "moved to Next — new") {
		t.Errorf("footer = %q, want the move notice", got)
	}
	// From Next, ] lands in In Progress's first state.
	m = showOn(t, m, todo.ColNext, "a")
	m = press(m, "]")
	if st := storedItem(t, path, "a").Status; st != todo.Working {
		t.Errorf("] on a Next task stored %v, want working", st)
	}
}

func TestBracketBackFromDoneIsWorking(t *testing.T) {
	m, path := newTestModelOn(t, "# C\n- [x] d\n  - [ ] s\n", todo.ColDone)
	m = selectTitle(t, m, "d")
	m = press(m, "[")
	if st := storedItem(t, path, "d").Status; st != todo.Working {
		t.Errorf("[ on a done task stored %v, want working", st)
	}
	if st := storedItem(t, path, "s").Status; st != todo.Open {
		t.Errorf("without a snapshot the subtask keeps its state, got %v", st)
	}
	if got := m.footer(200); !strings.Contains(got, "moved to In Progress — working") {
		t.Errorf("footer = %q", got)
	}
}

func TestBracketIntoDoneCascadesWithUndo(t *testing.T) {
	m, path := newTestModel(t, "# C\n- [ ] r\n  - [ ] s1\n  - [ ] s2\n")
	m = setStatusExternally(t, m, path, "r", todo.Review)
	m = showOn(t, m, todo.ColInProgress, "r")
	m = press(m, "]")
	for _, title := range []string{"r", "s1", "s2"} {
		if st := storedItem(t, path, title).Status; st != todo.Done {
			t.Fatalf("%s = %v after ] into Done, want done", title, st)
		}
	}
	m = showOn(t, m, todo.ColDone, "r")
	press(m, "x")
	if st := storedItem(t, path, "r").Status; st != todo.Review {
		t.Errorf("undo must restore r to review, got %v", st)
	}
	for _, title := range []string{"s1", "s2"} {
		if st := storedItem(t, path, title).Status; st != todo.Open {
			t.Errorf("undo must restore %s to open, got %v", title, st)
		}
	}
}

func TestBracketOutOfDoneRestoresSubtasks(t *testing.T) {
	m, path := newTestModel(t, "# C\n- [ ] r\n  - [ ] s1\n  - [x] s2\n")
	m = setStatusExternally(t, m, path, "r", todo.Review)
	m = showOn(t, m, todo.ColInProgress, "r")
	m = press(m, "]") // into Done: snapshot, cascade
	m = showOn(t, m, todo.ColDone, "r")
	m = press(m, "[") // out of Done: parent working, subtasks from the snapshot
	if st := storedItem(t, path, "r").Status; st != todo.Working {
		t.Errorf("[ out of Done must land r on working, got %v", st)
	}
	if st := storedItem(t, path, "s1").Status; st != todo.Open {
		t.Errorf("s1 must be restored to open, got %v", st)
	}
	if st := storedItem(t, path, "s2").Status; st != todo.Done {
		t.Errorf("s2 must be restored to done, got %v", st)
	}
	if len(m.snapshots) != 0 {
		t.Errorf("leaving Done must drop the snapshot, got %d", len(m.snapshots))
	}
}

func TestBracketOnSubtaskIsHint(t *testing.T) {
	m, path := newTestModel(t, "# C\n- [ ] a\n  - [ ] s\n")
	m = selectTitle(t, m, "s")
	before := storeRev(t, m)
	for _, key := range []string{"]", "["} {
		got := press(m, key)
		if got.status == "" {
			t.Errorf("%q on a subtask must explain itself in the footer", key)
		}
		if rev := storeRev(t, got); rev != before {
			t.Errorf("%q on a subtask wrote to the store", key)
		}
	}
	if st := storedItem(t, path, "s").Status; st != todo.Open {
		t.Errorf("s = %v, want open", st)
	}
}

func TestBracketAtEdgeIsNoop(t *testing.T) {
	m, _ := newTestModel(t, "# C\n- [ ] a\n- [x] d\n")
	m = selectTitle(t, m, "a")
	before := storeRev(t, m)
	got := press(m, "[")
	if got.status == "" || storeRev(t, got) != before {
		t.Errorf("[ on Backlog must only hint, status %q", got.status)
	}
	m = showOn(t, m, todo.ColDone, "d")
	got = press(m, "]")
	if got.status == "" || storeRev(t, got) != before {
		t.Errorf("] on Done must only hint, status %q", got.status)
	}
}

func TestBracketUnknownStatusIsBacklog(t *testing.T) {
	m, path := newTestModel(t, "# C\n- [ ] a\n")
	m = setStatusExternally(t, m, path, "a", todo.Status(9))
	m = selectTitle(t, m, "a")
	before := storeRev(t, m)
	got := press(m, "[")
	if got.status != "no column beyond Backlog" || storeRev(t, got) != before {
		t.Errorf("[ on an unknown status must hint as at Backlog's edge, status %q", got.status)
	}
	press(m, "]")
	if st := storedItem(t, path, "a").Status; st != todo.New {
		t.Errorf("] on an unknown status stored %v, want new", st)
	}
}

func TestStateCycleKey(t *testing.T) {
	m, path := newTestModel(t, "# C\n- [ ] a\n")
	m = setStatusExternally(t, m, path, "a", todo.New)
	m = showOn(t, m, todo.ColNext, "a")
	m = press(m, "t")
	if st := storedItem(t, path, "a").Status; st != todo.Ready {
		t.Fatalf("t on a new task stored %v, want ready", st)
	}
	if got := m.tree.selected(); got == nil || got.Title != "a" {
		t.Errorf("a stays in Next and selected, got %v", got)
	}
	m = press(m, "t")
	if st := storedItem(t, path, "a").Status; st != todo.New {
		t.Errorf("t on a ready task stored %v, want new", st)
	}
}

func TestStateCycleOnSubtaskIsHint(t *testing.T) {
	m, path := newTestModel(t, "# C\n- [ ] a\n  - [ ] s\n")
	m = selectTitle(t, m, "s")
	before := storeRev(t, m)
	m = press(m, "t")
	if m.status == "" || storeRev(t, m) != before {
		t.Errorf("t on a subtask must only hint, status %q", m.status)
	}
	if st := storedItem(t, path, "s").Status; st != todo.Open {
		t.Errorf("s = %v, want open", st)
	}
}

func TestUndoAccidentalCompleteRestoresExactState(t *testing.T) {
	m, path := newTestModel(t, "# C\n- [ ] a\n")
	m = setStatusExternally(t, m, path, "a", todo.New)
	m = showOn(t, m, todo.ColNext, "a")
	m = press(m, "x")
	if st := storedItem(t, path, "a").Status; st != todo.Done {
		t.Fatalf("x stored %v, want done", st)
	}
	if got := m.footer(200); !strings.Contains(got, "moved to Done — done") {
		t.Errorf("footer = %q", got)
	}
	m = showOn(t, m, todo.ColDone, "a")
	press(m, "x")
	if st := storedItem(t, path, "a").Status; st != todo.New {
		t.Errorf("undo must restore a to new, got %v", st)
	}
}

func TestReopenWithoutSnapshotIsWorking(t *testing.T) {
	m, path := newTestModelOn(t, "# C\n- [x] d\n  - [x] s\n", todo.ColDone)
	m = selectTitle(t, m, "d")
	press(m, "x")
	if st := storedItem(t, path, "d").Status; st != todo.Working {
		t.Errorf("reopening a done task without a snapshot stored %v, want working", st)
	}
	if st := storedItem(t, path, "s").Status; st != todo.Done {
		t.Errorf("its subtask keeps its state, got %v", st)
	}
}

func TestSubtaskToggle(t *testing.T) {
	m, path := newTestModel(t, "# C\n- [ ] a\n  - [ ] s\n")
	m = selectTitle(t, m, "s")
	m = press(m, "x")
	if st := storedItem(t, path, "s").Status; st != todo.Done {
		t.Fatalf("x on a subtask stored %v, want done", st)
	}
	if st := storedItem(t, path, "a").Status; st != todo.Open {
		t.Errorf("toggling a subtask must leave its task alone, got %v", st)
	}
	press(m, "x")
	if st := storedItem(t, path, "s").Status; st != todo.Open {
		t.Errorf("x again stored %v, want open", st)
	}
}

func TestOldKeysRemoved(t *testing.T) {
	m, path := newTestModel(t, "# C\n- [ ] a\n")
	m = selectTitle(t, m, "a")
	before := storeRev(t, m)
	m = press(m, "p", ">")
	if storeRev(t, m) != before {
		t.Errorf("p and > must not write")
	}
	if st := storedItem(t, path, "a").Status; st != todo.Open {
		t.Errorf("a = %v, want open", st)
	}
}

func TestEmptyTabKeysAreNoops(t *testing.T) {
	m, _ := newTestModelOn(t, "# C\n- [ ] a\n", todo.ColNext)
	if len(m.tree.rows) != 0 {
		t.Fatalf("precondition: Next must be empty, rows = %v", rowTitles(m.tree))
	}
	before := storeRev(t, m)
	for _, key := range []string{"]", "[", "t", "x", " ", "e", "d", "m"} {
		got := press(m, key)
		if got.mode != modeMain || got.tree.grabbed {
			t.Errorf("%q on an empty tab changed mode %v / grabbed %v", key, got.mode, got.tree.grabbed)
		}
		if storeRev(t, got) != before {
			t.Errorf("%q on an empty tab wrote to the store", key)
		}
	}
}

func TestTreeRowStatusGlyphs(t *testing.T) {
	d := parseDoc(t, "# Work\n\n- [/] working\n- [>] defer\n")
	tr := newTree(d)
	if got := tr.rowString(treeRow{item: find(d, "working")}, false); !strings.Contains(got, "◐") {
		t.Errorf("a working row should contain ◐, got %q", got)
	}
	if got := tr.rowString(treeRow{item: find(d, "defer")}, false); !strings.Contains(got, "⏸") {
		t.Errorf("a deferred row should contain ⏸, got %q", got)
	}
}

func TestStatusGlyphTable(t *testing.T) {
	cases := []struct {
		st      todo.Status
		subtask bool
		want    string
	}{
		{todo.Open, false, "○"},
		{todo.Deferred, false, "⏸"},
		{todo.New, false, "◌"},
		{todo.Ready, false, "◉"},
		{todo.Working, false, "◐"},
		{todo.Review, false, "◑"},
		{todo.Followup, false, "◒"},
		{todo.Done, false, "●"},
		{todo.Open, true, "○"},
		{todo.Done, true, "●"},
	}
	for _, c := range cases {
		if got, _ := statusGlyph(c.st, c.subtask); got != c.want {
			t.Errorf("statusGlyph(%v, subtask %v) = %q, want %q", c.st, c.subtask, got, c.want)
		}
	}
}

func TestTaskRowShowsSubtaskProgress(t *testing.T) {
	d := parseDoc(t, "# Work\n\n- [ ] a\n  - [x] s1\n  - [ ] s2\n- [ ] b\n")
	tr := newTree(d)
	if got := tr.rowString(treeRow{item: find(d, "a")}, false); !strings.Contains(got, "open") || !strings.Contains(got, "(1/2)") {
		t.Errorf("a task row must show its subtask progress after the state, got %q", got)
	}
	if got := tr.rowString(treeRow{item: find(d, "b")}, false); strings.Contains(got, "(") {
		t.Errorf("a task without subtasks shows no progress, got %q", got)
	}
	if got := tr.rowString(treeRow{item: find(d, "s1")}, false); strings.Contains(got, "(") {
		t.Errorf("a subtask row shows no progress, got %q", got)
	}
}

func TestItemMetaStatus(t *testing.T) {
	d := todotest.Parse(t, "# Work\n\n- [/] working\n- [>] defer\n")
	// itemMeta still labels working as "in progress"; Task 9 reworks the dialog.
	if got := itemMeta(find(d, "working")); !strings.Contains(got, "in progress") {
		t.Errorf("itemMeta for a working task should say 'in progress', got %q", got)
	}
	if got := itemMeta(find(d, "defer")); !strings.Contains(got, "deferred") {
		t.Errorf("itemMeta for a deferred task should say 'deferred', got %q", got)
	}
}

func TestFooterShowsBoardHints(t *testing.T) {
	m, _ := newTestModel(t, "# Work\n\n- [ ] a\n")
	got := ansi.Strip(m.footer(300))
	for _, want := range []string{"tab: Tab", "]/[: Column", "t: State", "space: Done", "q: Quit"} {
		if !strings.Contains(got, want) {
			t.Errorf("footer should hint %q, got %q", want, got)
		}
	}
	for _, gone := range []string{"Prog", "Defer"} {
		if strings.Contains(got, gone) {
			t.Errorf("footer must not hint the removed %q key, got %q", gone, got)
		}
	}
}
