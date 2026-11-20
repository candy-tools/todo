package tui

import (
	"strings"
	"testing"

	"github.com/candy-tools/todo/internal/todo"
	tea "github.com/charmbracelet/bubbletea"
)

// clearInput deletes the n characters before the cursor of the focused input.
func clearInput(m model, n int) model {
	for range n {
		m = send(m, tea.KeyMsg{Type: tea.KeyBackspace})
	}
	return m
}

// S opens the settings dialog on the list's reference prefix: typing one sets
// it (upper-cased), the dialog shows it when opened again, and clearing the
// field clears it.
func TestSettingsDialogSetsPrefix(t *testing.T) {
	m, path := newTestModel(t, "# B\n- [ ] a\n")
	m = press(m, "S")
	if m.mode != modeForm || m.form.kind != formSettings || m.target != targetSettings {
		t.Fatalf("S: mode %v kind %v target %v, want the settings dialog", m.mode, m.form.kind, m.target)
	}
	if v := m.form.view(); !strings.Contains(v, "Settings") || !strings.Contains(v, "Reference prefix") {
		t.Errorf("the settings dialog should be titled Settings with a Reference prefix field:\n%s", v)
	}
	m = press(typeText(m, "todo"), "enter")
	if m.mode != modeMain || m.err != nil || m.status != "" {
		t.Fatalf("after enter: mode %v err %v status %q", m.mode, m.err, m.status)
	}
	if m.doc.RefPrefix != "TODO" || loadSprintDoc(t, path).RefPrefix != "TODO" {
		t.Fatalf("RefPrefix = %q (stored %q), want TODO", m.doc.RefPrefix, loadSprintDoc(t, path).RefPrefix)
	}

	m = press(m, "S")
	if got := m.form.title.Value(); got != "TODO" {
		t.Fatalf("the reopened dialog shows %q, want TODO", got)
	}
	m = press(clearInput(m, 4), "enter")
	if m.mode != modeMain || m.err != nil {
		t.Fatalf("after clearing: mode %v err %v", m.mode, m.err)
	}
	if m.doc.RefPrefix != "" || loadSprintDoc(t, path).RefPrefix != "" {
		t.Errorf("RefPrefix = %q after clearing, want none", m.doc.RefPrefix)
	}
}

// An invalid prefix is refused: the footer says why and the dialog stays open
// with what was typed.
func TestSettingsInvalidPrefix(t *testing.T) {
	m, path := newTestModel(t, "# B\n- [ ] a\n")
	m = press(typeText(press(m, "S"), "1x"), "enter")
	if m.mode != modeForm || m.form.title.Value() != "1x" {
		t.Fatalf("mode %v value %q, want the dialog kept with 1x", m.mode, m.form.title.Value())
	}
	if got := m.footer(200); !strings.Contains(got, todo.ErrInvalidPrefix.Error()) {
		t.Errorf("footer = %q, want the invalid-prefix message", got)
	}
	if p := loadSprintDoc(t, path).RefPrefix; p != "" {
		t.Errorf("stored RefPrefix = %q, want none", p)
	}
}

// S opens the settings on every screen, the Sprints sidebar and Planning
// included, but not while a dialog is open: there it is just a key.
func TestSettingsKeyEverywhereButDialogs(t *testing.T) {
	m, _ := threeSprints(t)
	for _, keys := range [][]string{{"b"}, {"s"}, {"s", "tab"}, {"p"}} {
		got := press(m, append(keys, "S")...)
		if got.mode != modeForm || got.form.kind != formSettings {
			t.Errorf("%v then S: mode %v kind %v, want the settings dialog", keys, got.mode, got.form.kind)
		}
	}
	if got := press(m, "?", "S"); got.mode != modeHelp {
		t.Errorf("S in the help dialog: mode %v, want the help kept open", got.mode)
	}
	if got := press(m, "b", "n", "S"); got.form.kind != formTask || got.form.title.Value() != "S" {
		t.Errorf("S in the add form: kind %v title %q, want it typed", got.form.kind, got.form.title.Value())
	}
}

// The settings dialog's second field is the sprint reference prefix: empty, it
// shows the folder name's prefix the sprints fall back to; typing one sets it
// (upper-cased) and new sprints take their references from it.
func TestSettingsDialogSetsSprintPrefix(t *testing.T) {
	m, path := newTestModel(t, "# B\n- [ ] a\n")
	m = press(m, "S")
	v := m.form.view()
	if !strings.Contains(v, "Sprint reference prefix") || !strings.Contains(v, "SPRINT (folder name)") {
		t.Errorf("the settings dialog should have a Sprint reference prefix field showing the folder default:\n%s", v)
	}
	m = press(typeText(press(m, "tab"), "sp"), "enter")
	if m.mode != modeMain || m.err != nil || m.status != "" {
		t.Fatalf("after enter: mode %v err %v status %q", m.mode, m.err, m.status)
	}
	if got := loadSprintDoc(t, path).SprintPrefix; got != "SP" {
		t.Fatalf("stored SprintPrefix = %q, want SP", got)
	}
	if got := press(m, "S").form.ref.Value(); got != "SP" {
		t.Errorf("the reopened dialog shows sprint prefix %q, want SP", got)
	}
}

// A sprint prefix that is also the task prefix is refused: the footer says
// why, the dialog stays open, and neither prefix is stored.
func TestSettingsDialogPrefixClash(t *testing.T) {
	m, path := newTestModel(t, "# B\n- [ ] a\n")
	m = typeText(press(m, "S"), "todo")
	m = press(typeText(press(m, "tab"), "TODO"), "enter")
	if m.mode != modeForm {
		t.Fatalf("mode %v, want the dialog kept open", m.mode)
	}
	if got := m.footer(200); !strings.Contains(got, todo.ErrPrefixClash.Error()) {
		t.Errorf("footer = %q, want the clash message", got)
	}
	if d := loadSprintDoc(t, path); d.RefPrefix != "" || d.SprintPrefix != "" {
		t.Errorf("stored prefixes = %q, %q; want none", d.RefPrefix, d.SprintPrefix)
	}
}
