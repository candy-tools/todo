package tui

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/candy-tools/todo/internal/todo"
	"github.com/candy-tools/todo/internal/todo/todotest"
	tea "github.com/charmbracelet/bubbletea"
)

func TestFormRing(t *testing.T) {
	task := newForm(todo.Fields{}, formTask, false)
	if len(task.ring()) != 6 {
		t.Errorf("task form should have 6 focus stops, got %d", len(task.ring()))
	}
	sub := newForm(todo.Fields{}, formSubtask, false)
	if len(sub.ring()) != 5 {
		t.Errorf("subtask form should have 5 focus stops (no reference), got %d", len(sub.ring()))
	}
	cat := newForm(todo.Fields{}, formCategory, false)
	if len(cat.ring()) != 4 {
		t.Errorf("category form should have 4 focus stops (title, prefix, Save, Cancel), got %d", len(cat.ring()))
	}
	settings := newForm(todo.Fields{}, formSettings, true)
	if len(settings.ring()) != 3 {
		t.Errorf("settings form should have 3 focus stops (the prefix, Save, Cancel), got %d", len(settings.ring()))
	}
}

func TestFormFocusStepWraps(t *testing.T) {
	f := newForm(todo.Fields{}, formTask, false)
	if f.focus != focusTitle {
		t.Fatal("form should open focused on the title")
	}
	f.focusStep(-1) // wrap backwards from title -> cancel
	if f.focus != focusCancel {
		t.Errorf("stepping back from title should wrap to cancel, got %v", f.focus)
	}
}

func TestFormValuesTrim(t *testing.T) {
	f := newForm(todo.Fields{Title: "  hi  ", Ref: " APP-1 ", Description: "  body\n", Plan: "  docs/p.md "}, formTask, true)
	v := f.values()
	if v.Title != "hi" {
		t.Errorf("title = %q, want trimmed 'hi'", v.Title)
	}
	if v.Ref != "APP-1" {
		t.Errorf("ref = %q, want trimmed 'APP-1'", v.Ref)
	}
	if v.Plan != "docs/p.md" {
		t.Errorf("plan = %q, want trimmed 'docs/p.md'", v.Plan)
	}
	if v.Description != "  body" {
		t.Errorf("desc = %q, want trailing newline trimmed", v.Description)
	}
}

func TestFormPlanFieldInRing(t *testing.T) {
	f := newForm(todo.Fields{}, formTask, false)
	f.focusStep(1)
	if f.focus != focusRef {
		t.Errorf("tab should reach the reference field, got %v", f.focus)
	}
	f.focusStep(1)
	f.focusStep(1)
	if f.focus != focusPlan {
		t.Errorf("tab, tab, tab should reach the plan field, got %v", f.focus)
	}
	cat := newForm(todo.Fields{}, formCategory, false)
	for _, x := range cat.ring() {
		if x == focusPlan || x == focusDesc {
			t.Error("category form must have no plan or description field")
		}
	}
}

func TestFormViewTitles(t *testing.T) {
	f := newForm(todo.Fields{}, formTask, false)
	f.setWidth(100)
	if !strings.Contains(f.view(), "Reference") {
		t.Errorf("task form should show the Reference field")
	}
	sub := newForm(todo.Fields{}, formSubtask, false)
	sub.setWidth(100)
	if strings.Contains(sub.view(), "Reference") {
		t.Errorf("subtask form should not show a Reference field")
	}
	if !strings.Contains(f.view(), "Add task") {
		t.Errorf("task add form should be titled 'Add task'")
	}
	if !strings.Contains(f.view(), "Description") {
		t.Errorf("task form should show the Description field")
	}
	cat := newForm(todo.Fields{}, formCategory, true)
	cat.setWidth(100)
	if !strings.Contains(cat.view(), "Edit category") {
		t.Errorf("category edit form should be titled 'Edit category'")
	}
	if strings.Contains(cat.view(), "Description") {
		t.Errorf("category form should not show a Description field")
	}
}

// seedLongTask stores a task "a" whose description has 150 lines and whose plan
// is 600 characters — both beyond the edit widgets' former limits — and opens
// the edit form on it.
func seedLongTask(t *testing.T) (m model, path, desc, plan string) {
	t.Helper()
	m, path = newTestModel(t, "# B\n- [ ] a\n")
	lines := make([]string, 150)
	for i := range lines {
		lines[i] = fmt.Sprintf("line %d", i+1)
	}
	desc = strings.Join(lines, "\n")
	plan = "docs/" + strings.Repeat("p", 592) + ".md"
	a := find(m.doc, "a")
	if _, err := m.store.Edit(context.Background(), a.ID, todo.Changes{Description: &desc, Plan: &plan}); err != nil {
		t.Fatal(err)
	}
	m = pollOnce(t, m)
	m = selectTitle(t, m, "a")
	m = press(m, "e")
	if m.mode != modeForm {
		t.Fatalf("e should open the edit form, status %q", m.status)
	}
	return m, path, desc, plan
}

// A long description edited in the form is saved whole: every stored line is
// kept and a new line can still be added past the widget's visible height.
func TestEditLongDescriptionKeepsAllLines(t *testing.T) {
	m, path, desc, _ := seedLongTask(t)
	m = press(m, "tab", "tab") // past the reference to the description; the cursor sits at the end of the text
	m = typeText(m, "x")
	m = press(m, "enter")
	m = typeText(m, "y")
	press(m, "tab", "tab", "enter") // plan, Save
	if got := storedItem(t, path, "a").Description; got != desc+"x\ny" {
		t.Errorf("description has %d lines after the edit, want %d ending in \"x\\ny\"",
			strings.Count(got, "\n")+1, strings.Count(desc, "\n")+2)
	}
}

// A plan longer than the old 500-character input limit round-trips: deleting
// its last character stores the rest of it, not a truncated copy.
func TestEditLongPlanKeepsWholeValue(t *testing.T) {
	m, path, _, plan := seedLongTask(t)
	m = press(m, "tab", "tab", "tab") // past the reference and description to the plan; the cursor sits at the end
	m = send(m, tea.KeyMsg{Type: tea.KeyBackspace})
	m = typeText(m, "X")
	press(m, "tab", "enter") // Save
	want := plan[:len(plan)-1] + "X"
	if got := storedItem(t, path, "a").Plan; got != want {
		t.Errorf("plan is %d chars after the edit, want %d", len(got), len(want))
	}
}

// Editing only the title leaves a long description and plan byte-identical.
func TestEditTitleKeepsLongDescriptionAndPlan(t *testing.T) {
	m, path, desc, plan := seedLongTask(t)
	m = typeText(m, "2")
	press(m, "enter")
	it := storedItem(t, path, "a2")
	if it.Description != desc {
		t.Errorf("a title edit changed the description (%d chars, want %d)", len(it.Description), len(desc))
	}
	if it.Plan != plan {
		t.Errorf("a title edit changed the plan (%d chars, want %d)", len(it.Plan), len(plan))
	}
}

// Saving the edit form unchanged writes nothing, not even a store write that
// changes nothing (one would fail on the held lock): the list file stays put
// and the undo snapshots survive.
func TestEditSavedUnchangedWritesNothing(t *testing.T) {
	m, path := newTestModel(t, "# B\n- [ ] a\n  desc\n")
	a := find(m.doc, "a")
	m.snapshots[a.ID] = todo.DoneStates{a.ID: todo.Open}
	before := readList(t, path)
	m = selectTitle(t, m, "a")
	m = press(m, "e")
	if m.mode != modeForm {
		t.Fatalf("e should open the edit form, status %q", m.status)
	}
	todotest.HoldWriteLock(t, path)
	m = press(m, "enter")
	if m.mode != modeMain {
		t.Fatalf("enter should close the form, mode %v", m.mode)
	}
	if after := readList(t, path); !bytes.Equal(after, before) || m.err != nil {
		t.Errorf("an unchanged save wrote the list (err %v):\n%s", m.err, after)
	}
	if len(m.snapshots) != 1 {
		t.Errorf("an unchanged save must keep the undo snapshots, got %d", len(m.snapshots))
	}
}

// deleteStored deletes the item titled title from the store at path through a
// second handle, standing in for another process removing it while the TUI's
// form is open.
func deleteStored(t *testing.T, path, title string) {
	t.Helper()
	if _, err := openSecond(t, path).Delete(context.Background(), storedItem(t, path, title).ID); err != nil {
		t.Fatal(err)
	}
}

// submitLocked presses enter (Save) while another process holds the store's
// writer lock, which it releases again before returning.
func submitLocked(t *testing.T, m model, path string) model {
	t.Helper()
	t.Run("locked", func(t *testing.T) {
		todotest.HoldWriteLock(t, path)
		m = press(m, "enter")
	})
	return m
}

// wantFormKept fails the test unless a save that failed with ErrBusy left the
// form open with title in it and the busy error in the footer.
func wantFormKept(t *testing.T, m model, title string) {
	t.Helper()
	if m.mode != modeForm {
		t.Fatalf("a busy save closed the form (mode %v), discarding what was typed", m.mode)
	}
	if got := m.form.values().Title; got != title {
		t.Errorf("form title = %q after the busy save, want %q", got, title)
	}
	if got := m.footer(200); !strings.Contains(got, "save failed: "+todo.ErrBusy.Error()) {
		t.Errorf("footer = %q, want the busy message", got)
	}
}

// An edit saved while another process holds the writer lock keeps the form
// open with what was typed; saving again once the lock is gone renames the
// edited item, even though the rows shifted (a row above it was deleted).
func TestEditSaveWhileLockedKeepsForm(t *testing.T) {
	m, path := newTestModel(t, "# C\n- [ ] a\n- [ ] b\n- [ ] c\n")
	bID := find(m.doc, "b").ID
	m = press(selectTitle(t, m, "b"), "e")
	m = typeText(m, "2")
	deleteStored(t, path, "a")

	m = submitLocked(t, m, path)
	wantFormKept(t, m, "b2")
	if m.editing == nil || m.editing != find(m.doc, "b") {
		t.Fatalf("m.editing = %v, want b in the reloaded document", m.editing)
	}

	m = press(m, "enter")
	if m.mode != modeMain || m.err != nil {
		t.Fatalf("the retried save: mode %v, err %v", m.mode, m.err)
	}
	if got := storedItem(t, path, "b2").ID; got != bID {
		t.Errorf("the retried edit renamed item %s, want b (%s)", got, bID)
	}
	storedItem(t, path, "c") // c keeps its title
}

// An add saved while another process holds the writer lock keeps the form
// open with what was typed; saving again once the lock is gone adds the
// subtask under the task the first attempt aimed at, though the rows shifted.
func TestAddSaveWhileLockedKeepsForm(t *testing.T) {
	m, path := newTestModel(t, "# C\n- [ ] a\n- [ ] b\n- [ ] c\n")
	m = press(selectTitle(t, m, "b"), "N")
	m = typeText(m, "s")
	deleteStored(t, path, "a")

	m = submitLocked(t, m, path)
	wantFormKept(t, m, "s")
	if find(m.doc, "s") != nil {
		t.Errorf("the failed add still shows s in the list")
	}

	m = press(m, "enter")
	if m.mode != modeMain || m.err != nil {
		t.Fatalf("the retried save: mode %v, err %v", m.mode, m.err)
	}
	if p := storedItem(t, path, "s").Parent; p == nil || p.Title != "b" {
		t.Errorf("the retried add put s under %v, want b", p)
	}
	if s := find(m.doc, "s"); m.tree.selected() != s {
		t.Errorf("the new subtask is not selected after the retried add")
	}
}

// An edit whose item another process deleted while the form was open fails
// with ErrNotFound; there is nothing left to edit, so the form closes and the
// footer shows the error.
func TestEditSaveOfDeletedItemClosesForm(t *testing.T) {
	m, path := newTestModel(t, "# C\n- [ ] a\n- [ ] b\n")
	m = press(selectTitle(t, m, "b"), "e")
	m = typeText(m, "2")
	deleteStored(t, path, "b")

	m = press(m, "enter")
	if m.mode != modeMain {
		t.Errorf("mode = %v after editing a deleted item, want main", m.mode)
	}
	if !errors.Is(m.err, todo.ErrNotFound) {
		t.Errorf("m.err = %v, want ErrNotFound", m.err)
	}
	if got := m.footer(200); !strings.Contains(got, "save failed: "+todo.ErrNotFound.Error()) {
		t.Errorf("footer = %q, want the not-found error", got)
	}
}

// The category form has a Reference prefix field after the title: adding a
// category stores it upper-cased, and the edit form shows it and writes only
// the prefix when only it changed.
func TestCategoryFormPrefix(t *testing.T) {
	m, path := newTestModel(t, "# B\n- [ ] a\n")
	m = press(selectTitle(t, m, "B"), "c")
	if m.mode != modeForm || m.form.kind != formCategory {
		t.Fatalf("c: mode %v kind %v, want the category form", m.mode, m.form.kind)
	}
	if want := []formFocus{focusTitle, focusRef, focusSave, focusCancel}; !slices.Equal(m.form.ring(), want) {
		t.Errorf("category ring = %v, want %v", m.form.ring(), want)
	}
	if !strings.Contains(m.form.view(), "Reference prefix") {
		t.Errorf("the category form should show a Reference prefix field:\n%s", m.form.view())
	}
	m = typeText(m, "Work")
	m = typeText(press(m, "tab"), "work")
	m = press(m, "enter")
	if m.mode != modeMain || m.err != nil || m.status != "" {
		t.Fatalf("after enter: mode %v err %v status %q", m.mode, m.err, m.status)
	}
	if p := storedItem(t, path, "Work").Prefix; p != "WORK" {
		t.Fatalf("stored prefix = %q, want WORK", p)
	}

	m = press(selectTitle(t, m, "Work"), "e")
	if m.mode != modeForm || m.form.ref.Value() != "WORK" {
		t.Fatalf("e: mode %v prefix %q, want WORK", m.mode, m.form.ref.Value())
	}
	m = press(m, "tab")
	m = typeText(clearInput(m, 4), "ops")
	ch := m.form.changes()
	if ch.Prefix == nil || *ch.Prefix != "ops" || ch.Title != nil || ch.Ref != nil || ch.Description != nil || ch.Plan != nil {
		t.Errorf("changes = %+v, want the prefix only", ch)
	}
	m = press(m, "enter")
	if m.mode != modeMain || m.err != nil {
		t.Fatalf("after the edit: mode %v err %v status %q", m.mode, m.err, m.status)
	}
	if p := storedItem(t, path, "Work").Prefix; p != "OPS" {
		t.Errorf("stored prefix after the edit = %q, want OPS", p)
	}
}

// The sprint form has a Reference prefix field between the name and the goal.
func TestSprintFormPrefix(t *testing.T) {
	m, path := threeSprints(t)
	m = press(m, "s", "n")
	if want := []formFocus{focusTitle, focusRef, focusDesc, focusSave, focusCancel}; !slices.Equal(m.form.ring(), want) {
		t.Errorf("sprint ring = %v, want %v", m.form.ring(), want)
	}
	m = typeText(m, "S")
	m = typeText(press(m, "tab"), "spr")
	m = press(m, "enter")
	if m.mode != modeMain || m.err != nil || m.status != "" {
		t.Fatalf("after enter: mode %v err %v status %q", m.mode, m.err, m.status)
	}
	s, ok := loadSprintDoc(t, path).SprintByName("S")
	if !ok || s.Prefix != "SPR" {
		t.Fatalf("stored sprint = %+v (found %v), want prefix SPR", s, ok)
	}

	m = press(m, "e")
	if m.form.ref.Value() != "SPR" {
		t.Fatalf("e shows prefix %q, want SPR", m.form.ref.Value())
	}
	m = clearInput(press(m, "tab"), 3)
	if ch := m.form.sprintChanges(); ch.Prefix == nil || *ch.Prefix != "" || ch.Name != nil || ch.Goal != nil {
		t.Errorf("sprintChanges = %+v, want the prefix cleared only", ch)
	}
	m = press(m, "enter")
	if s, _ := loadSprintDoc(t, path).SprintByName("S"); s.Prefix != "" {
		t.Errorf("stored prefix after clearing = %q, want none", s.Prefix)
	}
}

// Where a reference prefix applies, the add-task form shows the reference it
// will get instead of an input, and the task gets the generated one; without
// one the reference stays a field to fill in.
func TestAddTaskFormAutoRef(t *testing.T) {
	m, path := newTestModel(t, "# Work\n- [ ] a\n# Home\n- [ ] b\n")
	work := find(m.doc, "Work")
	if _, err := m.store.Edit(context.Background(), work.ID, todo.Changes{Prefix: new("work")}); err != nil {
		t.Fatal(err)
	}
	m = pollOnce(t, m)

	m = press(selectTitle(t, m, "Work"), "n")
	if m.mode != modeForm || m.form.kind != formTask {
		t.Fatalf("n: mode %v kind %v, want the add-task form", m.mode, m.form.kind)
	}
	if slices.Contains(m.form.ring(), focusRef) {
		t.Errorf("ring = %v, want no reference stop", m.form.ring())
	}
	if v := m.form.view(); !strings.Contains(v, "WORK-… (automatic)") {
		t.Errorf("the form should show the automatic reference:\n%s", v)
	}
	m = press(typeText(m, "t"), "enter")
	if m.mode != modeMain || m.err != nil || m.status != "" {
		t.Fatalf("after enter: mode %v err %v status %q", m.mode, m.err, m.status)
	}
	if ref := storedItem(t, path, "t").Ref; ref != "WORK-001" {
		t.Errorf("the new task's ref = %q, want WORK-001", ref)
	}

	// The edit form keeps the reference editable.
	m = press(selectTitle(t, m, "t"), "e")
	if !slices.Contains(m.form.ring(), focusRef) || m.form.ref.Value() != "WORK-001" {
		t.Errorf("edit: ring %v ref %q, want an editable WORK-001", m.form.ring(), m.form.ref.Value())
	}

	m = press(m, "esc")
	m = press(selectTitle(t, m, "Home"), "n")
	if !slices.Contains(m.form.ring(), focusRef) || strings.Contains(m.form.view(), "(automatic)") {
		t.Errorf("without a prefix: ring %v, want the reference field", m.form.ring())
	}
}

// On a sprint board a new task joins the board's sprint, so the sprint's
// prefix is the one its form shows and its reference comes from.
func TestAddTaskFormAutoRefFromSprint(t *testing.T) {
	m, path := threeSprints(t)
	if _, err := m.store.EditSprint(context.Background(), "s00000a", todo.SprintChanges{Prefix: new("sa")}); err != nil {
		t.Fatal(err)
	}
	m = pollOnce(t, m)
	m = press(m, "s", "tab", "n")
	if m.mode != modeForm || m.form.autoRef != "SA" {
		t.Fatalf("n on A's board: mode %v autoRef %q, want SA (status %q)", m.mode, m.form.autoRef, m.status)
	}
	m = press(typeText(m, "t"), "enter")
	if ref := storedItem(t, path, "t").Ref; ref != "SA-001" {
		t.Errorf("the new task's ref = %q, want SA-001", ref)
	}
}
