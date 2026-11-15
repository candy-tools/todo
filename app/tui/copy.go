package tui

import (
	"github.com/atotto/clipboard"
	"github.com/candy-tools/todo/internal/todo"
	tea "github.com/charmbracelet/bubbletea"
)

// writeClipboard copies text to the system clipboard. It is a package variable
// so tests can stub it — and so the copy path never touches the real clipboard
// in CI.
var writeClipboard = clipboard.WriteAll

// copySelection copies the selected item to the system clipboard as plain text,
// so a task with a multi-line description can be grabbed whole: the edit dialog
// word-wraps it inside a bordered box, which the terminal's mouse selection can't
// cleanly capture. On the platforms without a clipboard tool (a bare Linux box),
// it reports how to get one; any copy error and the success both show in the footer.
func (m model) copySelection() (tea.Model, tea.Cmd) {
	it := m.focused().selected()
	if it == nil {
		return m, nil
	}
	if clipboard.Unsupported {
		m.status = "No clipboard tool found — install xclip, xsel or wl-clipboard."
		return m, nil
	}
	if err := writeClipboard(itemCopyText(it)); err != nil {
		m.status = "Copy failed: " + err.Error()
		return m, nil
	}
	m.status = "Copied to clipboard."
	return m, nil
}

// itemCopyText renders an item as plain text for the clipboard: the title, then
// a blank line and the raw (unwrapped) description when the item is a task that
// has one. This is the whole item as text, independent of how the pane wraps it.
func itemCopyText(it *todo.Item) string {
	if it.IsTask() && it.Description != "" {
		return it.Title + "\n\n" + it.Description
	}
	return it.Title
}
