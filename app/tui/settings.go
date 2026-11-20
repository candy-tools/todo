package tui

import (
	"context"

	"github.com/candy-tools/todo/internal/todo"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

// openSettings opens the settings dialog (S) on the list's settings: today its
// reference prefix, the last resort of a new task's generated reference.
func (m model) openSettings() (tea.Model, tea.Cmd) {
	m.target = targetSettings
	m.form = newForm(todo.Fields{Prefix: m.doc.RefPrefix}, formSettings, true)
	m.form.setWidth(m.width)
	m.mode = modeForm
	return m, textinput.Blink
}

// submitSettings stores the open settings dialog, for submitForm: the list's
// reference prefix, which the store upper-cases; an empty one clears it. An
// unchanged dialog writes nothing. A failed or refused write (an invalid
// prefix) reopens the dialog with what was typed, the reason in the footer.
func (m *model) submitSettings() {
	v := m.form.values()
	if v.Prefix == m.form.start.Prefix {
		return
	}
	if !m.commit("", func(ctx context.Context, st *todo.Store) (todo.WriteResult, error) {
		return st.SetRefPrefix(ctx, v.Prefix)
	}) {
		m.mode = modeForm
	}
}
