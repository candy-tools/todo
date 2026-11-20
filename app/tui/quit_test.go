package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// quitsConfirmed presses key, which must open the quit dialog without quitting,
// then confirms it with enter, and reports whether that quits.
func quitsConfirmed(t *testing.T, m model, key string) bool {
	t.Helper()
	nm, cmd := m.Update(keyMsg(key))
	m = nm.(model)
	if quits(cmd) {
		t.Fatalf("%q must ask before quitting, but quit at once", key)
	}
	if m.mode != modeConfirm || m.confirmAction != confirmQuit {
		t.Fatalf("%q: mode %v action %v, want the quit dialog", key, m.mode, m.confirmAction)
	}
	_, cmd = m.Update(keyMsg("enter"))
	return quits(cmd)
}

func TestQAsksBeforeQuittingWithYesFocused(t *testing.T) {
	m, _ := newTestModel(t, "# Work\n\n- [ ] a\n")
	nm, cmd := m.Update(keyMsg("q"))
	m = nm.(model)
	if quits(cmd) {
		t.Fatal("q must open the quit dialog, not quit at once")
	}
	if m.mode != modeConfirm || m.confirmAction != confirmQuit || !m.confirmOnYes {
		t.Fatalf("q: mode %v action %v yes-focused %v, want the quit dialog on Yes", m.mode, m.confirmAction, m.confirmOnYes)
	}
	v := m.View()
	for _, want := range []string{"Quit?", "Yes", "No"} {
		if !strings.Contains(v, want) {
			t.Errorf("the quit dialog must show %q", want)
		}
	}
	if strings.Contains(v, "Delete") {
		t.Error("the quit dialog must not borrow the delete dialog's labels")
	}
	if _, cmd := m.Update(keyMsg("enter")); !quits(cmd) {
		t.Error("enter in the quit dialog (Yes focused) must quit")
	}
}

func TestEscInQuitDialogStays(t *testing.T) {
	m, _ := newTestModel(t, "# Work\n\n- [ ] a\n")
	m = press(m, "q")
	nm, cmd := m.Update(keyMsg("esc"))
	m = nm.(model)
	if quits(cmd) || m.mode != modeMain {
		t.Fatalf("esc in the quit dialog: quit %v mode %v, want back to the main view", quits(cmd), m.mode)
	}
}

func TestQuitDialogKeys(t *testing.T) {
	m, _ := newTestModel(t, "# Work\n\n- [ ] a\n")
	if _, cmd := press(m, "q").Update(keyMsg("y")); !quits(cmd) {
		t.Error("y in the quit dialog must quit")
	}
	if nm, cmd := press(m, "q").Update(keyMsg("n")); quits(cmd) || nm.(model).mode != modeMain {
		t.Error("n in the quit dialog must stay")
	}
	// tab moves the focus to No, so enter stays.
	if nm, cmd := press(m, "q", "tab").Update(keyMsg("enter")); quits(cmd) || nm.(model).mode != modeMain {
		t.Error("enter on No must stay")
	}
}

func TestEscWithNothingToClearAsksBeforeQuitting(t *testing.T) {
	m, _ := newTestModel(t, "# Work\n\n- [ ] a\n")
	if !quitsConfirmed(t, m, "esc") {
		t.Error("esc with nothing to clear must quit once confirmed")
	}
}

func TestCtrlCQuitsWithoutAsking(t *testing.T) {
	m, _ := newTestModel(t, "# Work\n\n- [ ] a\n")
	if _, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC}); !quits(cmd) {
		t.Error("ctrl+c must quit at once")
	}
}

func TestCancelledQuitStoresNothing(t *testing.T) {
	m, _ := threeSprints(t)
	m = press(m, "s", "down") // browse to another sprint: quitting would store it
	before := storeFP(t, m)
	m = press(m, "q", "esc")
	if storeFP(t, m) != before {
		t.Error("a cancelled quit must not write the view")
	}
}
