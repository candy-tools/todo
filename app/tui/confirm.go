package tui

import (
	"fmt"
	"strings"

	"github.com/candy-tools/todo/internal/todo"
	"github.com/charmbracelet/lipgloss"
)

// deletionQuestion phrases the delete prompt, noting how many nested items are
// removed along with the selected one.
func deletionQuestion(it *todo.Item) string {
	kind := "task"
	if it.Kind == todo.Category {
		kind = "category"
	}
	if n := len(it.Descendants()); n > 0 {
		return fmt.Sprintf("Delete %s %q and its %d nested item(s)?", kind, it.Title, n)
	}
	return fmt.Sprintf("Delete %s %q?", kind, it.Title)
}

// confirmModal renders the delete confirmation. Focus defaults to Cancel
// (onDelete=false) so a stray keystroke can't delete — the dibs convention for
// destructive dialogs.
func confirmModal(question string, onDelete bool, termWidth int) string {
	if termWidth <= 0 {
		termWidth = 80
	}
	buttons := lipgloss.JoinHorizontal(lipgloss.Top,
		button("Delete", onDelete), "   ", button("Cancel", !onDelete))
	sep := helpTextStyle.Render(" · ")
	help := hint("y", "Delete") + sep + hint("n/esc", "Cancel") + sep + hint("tab", "Move")

	contentW := lipgloss.Width(question)
	for _, el := range []string{help, buttons} {
		if w := lipgloss.Width(el); w > contentW {
			contentW = w
		}
	}
	const pad = 2
	width := contentW + 2 + 2*pad
	if max := termWidth - 8; width > max {
		width = max
	}
	if width < 30 {
		width = 30
	}
	contentW = width - 2 - 2*pad

	buttonRow := lipgloss.NewStyle().Width(contentW).Align(lipgloss.Right).Render(buttons)
	body := padBody(question+"\n\n"+buttonRow+"\n\n"+help, pad)
	return titledBox("Confirm delete", body, width, lipgloss.Height(body)+2, true)
}

// padBody indents every non-empty line by pad columns and adds a blank row
// above and below, so the content floats inside the border.
func padBody(body string, pad int) string {
	margin := strings.Repeat(" ", pad)
	lines := strings.Split(body, "\n")
	for i, l := range lines {
		if l != "" {
			lines[i] = margin + l
		}
	}
	return "\n" + strings.Join(lines, "\n") + "\n"
}
