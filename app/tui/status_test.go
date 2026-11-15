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
		t.Fatalf("%q is not shown on %s (rows %v)", title, m.view.column.Title(), rowTitles(m.tree))
	}
	return m
}

func TestBracketMovesToNextColumn(t *testing.T) {
	m, path := newTestModel(t, "# C\n- [ ] a\n- [ ] b\n")
	m = press(m, "down") // a, row 1
	m = press(m, "p")
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
	m = press(m, "p")
	if st := storedItem(t, path, "a").Status; st != todo.Working {
		t.Errorf("] on a Next task stored %v, want working", st)
	}
}

func TestBracketBackFromDoneIsWorking(t *testing.T) {
	m, path := newTestModelOn(t, "# C\n- [x] d\n  - [ ] s\n", todo.ColDone)
	m = selectTitle(t, m, "d")
	m = press(m, "P")
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
	m = press(m, "p")
	for _, title := range []string{"r", "s1", "s2"} {
		if st := storedItem(t, path, title).Status; st != todo.Done {
			t.Fatalf("%s = %v after ] into Done, want done", title, st)
		}
	}
	m = showOn(t, m, todo.ColDone, "r")
	press(m, "t", "enter") // t starts on the state r had before: enter undoes
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
	m = press(m, "p") // into Done: snapshot, cascade
	m = showOn(t, m, todo.ColDone, "r")
	m = press(m, "P") // out of Done: parent working, subtasks from the snapshot
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
	for _, key := range []string{"p", "P"} {
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
	got := press(m, "P")
	if got.status == "" || storeRev(t, got) != before {
		t.Errorf("[ on Backlog must only hint, status %q", got.status)
	}
	m = showOn(t, m, todo.ColDone, "d")
	got = press(m, "p")
	if got.status == "" || storeRev(t, got) != before {
		t.Errorf("] on Done must only hint, status %q", got.status)
	}
}

func TestBracketUnknownStatusIsBacklog(t *testing.T) {
	m, path := newTestModel(t, "# C\n- [ ] a\n")
	m = setStatusExternally(t, m, path, "a", todo.Status(9))
	m = selectTitle(t, m, "a")
	before := storeRev(t, m)
	got := press(m, "P")
	if got.status != "no column beyond Backlog" || storeRev(t, got) != before {
		t.Errorf("[ on an unknown status must hint as at Backlog's edge, status %q", got.status)
	}
	press(m, "p")
	if st := storedItem(t, path, "a").Status; st != todo.New {
		t.Errorf("] on an unknown status stored %v, want new", st)
	}
}

func TestSpaceAndXNeverChangeState(t *testing.T) {
	m, path := newTestModel(t, "# C\n- [ ] a\n  - [ ] s\n- [/] w\n")
	before := storeRev(t, m)
	for _, sel := range []struct {
		col   todo.Column
		title string
	}{{todo.ColBacklog, "a"}, {todo.ColBacklog, "s"}, {todo.ColInProgress, "w"}} {
		m = showOn(t, m, sel.col, sel.title)
		m = press(m, "space", "x")
		if storeRev(t, m) != before {
			t.Fatalf("space/x on %s must not write", sel.title)
		}
	}
	if st := storedItem(t, path, "w").Status; st != todo.Working {
		t.Errorf("w = %v, want working", st)
	}
}

func TestStatePickerSetsState(t *testing.T) {
	m, path := newTestModel(t, "# C\n- [ ] a\n")
	m = selectTitle(t, m, "a")
	m = press(m, "t")
	if m.mode != modeStatePick || m.pick.selected() != todo.Open {
		t.Fatalf("t should open the picker on the current state, mode %v, on %v", m.mode, m.pick.selected())
	}
	// From open: deferred, won't do, then the Next heading is skipped to new, ready.
	m = press(m, "down", "down", "down", "down")
	if got := m.pick.selected(); got != todo.Ready {
		t.Fatalf("cursor on %v, want ready", got)
	}
	m = press(m, "enter")
	if m.mode != modeMain {
		t.Errorf("enter must close the picker, mode %v", m.mode)
	}
	if st := storedItem(t, path, "a").Status; st != todo.Ready {
		t.Errorf("picker stored %v, want ready", st)
	}
	if !strings.Contains(m.footer(200), "moved to Next — ready") {
		t.Errorf("footer = %q, want the move notice", m.footer(200))
	}
}

func TestStatePickerDoneAndCancel(t *testing.T) {
	m, path := newTestModel(t, "# C\n- [ ] a\n  - [ ] s\n")
	m = selectTitle(t, m, "a")
	before := storeRev(t, m)
	m = press(m, "t", "down", "esc")
	if m.mode != modeMain || storeRev(t, m) != before {
		t.Fatalf("esc must cancel without writing")
	}
	m = press(m, "t")
	for range 20 {
		m = press(m, "down") // stops on the last state, done
	}
	m = press(m, "enter")
	for _, title := range []string{"a", "s"} {
		if st := storedItem(t, path, title).Status; st != todo.Done {
			t.Errorf("%s = %v, want done (picking done completes with the cascade)", title, st)
		}
	}
}

func TestStatePickerSubtaskOffersOpenDone(t *testing.T) {
	m, path := newTestModel(t, "# C\n- [ ] a\n  - [ ] s\n")
	m = selectTitle(t, m, "s")
	m = press(m, "t")
	if len(m.pick.rows) != 2 {
		t.Fatalf("a subtask picker offers open and done only, got %d rows", len(m.pick.rows))
	}
	m = press(m, "down", "enter")
	if st := storedItem(t, path, "s").Status; st != todo.Done {
		t.Errorf("s = %v, want done", st)
	}
}

func TestStateFilter(t *testing.T) {
	m, _ := newTestModel(t, "# C\n- [ ] a\n- [>] b\n# Empty\n")
	m = press(m, "f")
	if m.mode != modeStateFilter || len(m.pick.rows) != 3 {
		t.Fatalf("f should open the Backlog state filter, mode %v, rows %d", m.mode, len(m.pick.rows))
	}
	m = press(m, "down", "space", "enter") // tick deferred
	if got := rowTitles(m.tree); len(got) != 2 || got[0] != "C" || got[1] != "b" {
		t.Fatalf("filtered rows = %v, want [C b]", got)
	}
	if got := ansi.Strip(m.footer(200)); !strings.Contains(got, "states: deferred") {
		t.Errorf("footer should show the state filter, got %q", got)
	}
	// Other tabs ignore Backlog's states.
	m.setColumn(todo.ColDone)
	if len(m.view.activeStates()) != 0 {
		t.Errorf("Done must not inherit Backlog's state filter")
	}
	m.setColumn(todo.ColBacklog)
	m = press(m, "esc")
	if len(m.view.activeStates()) != 0 || !hasRow(m, "a") {
		t.Errorf("esc must clear the state filter, rows %v", rowTitles(m.tree))
	}
}

func TestUndoAccidentalCompleteRestoresExactState(t *testing.T) {
	m, path := newTestModel(t, "# C\n- [ ] a\n")
	m = setStatusExternally(t, m, path, "a", todo.New)
	m = showOn(t, m, todo.ColNext, "a")
	m = pickState(t, m, todo.Done)
	if st := storedItem(t, path, "a").Status; st != todo.Done {
		t.Fatalf("picking done stored %v, want done", st)
	}
	if got := m.footer(200); !strings.Contains(got, "moved to Done — done") {
		t.Errorf("footer = %q", got)
	}
	m = showOn(t, m, todo.ColDone, "a")
	press(m, "t", "enter") // t starts on the state a had before: enter undoes
	if st := storedItem(t, path, "a").Status; st != todo.New {
		t.Errorf("undo must restore a to new, got %v", st)
	}
}

func TestReopenWithoutSnapshotKeepsSubtasks(t *testing.T) {
	m, path := newTestModelOn(t, "# C\n- [x] d\n  - [x] s\n", todo.ColDone)
	m = selectTitle(t, m, "d")
	if m2 := press(m, "t"); m2.pick.selected() != todo.Done {
		t.Errorf("without a snapshot t starts on the current state, got %v", m2.pick.selected())
	}
	pickState(t, m, todo.Working)
	if st := storedItem(t, path, "d").Status; st != todo.Working {
		t.Errorf("reopening a done task stored %v, want working", st)
	}
	if st := storedItem(t, path, "s").Status; st != todo.Done {
		t.Errorf("its subtask keeps its state, got %v", st)
	}
}

// A subtask another process adds before the next poll is not in the TUI's
// tree, but completing its task must still cascade to it: the store reads the
// tree at write time.
func TestCompleteCascadesToSubtaskAddedExternally(t *testing.T) {
	m, path := newTestModel(t, "# C\n- [ ] a\n")
	a := find(m.doc, "a").ID
	if _, err := openSecond(t, path).AddTask(context.Background(), todo.Fields{Title: "s"}, todo.Open, a, 0); err != nil {
		t.Fatalf("external add: %v", err)
	}
	m = selectTitle(t, m, "a")
	pickState(t, m, todo.Done)
	if st := storedItem(t, path, "a").Status; st != todo.Done {
		t.Fatalf("completing a stored %v, want done", st)
	}
	if st := storedItem(t, path, "s").Status; st != todo.Done {
		t.Errorf("the externally added subtask = %v, want done (completing a parent completes its subtasks)", st)
	}
}

func TestSubtaskToggle(t *testing.T) {
	m, path := newTestModel(t, "# C\n- [ ] a\n  - [ ] s\n")
	m = selectTitle(t, m, "s")
	m = pickState(t, m, todo.Done)
	if st := storedItem(t, path, "s").Status; st != todo.Done {
		t.Fatalf("picking done on a subtask stored %v, want done", st)
	}
	if st := storedItem(t, path, "a").Status; st != todo.Open {
		t.Errorf("completing a subtask must leave its task alone, got %v", st)
	}
	pickState(t, m, todo.Open)
	if st := storedItem(t, path, "s").Status; st != todo.Open {
		t.Errorf("picking open stored %v, want open", st)
	}
}

func TestOldKeysRemoved(t *testing.T) {
	m, path := newTestModel(t, "# C\n- [ ] a\n")
	m = selectTitle(t, m, "a")
	before := storeRev(t, m)
	m = press(m, "]", "[", ">")
	if storeRev(t, m) != before {
		t.Errorf("], [ and > must not write")
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
	for _, key := range []string{"p", "P", "t", "x", " ", "e", "d", "m"} {
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
	tr := newTree(d, newViewState())
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
	tr := newTree(d, newViewState())
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
	if got := itemMeta(find(d, "working")); !strings.Contains(got, "In Progress · working") {
		t.Errorf("itemMeta for a working task should say 'In Progress · working', got %q", got)
	}
	if got := itemMeta(find(d, "defer")); !strings.Contains(got, "Backlog · deferred") {
		t.Errorf("itemMeta for a deferred task should say 'deferred', got %q", got)
	}
}

func TestHelpShowsBoardKeys(t *testing.T) {
	got := ansi.Strip(helpModal(200, 60, 0))
	for _, want := range []string{"Next / previous tab", "Move the task to the next / previous column", "Set the task's state", "Filter the tab by state", "Fold / unfold a category", "Quit"} {
		if !strings.Contains(got, want) {
			t.Errorf("help dialog should list %q, got:\n%s", want, got)
		}
	}
}

func TestReopenFromSnapshotIntoPickedState(t *testing.T) {
	m, path := newTestModel(t, "# C\n- [ ] r\n  - [ ] s1\n  - [x] s2\n")
	m = setStatusExternally(t, m, path, "r", todo.Review)
	m = showOn(t, m, todo.ColInProgress, "r")
	m = press(m, "p") // into Done: snapshot, cascade
	if len(m.snapshots) != 1 {
		t.Fatalf("completing r must take one snapshot, got %d", len(m.snapshots))
	}
	m = showOn(t, m, todo.ColDone, "r")
	m = pickState(t, m, todo.New) // out of Done into the picked state
	if st := storedItem(t, path, "r").Status; st != todo.New {
		t.Errorf("picking new out of Done must store r as new, got %v", st)
	}
	if st := storedItem(t, path, "s1").Status; st != todo.Open {
		t.Errorf("s1 must be restored to open, got %v", st)
	}
	if st := storedItem(t, path, "s2").Status; st != todo.Done {
		t.Errorf("s2 must be restored to done, got %v", st)
	}
	if len(m.snapshots) != 0 {
		t.Errorf("leaving Done must consume the snapshot, got %d", len(m.snapshots))
	}
}
