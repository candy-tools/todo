package todo_test

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/candy-tools/todo/internal/todo"
	"github.com/candy-tools/todo/internal/todo/todotest"
)

// opDoc parses md and gives every item an ID, the starting point of every op
// test.
func opDoc(t *testing.T, md string) *todo.Document {
	t.Helper()
	d := todotest.Parse(t, md)
	d.AssignIDs()
	return d
}

// idOf returns the ID of the first item titled title, failing the test when
// there is none.
func idOf(t *testing.T, d *todo.Document, title string) string {
	t.Helper()
	it := find(d, title)
	if it == nil {
		t.Fatalf("no item titled %q", title)
	}
	return it.ID
}

// wantDoc fails the test when d does not render as want.
func wantDoc(t *testing.T, d *todo.Document, want string) {
	t.Helper()
	if got := todotest.Render(d); got != want {
		t.Errorf("document =\n%s\nwant\n%s", got, want)
	}
}

// wantErr fails the test when err is not target.
func wantErr(t *testing.T, err, target error) {
	t.Helper()
	if !errors.Is(err, target) {
		t.Fatalf("err = %v, want %v", err, target)
	}
}

// wantNoErr fails the test on err.
func wantNoErr(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// structure renders d with every item's ID and parent ID, so a comparison also
// catches a change Render cannot show.
func structure(d *todo.Document) string {
	var b strings.Builder
	for _, it := range d.Enumerate() {
		parent := ""
		if it.Parent != nil {
			parent = it.Parent.ID
		}
		fmt.Fprintf(&b, "%s<%s %d %q %s %d %q %q\n", it.ID, parent, it.Kind, it.Title,
			it.Status, it.Level, it.Description, it.Plan)
	}
	return todotest.Render(d) + b.String()
}

// isNewID reports whether id has the shape of a generated ID.
func isNewID(id string) bool {
	const alphabet = "0123456789abcdefghjkmnpqrstvwxyz"
	if len(id) != 7 {
		return false
	}
	for _, r := range id {
		if !strings.ContainsRune(alphabet, r) {
			return false
		}
	}
	return true
}

// --- AddCategory / AppendCategory ---

func TestOpAddCategoryAtRootAfterAnchor(t *testing.T) {
	d := opDoc(t, "# A\n\n# B\n")
	id, err := d.AddCategory("  C  ", "", idOf(t, d, "A"))
	wantNoErr(t, err)
	if !isNewID(id) || idOf(t, d, "C") != id {
		t.Errorf("AddCategory returned %q, want the new category's generated ID", id)
	}
	wantDoc(t, d, "# A\n\n# C\n\n# B\n")
}

func TestOpAddCategoryFirstAtRoot(t *testing.T) {
	d := opDoc(t, "# A\n")
	_, err := d.AddCategory("C", "", "")
	wantNoErr(t, err)
	wantDoc(t, d, "# C\n\n# A\n")
}

func TestOpAddCategoryUnderCategoryTakesLevel(t *testing.T) {
	d := opDoc(t, "# A\n\n## B\n")
	_, err := d.AddCategory("C", idOf(t, d, "B"), "")
	wantNoErr(t, err)
	wantDoc(t, d, "# A\n\n## B\n\n### C\n")
}

func TestOpAddCategoryUnderTaskWrongParent(t *testing.T) {
	d := opDoc(t, "# A\n\n- [ ] x\n")
	_, err := d.AddCategory("C", idOf(t, d, "x"), "")
	wantErr(t, err, todo.ErrWrongParent)
}

func TestOpAddCategoryAfterTaskGoesFirstAmongCategories(t *testing.T) {
	d := opDoc(t, "# A\n\n- [ ] x\n- [ ] y\n\n## B\n")
	_, err := d.AddCategory("C", idOf(t, d, "A"), idOf(t, d, "x"))
	wantNoErr(t, err)
	wantDoc(t, d, "# A\n\n- [ ] x\n- [ ] y\n\n## C\n\n## B\n")
}

func TestOpAddCategoryFirstStaysAfterTasks(t *testing.T) {
	d := opDoc(t, "# A\n\n- [ ] x\n\n## B\n")
	_, err := d.AddCategory("C", idOf(t, d, "A"), "")
	wantNoErr(t, err)
	wantDoc(t, d, "# A\n\n- [ ] x\n\n## C\n\n## B\n")
}

func TestOpAddCategoryAnchorNotChildConflicts(t *testing.T) {
	d := opDoc(t, "# A\n\n## B\n\n# C\n")
	_, err := d.AddCategory("D", idOf(t, d, "C"), idOf(t, d, "B"))
	wantErr(t, err, todo.ErrConflict)
}

func TestOpAddCategoryMissingParentNotFound(t *testing.T) {
	d := opDoc(t, "# A\n")
	_, err := d.AddCategory("C", "zzzzzzz", "")
	wantErr(t, err, todo.ErrNotFound)
	if !strings.Contains(err.Error(), "id zzzzzzz") {
		t.Errorf("err = %v, want it to name the missing id", err)
	}
}

func TestOpAddCategoryInvalidTitle(t *testing.T) {
	d := opDoc(t, "# A\n")
	_, err := d.AddCategory("   ", "", "")
	wantErr(t, err, todo.ErrInvalidTitle)
}

func TestOpAppendCategoryLast(t *testing.T) {
	d := opDoc(t, "# A\n\n- [ ] x\n\n## B\n\n# Z\n")
	_, err := d.AppendCategory("C", idOf(t, d, "A"))
	wantNoErr(t, err)
	_, err = d.AppendCategory("R", "")
	wantNoErr(t, err)
	wantDoc(t, d, "# A\n\n- [ ] x\n\n## B\n\n## C\n\n# Z\n\n# R\n")
}

func TestOpAppendCategoryUnderTaskWrongParent(t *testing.T) {
	d := opDoc(t, "# A\n\n- [ ] x\n")
	_, err := d.AppendCategory("C", idOf(t, d, "x"))
	wantErr(t, err, todo.ErrWrongParent)
}

// --- AddTask / AppendTask ---

func TestOpAddTaskAfterAnchorKeepsFields(t *testing.T) {
	d := opDoc(t, "# A\n\n- [ ] x\n- [ ] z\n")
	f := todo.Fields{Title: " y ", Description: "notes", Plan: "plan.md"}
	id, err := d.AddTask(f, todo.Review, idOf(t, d, "A"), idOf(t, d, "x"))
	wantNoErr(t, err)
	wantDoc(t, d, "# A\n\n- [ ] x\n- [review] y\n  notes\n- [ ] z\n")
	it, _ := d.ByID(id)
	if it == nil || it.Plan != "plan.md" || it.Title != "y" {
		t.Errorf("added task = %+v, want title y and plan plan.md", it)
	}
}

func TestOpAddTaskFirst(t *testing.T) {
	d := opDoc(t, "# A\n\n- [ ] x\n")
	_, err := d.AddTask(todo.Fields{Title: "y"}, todo.Open, idOf(t, d, "A"), "")
	wantNoErr(t, err)
	wantDoc(t, d, "# A\n\n- [ ] y\n- [ ] x\n")
}

func TestOpAddSubtask(t *testing.T) {
	d := opDoc(t, "# A\n\n- [/] x\n")
	_, err := d.AddTask(todo.Fields{Title: "s"}, todo.Done, idOf(t, d, "x"), "")
	wantNoErr(t, err)
	wantDoc(t, d, "# A\n\n- [/] x\n  - [x] s\n")
}

func TestOpAddTaskAtRootWrongParent(t *testing.T) {
	d := opDoc(t, "# A\n")
	_, err := d.AddTask(todo.Fields{Title: "y"}, todo.Open, "", "")
	wantErr(t, err, todo.ErrWrongParent)
}

func TestOpAddTaskUnderSubtaskTooDeep(t *testing.T) {
	d := opDoc(t, "# A\n\n- [ ] x\n  - [ ] s\n")
	_, err := d.AddTask(todo.Fields{Title: "y"}, todo.Open, idOf(t, d, "s"), "")
	wantErr(t, err, todo.ErrTooDeep)
}

func TestOpAddSubtaskBoardStateRejected(t *testing.T) {
	d := opDoc(t, "# A\n\n- [ ] x\n")
	_, err := d.AddTask(todo.Fields{Title: "s"}, todo.Working, idOf(t, d, "x"), "")
	wantErr(t, err, todo.ErrSubtaskStatus)
}

func TestOpAddTaskAfterCategoryConflicts(t *testing.T) {
	d := opDoc(t, "# A\n\n- [ ] x\n\n## B\n")
	_, err := d.AddTask(todo.Fields{Title: "y"}, todo.Open, idOf(t, d, "A"), idOf(t, d, "B"))
	wantErr(t, err, todo.ErrConflict)
}

func TestOpAddTaskAnchorNotChildConflicts(t *testing.T) {
	d := opDoc(t, "# A\n\n- [ ] x\n\n# B\n")
	_, err := d.AddTask(todo.Fields{Title: "y"}, todo.Open, idOf(t, d, "B"), idOf(t, d, "x"))
	wantErr(t, err, todo.ErrConflict)
}

func TestOpAddTaskMissingParentNotFound(t *testing.T) {
	d := opDoc(t, "# A\n")
	_, err := d.AddTask(todo.Fields{Title: "y"}, todo.Open, "zzzzzzz", "")
	wantErr(t, err, todo.ErrNotFound)
}

func TestOpAddTaskInvalidTitle(t *testing.T) {
	d := opDoc(t, "# A\n")
	_, err := d.AddTask(todo.Fields{Title: "a\tb"}, todo.Open, idOf(t, d, "A"), "")
	wantErr(t, err, todo.ErrInvalidTitle)
}

func TestOpAppendTaskBeforeSubcategories(t *testing.T) {
	d := opDoc(t, "# A\n\n- [ ] x\n\n## B\n")
	_, err := d.AppendTask(todo.Fields{Title: "y"}, todo.Open, idOf(t, d, "A"))
	wantNoErr(t, err)
	wantDoc(t, d, "# A\n\n- [ ] x\n- [ ] y\n\n## B\n")
}

func TestOpAppendSubtaskLast(t *testing.T) {
	d := opDoc(t, "# A\n\n- [ ] x\n  - [ ] s\n")
	_, err := d.AppendTask(todo.Fields{Title: "t"}, todo.Open, idOf(t, d, "x"))
	wantNoErr(t, err)
	wantDoc(t, d, "# A\n\n- [ ] x\n  - [ ] s\n  - [ ] t\n")
}

func TestOpAppendTaskRules(t *testing.T) {
	d := opDoc(t, "# A\n\n- [ ] x\n  - [ ] s\n")
	_, err := d.AppendTask(todo.Fields{Title: "y"}, todo.Open, "")
	wantErr(t, err, todo.ErrWrongParent)
	_, err = d.AppendTask(todo.Fields{Title: "y"}, todo.Open, idOf(t, d, "s"))
	wantErr(t, err, todo.ErrTooDeep)
	_, err = d.AppendTask(todo.Fields{Title: "y"}, todo.Deferred, idOf(t, d, "x"))
	wantErr(t, err, todo.ErrSubtaskStatus)
}

func TestOpAddedIDsAreUnique(t *testing.T) {
	d := opDoc(t, "# A\n")
	seen := map[string]bool{idOf(t, d, "A"): true}
	for i := range 50 {
		id, err := d.AppendTask(todo.Fields{Title: fmt.Sprint("t", i)}, todo.Open, idOf(t, d, "A"))
		wantNoErr(t, err)
		if !isNewID(id) || seen[id] {
			t.Fatalf("AppendTask returned %q, want a fresh generated ID", id)
		}
		seen[id] = true
	}
}

// --- Move ---

func TestOpMoveIntoOwnSubtree(t *testing.T) {
	d := opDoc(t, "# A\n\n## B\n\n### C\n")
	err := d.Move(idOf(t, d, "A"), idOf(t, d, "C"), "")
	wantErr(t, err, todo.ErrInvalidMove)
}

func TestOpMoveIntoItself(t *testing.T) {
	d := opDoc(t, "# A\n")
	err := d.Move(idOf(t, d, "A"), idOf(t, d, "A"), "")
	wantErr(t, err, todo.ErrInvalidMove)
}

func TestOpMoveTaskToRootWrongParent(t *testing.T) {
	d := opDoc(t, "# A\n\n- [ ] x\n")
	err := d.Move(idOf(t, d, "x"), "", "")
	wantErr(t, err, todo.ErrWrongParent)
}

func TestOpMoveCategoryUnderTaskWrongParent(t *testing.T) {
	d := opDoc(t, "# A\n\n- [ ] x\n\n# B\n")
	err := d.Move(idOf(t, d, "B"), idOf(t, d, "x"), "")
	wantErr(t, err, todo.ErrWrongParent)
}

func TestOpMoveTaskWithChildrenUnderTaskTooDeep(t *testing.T) {
	d := opDoc(t, "# A\n\n- [ ] x\n  - [ ] s\n- [ ] y\n")
	err := d.Move(idOf(t, d, "x"), idOf(t, d, "y"), "")
	wantErr(t, err, todo.ErrTooDeep)
}

func TestOpMoveTaskUnderSubtaskTooDeep(t *testing.T) {
	d := opDoc(t, "# A\n\n- [ ] x\n  - [ ] s\n- [ ] y\n")
	err := d.Move(idOf(t, d, "y"), idOf(t, d, "s"), "")
	wantErr(t, err, todo.ErrTooDeep)
}

func TestOpMoveSubtaskOutTakesParentStatus(t *testing.T) {
	d := opDoc(t, "# A\n\n- [/] x\n  - [ ] s\n")
	err := d.Move(idOf(t, d, "s"), idOf(t, d, "A"), idOf(t, d, "x"))
	wantNoErr(t, err)
	wantDoc(t, d, "# A\n\n- [/] x\n- [/] s\n")
}

func TestOpMoveSubtaskOutOfDoneParentIsWorking(t *testing.T) {
	d := opDoc(t, "# A\n\n- [x] x\n  - [ ] s\n")
	err := d.Move(idOf(t, d, "s"), idOf(t, d, "A"), "")
	wantNoErr(t, err)
	wantDoc(t, d, "# A\n\n- [/] s\n- [x] x\n")
}

func TestOpMoveTaskUnderTaskOpens(t *testing.T) {
	d := opDoc(t, "# A\n\n- [ ] x\n- [>] y\n- [x] z\n")
	wantNoErr(t, d.Move(idOf(t, d, "y"), idOf(t, d, "x"), ""))
	wantNoErr(t, d.Move(idOf(t, d, "z"), idOf(t, d, "x"), idOf(t, d, "y")))
	wantDoc(t, d, "# A\n\n- [ ] x\n  - [ ] y\n  - [x] z\n")
}

func TestOpMoveSubtaskBetweenTasksStaysSubtask(t *testing.T) {
	d := opDoc(t, "# A\n\n- [/] x\n  - [ ] s\n- [ ] y\n")
	wantNoErr(t, d.Move(idOf(t, d, "s"), idOf(t, d, "y"), ""))
	wantDoc(t, d, "# A\n\n- [/] x\n- [ ] y\n  - [ ] s\n")
}

func TestOpMoveBetweenCategoriesKeepsStatus(t *testing.T) {
	d := opDoc(t, "# A\n\n- [>] x\n  - [ ] s\n\n# B\n\n- [ ] y\n")
	wantNoErr(t, d.Move(idOf(t, d, "x"), idOf(t, d, "B"), idOf(t, d, "y")))
	wantDoc(t, d, "# A\n\n# B\n\n- [ ] y\n- [>] x\n  - [ ] s\n")
}

func TestOpMoveWithinParentReorders(t *testing.T) {
	d := opDoc(t, "# A\n\n- [ ] x\n- [ ] y\n- [ ] z\n")
	wantNoErr(t, d.Move(idOf(t, d, "x"), idOf(t, d, "A"), idOf(t, d, "z")))
	wantNoErr(t, d.Move(idOf(t, d, "z"), idOf(t, d, "A"), ""))
	wantDoc(t, d, "# A\n\n- [ ] z\n- [ ] y\n- [ ] x\n")
}

func TestOpMoveCategoryRelevelsSubtree(t *testing.T) {
	d := opDoc(t, "# A\n\n## B\n\n### C\n\n- [ ] x\n\n# D\n")
	wantNoErr(t, d.Move(idOf(t, d, "B"), "", idOf(t, d, "D")))
	wantDoc(t, d, "# A\n\n# D\n\n# B\n\n## C\n\n- [ ] x\n")
	wantNoErr(t, d.Move(idOf(t, d, "A"), idOf(t, d, "C"), ""))
	wantDoc(t, d, "# D\n\n# B\n\n## C\n\n- [ ] x\n\n### A\n")
}

func TestOpMoveCategoryAfterTaskGoesFirstAmongCategories(t *testing.T) {
	d := opDoc(t, "# A\n\n- [ ] x\n\n## B\n\n# C\n")
	wantNoErr(t, d.Move(idOf(t, d, "C"), idOf(t, d, "A"), idOf(t, d, "x")))
	wantDoc(t, d, "# A\n\n- [ ] x\n\n## C\n\n## B\n")
}

func TestOpMoveTaskAfterCategoryConflicts(t *testing.T) {
	d := opDoc(t, "# A\n\n- [ ] x\n\n## B\n\n# C\n\n- [ ] y\n")
	err := d.Move(idOf(t, d, "y"), idOf(t, d, "A"), idOf(t, d, "B"))
	wantErr(t, err, todo.ErrConflict)
}

func TestOpMoveAnchorGoneConflicts(t *testing.T) {
	d := opDoc(t, "# A\n\n- [ ] x\n- [ ] y\n\n# B\n")
	anchor := idOf(t, d, "x")
	wantNoErr(t, d.Delete(anchor))
	err := d.Move(idOf(t, d, "y"), idOf(t, d, "B"), anchor)
	wantErr(t, err, todo.ErrConflict)
}

func TestOpMoveAfterItselfConflicts(t *testing.T) {
	d := opDoc(t, "# A\n\n- [ ] x\n- [ ] y\n")
	err := d.Move(idOf(t, d, "y"), idOf(t, d, "A"), idOf(t, d, "y"))
	wantErr(t, err, todo.ErrConflict)
}

func TestOpMoveMissingNotFound(t *testing.T) {
	d := opDoc(t, "# A\n\n- [ ] x\n")
	err := d.Move("zzzzzzz", idOf(t, d, "A"), "")
	wantErr(t, err, todo.ErrNotFound)
	err = d.Move(idOf(t, d, "x"), "zzzzzzz", "")
	wantErr(t, err, todo.ErrNotFound)
}

// --- Edit ---

func TestOpEditSetsTaskFields(t *testing.T) {
	d := opDoc(t, "# A\n\n- [ ] x\n  old\n")
	title, desc, plan := " y ", "new", "p.md"
	changed, err := d.Edit(idOf(t, d, "x"), todo.Changes{Title: &title, Description: &desc, Plan: &plan})
	wantNoErr(t, err)
	if !changed {
		t.Error("changed = false, want true")
	}
	wantDoc(t, d, "# A\n\n- [ ] y\n  new\n")
	if it := find(d, "y"); it.Plan != "p.md" {
		t.Errorf("plan = %q, want p.md", it.Plan)
	}
}

func TestOpEditKeepsNilFields(t *testing.T) {
	d := opDoc(t, "# A\n\n- [ ] x\n  old\n")
	plan := "p.md"
	changed, err := d.Edit(idOf(t, d, "x"), todo.Changes{Plan: &plan})
	wantNoErr(t, err)
	if !changed {
		t.Error("changed = false, want true")
	}
	wantDoc(t, d, "# A\n\n- [ ] x\n  old\n")
}

func TestOpEditNoChange(t *testing.T) {
	d := opDoc(t, "# A\n\n- [ ] x\n  old\n")
	before := structure(d)
	title, desc := "  x ", "old"
	changed, err := d.Edit(idOf(t, d, "x"), todo.Changes{Title: &title, Description: &desc})
	wantNoErr(t, err)
	if changed {
		t.Error("changed = true, want false")
	}
	changed, err = d.Edit(idOf(t, d, "x"), todo.Changes{})
	wantNoErr(t, err)
	if changed {
		t.Error("changed = true for empty changes, want false")
	}
	if got := structure(d); got != before {
		t.Errorf("document changed:\n%s\nwant\n%s", got, before)
	}
}

func TestOpEditCategoryIgnoresDescription(t *testing.T) {
	d := opDoc(t, "# A\n")
	desc, plan := "notes", "p.md"
	changed, err := d.Edit(idOf(t, d, "A"), todo.Changes{Description: &desc, Plan: &plan})
	wantNoErr(t, err)
	if changed {
		t.Error("changed = true, want false")
	}
	if it := find(d, "A"); it.Description != "" || it.Plan != "" {
		t.Errorf("category = %+v, want no description or plan", it)
	}
}

func TestOpEditCategoryTitle(t *testing.T) {
	d := opDoc(t, "# A\n\n## B\n")
	title := "C"
	changed, err := d.Edit(idOf(t, d, "B"), todo.Changes{Title: &title})
	wantNoErr(t, err)
	if !changed {
		t.Error("changed = false, want true")
	}
	wantDoc(t, d, "# A\n\n## C\n")
}

func TestOpEditInvalidTitle(t *testing.T) {
	d := opDoc(t, "# A\n")
	title := ""
	_, err := d.Edit(idOf(t, d, "A"), todo.Changes{Title: &title})
	wantErr(t, err, todo.ErrInvalidTitle)
}

func TestOpEditMissingNotFound(t *testing.T) {
	d := opDoc(t, "# A\n")
	title := "B"
	_, err := d.Edit("zzzzzzz", todo.Changes{Title: &title})
	wantErr(t, err, todo.ErrNotFound)
}

// --- SetStatuses / CascadeStatus / Complete ---

func TestOpSetStatusesApplies(t *testing.T) {
	d := opDoc(t, "# A\n\n- [ ] x\n  - [ ] s\n- [ ] y\n")
	err := d.SetStatuses(todo.DoneStates{idOf(t, d, "x"): todo.Review, idOf(t, d, "s"): todo.Done})
	wantNoErr(t, err)
	wantDoc(t, d, "# A\n\n- [review] x\n  - [x] s\n- [ ] y\n")
}

func TestOpSetStatusesAllOrNothing(t *testing.T) {
	d := opDoc(t, "# A\n\n- [ ] x\n")
	err := d.SetStatuses(todo.DoneStates{idOf(t, d, "x"): todo.Done, "zzzzzzz": todo.Done})
	wantErr(t, err, todo.ErrNotFound)
	wantDoc(t, d, "# A\n\n- [ ] x\n")
}

func TestOpSetStatusesCategoryNotTask(t *testing.T) {
	d := opDoc(t, "# A\n\n- [ ] x\n")
	err := d.SetStatuses(todo.DoneStates{idOf(t, d, "x"): todo.Done, idOf(t, d, "A"): todo.Done})
	wantErr(t, err, todo.ErrNotTask)
	wantDoc(t, d, "# A\n\n- [ ] x\n")
}

func TestOpSetStatusesSubtaskBoardState(t *testing.T) {
	d := opDoc(t, "# A\n\n- [ ] x\n  - [ ] s\n")
	err := d.SetStatuses(todo.DoneStates{idOf(t, d, "x"): todo.Done, idOf(t, d, "s"): todo.Working})
	wantErr(t, err, todo.ErrSubtaskStatus)
	wantDoc(t, d, "# A\n\n- [ ] x\n  - [ ] s\n")
}

func TestOpCascadeBoardStateOpensSubtasks(t *testing.T) {
	d := opDoc(t, "# A\n\n- [ ] x\n  - [x] s\n  - [ ] t\n")
	wantNoErr(t, d.CascadeStatus(idOf(t, d, "x"), todo.Working))
	wantDoc(t, d, "# A\n\n- [/] x\n  - [ ] s\n  - [ ] t\n")
}

func TestOpCascadeDoneCompletesSubtree(t *testing.T) {
	d := opDoc(t, "# A\n\n- [ ] x\n  - [ ] s\n- [ ] y\n")
	wantNoErr(t, d.CascadeStatus(idOf(t, d, "x"), todo.Done))
	wantDoc(t, d, "# A\n\n- [x] x\n  - [x] s\n- [ ] y\n")
}

func TestOpCascadeRules(t *testing.T) {
	d := opDoc(t, "# A\n\n- [ ] x\n  - [ ] s\n")
	wantErr(t, d.CascadeStatus(idOf(t, d, "A"), todo.Done), todo.ErrNotTask)
	wantErr(t, d.CascadeStatus(idOf(t, d, "s"), todo.Working), todo.ErrSubtaskStatus)
	wantErr(t, d.CascadeStatus("zzzzzzz", todo.Done), todo.ErrNotFound)
	wantNoErr(t, d.CascadeStatus(idOf(t, d, "s"), todo.Done))
	wantDoc(t, d, "# A\n\n- [ ] x\n  - [x] s\n")
}

func TestOpComplete(t *testing.T) {
	d := opDoc(t, "# A\n\n- [ ] x\n  - [ ] s\n- [/] y\n  - [ ] t\n")
	wantNoErr(t, d.Complete(idOf(t, d, "x"), false))
	wantNoErr(t, d.Complete(idOf(t, d, "y"), true))
	wantDoc(t, d, "# A\n\n- [x] x\n  - [ ] s\n- [x] y\n  - [x] t\n")
	wantErr(t, d.Complete(idOf(t, d, "A"), false), todo.ErrNotTask)
	wantErr(t, d.Complete(idOf(t, d, "A"), true), todo.ErrNotTask)
}

// --- Reopen ---

func TestOpReopenKeepsSubtasks(t *testing.T) {
	d := opDoc(t, "# A\n\n- [x] x\n  - [x] s\n")
	wantNoErr(t, d.Reopen(idOf(t, d, "x"), todo.Working, todo.ReopenOptions{}))
	wantDoc(t, d, "# A\n\n- [/] x\n  - [x] s\n")
}

func TestOpReopenCascadeOpensSubtasks(t *testing.T) {
	d := opDoc(t, "# A\n\n- [x] x\n  - [x] s\n")
	wantNoErr(t, d.Reopen(idOf(t, d, "x"), todo.Working, todo.ReopenOptions{Cascade: true}))
	wantDoc(t, d, "# A\n\n- [/] x\n  - [ ] s\n")
}

func TestOpReopenRestoreSkipsDeleted(t *testing.T) {
	d := opDoc(t, "# A\n\n- [ ] x\n  - [ ] s\n  - [x] t\n  - [ ] u\n")
	x := find(d, "x")
	snap := todo.SnapshotDone(x)
	wantNoErr(t, d.CascadeStatus(x.ID, todo.Done))
	wantNoErr(t, d.Delete(idOf(t, d, "u")))
	err := d.Reopen(x.ID, todo.Working, todo.ReopenOptions{Restore: snap})
	wantNoErr(t, err)
	wantDoc(t, d, "# A\n\n- [/] x\n  - [ ] s\n  - [x] t\n")
}

func TestOpReopenRestoreAndCascade(t *testing.T) {
	d := opDoc(t, "# A\n\n- [x] x\n")
	o := todo.ReopenOptions{Restore: todo.DoneStates{}, Cascade: true}
	wantErr(t, d.Reopen(idOf(t, d, "x"), todo.Working, o), todo.ErrRestoreAndCascade)
	wantErr(t, d.Reopen("zzzzzzz", todo.Working, o), todo.ErrRestoreAndCascade)
}

func TestOpReopenRules(t *testing.T) {
	d := opDoc(t, "# A\n\n- [x] x\n  - [x] s\n")
	wantErr(t, d.Reopen("zzzzzzz", todo.Working, todo.ReopenOptions{}), todo.ErrNotFound)
	wantErr(t, d.Reopen(idOf(t, d, "A"), todo.Working, todo.ReopenOptions{}), todo.ErrNotTask)
	wantErr(t, d.Reopen(idOf(t, d, "s"), todo.Working, todo.ReopenOptions{}), todo.ErrSubtaskStatus)
	wantErr(t, d.Reopen(idOf(t, d, "s"), todo.Working, todo.ReopenOptions{Cascade: true}), todo.ErrSubtaskStatus)
	wantDoc(t, d, "# A\n\n- [x] x\n  - [x] s\n")
}

// --- SetState ---

// setState runs d.SetState and returns its error, failing the test when an
// op that failed still reports a change.
func setState(t *testing.T, d *todo.Document, id string, st todo.Status, o todo.StateOptions) error {
	t.Helper()
	changed, err := d.SetState(id, st, o)
	if err != nil && changed {
		t.Fatalf("SetState failed (%v) but reported a change", err)
	}
	return err
}

func TestOpSetStateIntoDoneCompletes(t *testing.T) {
	d := opDoc(t, "# A\n\n- [ ] x\n  - [ ] s\n- [/] y\n  - [ ] t\n")
	wantNoErr(t, setState(t, d, idOf(t, d, "x"), todo.Done, todo.StateOptions{}))
	wantNoErr(t, setState(t, d, idOf(t, d, "y"), todo.Done, todo.StateOptions{Cascade: true}))
	wantDoc(t, d, "# A\n\n- [x] x\n  - [ ] s\n- [x] y\n  - [x] t\n")
}

// Out of Done the task reopens: the subtasks keep their states, or are all
// opened with Cascade.
func TestOpSetStateOutOfDoneReopens(t *testing.T) {
	d := opDoc(t, "# A\n\n- [x] x\n  - [x] s\n- [x] y\n  - [x] t\n")
	wantNoErr(t, setState(t, d, idOf(t, d, "x"), todo.Working, todo.StateOptions{}))
	wantNoErr(t, setState(t, d, idOf(t, d, "y"), todo.Deferred, todo.StateOptions{Cascade: true}))
	wantDoc(t, d, "# A\n\n- [/] x\n  - [x] s\n- [>] y\n  - [ ] t\n")
}

// Out of Done with Restore the subtasks the complete left done get their
// snapshot states back; one changed since keeps its state.
func TestOpSetStateOutOfDoneRestores(t *testing.T) {
	d := opDoc(t, "# A\n\n- [/] x\n  - [ ] s\n  - [x] t\n  - [ ] u\n")
	x := find(d, "x")
	snap := todo.SnapshotDone(x)
	wantNoErr(t, setState(t, d, x.ID, todo.Done, todo.StateOptions{Cascade: true}))
	wantNoErr(t, d.SetStatuses(todo.DoneStates{idOf(t, d, "u"): todo.Open})) // changed since
	snap[idOf(t, d, "u")] = todo.Done
	wantNoErr(t, setState(t, d, x.ID, todo.Review, todo.StateOptions{Restore: snap}))
	wantDoc(t, d, "# A\n\n- [review] x\n  - [ ] s\n  - [x] t\n  - [ ] u\n")
}

// A task not in Done, set to another state that is not Done, just takes it:
// Restore does not apply (there is no complete to undo), Cascade opens the
// subtasks.
func TestOpSetStatePlain(t *testing.T) {
	d := opDoc(t, "# A\n\n- [ ] x\n  - [x] s\n- [ ] y\n  - [x] t\n")
	x, s := idOf(t, d, "x"), idOf(t, d, "s")
	wantNoErr(t, setState(t, d, x, todo.Working, todo.StateOptions{Restore: todo.DoneStates{s: todo.Open}}))
	wantNoErr(t, setState(t, d, idOf(t, d, "y"), todo.Ready, todo.StateOptions{Cascade: true}))
	wantDoc(t, d, "# A\n\n- [/] x\n  - [x] s\n- [ready] y\n  - [ ] t\n")
}

// Cascade applies to every change, not only into or out of Done: a plain
// change of a task that is not done opens its subtasks, which `todo reopen
// --cascade` on a task that is not done relies on.
func TestOpSetStateCascadeOnPlainChangeOpensSubtasks(t *testing.T) {
	d := opDoc(t, "# A\n\n- [/] x\n  - [x] s\n  - [x] t\n")
	wantNoErr(t, setState(t, d, idOf(t, d, "x"), todo.Deferred, todo.StateOptions{Cascade: true}))
	wantDoc(t, d, "# A\n\n- [>] x\n  - [ ] s\n  - [ ] t\n")
	changed, err := d.SetState(idOf(t, d, "x"), todo.Deferred, todo.StateOptions{Cascade: true})
	if err != nil || changed {
		t.Errorf("the same cascade again = %v, %v; want no change", changed, err)
	}
}

func TestOpSetStateSubtask(t *testing.T) {
	d := opDoc(t, "# A\n\n- [/] x\n  - [ ] s\n")
	s := idOf(t, d, "s")
	wantNoErr(t, setState(t, d, s, todo.Done, todo.StateOptions{}))
	wantDoc(t, d, "# A\n\n- [/] x\n  - [x] s\n")
	wantNoErr(t, setState(t, d, s, todo.Open, todo.StateOptions{}))
	wantDoc(t, d, "# A\n\n- [/] x\n  - [ ] s\n")
}

func TestOpSetStateNoChange(t *testing.T) {
	d := opDoc(t, "# A\n\n- [/] x\n  - [x] s\n")
	for _, o := range []todo.StateOptions{{}, {Restore: todo.DoneStates{}}} {
		changed, err := d.SetState(idOf(t, d, "x"), todo.Working, o)
		if err != nil || changed {
			t.Errorf("SetState to the current state (%+v) = %v, %v; want no change", o, changed, err)
		}
	}
	if changed, err := d.SetState(idOf(t, d, "x"), todo.Working, todo.StateOptions{Cascade: true}); err != nil || !changed {
		t.Errorf("SetState with a cascade that opens a subtask = %v, %v; want a change", changed, err)
	}
}

func TestOpSetStateRules(t *testing.T) {
	d := opDoc(t, "# A\n\n- [x] x\n  - [x] s\n")
	both := todo.StateOptions{Restore: todo.DoneStates{}, Cascade: true}
	wantErr(t, setState(t, d, idOf(t, d, "x"), todo.Working, both), todo.ErrRestoreAndCascade)
	wantErr(t, setState(t, d, "zzzzzzz", todo.Working, todo.StateOptions{}), todo.ErrNotFound)
	wantErr(t, setState(t, d, idOf(t, d, "A"), todo.Done, todo.StateOptions{}), todo.ErrNotTask)
	wantErr(t, setState(t, d, idOf(t, d, "s"), todo.Working, todo.StateOptions{}), todo.ErrSubtaskStatus)
	wantErr(t, setState(t, d, idOf(t, d, "s"), todo.Review, todo.StateOptions{Cascade: true}), todo.ErrSubtaskStatus)
	wantDoc(t, d, "# A\n\n- [x] x\n  - [x] s\n")
}

// --- Delete / Prune ---

func TestOpDeleteCategorySubtree(t *testing.T) {
	d := opDoc(t, "# A\n\n- [ ] x\n  - [ ] s\n\n## B\n\n- [ ] y\n\n# C\n")
	b, s := idOf(t, d, "B"), idOf(t, d, "s")
	wantNoErr(t, d.Delete(idOf(t, d, "A")))
	wantDoc(t, d, "# C\n")
	for _, id := range []string{b, s} {
		if _, ok := d.ByID(id); ok {
			t.Errorf("item %s still in the document", id)
		}
	}
}

func TestOpDeleteSubtask(t *testing.T) {
	d := opDoc(t, "# A\n\n- [ ] x\n  - [ ] s\n")
	wantNoErr(t, d.Delete(idOf(t, d, "s")))
	wantDoc(t, d, "# A\n\n- [ ] x\n")
}

func TestOpDeleteMissingNotFound(t *testing.T) {
	d := opDoc(t, "# A\n")
	wantErr(t, d.Delete("zzzzzzz"), todo.ErrNotFound)
	wantErr(t, d.Delete(""), todo.ErrNotFound)
}

func TestOpPruneKeepsDoneTaskWithOpenSubtask(t *testing.T) {
	d := opDoc(t, "# A\n\n- [x] x\n  - [ ] s\n  - [x] t\n- [x] y\n  - [x] u\n- [ ] z\n\n## B\n\n- [x] w\n")
	want := d.RemovableDone()
	if got := d.Prune(); got != want || got != 4 {
		t.Errorf("Prune() = %d, want %d (RemovableDone before) = 4", got, want)
	}
	wantDoc(t, d, "# A\n\n- [x] x\n  - [ ] s\n- [ ] z\n\n## B\n")
	if got := d.Prune(); got != 0 {
		t.Errorf("second Prune() = %d, want 0", got)
	}
}

// --- all-or-nothing ---

func TestOpErrorsLeaveDocumentUnchanged(t *testing.T) {
	const md = "# A\n\n- [/] x\n  old\n  - [x] s\n- [ ] y\n\n## B\n\n### C\n\n# D\n"
	str := func(s string) *string { return &s }
	cases := []struct {
		name string
		op   func(d *todo.Document, id func(string) string) error
	}{
		{"AddCategory under task", func(d *todo.Document, id func(string) string) error {
			_, err := d.AddCategory("N", id("x"), "")
			return err
		}},
		{"AddCategory anchor conflict", func(d *todo.Document, id func(string) string) error {
			_, err := d.AddCategory("N", id("D"), id("B"))
			return err
		}},
		{"AddCategory bad title", func(d *todo.Document, id func(string) string) error {
			_, err := d.AddCategory("", "", "")
			return err
		}},
		{"AppendCategory missing parent", func(d *todo.Document, _ func(string) string) error {
			_, err := d.AppendCategory("N", "zzzzzzz")
			return err
		}},
		{"AddTask after category", func(d *todo.Document, id func(string) string) error {
			_, err := d.AddTask(todo.Fields{Title: "n"}, todo.Open, id("A"), id("B"))
			return err
		}},
		{"AddTask at root", func(d *todo.Document, _ func(string) string) error {
			_, err := d.AddTask(todo.Fields{Title: "n"}, todo.Open, "", "")
			return err
		}},
		{"AppendTask too deep", func(d *todo.Document, id func(string) string) error {
			_, err := d.AppendTask(todo.Fields{Title: "n"}, todo.Open, id("s"))
			return err
		}},
		{"AppendTask subtask state", func(d *todo.Document, id func(string) string) error {
			_, err := d.AppendTask(todo.Fields{Title: "n"}, todo.Working, id("y"))
			return err
		}},
		{"Move into own subtree", func(d *todo.Document, id func(string) string) error {
			return d.Move(id("A"), id("C"), "")
		}},
		{"Move task with children under task", func(d *todo.Document, id func(string) string) error {
			return d.Move(id("x"), id("y"), "")
		}},
		{"Move anchor gone", func(d *todo.Document, id func(string) string) error {
			return d.Move(id("y"), id("D"), "zzzzzzz")
		}},
		{"Move subtask out, anchor gone", func(d *todo.Document, id func(string) string) error {
			return d.Move(id("s"), id("A"), "zzzzzzz")
		}},
		{"Move after itself", func(d *todo.Document, id func(string) string) error {
			return d.Move(id("y"), id("A"), id("y"))
		}},
		{"Move missing", func(d *todo.Document, id func(string) string) error {
			return d.Move("zzzzzzz", id("A"), "")
		}},
		{"Move task to root", func(d *todo.Document, id func(string) string) error {
			return d.Move(id("s"), "", "")
		}},
		{"Edit bad title", func(d *todo.Document, id func(string) string) error {
			_, err := d.Edit(id("x"), todo.Changes{Title: str("a\nb"), Description: str("new")})
			return err
		}},
		{"Edit missing", func(d *todo.Document, _ func(string) string) error {
			_, err := d.Edit("zzzzzzz", todo.Changes{Title: str("n")})
			return err
		}},
		{"SetStatuses missing", func(d *todo.Document, id func(string) string) error {
			return d.SetStatuses(todo.DoneStates{id("y"): todo.Done, "zzzzzzz": todo.Done})
		}},
		{"SetStatuses category", func(d *todo.Document, id func(string) string) error {
			return d.SetStatuses(todo.DoneStates{id("y"): todo.Done, id("A"): todo.Done})
		}},
		{"SetStatuses subtask state", func(d *todo.Document, id func(string) string) error {
			return d.SetStatuses(todo.DoneStates{id("y"): todo.Done, id("s"): todo.Review})
		}},
		{"CascadeStatus category", func(d *todo.Document, id func(string) string) error {
			return d.CascadeStatus(id("A"), todo.Done)
		}},
		{"Complete missing", func(d *todo.Document, _ func(string) string) error {
			return d.Complete("zzzzzzz", true)
		}},
		{"Reopen restore and cascade", func(d *todo.Document, id func(string) string) error {
			return d.Reopen(id("x"), todo.Open, todo.ReopenOptions{Restore: todo.DoneStates{id("s"): todo.Done}, Cascade: true})
		}},
		{"Reopen restore subtask state", func(d *todo.Document, id func(string) string) error {
			return d.Reopen(id("x"), todo.Open, todo.ReopenOptions{Restore: todo.DoneStates{id("s"): todo.Working}})
		}},
		{"Delete missing", func(d *todo.Document, _ func(string) string) error {
			return d.Delete("zzzzzzz")
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d := opDoc(t, md)
			before := structure(d)
			if err := tc.op(d, func(title string) string { return idOf(t, d, title) }); err == nil {
				t.Fatal("op succeeded, want an error")
			}
			if got := structure(d); got != before {
				t.Errorf("document changed:\n%s\nwant\n%s", got, before)
			}
		})
	}
}

// --- AssignIDs ---

func TestAssignIDsFillsOnlyEmpty(t *testing.T) {
	d := todotest.Parse(t, "# A\n\n- [ ] x\n  - [ ] s\n\n## B\n")
	find(d, "x").ID = "keep123"
	d.AssignIDs()
	seen := map[string]bool{}
	for _, it := range d.Enumerate() {
		if it.Title == "x" {
			if it.ID != "keep123" {
				t.Errorf("x.ID = %q, want the existing keep123", it.ID)
			}
		} else if !isNewID(it.ID) {
			t.Errorf("%s.ID = %q, want a generated ID", it.Title, it.ID)
		}
		if seen[it.ID] {
			t.Errorf("ID %q used twice", it.ID)
		}
		seen[it.ID] = true
	}
	before := structure(d)
	d.AssignIDs()
	if got := structure(d); got != before {
		t.Errorf("second AssignIDs changed IDs:\n%s\nwant\n%s", got, before)
	}
}
