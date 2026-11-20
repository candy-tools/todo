package tui

import (
	"context"

	"github.com/candy-tools/todo/internal/todo"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

// openSettings opens the settings dialog (S) on the list's settings: its task
// reference prefix, the last resort of a new task's generated reference, and
// its sprint reference prefix. An empty sprint prefix shows, dimmed, the one
// new sprints fall back to, made from the folder name.
func (m model) openSettings() (tea.Model, tea.Cmd) {
	m.target = targetSettings
	m.form = newForm(todo.Fields{Prefix: m.doc.RefPrefix, Ref: m.doc.SprintPrefix}, formSettings, true)
	if m.doc.SprintPrefix == "" {
		m.form.ref.Placeholder = m.doc.SprintRefPrefix() + " (folder name)"
	}
	m.form.setWidth(m.width)
	m.mode = modeForm
	return m, textinput.Blink
}

// submitSettings stores the open settings dialog, for submitForm: the prefixes
// that changed, in one write, which upper-cases them; an empty one clears it.
// An unchanged dialog writes nothing. A failed or refused write (an invalid
// prefix, or a sprint prefix that is a task prefix) reopens the dialog with
// what was typed, the reason in the footer.
func (m *model) submitSettings() {
	v := m.form.values()
	var st todo.Settings
	if v.Prefix != m.form.start.Prefix {
		st.RefPrefix = &v.Prefix
	}
	if v.Ref != m.form.start.Ref {
		st.SprintPrefix = &v.Ref
	}
	if st == (todo.Settings{}) {
		return
	}
	if !m.commit("", func(ctx context.Context, s *todo.Store) (todo.WriteResult, error) {
		return s.UpdateSettings(ctx, st)
	}) {
		m.mode = modeForm
	}
}
