package tui

import (
	"strings"
	"testing"
)

const refList = `{"version": 4, "categories": [{"id": "c000000", "title": "Work"}], "tasks": [
	{"id": "t000000", "category": "c000000", "ref": "APP-001", "title": "a", "status": "open",
	 "subtasks": [{"id": "s000000", "title": "sub"}]}]}`

// A task's reference shows before its title, as "[APP-001] a".
func TestRowShowsRef(t *testing.T) {
	m, _ := newJSONModel(t, refList)
	if row := rowOf(t, m.tree, "a"); !strings.Contains(row, "[APP-001] a") {
		t.Errorf("row = %q, want the ref before the title", row)
	}
}

// The task form's second field is the reference; a new task saved with one
// stores it.
func TestNewTaskWithRef(t *testing.T) {
	m, path := newTestModel(t, "# B\n- [ ] a\n")
	m = press(m, "n")
	m = typeText(m, "fresh")
	m = press(m, "tab")
	m = typeText(m, "APP-9")
	press(m, "enter")
	if it := storedItem(t, path, "fresh"); it.Ref != "APP-9" {
		t.Errorf("new task ref = %q, want APP-9", it.Ref)
	}
}

// A subtask has no reference, so adding one shows no Reference field.
func TestNewSubtaskFormHasNoRef(t *testing.T) {
	m, _ := newTestModel(t, "# B\n- [ ] a\n")
	m = press(m, "down") // a
	m = press(m, "N")
	if m.mode != modeForm {
		t.Fatalf("N should open the form, status %q", m.status)
	}
	if m.form.kind != formSubtask {
		t.Errorf("form kind = %v, want the subtask form", m.form.kind)
	}
}

// Saving a reference another task holds fails: the form stays open with what
// was typed and the reason in the footer.
func TestEditDuplicateRefKeepsForm(t *testing.T) {
	m, path := newJSONModel(t, `{"version": 4, "categories": [{"id": "c000000", "title": "Work"}], "tasks": [
		{"id": "t000000", "category": "c000000", "ref": "APP-001", "title": "a", "status": "open"},
		{"id": "t000001", "category": "c000000", "title": "b", "status": "open"}]}`)
	m.tree.selectID("t000001")
	m = press(m, "e")
	m = press(m, "tab")
	m = typeText(m, "app-001")
	m = press(m, "enter")
	if m.mode != modeForm {
		t.Fatalf("a refused save closed the form (mode %v)", m.mode)
	}
	if got := m.form.values().Ref; got != "app-001" {
		t.Errorf("form ref = %q after the refused save", got)
	}
	if got := m.footer(200); !strings.Contains(got, "duplicate reference") {
		t.Errorf("footer = %q, want the duplicate reference reason", got)
	}
	if it := storedItem(t, path, "b"); it.Ref != "" {
		t.Errorf("stored ref = %q, want none", it.Ref)
	}
}

// Indenting a task that has a reference under the task above is refused, as a
// subtask holds none: nothing moves and the footer says why.
func TestIndentRefTaskRefused(t *testing.T) {
	m, path := newJSONModel(t, `{"version": 4, "categories": [{"id": "c000000", "title": "Work"}], "tasks": [
		{"id": "t000000", "category": "c000000", "title": "a", "status": "open"},
		{"id": "t000001", "category": "c000000", "ref": "APP-002", "title": "b", "status": "open"}]}`)
	before := storeFP(t, m)
	got := press(selectTitle(t, m, "b"), "m", "right")
	if !strings.Contains(got.status, "reference") {
		t.Errorf("status = %q, want the reference refusal", got.status)
	}
	if storeFP(t, got) != before || storedItem(t, path, "b").Ref != "APP-002" {
		t.Error("a refused indent wrote to the store")
	}
}
