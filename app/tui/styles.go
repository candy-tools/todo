package tui

import (
	"strings"

	"github.com/candy-tools/todo/internal/todo"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// Palette (256-colour), mirroring the dibs TUI so the two apps feel of a piece.
var (
	colAccent = lipgloss.Color("205") // hot pink: focus border + key hints + selection
	colDim    = lipgloss.Color("240") // gray: unfocused borders, labels, hints
	colTitle  = lipgloss.Color("141") // purple: app name in the header
	colDone   = lipgloss.Color("10")  // green: a completed task's checkbox
	colProg   = lipgloss.Color("214") // orange: a task in progress
	colDefer  = lipgloss.Color("39")  // blue: a deferred task
	colNext   = lipgloss.Color("87")  // cyan: a task in Next (new, ready)
	colReview = lipgloss.Color("220") // gold: a task in review or follow-up
)

var (
	titleActiveStyle = lipgloss.NewStyle().Foreground(colAccent).Bold(true)
	titleDimStyle    = lipgloss.NewStyle().Foreground(colDim)
	selectedRowStyle = lipgloss.NewStyle().Foreground(colAccent).Bold(true)
	// grabbedRowStyle marks the row picked up in move mode: reverse-video accent,
	// so a grabbed item reads as lifted off the list, distinct from a plain
	// selection.
	grabbedRowStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("0")).Background(colAccent).Bold(true)
	categoryStyle   = lipgloss.NewStyle().Foreground(colTitle).Bold(true)
	doneStyle       = lipgloss.NewStyle().Foreground(colDone)
	progStyle       = lipgloss.NewStyle().Foreground(colProg)
	deferStyle      = lipgloss.NewStyle().Foreground(colDefer)
	nextStyle       = lipgloss.NewStyle().Foreground(colNext)
	reviewStyle     = lipgloss.NewStyle().Foreground(colReview)
	doneTitleStyle  = lipgloss.NewStyle().Foreground(colDim).Strikethrough(true)
	labelStyle      = lipgloss.NewStyle().Faint(true)
	errStyle        = lipgloss.NewStyle().Foreground(lipgloss.Color("9"))

	helpKeyStyle   = lipgloss.NewStyle().Foreground(colAccent).Bold(true)
	helpTextStyle  = lipgloss.NewStyle().Foreground(colDim)
	headerAppStyle = lipgloss.NewStyle().Foreground(colTitle).Bold(true)
	headerHintSty  = lipgloss.NewStyle().Foreground(colDim)

	focusLabelStyle = lipgloss.NewStyle().Foreground(colAccent)

	// matchStyle marks a search hit: black on bright yellow so it stands out both
	// on a plain row and behind the accent-coloured selected row.
	matchStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("0")).Background(lipgloss.Color("11")).Bold(true)
	// plainStyle is the identity style: the highlight base for unstyled text.
	plainStyle = lipgloss.NewStyle()
)

// highlight renders s with every case-insensitive occurrence of query drawn in
// matchStyle and the rest in base. An empty query just renders s in base.
// Matching is byte-indexed on the lower-cased strings, which lines up with the
// original for the ASCII/Latin text these lists hold.
func highlight(s, query string, base lipgloss.Style) string {
	if query == "" {
		return base.Render(s)
	}
	hay := strings.ToLower(s)
	needle := strings.ToLower(query)
	var b strings.Builder
	for {
		i := strings.Index(hay, needle)
		if i < 0 {
			b.WriteString(base.Render(s))
			return b.String()
		}
		b.WriteString(base.Render(s[:i]))
		b.WriteString(matchStyle.Render(s[i : i+len(needle)]))
		s = s[i+len(needle):]
		hay = hay[i+len(needle):]
	}
}

// statusGlyph returns a task's glyph and its colour for status st. The glyphs
// extend the circle family: open ○, deferred ⏸, won't do ⊘, new ◌, ready ◉,
// working ◐, review ◑, followup ◒, done ●. A subtask is only ever open or done,
// so it shows ○ or ●; an unknown status reads as open.
func statusGlyph(st todo.Status, subtask bool) (string, lipgloss.Style) {
	if subtask {
		if st == todo.Done {
			return "●", doneStyle
		}
		return "○", plainStyle
	}
	switch st {
	case todo.Deferred:
		return "⏸", deferStyle
	case todo.WontDo:
		return "⊘", helpTextStyle
	case todo.New:
		return "◌", nextStyle
	case todo.Ready:
		return "◉", nextStyle
	case todo.Working:
		return "◐", progStyle
	case todo.Review:
		return "◑", reviewStyle
	case todo.Followup:
		return "◒", reviewStyle
	case todo.Done:
		return "●", doneStyle
	}
	return "○", plainStyle
}

// hint renders one "key: label" help entry — key accented, ": label" dim.
func hint(k, label string) string {
	return helpKeyStyle.Render(k) + helpTextStyle.Render(": "+label)
}

// titledBox renders a rounded-border box of exactly width x height cells with
// title embedded in the top border and body clipped/padded to fit. Border and
// title use the accent colour when focused, dim otherwise. Adapted verbatim in
// spirit from the dibs TUI.
func titledBox(title, body string, width, height int, focused bool) string {
	if width < 4 {
		width = 4
	}
	if height < 3 {
		height = 3
	}
	iw := width - 2
	ih := height - 2

	border := lipgloss.NewStyle().Foreground(colDim)
	ts := titleDimStyle
	if focused {
		border = lipgloss.NewStyle().Foreground(colAccent)
		ts = titleActiveStyle
	}

	label := " " + title + " "
	maxLabel := iw - 3
	if maxLabel < 0 {
		maxLabel = 0
	}
	if lipgloss.Width(label) > maxLabel {
		label = ansi.Truncate(label, maxLabel, "")
	}
	fill := iw - lipgloss.Width(label) - 1
	if fill < 0 {
		fill = 0
	}
	top := border.Render("╭─") + ts.Render(label) + border.Render(strings.Repeat("─", fill)+"╮")

	lines := strings.Split(body, "\n")
	rows := make([]string, 0, ih)
	for i := 0; i < ih; i++ {
		content := ""
		if i < len(lines) {
			content = lines[i]
		}
		content = ansi.Truncate(content, iw, "")
		if pad := iw - lipgloss.Width(content); pad > 0 {
			content += strings.Repeat(" ", pad)
		}
		rows = append(rows, border.Render("│")+content+border.Render("│"))
	}

	bottom := border.Render("╰" + strings.Repeat("─", iw) + "╯")
	return strings.Join(append(append([]string{top}, rows...), bottom), "\n")
}
