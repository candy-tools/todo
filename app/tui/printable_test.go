package tui

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"unicode"

	"github.com/candy-tools/todo/internal/todo"
	"github.com/candy-tools/todo/internal/todo/todotest"
	tea "github.com/charmbracelet/bubbletea"
)

// hostile is a title holding terminal escapes (a colour, an OSC window-title
// set ended by BEL, a screen clear) that an older version could have stored.
const hostile = "evil\x1b[31m\x1b]0;pwn\x07\x1b[2J"

// assertNoEscapes fails when s carries any piece of the hostile title's escapes.
func assertNoEscapes(t *testing.T, what, s string) {
	t.Helper()
	for _, bad := range []string{"\x1b[31m", "\x1b]0;", "\x07", "\x1b[2J"} {
		if strings.Contains(s, bad) {
			t.Errorf("%s carries the stored escape %q: %q", what, bad, s)
		}
	}
}

// TestStoredControlCharactersAreNotDrawn draws a list whose titles carry
// terminal escapes. A valid list file cannot hold such a title any more, so
// the list is built in memory: the tree must not draw them all the same.
func TestStoredControlCharactersAreNotDrawn(t *testing.T) {
	doc := parseDoc(t, "# Cat\n\n- [ ] task\n")
	for _, it := range doc.Enumerate() {
		it.Title += hostile
	}
	for _, pane := range []paneKind{paneMixed, paneTasks} {
		tr := newTree(doc, newViewState())
		tr.setPane(pane)
		for i, r := range tr.rows {
			for _, sel := range []bool{false, true} {
				assertNoEscapes(t, "a tree row", tr.rowString(r, sel))
			}
			if i == 0 && pane == paneTasks && r.kind != rowHeader {
				t.Errorf("the task pane should start with the category header, got kind %v", r.kind)
			}
		}
	}
}

// Legacy field values holding control and bidi characters an older version or
// another tool could have stored, and what the edit form shows for each.
const (
	legacyTitle     = "task\u202e\u2066gnp.exe\u2069\ufeff\x1b[2J"
	legacyDesc      = "one\u202e\r\ntwo\x1b[31m\u2028\n\tthree\u2066"
	legacyPlan      = "plan\u202e.md\x07"
	cleanLegacyTit  = "taskgnp.exe[2J"
	cleanLegacyDesc = "one\ntwo[31m\n    three"
	cleanLegacyPlan = "plan.md"
)

// newLegacyModel is newTestModel on a list whose task "task" holds the legacy
// description and plan, written to the file past the store's checks, and the
// legacy title. A valid list file cannot hold that title, so it is set on the
// loaded list in memory only, standing in for one an older version kept.
func newLegacyModel(t *testing.T) (model, string) {
	t.Helper()
	path := todotest.Seed(t, "# Cat\n\n- [ ] task\n")
	setRawTaskFields(t, path, "task", legacyDesc, legacyPlan)
	st := todo.NewStore(path, todo.WithBusyTimeout(BusyTimeout))
	m, err := newModel(st, "")
	if err != nil {
		t.Fatalf("newModel: %v", err)
	}
	find(m.doc, "task").Title = legacyTitle
	m.adoptAll(m.doc, "", false)
	m = send(m, tea.WindowSizeMsg{Width: 100, Height: 30})
	m = selectTitle(t, m, legacyTitle)
	m = press(m, "e")
	if m.mode != modeForm {
		t.Fatalf("e should open the edit form, status %q", m.status)
	}
	return m, path
}

// unsafeIn returns the runes of s that must not reach the terminal: control
// characters other than the newline, bidi controls, line and paragraph
// separators and the byte order mark.
func unsafeIn(s string) []rune {
	var bad []rune
	for _, r := range s {
		if r != '\n' && (unicode.IsControl(r) || r >= '\u202a' && r <= '\u202e' || r >= '\u2066' && r <= '\u2069' ||
			r == '\u2028' || r == '\u2029' || r == '\ufeff') {
			bad = append(bad, r)
		}
	}
	return bad
}

func TestEditFormShowsNoStoredControlCharacters(t *testing.T) {
	m, _ := newLegacyModel(t)
	v := m.form.values()
	title, desc, plan := v.Title, v.Description, v.Plan
	for what, v := range map[string]string{"title": title, "description": desc, "plan": plan} {
		if bad := unsafeIn(v); len(bad) > 0 {
			t.Errorf("the form's %s %q holds unsafe runes %q", what, v, bad)
		}
	}
	if title != cleanLegacyTit || desc != cleanLegacyDesc || plan != cleanLegacyPlan {
		t.Errorf("form values = %q, %q, %q; want %q, %q, %q", title, desc, plan, cleanLegacyTit, cleanLegacyDesc, cleanLegacyPlan)
	}
	// The rendered form carries lipgloss styling escapes, so look for the stored
	// characters other than ESC.
	for _, bad := range unsafeIn(m.form.view()) {
		if bad != '\x1b' {
			t.Errorf("the rendered form holds the unsafe rune %q", bad)
		}
	}
}

func TestEditUntouchedLegacyItemWritesNothing(t *testing.T) {
	m, path := newLegacyModel(t)
	before := readList(t, path)
	todotest.HoldWriteLock(t, path) // a store write, even one changing nothing, fails
	m = press(m, "enter")
	if m.mode != modeMain {
		t.Fatalf("enter should close the form, mode %v", m.mode)
	}
	if after := readList(t, path); !bytes.Equal(after, before) || m.err != nil {
		t.Errorf("saving an untouched legacy item wrote the list (err %v):\n%s", m.err, after)
	}
}

func TestEditLegacyItemWritesOnlyTheEditedField(t *testing.T) {
	m, path := newLegacyModel(t)
	m = press(m, "tab", "tab") // past the reference to the description
	m = typeText(m, "x")
	want := m.form.values().Description
	if want == cleanLegacyDesc {
		t.Fatal("typing should have changed the description")
	}
	m = press(m, "tab", "tab", "enter") // past the plan to Save
	if m.mode != modeMain {
		t.Fatalf("Save should close the form, mode %v", m.mode)
	}
	// Found by its stored title, so the untouched title was not rewritten.
	it := storedItem(t, path, "task")
	if it.Description != want {
		t.Errorf("stored description = %q, want %q", it.Description, want)
	}
	if it.Plan != legacyPlan {
		t.Errorf("the untouched plan was rewritten: %q, want %q", it.Plan, legacyPlan)
	}
}

// setRawTaskFields sets the description and plan of the task titled title in
// the list file at path, past the store's checks, standing in for another
// tool (or an older version) that wrote them.
func setRawTaskFields(t *testing.T, path, title, desc, plan string) {
	t.Helper()
	var l map[string]any
	if err := json.Unmarshal(readList(t, path), &l); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	tasks, _ := l["tasks"].([]any)
	found := false
	for _, x := range tasks {
		if task, ok := x.(map[string]any); ok && task["title"] == title {
			task["description"], task["plan"] = desc, plan
			found = true
		}
	}
	if !found {
		t.Fatalf("no task %q in the list file", title)
	}
	data, err := json.Marshal(l)
	if err != nil {
		t.Fatalf("encode list: %v", err)
	}
	writeList(t, path, data)
}
