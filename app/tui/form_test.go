package tui

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/candy-tools/todo/internal/todo"
	"github.com/candy-tools/todo/internal/todo/todotest"
	tea "github.com/charmbracelet/bubbletea"
)

func TestFormRing(t *testing.T) {
	task := newForm("", "", "", true, false)
	if len(task.ring()) != 5 {
		t.Errorf("task form should have 5 focus stops, got %d", len(task.ring()))
	}
	cat := newForm("", "", "", false, false)
	if len(cat.ring()) != 3 {
		t.Errorf("category form should have 3 focus stops (no description), got %d", len(cat.ring()))
	}
}

func TestFormFocusStepWraps(t *testing.T) {
	f := newForm("", "", "", true, false)
	if f.focus != focusTitle {
		t.Fatal("form should open focused on the title")
	}
	f.focusStep(-1) // wrap backwards from title -> cancel
	if f.focus != focusCancel {
		t.Errorf("stepping back from title should wrap to cancel, got %v", f.focus)
	}
}

func TestFormValuesTrim(t *testing.T) {
	f := newForm("  hi  ", "  body\n", "  docs/p.md ", true, true)
	title, desc, plan := f.values()
	if title != "hi" {
		t.Errorf("title = %q, want trimmed 'hi'", title)
	}
	if plan != "docs/p.md" {
		t.Errorf("plan = %q, want trimmed 'docs/p.md'", plan)
	}
	if desc != "  body" {
		t.Errorf("desc = %q, want trailing newline trimmed", desc)
	}
}

func TestFormPlanFieldInRing(t *testing.T) {
	f := newForm("", "", "", true, false)
	f.focusStep(1)
	f.focusStep(1)
	if f.focus != focusPlan {
		t.Errorf("tab, tab should reach the plan field, got %v", f.focus)
	}
	cat := newForm("", "", "", false, false)
	for _, x := range cat.ring() {
		if x == focusPlan {
			t.Error("category form must have no plan field")
		}
	}
}

func TestFormViewTitles(t *testing.T) {
	f := newForm("", "", "", true, false)
	f.setWidth(100)
	if !strings.Contains(f.view(), "Add task") {
		t.Errorf("task add form should be titled 'Add task'")
	}
	if !strings.Contains(f.view(), "Description") {
		t.Errorf("task form should show the Description field")
	}
	cat := newForm("", "", "", false, true)
	cat.setWidth(100)
	if !strings.Contains(cat.view(), "Rename category") {
		t.Errorf("category edit form should be titled 'Rename category'")
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
	if err := m.store.Edit(context.Background(), a.ID, todo.Changes{Description: &desc, Plan: &plan}); err != nil {
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
	m = press(m, "tab") // description; the cursor sits at the end of the text
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
	m = press(m, "tab", "tab") // plan; the cursor sits at the end
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

// Saving the edit form unchanged writes nothing: the store's revision stays
// put and the undo snapshots survive.
func TestEditSavedUnchangedWritesNothing(t *testing.T) {
	m, _ := newTestModel(t, "# B\n- [ ] a\n  desc\n")
	a := find(m.doc, "a")
	m.snapshots[a.ID] = todo.DoneStates{a.ID: todo.Open}
	before, err := m.store.Revision(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	m = selectTitle(t, m, "a")
	m = press(m, "e")
	if m.mode != modeForm {
		t.Fatalf("e should open the edit form, status %q", m.status)
	}
	m = press(m, "enter")
	if m.mode != modeMain {
		t.Fatalf("enter should close the form, mode %v", m.mode)
	}
	after, err := m.store.Revision(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if after != before {
		t.Errorf("an unchanged save moved the revision from %d to %d", before, after)
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
	if err := openSecond(t, path).Delete(context.Background(), storedItem(t, path, title).ID); err != nil {
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
	if got, _, _ := m.form.values(); got != title {
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
