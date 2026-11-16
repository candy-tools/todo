package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// helpBinding is one row of the shortcuts dialog: the key(s) and what they do.
type helpBinding struct{ keys, desc string }

// helpSection groups related bindings under a heading in the shortcuts dialog.
type helpSection struct {
	title    string
	bindings []helpBinding
}

// helpSections is the content of the "?" shortcuts dialog — every main-view key,
// grouped. Keep it in step with updateMain / updateMainItemKey and the Keys table
// in DOCUMENTATION.md.
var helpSections = []helpSection{
	{"Navigate", []helpBinding{
		{"↑ ↓  w s", "Move the selection"},
		{"PgUp PgDn", "Move by a page"},
		{"← →", "Collapse / expand"},
		{"space", "Fold / unfold a category"},
		{"tab  shift+tab", "Next / previous tab"},
		{"1–4", "Jump to Backlog / Next / In Progress / Done"},
		{"/", "Search / filter (esc clears)"},
		{"f", "Filter the tab by state (esc clears)"},
	}},
	{"Tasks", []helpBinding{
		{"p P", "Move the task to the next / previous column"},
		{"t", "Set the task's state (also completes / reopens)"},
		{"m", "Grab to reorder (arrows move, m/enter/esc drop)"},
		{"y", "Copy the item to the clipboard"},
	}},
	{"Edit", []helpBinding{
		{"enter  e", "Open / edit the selected item"},
		{"n", "New task as a sibling"},
		{"N", "New task as a child (subtask)"},
		{"c", "Add a category (Backlog only)"},
		{"d", "Delete the selected item"},
		{"D", "Remove every completed task"},
	}},
	{"Views", []helpBinding{
		{"k", "All states in one list / back to the board"},
		{"v", "Split: categories sidebar + task list"},
		{"x", "Switch pane (split view)"},
	}},
	{"App", []helpBinding{
		{"?", "Show / hide this help"},
		{"q", "Quit"},
		{"esc", "Clear search / state filter, or quit"},
	}},
}

// helpPad is the horizontal padding inside the shortcuts dialog's border.
const helpPad = 2

// helpChrome is how many dialog rows are not scrollable content: the two
// borders, the blank rows above and below, and the blank row plus close hint.
const helpChrome = 6

// helpContent lays the help sections out for the terminal width: per section a
// heading and an aligned key column with descriptions, in two columns when they
// fit, otherwise stacked. It returns the content's lines, unpadded.
func helpContent(termWidth int) []string {
	keyW := 0
	for _, s := range helpSections {
		for _, b := range s.bindings {
			keyW = max(keyW, lipgloss.Width(b.keys))
		}
	}
	keyCol := lipgloss.NewStyle().Width(keyW + 2)
	section := func(s helpSection) string {
		lines := []string{categoryStyle.Render(s.title)}
		for _, bd := range s.bindings {
			lines = append(lines, keyCol.Render(helpKeyStyle.Render(bd.keys))+helpTextStyle.Render(bd.desc))
		}
		return strings.Join(lines, "\n")
	}
	stack := func(ss []helpSection) string {
		blocks := make([]string, len(ss))
		for i, s := range ss {
			blocks[i] = section(s)
		}
		return strings.Join(blocks, "\n\n")
	}

	frame := 2 + 2*helpPad // borders plus the padding on either side
	half := (len(helpSections) + 1) / 2
	content := lipgloss.JoinHorizontal(lipgloss.Top,
		stack(helpSections[:half]), strings.Repeat(" ", 4), stack(helpSections[half:]))
	if lipgloss.Width(content)+frame > termWidth-4 {
		content = stack(helpSections)
	}
	return strings.Split(content, "\n")
}

// helpVisibleRows is how many content lines of a total fit in the dialog on a
// terminal termHeight rows tall (at least one).
func helpVisibleRows(total, termHeight int) int {
	return max(min(total, termHeight-helpChrome), 1)
}

// helpMaxOffset is the furthest the shortcuts dialog can scroll at this
// terminal size; 0 when everything fits.
func helpMaxOffset(termWidth, termHeight int) int {
	if termWidth <= 0 {
		termWidth, termHeight = 80, 24
	}
	lines := helpContent(termWidth)
	return len(lines) - helpVisibleRows(len(lines), termHeight)
}

// helpModal renders the shortcuts dialog opened with "?", sized to the
// terminal. When the content is taller than the terminal it shows the window
// starting at offset (clamped) and the pinned hint row offers ↑↓ scrolling.
func helpModal(termWidth, termHeight, offset int) string {
	if termWidth <= 0 {
		termWidth, termHeight = 80, 24
	}
	lines := helpContent(termWidth)
	visible := helpVisibleRows(len(lines), termHeight)
	offset = max(min(offset, len(lines)-visible), 0)

	closeHint := hint("?/esc/q", "Close")
	if visible < len(lines) {
		closeHint = hint("↑↓", "Scroll") + helpTextStyle.Render(" · ") + closeHint
	}
	window := strings.Join(lines[offset:offset+visible], "\n")
	body := padBody(window+"\n\n"+closeHint, helpPad)

	width := min(lipgloss.Width(body)+2+helpPad, termWidth-4)
	return titledBox("Shortcuts", body, width, lipgloss.Height(body)+2, true)
}

// updateHelp handles keys while the shortcuts dialog is open: ?, esc, q or
// enter close it (q does not quit from here), the arrows and PgUp/PgDn scroll it
// when it is taller than the terminal; everything else is ignored.
func (m model) updateHelp(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	maxOff := helpMaxOffset(m.width, m.height)
	switch msg.String() {
	case "?", "esc", "q", "enter":
		m.mode = modeMain
	case "up", "w":
		m.helpOffset--
	case "down", "s":
		m.helpOffset++
	case "pgup":
		m.helpOffset -= 10
	case "pgdown":
		m.helpOffset += 10
	}
	m.helpOffset = max(min(m.helpOffset, maxOff), 0)
	return m, nil
}
