package tui

import (
	"context"
	"strings"
	"testing"

	"github.com/candy-tools/todo/internal/todo"
	"github.com/candy-tools/todo/internal/todo/todotest"
	"github.com/charmbracelet/x/ansi"
)

// board has one task in Backlog, In Progress (with a subtask) and Done, an
// empty category, and nothing in Next.
const board = "# A\n## Empty\n# B\n- [ ] backlogTask\n- [/] working\n  - [ ] sub\n- [x] doneTask\n"

// setStatusExternally sets the task titled title to st through a second store
// handle and polls the change into m. A backlog task set to a sprint state
// first joins the Imported sprint (made when the list has none), so it shows
// on the board toTab opens.
func setStatusExternally(t *testing.T, m model, path, title string, st todo.Status) model {
	t.Helper()
	it := find(m.doc, title)
	if it == nil {
		t.Fatalf("no item %q", title)
	}
	ctx, st2 := context.Background(), openSecond(t, path)
	if st.InSprint() && !it.IsSubtask() && it.Sprint == "" {
		id := importedID(m)
		if _, ok := m.doc.SprintByID(id); !ok {
			var err error
			if id, _, err = st2.AddSprint(ctx, todo.ImportedSprintName, ""); err != nil {
				t.Fatalf("external add sprint: %v", err)
			}
		}
		if _, err := st2.Assign(ctx, it.ID, id); err != nil {
			t.Fatalf("external assign: %v", err)
		}
	}
	if _, err := st2.SetState(ctx, it.ID, st, todo.StateOptions{}); err != nil {
		t.Fatalf("external set status: %v", err)
	}
	return pollOnce(t, m)
}

// setStatusInMemory sets the task titled title to st in m's list only, and
// rebuilds the trees. Tests use it for a status a valid list file cannot hold
// (an unknown one, such as a newer todo's), which the view must still handle.
func setStatusInMemory(t *testing.T, m model, title string, st todo.Status) model {
	t.Helper()
	it := find(m.doc, title)
	if it == nil {
		t.Fatalf("no item %q", title)
	}
	it.Status = st
	m.adoptAll(m.doc, "", false)
	return m
}

// storedItem loads the store at path through a fresh handle and returns the item
// titled title.
func storedItem(t *testing.T, path, title string) *todo.Item {
	t.Helper()
	doc, _, err := openSecond(t, path).Load(context.Background())
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	it := find(doc, title)
	if it == nil {
		t.Fatalf("no stored item %q", title)
	}
	return it
}

// rowOf returns the rendered, unselected row of the item titled title.
func rowOf(t *testing.T, tr tree, title string) string {
	t.Helper()
	for _, r := range tr.rows {
		if r.kind == rowItem && r.item.Title == title {
			return tr.rowString(r, false)
		}
	}
	t.Fatalf("no row %q in %v", title, rowTitles(tr))
	return ""
}

func TestStartsOnBacklog(t *testing.T) {
	m, _ := newTestModel(t, board)
	if m.view.screen != screenBacklog {
		t.Fatalf("start screen = %v, want the Backlog", m.view.screen)
	}
	got := rowTitles(m.tree)
	for _, want := range []string{"A", "Empty", "B", "backlogTask"} {
		if !contains(got, want) {
			t.Errorf("Backlog rows %v miss %q", got, want)
		}
	}
	for _, hidden := range []string{"working", "sub", "doneTask"} {
		if contains(got, hidden) {
			t.Errorf("Backlog rows %v must not show %q", got, hidden)
		}
	}
}

func TestBoardShowsSectionsAndHidesEmptyCategories(t *testing.T) {
	m, _ := newTestModelOn(t, board, todo.ColInProgress)
	for c, want := range map[todo.Column][]string{
		todo.ColNext:       nil,
		todo.ColInProgress: {"working", "sub"},
		todo.ColDone:       {"doneTask"},
	} {
		if got := sectionTitles(m.tree, c); !equalStrings(got, want) {
			t.Errorf("%v rows = %v, want %v", c, got, want)
		}
	}
	var headers []string
	for _, r := range m.tree.rows {
		switch r.kind {
		case rowHeader:
			headers = append(headers, r.label)
		case rowNewCategory:
			t.Errorf("a board must have no placeholder row")
		}
	}
	if want := []string{"B", "B"}; !equalStrings(headers, want) {
		t.Errorf("category headers = %v, want %v (B in In Progress and in Done; no empty category)", headers, want)
	}
}

func TestEmptySprintShowsNothingLine(t *testing.T) {
	m, _ := newJSONModel(t, sprintJSON(t, nil, []seedSprint{{ID: "s00000a", Name: "A"}},
		seedTask{ID: "t000001", Title: "backlogTask", Status: "open"}))
	m = press(m, "s", "tab")
	if out := ansi.Strip(m.View()); !strings.Contains(out, "nothing in this sprint") || strings.Contains(out, "━━ Next") {
		t.Errorf("an empty sprint must say so, without sections:\n%s", out)
	}
	if sel := m.tree.selected(); sel != nil {
		t.Errorf("an empty board must have no selection, got %q", sel.Title)
	}
	// Navigation and the section keys on an empty board are harmless.
	m = press(m, "down", "up", "left", "right", "pgdown", "]", "[")
	if sel := m.tree.selected(); sel != nil {
		t.Errorf("still no selection expected, got %q", sel.Title)
	}
	if m = press(m, "2"); m.status != "nothing in In Progress" {
		t.Errorf("2 on an empty board: footer = %q", m.status)
	}
	for _, key := range []string{"n", "N"} {
		got := press(m, key)
		if got.mode != modeMain {
			t.Errorf("%q on an empty board must not open the form, mode = %v", key, got.mode)
		}
		if want := "add it in the Backlog (b, n), then assign it with a"; got.status != want {
			t.Errorf("%q footer = %q, want %q", key, got.status, want)
		}
	}
}

func TestEmptySectionShowsItsRuleAlone(t *testing.T) {
	m, _ := newTestModelOn(t, board, todo.ColInProgress)
	out := ansi.Strip(m.View())
	// The board's In Progress rule comes after its Next rule; the sidebar's own
	// In Progress section may come before.
	next := strings.Index(out, "━━ Next 0 ━━")
	progress := strings.Index(out[max(next, 0):], "━━ In Progress 1 ━━")
	if next < 0 || progress < 0 {
		t.Fatalf("the board misses its sections:\n%s", out)
	}
	if between := out[next : next+progress]; strings.Contains(between, "working") || strings.Count(between, "\n") != 1 {
		t.Errorf("the empty Next section must be its rule alone:\n%s", out)
	}
}

func TestSearchWithNoMatchesSaysSo(t *testing.T) {
	m, _ := newTestModelOn(t, board, todo.ColInProgress)
	m = press(m, "/")
	m = typeText(m, "zzz")
	m = press(m, "enter")
	out := m.View()
	if !strings.Contains(out, "no matches in this sprint") {
		t.Errorf("an empty search result must say so:\n%s", out)
	}
	if strings.Contains(out, "nothing in") {
		t.Errorf("a search with no matches must not read as an empty sprint:\n%s", out)
	}
}

func TestBoardKeepsOwnSelectionAndFolds(t *testing.T) {
	m, _ := newTestModelOn(t, board, todo.ColInProgress) // on working
	m = press(m, "left")                                 // fold working's subtasks
	if got := rowTitles(m.tree); contains(got, "sub") {
		t.Fatalf("working should be folded, rows = %v", got)
	}
	m = press(m, "b")
	if !contains(rowTitles(m.tree), "backlogTask") {
		t.Fatalf("B must stay expanded on Backlog, rows = %v", rowTitles(m.tree))
	}
	m = press(m, "down", "down", "down") // move the Backlog cursor to backlogTask
	if got := m.tree.selected().Title; got != "backlogTask" {
		t.Fatalf("Backlog selection = %q, want backlogTask", got)
	}
	m = press(m, "k") // back to the board
	if got := rowTitles(m.tree); contains(got, "sub") {
		t.Errorf("back on the board working must still be folded, rows = %v", got)
	}
	if got := m.tree.selected().Title; got != "working" {
		t.Errorf("board selection = %q, want working", got)
	}
	m = press(m, "b")
	if got := m.tree.selected().Title; got != "backlogTask" {
		t.Errorf("back on Backlog selection = %q, want backlogTask", got)
	}
}

func TestSectionCounts(t *testing.T) {
	m, _ := newTestModel(t, board)
	if out := ansi.Strip(m.View()); strings.Contains(out, "━━") {
		t.Errorf("the Backlog has no sections:\n%s", out)
	}
	toTab(&m, todo.ColNext)
	out := ansi.Strip(m.View())
	for _, want := range []string{"━━ Next 0 ━━", "━━ In Progress 1 ━━", "━━ Done 1 ━━"} {
		if !strings.Contains(out, want) {
			t.Errorf("the board misses %q:\n%s", want, out)
		}
	}
	if strings.Contains(m.header(100), "[b Backlog]") {
		t.Errorf("a board's header must not mark the Backlog: %q", m.header(100))
	}
}

func TestNewTaskTakesSectionFirstState(t *testing.T) {
	m, path := newTestModel(t, board)
	m = setStatusExternally(t, m, path, "backlogTask", todo.New)
	toTab(&m, todo.ColNext)
	if got := m.tree.selected().Title; got != "backlogTask" {
		t.Fatalf("Next selection = %q, want backlogTask", got)
	}
	m = press(m, "n")
	m = typeText(m, "idea")
	m = press(m, "enter")
	if it := storedItem(t, path, "idea"); it.Status != todo.New || it.Sprint != m.view.sprint {
		t.Errorf("a task added in Next = %v in sprint %q, want new in %q", it.Status, it.Sprint, m.view.sprint)
	}
	if got := m.tree.selected().Title; got != "idea" {
		t.Errorf("the new task should be selected, got %q", got)
	}
	toSection(&m, todo.ColDone)
	m = press(m, "n")
	m = typeText(m, "finished")
	m = press(m, "enter")
	if it := storedItem(t, path, "finished"); it.Status != todo.Done || it.Sprint != m.view.sprint {
		t.Errorf("a task added in Done = %v in sprint %q, want done in %q", it.Status, it.Sprint, m.view.sprint)
	}
}

func TestNewTaskOnTaskTakesSectionFirstState(t *testing.T) {
	m, path := newTestModelOn(t, board, todo.ColInProgress) // on working
	m = press(m, "n")
	m = typeText(m, "sibling")
	m = press(m, "enter")
	if it := storedItem(t, path, "sibling"); it.Status != todo.Working || it.Sprint != m.view.sprint {
		t.Errorf("a task added in In Progress = %v in sprint %q, want working in %q", it.Status, it.Sprint, m.view.sprint)
	}
	if got := m.tree.selected(); got == nil || got.Title != "sibling" {
		t.Errorf("the new task should be selected on the board, got %v", got)
	}
}

func TestNewSubtaskIsOpen(t *testing.T) {
	m, path := newTestModelOn(t, board, todo.ColInProgress) // on working
	m = press(m, "N")
	m = typeText(m, "child")
	m = press(m, "enter")
	if st := storedItem(t, path, "child").Status; st != todo.Open {
		t.Errorf("a subtask added in In Progress = %v, want open", st)
	}
	// n on a subtask adds a sibling subtask, also open.
	m.tree.selectID(find(m.doc, "child").ID)
	m = press(m, "n")
	m = typeText(m, "child2")
	m = press(m, "enter")
	if it := storedItem(t, path, "child2"); it.Status != todo.Open || !it.IsSubtask() {
		t.Errorf("n on a subtask must add an open subtask, got %v (subtask %v)", it.Status, it.IsSubtask())
	}
}

func TestSubtaskCannotHaveSubtasks(t *testing.T) {
	m, _ := newTestModelOn(t, board, todo.ColInProgress)
	m = press(m, "down") // sub
	if got := m.tree.selected().Title; got != "sub" {
		t.Fatalf("selection = %q, want sub", got)
	}
	m = press(m, "N")
	if m.mode != modeMain {
		t.Fatalf("N on a subtask must not open the form, mode = %v", m.mode)
	}
	if m.status != "subtasks can't have subtasks" {
		t.Errorf("footer = %q", m.status)
	}
}

func TestCategoryKeysBacklogOnly(t *testing.T) {
	m, _ := newTestModelOn(t, board, todo.ColInProgress)
	m = press(m, "c")
	if m.mode != modeMain {
		t.Fatalf("c outside Backlog must not open the form, mode = %v", m.mode)
	}
	if m.status != "categories are managed in Backlog" {
		t.Errorf("footer = %q", m.status)
	}

	// The board's tasks keep their keys.
	if got := press(m, "e"); got.mode != modeForm {
		t.Errorf("e on a task outside Backlog must open the form, mode = %v", got.mode)
	}

	// Back on Backlog the category keys work.
	toTab(&m, todo.ColBacklog)
	if got := press(m, "c"); got.mode != modeForm {
		t.Errorf("c on Backlog must open the form, mode = %v", got.mode)
	}
}

func TestReadyWithoutPlanFlagged(t *testing.T) {
	m, _ := newJSONModel(t, `{"version": 3, "categories": [{"id": "c000000", "title": "B"}], "tasks": [
		{"id": "t000001", "category": "c000000", "title": "r", "status": "ready"},
		{"id": "t000002", "category": "c000000", "title": "planned", "status": "ready", "plan": "docs/plan.md"}]}`)
	toTab(&m, todo.ColNext)
	if row := rowOf(t, m.tree, "r"); !strings.Contains(row, "ready") || !strings.Contains(row, "⚠ no plan") {
		t.Errorf("a ready task without a plan must be flagged: %q", row)
	}
	if row := rowOf(t, m.tree, "planned"); strings.Contains(row, "no plan") {
		t.Errorf("a ready task with a plan must not be flagged: %q", row)
	}
}

func TestTaskRowGlyphsAndStateNames(t *testing.T) {
	m, _ := newJSONModel(t, `{"version": 3, "categories": [{"id": "c000000", "title": "B"}], "tasks": [
		{"id": "t000001", "category": "c000000", "title": "t", "status": "review", "subtasks": [{"id": "s000001", "title": "s"}]}]}`)
	toTab(&m, todo.ColInProgress)
	if row := rowOf(t, m.tree, "t"); !strings.Contains(row, "◑ t") || !strings.Contains(row, "review") {
		t.Errorf("review row = %q", row)
	}
	row := rowOf(t, m.tree, "s")
	if !strings.Contains(row, "○ s") {
		t.Errorf("subtask row = %q", row)
	}
	if strings.Contains(row, "open") {
		t.Errorf("a subtask row shows no state name: %q", row)
	}
}

func TestCategoryWithNothingShownHasNoFold(t *testing.T) {
	m, _ := newTestModel(t, "# A\n- [/] w\n")
	if row := rowOf(t, m.tree, "A"); strings.Contains(row, "▾") {
		t.Errorf("a Backlog category whose tasks are all elsewhere must not show a fold caret: %q", row)
	}
	m = press(m, "left")
	if m.tree.collapsed[find(m.doc, "A").ID] {
		t.Errorf("a category with no shown children must not fold")
	}
	toTab(&m, todo.ColInProgress)
	if got := rowTitles(m.tree); !equalStrings(got, []string{"w"}) {
		t.Errorf("on the board A is a heading, not a row: rows %v", got)
	}
	if m = press(m, "z"); m.status != msgNoFoldOnBoard {
		t.Errorf("z on the board: footer %q", m.status)
	}
}

func TestCategoryCountsOnBacklog(t *testing.T) {
	m, _ := newTestModel(t, board)
	if row := rowOf(t, m.tree, "B"); !strings.Contains(row, "(1/4)") {
		t.Errorf("Backlog category note keeps done/total: %q", row)
	}
}

func TestUnknownStatusRendersInBacklog(t *testing.T) {
	m, _ := newTestModel(t, board)
	m = setStatusInMemory(t, m, "backlogTask", todo.Status(9))
	if row := rowOf(t, m.tree, "backlogTask"); !strings.Contains(row, "status(9)") {
		t.Errorf("an unknown status must render visibly in Backlog: %q", row)
	}
}

func TestExternalMoveFollowsTask(t *testing.T) {
	m, path := newTestModelOn(t, "# B\n- [/] working\n  - [ ] sub\n- [/] other\n", todo.ColInProgress) // on working
	m = setStatusExternally(t, m, path, "working", todo.Done)
	if got := m.tree.selected(); got == nil || got.Title != "working" || m.boardColumn() != todo.ColDone {
		t.Fatalf("selection after the task moved = %v in %v, want working in Done", got, m.boardColumn())
	}
	if got := sectionTitles(m.tree, todo.ColInProgress); !equalStrings(got, []string{"other"}) {
		t.Errorf("the done task must leave In Progress, its rows = %v", got)
	}
}

func TestSearchCarriesAcrossScreens(t *testing.T) {
	m, _ := newTestModel(t, board)
	m = press(m, "/")
	m = typeText(m, "work")
	m = press(m, "enter")
	if got := rowTitles(m.tree); len(got) != 0 {
		t.Errorf("Backlog has no match for \"work\", rows = %v", got)
	}
	m = press(m, "k", "tab") // the Imported sprint's board
	if m.view.filter != "work" {
		t.Fatalf("the filter must carry over, got %q", m.view.filter)
	}
	if got := rowTitles(m.tree); !equalStrings(got, []string{"working", "sub"}) {
		t.Errorf("the board must show only the match, rows = %v", got)
	}
	m = press(m, "esc")
	if m.view.filter != "" {
		t.Errorf("esc must clear the filter, got %q", m.view.filter)
	}
	if got, want := rowTitles(m.tree), []string{"working", "sub", "doneTask"}; !equalStrings(got, want) {
		t.Errorf("rows after clearing = %v, want %v", got, want)
	}
}

func TestEditKeepsPlan(t *testing.T) {
	m, path := newTestModel(t, "# B\n- [ ] a\n")
	a := find(m.doc, "a")
	if _, err := m.store.Edit(context.Background(), a.ID, todo.Changes{Plan: new("docs/a.md")}); err != nil {
		t.Fatal(err)
	}
	m = pollOnce(t, m)
	m.tree.selectID(a.ID)
	m = press(m, "e")
	m = typeText(m, "2")
	m = press(m, "enter")
	it := storedItem(t, path, "a2")
	if it.Plan != "docs/a.md" {
		t.Errorf("an edit must keep the plan, got %q", it.Plan)
	}
}

// An agent setting the plan while the edit form is open must survive the save:
// the form writes only the fields the user changed.
func TestEditKeepsPlanSetWhileFormOpen(t *testing.T) {
	m, path := newTestModel(t, "# B\n- [ ] a\n")
	a := find(m.doc, "a")
	m.tree.selectID(a.ID)
	m = press(m, "e")
	if _, err := openSecond(t, path).Edit(context.Background(), a.ID, todo.Changes{Plan: new("docs/agent.md")}); err != nil {
		t.Fatal(err)
	}
	m = pollOnce(t, m) // deferred: the form is open
	m = typeText(m, "2")
	m = press(m, "enter")
	if it := storedItem(t, path, "a2"); it.Plan != "docs/agent.md" {
		t.Errorf("an edit must keep a plan set while the form was open, got %q", it.Plan)
	}
}

func TestEditSavesPlan(t *testing.T) {
	m, path := newTestModelOn(t, "# B\n- [ ] a\n", todo.ColBacklog)
	a := find(m.doc, "a")
	m = setStatusExternally(t, m, path, "a", todo.Ready)
	toTab(&m, todo.ColNext)
	m.tree.selectID(a.ID)
	if row := rowOf(t, m.tree, "a"); !strings.Contains(row, "⚠ no plan") {
		t.Fatalf("precondition: ready task without plan is flagged: %q", row)
	}
	m = press(m, "e")
	m = press(m, "tab", "tab", "tab") // past the reference and description to the plan
	m = typeText(m, "docs/p.md")
	m = press(m, "enter")
	if it := storedItem(t, path, "a"); it.Plan != "docs/p.md" {
		t.Errorf("plan = %q, want docs/p.md", it.Plan)
	}
	toTab(&m, todo.ColNext)
	if row := rowOf(t, m.tree, "a"); strings.Contains(row, "no plan") {
		t.Errorf("flag should disappear once a plan is set: %q", row)
	}
}

func TestNewTaskWithPlan(t *testing.T) {
	m, path := newTestModel(t, "# B\n- [ ] a\n")
	m = press(m, "n")
	m = typeText(m, "fresh")
	m = press(m, "tab", "tab", "tab") // past the reference and description to the plan
	m = typeText(m, "docs/n.md")
	press(m, "enter")
	if it := storedItem(t, path, "fresh"); it.Plan != "docs/n.md" {
		t.Errorf("new task plan = %q, want docs/n.md", it.Plan)
	}
}

func TestItemMetaShowsColumnAndState(t *testing.T) {
	d := todotest.Parse(t, "# B\n- [ ] r\n")
	it := find(d, "r")
	it.Status = todo.Ready
	got := itemMeta(it)
	if !strings.Contains(got, "Next · ready") || !strings.Contains(got, "⚠ no plan") {
		t.Errorf("meta = %q", got)
	}
	it.Plan = "docs/p.md"
	if got := itemMeta(it); strings.Contains(got, "no plan") {
		t.Errorf("meta with plan = %q", got)
	}
}
