package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

func TestHelpOpensAndCloses(t *testing.T) {
	for _, closeKey := range []string{"?", "esc", "q", "enter"} {
		m, _ := newTestModel(t, "# Work\n\n- [ ] a\n")
		m = press(m, "?")
		if m.mode != modeHelp {
			t.Fatalf("? should open the shortcuts dialog, mode = %v", m.mode)
		}
		if out := ansi.Strip(m.View()); !strings.Contains(out, "Shortcuts") || !strings.Contains(out, "Move the task to the next / previous column") {
			t.Fatalf("the view should show the shortcuts dialog:\n%s", out)
		}
		next, cmd := m.Update(keyMsg(closeKey))
		m = next.(model)
		if m.mode != modeMain {
			t.Errorf("%q should close the dialog, mode = %v", closeKey, m.mode)
		}
		if cmd != nil {
			if _, quit := cmd().(tea.QuitMsg); quit {
				t.Errorf("%q in the dialog must close it, not quit", closeKey)
			}
		}
	}
}

func TestHelpSwallowsOtherKeys(t *testing.T) {
	m, _ := newTestModel(t, "# Work\n\n- [ ] a\n")
	m = press(m, "?", "d", "x")
	if m.mode != modeHelp {
		t.Errorf("other keys must not act behind the dialog, mode = %v", m.mode)
	}
}

func TestHelpFitsNarrowTerminal(t *testing.T) {
	out := helpModal(60, 20, 0)
	for _, l := range strings.Split(out, "\n") {
		if w := ansi.StringWidth(l); w > 56 {
			t.Errorf("line wider than the terminal allows (%d): %q", w, ansi.Strip(l))
		}
	}
	if h := len(strings.Split(out, "\n")); h > 20 {
		t.Errorf("dialog taller than the terminal: %d rows", h)
	}
}

func TestFooterPointsAtHelp(t *testing.T) {
	m, _ := newTestModel(t, "# Work\n")
	got := ansi.Strip(m.footer(200))
	if !strings.Contains(got, "?: Help") {
		t.Errorf("the footer should advertise the ? help dialog:\n%s", got)
	}
	if strings.Contains(got, "Column") {
		t.Errorf("the full key list belongs in the help dialog, not the footer:\n%s", got)
	}
}

func TestHelpScrollsWhenTooTall(t *testing.T) {
	m, _ := newTestModel(t, "# Work\n\n- [ ] a\n")
	m = send(m, tea.WindowSizeMsg{Width: 80, Height: 24})
	m = press(m, "?")
	if out := ansi.Strip(m.View()); !strings.Contains(out, "Scroll") || strings.Contains(out, "Show / hide this help") {
		t.Fatalf("on 80x24 the dialog should be clipped with a scroll hint:\n%s", out)
	}
	for range 50 {
		m = press(m, "down")
	}
	if m.helpOffset != helpMaxOffset(80, 24) {
		t.Errorf("scrolling must stop at the end: offset %d, max %d", m.helpOffset, helpMaxOffset(80, 24))
	}
	if out := ansi.Strip(m.View()); !strings.Contains(out, "Show / hide this help") {
		t.Errorf("scrolled to the end the last section should show:\n%s", out)
	}
	m = press(m, "pgup", "pgup", "pgup")
	if m.helpOffset != 0 {
		t.Errorf("scrolling up must stop at the top, offset %d", m.helpOffset)
	}
}
